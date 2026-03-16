//go:build !js

package webgl

import "github.com/michaelraines/future-render/internal/backend"

// RenderTarget implements backend.RenderTarget for WebGL2.
// Wraps a soft.RenderTarget, representing a WebGL2 framebuffer object (FBO).
type RenderTarget struct {
	backend.RenderTarget // delegates all RenderTarget methods to inner
}

// InnerRenderTarget returns the wrapped soft render target for encoder unwrapping.
func (rt *RenderTarget) InnerRenderTarget() backend.RenderTarget { return rt.RenderTarget }
