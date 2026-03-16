//go:build dx12native

package dx12

import (
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/d3d12"
)

// Texture implements backend.Texture for DX12 via ID3D12Resource.
type Texture struct {
	dev        *Device
	resource   d3d12.Resource
	w, h       int
	format     backend.TextureFormat
	dxgiFormat int
}

// InnerTexture returns nil for GPU textures (no soft delegation).
func (t *Texture) InnerTexture() backend.Texture { return nil }

// Upload uploads pixel data to the texture via upload heap.
func (t *Texture) Upload(data []byte, _ int) {
	if len(data) == 0 || t.dev.uploadMapped == 0 {
		return
	}
	n := len(data)
	if n > t.dev.uploadSize {
		n = t.dev.uploadSize
	}
	// Copy to upload buffer.
	dst := unsafe.Slice((*byte)(unsafe.Pointer(t.dev.uploadMapped)), n)
	copy(dst, data[:n])

	// In a full implementation, we'd record a CopyTextureRegion command
	// from the upload buffer to the texture resource.
}

// UploadRegion uploads pixel data to a rectangular region.
func (t *Texture) UploadRegion(data []byte, _, _, _, _, _ int) {
	t.Upload(data, 0)
}

// ReadPixels reads RGBA pixel data from the texture.
func (t *Texture) ReadPixels(dst []byte) {
	if len(dst) == 0 {
		return
	}
	// In a full implementation, this would copy the texture to a readback
	// heap resource, then read the mapped data.
	for i := range dst {
		dst[i] = 0
	}
}

// Width returns the texture width.
func (t *Texture) Width() int { return t.w }

// Height returns the texture height.
func (t *Texture) Height() int { return t.h }

// Format returns the texture format.
func (t *Texture) Format() backend.TextureFormat { return t.format }

// Dispose releases the ID3D12Resource.
func (t *Texture) Dispose() {
	if t.resource != 0 {
		d3d12.Release(uintptr(t.resource))
		t.resource = 0
	}
}
