//go:build !metal

package metal

import "github.com/michaelraines/future-render/internal/backend"

// Texture implements backend.Texture for Metal.
// Models an MTLTexture object.
type Texture struct {
	backend.Texture     // delegates all Texture methods to inner
	pixelFormat     int // MTLPixelFormat
	usage           int // MTLTextureUsage
}

// InnerTexture returns the wrapped soft texture for encoder unwrapping.
func (t *Texture) InnerTexture() backend.Texture { return t.Texture }
