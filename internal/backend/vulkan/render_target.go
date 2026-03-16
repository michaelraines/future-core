//go:build !vulkan

package vulkan

import "github.com/michaelraines/future-render/internal/backend"

// RenderTarget implements backend.RenderTarget for Vulkan.
// Models a VkFramebuffer + VkRenderPass pair.
type RenderTarget struct {
	backend.RenderTarget // delegates all RenderTarget methods to inner
}

// InnerRenderTarget returns the wrapped soft render target for encoder unwrapping.
func (rt *RenderTarget) InnerRenderTarget() backend.RenderTarget { return rt.RenderTarget }
