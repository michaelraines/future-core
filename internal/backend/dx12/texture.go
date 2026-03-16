//go:build !dx12native

package dx12

import "github.com/michaelraines/future-render/internal/backend"

// Texture implements backend.Texture for DirectX 12.
// Models an ID3D12Resource (committed texture resource).
type Texture struct {
	backend.Texture     // delegates all Texture methods to inner
	dxgiFormat      int // DXGI_FORMAT
}

// InnerTexture returns the wrapped soft texture for encoder unwrapping.
func (t *Texture) InnerTexture() backend.Texture { return t.Texture }
