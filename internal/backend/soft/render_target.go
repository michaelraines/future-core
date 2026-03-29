package soft

import "github.com/michaelraines/future-core/internal/backend"

// RenderTarget implements backend.RenderTarget with CPU-side textures.
type RenderTarget struct {
	id       uint64
	color    *Texture
	depth    backend.Texture
	rtWidth  int
	rtHeight int
	disposed bool
}

// ColorTexture returns the color attachment texture.
func (rt *RenderTarget) ColorTexture() backend.Texture { return rt.color }

// DepthTexture returns the depth attachment texture, if any.
func (rt *RenderTarget) DepthTexture() backend.Texture { return rt.depth }

// Width returns the render target width.
func (rt *RenderTarget) Width() int { return rt.rtWidth }

// Height returns the render target height.
func (rt *RenderTarget) Height() int { return rt.rtHeight }

// Dispose releases the render target and its attachments.
func (rt *RenderTarget) Dispose() {
	rt.disposed = true
	rt.color.Dispose()
	if rt.depth != nil {
		rt.depth.Dispose()
	}
}
