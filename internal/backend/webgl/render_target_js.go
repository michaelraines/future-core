//go:build js

package webgl

import (
	"syscall/js"

	"github.com/michaelraines/future-core/internal/backend"
)

// RenderTarget implements backend.RenderTarget for WebGL2.
type RenderTarget struct {
	gl         js.Value
	fbo        js.Value
	stencilRB  js.Value // packed depth24+stencil8 renderbuffer, js.Null() when unused
	colorTex   *Texture
	depthTex   backend.Texture
	w, h       int
	hasStencil bool
}

// HasStencil reports whether a stencil attachment was requested. WebGL
// encoder-side stencil wiring is follow-up work; the RT tracks the flag
// for consistency with other backends.
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

// Dispose releases the render target's framebuffer and textures.
func (rt *RenderTarget) Dispose() {
	rt.gl.Call("deleteFramebuffer", rt.fbo)
	if !rt.stencilRB.IsNull() && !rt.stencilRB.IsUndefined() {
		rt.gl.Call("deleteRenderbuffer", rt.stencilRB)
	}
	if rt.colorTex != nil {
		rt.colorTex.Dispose()
	}
	if rt.depthTex != nil {
		rt.depthTex.Dispose()
	}
}
