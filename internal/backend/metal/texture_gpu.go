//go:build metal

package metal

import (
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/mtl"
)

// Texture implements backend.Texture for Metal via MTLTexture.
type Texture struct {
	dev         *Device
	handle      mtl.Texture
	w, h        int
	format      backend.TextureFormat
	pixelFormat int
	usage       int
}

// InnerTexture returns nil for GPU textures (no soft delegation).
func (t *Texture) InnerTexture() backend.Texture { return nil }

// Upload uploads pixel data to the texture via replaceRegion.
func (t *Texture) Upload(data []byte, _ int) {
	if len(data) == 0 || t.handle == 0 {
		return
	}
	bpp := bytesPerPixel(t.format)
	bytesPerRow := uint64(t.w * bpp)
	region := mtl.Region{
		Size: mtl.Size{Width: uint64(t.w), Height: uint64(t.h), Depth: 1},
	}
	mtl.TextureReplaceRegion(t.handle, region, 0, unsafe.Pointer(&data[0]), bytesPerRow)
}

// UploadRegion uploads pixel data to a rectangular region.
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
	mtl.TextureReplaceRegion(t.handle, region, 0, unsafe.Pointer(&data[0]), bytesPerRow)
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
