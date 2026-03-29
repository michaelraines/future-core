package soft

import "github.com/michaelraines/future-core/internal/backend"

// Texture implements backend.Texture as a CPU-side pixel buffer.
type Texture struct {
	id       uint64
	w, h     int
	fmt      backend.TextureFormat
	filter   backend.TextureFilter
	pixels   []byte
	bpp      int
	disposed bool
}

// Upload replaces the entire texture data.
func (t *Texture) Upload(data []byte, _ int) {
	copy(t.pixels, data)
}

// UploadRegion uploads pixel data to a rectangular region.
func (t *Texture) UploadRegion(data []byte, x, y, width, height, _ int) {
	rowBytes := width * t.bpp
	for row := 0; row < height; row++ {
		srcStart := row * rowBytes
		srcEnd := srcStart + rowBytes
		dstStart := ((y+row)*t.w + x) * t.bpp
		dstEnd := dstStart + rowBytes
		if srcEnd <= len(data) && dstEnd <= len(t.pixels) {
			copy(t.pixels[dstStart:dstEnd], data[srcStart:srcEnd])
		}
	}
}

// ReadPixels copies the texture data to dst.
func (t *Texture) ReadPixels(dst []byte) {
	copy(dst, t.pixels)
}

// Width returns the texture width.
func (t *Texture) Width() int { return t.w }

// Height returns the texture height.
func (t *Texture) Height() int { return t.h }

// Format returns the texture format.
func (t *Texture) Format() backend.TextureFormat { return t.fmt }

// Dispose releases the texture.
func (t *Texture) Dispose() {
	t.disposed = true
	t.pixels = nil
}

// ID returns the texture's unique identifier.
func (t *Texture) ID() uint64 { return t.id }

// Pixels returns the raw pixel data. For testing/conformance only.
func (t *Texture) Pixels() []byte { return t.pixels }
