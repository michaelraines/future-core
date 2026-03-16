//go:build !dx12native

package dx12

import "github.com/michaelraines/future-render/internal/backend"

// RenderTarget implements backend.RenderTarget for DirectX 12.
// Models RTV (render target view) and DSV (depth stencil view) descriptors.
type RenderTarget struct {
	backend.RenderTarget // delegates all RenderTarget methods to inner
}

// InnerRenderTarget returns the wrapped soft render target for encoder unwrapping.
func (rt *RenderTarget) InnerRenderTarget() backend.RenderTarget { return rt.RenderTarget }
