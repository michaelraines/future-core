//go:build !js

package webgl

import "github.com/michaelraines/future-render/internal/backend"

// Texture implements backend.Texture for WebGL2.
// Wraps a soft.Texture and adds WebGL2-specific metadata (GL target, format).
type Texture struct {
	backend.Texture     // delegates all Texture methods to inner
	glTarget        int // GL texture target (e.g. GL_TEXTURE_2D)
	glFormat        int // GL internal format (e.g. GL_RGBA)
}

// InnerTexture returns the wrapped soft texture for encoder unwrapping.
func (t *Texture) InnerTexture() backend.Texture { return t.Texture }
