//go:build wgpunative

package webgpu

import (
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
}

// ReadPixels reads texture data back to CPU.
func (t *Texture) ReadPixels(_ []byte) {
	// WebGPU requires a copy-to-buffer-then-map workflow for readbacks.
	// This is a placeholder for the full implementation.
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
