package vulkan

import "github.com/michaelraines/future-render/internal/backend"

// RenderTarget implements backend.RenderTarget for Vulkan.
// Models a VkFramebuffer + VkRenderPass pair.
type RenderTarget struct {
	inner backend.RenderTarget
}

// ColorTexture returns the color attachment texture.
func (rt *RenderTarget) ColorTexture() backend.Texture {
	return rt.inner.ColorTexture()
}

// DepthTexture returns the depth attachment texture, if any.
func (rt *RenderTarget) DepthTexture() backend.Texture {
	return rt.inner.DepthTexture()
}

// Width returns the render target width.
func (rt *RenderTarget) Width() int { return rt.inner.Width() }

// Height returns the render target height.
func (rt *RenderTarget) Height() int { return rt.inner.Height() }

// Dispose releases the render target.
func (rt *RenderTarget) Dispose() {
	rt.inner.Dispose()
}
