//go:build !metal

package metal

import "github.com/michaelraines/future-render/internal/backend"

// RenderTarget implements backend.RenderTarget for Metal.
// Models an MTLRenderPassDescriptor with color/depth attachments.
type RenderTarget struct {
	backend.RenderTarget // delegates all RenderTarget methods to inner
}

// InnerRenderTarget returns the wrapped soft render target for encoder unwrapping.
func (rt *RenderTarget) InnerRenderTarget() backend.RenderTarget { return rt.RenderTarget }
