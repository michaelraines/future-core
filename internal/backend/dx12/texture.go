package dx12

import "github.com/michaelraines/future-render/internal/backend"

// Texture implements backend.Texture for DirectX 12.
// Models an ID3D12Resource (committed texture resource).
type Texture struct {
	inner      backend.Texture
	dxgiFormat int // DXGI_FORMAT
}

// Upload replaces the entire texture data.
func (t *Texture) Upload(data []byte, level int) { t.inner.Upload(data, level) }

// UploadRegion uploads pixel data to a rectangular region.
func (t *Texture) UploadRegion(data []byte, x, y, width, height, level int) {
	t.inner.UploadRegion(data, x, y, width, height, level)
}

// ReadPixels reads RGBA pixel data from the texture.
func (t *Texture) ReadPixels(dst []byte) { t.inner.ReadPixels(dst) }

// Width returns the texture width.
func (t *Texture) Width() int { return t.inner.Width() }

// Height returns the texture height.
func (t *Texture) Height() int { return t.inner.Height() }

// Format returns the texture format.
func (t *Texture) Format() backend.TextureFormat { return t.inner.Format() }

// Dispose releases the texture.
func (t *Texture) Dispose() { t.inner.Dispose() }
