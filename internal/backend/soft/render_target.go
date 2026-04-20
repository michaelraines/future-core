package soft

import "github.com/michaelraines/future-core/internal/backend"

// RenderTarget implements backend.RenderTarget with CPU-side textures.
type RenderTarget struct {
	id         uint64
	color      *Texture
	depth      backend.Texture
	rtWidth    int
	rtHeight   int
	hasStencil bool // true when a stencil attachment was requested
	disposed   bool
}

// HasStencil reports whether the render target was created with a stencil
// attachment. The sprite pass uses this in combination with
// DeviceCapabilities.SupportsStencil to decide whether fill-rule batches
// can be routed through the stencil path.
func (rt *RenderTarget) HasStencil() bool { return rt.hasStencil }

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
