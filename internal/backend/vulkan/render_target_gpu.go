//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/vk"
)

// RenderTarget implements backend.RenderTarget for Vulkan.
//
// Each RT owns a dedicated framebuffer bound to its color attachment's
// image view (plus a private depth-stencil attachment) and a pair of
// VkRenderPass objects — one with LoadOp=Clear for the first batch
// entering the RT, one with LoadOp=Load for subsequent re-entries that
// must preserve earlier content within the frame. The encoder picks
// between them in BeginRenderPass based on the caller's LoadAction.
// Pipelines cache a VkPipeline per VkRenderPass they're asked to run
// against, so the pipeline side of this split is already covered.
type RenderTarget struct {
	dev        *Device
	colorTex   *Texture
	depthTex   backend.Texture
	w, h       int
	hasStencil bool

	// Vulkan resources for this render target.
	renderPass     vk.RenderPass // LoadOp=Clear path (selected when LoadAction=Clear)
	renderPassLoad vk.RenderPass // LoadOp=Load path (selected when LoadAction=Load)
	framebuffer    vk.Framebuffer

	// Per-RT depth-stencil attachment (color attaches via colorTex.view).
	dsImage vk.Image
	dsMem   vk.DeviceMemory
	dsView  vk.ImageView
}

// HasStencil reports whether the render target was created with a stencil
// attachment.
func (rt *RenderTarget) HasStencil() bool { return rt.hasStencil }

// InnerRenderTarget returns nil for GPU render targets (no soft delegation).
func (rt *RenderTarget) InnerRenderTarget() backend.RenderTarget { return nil }

// ColorTexture returns the color attachment texture.
func (rt *RenderTarget) ColorTexture() backend.Texture { return rt.colorTex }

// DepthTexture returns the depth attachment texture, if any.
func (rt *RenderTarget) DepthTexture() backend.Texture { return rt.depthTex }

// Width returns the render target width.
func (rt *RenderTarget) Width() int { return rt.w }

// Height returns the render target height.
func (rt *RenderTarget) Height() int { return rt.h }

// Dispose releases the render target's Vulkan resources and textures.
// Waits for device idle first because pipelines baked against our render
// passes may still be in flight; destroying the render pass under them
// is undefined behaviour.
func (rt *RenderTarget) Dispose() {
	if rt.dev != nil && rt.dev.device != 0 {
		vk.DeviceWaitIdle(rt.dev.device)
		if rt.framebuffer != 0 {
			vk.DestroyFramebuffer(rt.dev.device, rt.framebuffer)
			rt.framebuffer = 0
		}
		if rt.renderPass != 0 {
			vk.DestroyRenderPass(rt.dev.device, rt.renderPass)
			rt.renderPass = 0
		}
		if rt.renderPassLoad != 0 {
			vk.DestroyRenderPass(rt.dev.device, rt.renderPassLoad)
			rt.renderPassLoad = 0
		}
		if rt.dsView != 0 || rt.dsImage != 0 || rt.dsMem != 0 {
			rt.dev.destroyDepthStencilTexture(rt.dsImage, rt.dsMem, rt.dsView)
			rt.dsView = 0
			rt.dsImage = 0
			rt.dsMem = 0
		}
	}
	if rt.colorTex != nil {
		rt.colorTex.Dispose()
	}
	if rt.depthTex != nil {
		rt.depthTex.Dispose()
	}
}
