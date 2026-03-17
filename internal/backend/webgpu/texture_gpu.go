//go:build wgpunative

package webgpu

import (
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/wgpu"
)

// Texture implements backend.Texture for WebGPU via wgpu-native.
type Texture struct {
	dev    *Device
	handle wgpu.Texture
	view   wgpu.TextureView
	w, h   int
	format backend.TextureFormat
}

// InnerTexture returns nil for GPU textures (no soft delegation).
func (t *Texture) InnerTexture() backend.Texture { return nil }

// Upload replaces the entire texture data.
func (t *Texture) Upload(data []byte, mipLevel int) {
	if len(data) == 0 || t.dev.queue == 0 {
		return
	}

	bpp := bytesPerPixel(t.format)
	dst := wgpu.ImageCopyTexture{
		Texture_: t.handle,
		MipLevel: uint32(mipLevel),
		Origin:   wgpu.Origin3D{},
		Aspect:   0, // All
	}
	layout := wgpu.TextureDataLayout{
		BytesPerRow:  uint32(t.w * bpp),
		RowsPerImage: uint32(t.h),
	}
	size := wgpu.Extent3D{
		Width:              uint32(t.w),
		Height:             uint32(t.h),
		DepthOrArrayLayers: 1,
	}
	wgpu.QueueWriteTexture(t.dev.queue, &dst,
		unsafe.Pointer(&data[0]), uint64(len(data)), &layout, &size)
	runtime.KeepAlive(data)
}

// UploadRegion uploads a sub-region of texture data.
func (t *Texture) UploadRegion(data []byte, x, y, w, h, mipLevel int) {
	if len(data) == 0 || t.dev.queue == 0 {
		return
	}

	bpp := bytesPerPixel(t.format)
	dst := wgpu.ImageCopyTexture{
		Texture_: t.handle,
		MipLevel: uint32(mipLevel),
		Origin:   wgpu.Origin3D{X: uint32(x), Y: uint32(y)},
		Aspect:   0,
	}
	layout := wgpu.TextureDataLayout{
		BytesPerRow:  uint32(w * bpp),
		RowsPerImage: uint32(h),
	}
	size := wgpu.Extent3D{
		Width:              uint32(w),
		Height:             uint32(h),
		DepthOrArrayLayers: 1,
	}
	wgpu.QueueWriteTexture(t.dev.queue, &dst,
		unsafe.Pointer(&data[0]), uint64(len(data)), &layout, &size)
	runtime.KeepAlive(data)
}

// ReadPixels reads texture data back to CPU via copy-to-buffer + map.
func (t *Texture) ReadPixels(dst []byte) {
	if len(dst) == 0 || t.dev.device == 0 || t.dev.queue == 0 {
		return
	}

	bpp := bytesPerPixel(t.format)
	// Align bytes per row to 256 (WebGPU requirement for buffer copies).
	bytesPerRow := uint32(t.w * bpp)
	alignedBytesPerRow := (bytesPerRow + 255) &^ 255
	dataSize := uint64(alignedBytesPerRow) * uint64(t.h)

	// Create a staging buffer for readback.
	bufDesc := wgpu.BufferDescriptor{
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
		Size:  dataSize,
	}
	stagingBuf := wgpu.DeviceCreateBuffer(t.dev.device, &bufDesc)
	if stagingBuf == 0 {
		return
	}

	// Encode copy texture → buffer.
	enc := wgpu.DeviceCreateCommandEncoder(t.dev.device)
	src := wgpu.ImageCopyTexture{
		Texture_: t.handle,
		MipLevel: 0,
		Origin:   wgpu.Origin3D{},
		Aspect:   0,
	}
	dstCopy := wgpu.ImageCopyBuffer{
		Layout: wgpu.TextureDataLayout{
			BytesPerRow:  alignedBytesPerRow,
			RowsPerImage: uint32(t.h),
		},
		Buffer_: stagingBuf,
	}
	copySize := wgpu.Extent3D{
		Width:              uint32(t.w),
		Height:             uint32(t.h),
		DepthOrArrayLayers: 1,
	}
	wgpu.CommandEncoderCopyTextureToBuffer(enc, &src, &dstCopy, &copySize)
	cmdBuf := wgpu.CommandEncoderFinish(enc)
	wgpu.QueueSubmit(t.dev.queue, []wgpu.CommandBuffer{cmdBuf})
	wgpu.CommandBufferRelease(cmdBuf)
	wgpu.CommandEncoderRelease(enc)

	// Map the staging buffer and copy data.
	wgpu.BufferMapAsync(stagingBuf, wgpu.MapModeRead, 0, dataSize)
	wgpu.DevicePoll(t.dev.device, true)

	ptr := wgpu.BufferGetMappedRange(stagingBuf, 0, dataSize)
	if ptr != 0 {
		// Copy row by row to handle alignment padding.
		srcSlice := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), dataSize)
		dstOffset := 0
		for row := 0; row < t.h; row++ {
			srcStart := int(alignedBytesPerRow) * row
			n := int(bytesPerRow)
			if dstOffset+n > len(dst) {
				n = len(dst) - dstOffset
			}
			if n <= 0 {
				break
			}
			copy(dst[dstOffset:dstOffset+n], srcSlice[srcStart:srcStart+n])
			dstOffset += n
		}
	}

	wgpu.BufferUnmap(stagingBuf)
	wgpu.BufferDestroy(stagingBuf)
	wgpu.BufferRelease(stagingBuf)
}

// Width returns the texture width.
func (t *Texture) Width() int { return t.w }

// Height returns the texture height.
func (t *Texture) Height() int { return t.h }

// Format returns the texture format.
func (t *Texture) Format() backend.TextureFormat { return t.format }

// Dispose releases the texture and its view.
func (t *Texture) Dispose() {
	if t.view != 0 {
		wgpu.TextureViewRelease(t.view)
		t.view = 0
	}
	if t.handle != 0 {
		wgpu.TextureDestroy(t.handle)
		wgpu.TextureRelease(t.handle)
		t.handle = 0
	}
}
