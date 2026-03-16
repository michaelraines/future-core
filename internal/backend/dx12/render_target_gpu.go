//go:build dx12native

package dx12

import (
	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/d3d12"
)

// RenderTarget implements backend.RenderTarget for DX12.
type RenderTarget struct {
	dev      *Device
	colorTex *Texture
	depthTex backend.Texture
	w, h     int

	// DX12 resources for this render target.
	rtvHandle d3d12.CPUDescriptorHandle
}

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

// Dispose releases the render target's textures.
func (rt *RenderTarget) Dispose() {
	if rt.colorTex != nil {
		rt.colorTex.Dispose()
	}
	if rt.depthTex != nil {
		rt.depthTex.Dispose()
	}
}
