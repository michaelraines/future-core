//go:build !(darwin || linux || freebsd || windows) || soft

package webgpu

import "github.com/michaelraines/future-core/internal/backend"

// RenderTarget implements backend.RenderTarget for WebGPU.
type RenderTarget struct {
	backend.RenderTarget // delegates all RenderTarget methods to inner
}

// InnerRenderTarget returns the wrapped soft render target for encoder unwrapping.
func (rt *RenderTarget) InnerRenderTarget() backend.RenderTarget { return rt.RenderTarget }
