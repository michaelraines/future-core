//go:build darwin && !soft

package metal

import (
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/mtl"
)

// Texture implements backend.Texture for Metal via MTLTexture.
type Texture struct {
	dev         *Device
	handle      mtl.Texture
	w, h        int
	format      backend.TextureFormat
	pixelFormat int
	usage       int
	storageMode int // mtl.StorageMode{Shared,Private,...}
}

// InnerTexture returns nil for GPU textures (no soft delegation).
func (t *Texture) InnerTexture() backend.Texture { return nil }

// Upload uploads pixel data to the texture.
// On Shared storage uses replaceRegion (direct CPU write).
// On Private storage routes through a staging buffer + blit copy.
func (t *Texture) Upload(data []byte, _ int) {
	if len(data) == 0 || t.handle == 0 {
		return
	}
	bpp := bytesPerPixel(t.format)
	bytesPerRow := uint64(t.w * bpp)
	region := mtl.Region{
		Size: mtl.Size{Width: uint64(t.w), Height: uint64(t.h), Depth: 1},
	}
	if t.storageMode == mtl.StorageModePrivate {
		t.uploadViaStaging(data, region, bytesPerRow)
		return
	}
	mtl.TextureReplaceRegion(t.handle, region, 0, unsafe.Pointer(&data[0]), bytesPerRow)
}

// UploadRegion uploads pixel data to a rectangular region.
// On Shared storage uses replaceRegion (direct CPU write).
// On Private storage routes through a staging buffer + blit copy.
func (t *Texture) UploadRegion(data []byte, x, y, w, h, _ int) {
	if len(data) == 0 || t.handle == 0 {
		return
	}
	bpp := bytesPerPixel(t.format)
	bytesPerRow := uint64(w * bpp)
	region := mtl.Region{
		Origin: mtl.Origin{X: uint64(x), Y: uint64(y)},
		Size:   mtl.Size{Width: uint64(w), Height: uint64(h), Depth: 1},
	}
	if t.storageMode == mtl.StorageModePrivate {
		t.uploadViaStaging(data, region, bytesPerRow)
		return
	}
	mtl.TextureReplaceRegion(t.handle, region, 0, unsafe.Pointer(&data[0]), bytesPerRow)
}

// uploadViaStaging copies pixel data into a temporary Shared MTLBuffer,
// then schedules a blit copy from the buffer into the Private texture.
// Synchronous — the blit cmd buffer is committed and waited on so the
// CPU-supplied bytes are GPU-visible by the time the call returns.
//
// This is the route for Private-storage textures whose contents are
// supplied via the engine's CPU upload paths (sprite-atlas pages,
// pixel uploads via WritePixels). Private-storage RTs cannot accept
// `replaceRegion:` directly — that's CPU-write-into-GPU-memory which
// is invalid on Apple Silicon's unified-memory boundary.
func (t *Texture) uploadViaStaging(data []byte, region mtl.Region, bytesPerRow uint64) {
	if t.dev == nil || t.dev.commandQueue == 0 {
		return
	}
	staging := mtl.DeviceNewBuffer(t.dev.device, uint64(len(data)), mtl.ResourceStorageModeShared)
	if staging == 0 {
		return
	}
	defer mtl.BufferRelease(staging)

	contents := mtl.BufferContents(staging)
	if contents == 0 {
		return
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(contents)), len(data))
	copy(dst, data)

	cmdBuf := mtl.CommandQueueCommandBuffer(t.dev.commandQueue)
	if cmdBuf == 0 {
		return
	}
	blit := mtl.CommandBufferBlitCommandEncoder(cmdBuf)
	if blit == 0 {
		mtl.CommandBufferCommit(cmdBuf)
		return
	}
	mtl.BlitCommandEncoderCopyFromBufferToTexture(blit,
		staging, 0, bytesPerRow, bytesPerRow*region.Size.Height,
		region.Size,
		t.handle, 0, 0,
		region.Origin)
	mtl.BlitCommandEncoderEndEncoding(blit)
	mtl.CommandBufferCommit(cmdBuf)
	mtl.CommandBufferWaitUntilCompleted(cmdBuf)
}

// ReadPixels reads RGBA pixel data from the texture.
func (t *Texture) ReadPixels(dst []byte) {
	if len(dst) == 0 || t.handle == 0 {
		return
	}
	bpp := bytesPerPixel(t.format)
	bytesPerRow := uint64(t.w * bpp)
	region := mtl.Region{
		Size: mtl.Size{Width: uint64(t.w), Height: uint64(t.h), Depth: 1},
	}
	mtl.TextureGetBytes(t.handle, unsafe.Pointer(&dst[0]), bytesPerRow, region, 0)
}

// Width returns the texture width.
func (t *Texture) Width() int { return t.w }

// Height returns the texture height.
func (t *Texture) Height() int { return t.h }

// Format returns the texture format.
func (t *Texture) Format() backend.TextureFormat { return t.format }

// Dispose releases the MTLTexture.
func (t *Texture) Dispose() {
	if t.handle != 0 {
		mtl.TextureRelease(t.handle)
		t.handle = 0
	}
}
