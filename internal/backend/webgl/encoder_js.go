//go:build js

package webgl

import (
	"syscall/js"

	"github.com/michaelraines/future-render/internal/backend"
)

// Encoder implements backend.CommandEncoder for WebGL2 using syscall/js.
type Encoder struct {
	gl     js.Value
	width  int
	height int

	inRenderPass    bool
	currentPipeline *Pipeline
	indexFormat     backend.IndexFormat
}

// BeginRenderPass begins a WebGL2 render pass.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	if desc.Target != nil {
		if rt, ok := desc.Target.(*RenderTarget); ok {
			e.gl.Call("bindFramebuffer", e.gl.Get("FRAMEBUFFER").Int(), rt.fbo)
			e.gl.Call("viewport", 0, 0, rt.w, rt.h)
		}
	} else {
		e.gl.Call("bindFramebuffer", e.gl.Get("FRAMEBUFFER").Int(), js.Null())
		e.gl.Call("viewport", 0, 0, e.width, e.height)
	}

	if desc.LoadAction == backend.LoadActionClear {
		c := desc.ClearColor
		e.gl.Call("clearColor", c[0], c[1], c[2], c[3])
		e.gl.Call("clear", e.gl.Get("COLOR_BUFFER_BIT").Int()|e.gl.Get("DEPTH_BUFFER_BIT").Int())
	}

	e.inRenderPass = true
}

// EndRenderPass ends the current render pass.
func (e *Encoder) EndRenderPass() {
	if e.inRenderPass {
		e.gl.Call("bindFramebuffer", e.gl.Get("FRAMEBUFFER").Int(), js.Null())
		e.inRenderPass = false
	}
}

// SetPipeline applies pipeline state (blend mode, shader program, vertex attributes).
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	if p, ok := pipeline.(*Pipeline); ok {
		e.currentPipeline = p
		e.applyBlendMode(p.desc.BlendMode)
		p.bind()
	}
}

// applyBlendMode sets WebGL2 blend state from a backend blend mode.
func (e *Encoder) applyBlendMode(mode backend.BlendMode) {
	switch mode {
	case backend.BlendSourceOver:
		e.gl.Call("enable", e.gl.Get("BLEND").Int())
		e.gl.Call("blendFunc",
			e.gl.Get("SRC_ALPHA").Int(),
			e.gl.Get("ONE_MINUS_SRC_ALPHA").Int())
	case backend.BlendAdditive:
		e.gl.Call("enable", e.gl.Get("BLEND").Int())
		e.gl.Call("blendFunc",
			e.gl.Get("SRC_ALPHA").Int(),
			e.gl.Get("ONE").Int())
	case backend.BlendMultiplicative:
		e.gl.Call("enable", e.gl.Get("BLEND").Int())
		e.gl.Call("blendFunc",
			e.gl.Get("DST_COLOR").Int(),
			e.gl.Get("ZERO").Int())
	case backend.BlendPremultiplied:
		e.gl.Call("enable", e.gl.Get("BLEND").Int())
		e.gl.Call("blendFunc",
			e.gl.Get("ONE").Int(),
			e.gl.Get("ONE_MINUS_SRC_ALPHA").Int())
	default:
		e.gl.Call("disable", e.gl.Get("BLEND").Int())
	}
}

// SetVertexBuffer binds a vertex buffer to a slot.
func (e *Encoder) SetVertexBuffer(buf backend.Buffer, slot int) {
	if b, ok := buf.(*Buffer); ok {
		target := glBufferTarget(e.gl, b.usage)
		e.gl.Call("bindBuffer", target, b.handle)
	}
}

// SetIndexBuffer binds an index buffer.
func (e *Encoder) SetIndexBuffer(buf backend.Buffer, format backend.IndexFormat) {
	if b, ok := buf.(*Buffer); ok {
		e.gl.Call("bindBuffer", e.gl.Get("ELEMENT_ARRAY_BUFFER").Int(), b.handle)
		e.indexFormat = format
	}
}

// SetTexture binds a texture to a texture unit.
func (e *Encoder) SetTexture(tex backend.Texture, slot int) {
	if t, ok := tex.(*Texture); ok {
		e.gl.Call("activeTexture", e.gl.Get("TEXTURE0").Int()+slot)
		e.gl.Call("bindTexture", e.gl.Get("TEXTURE_2D").Int(), t.handle)
	}
}

// SetTextureFilter sets the texture filter for a slot.
func (e *Encoder) SetTextureFilter(slot int, filter backend.TextureFilter) {
	e.gl.Call("activeTexture", e.gl.Get("TEXTURE0").Int()+slot)
	tex2D := e.gl.Get("TEXTURE_2D").Int()
	glFilter := e.gl.Get("NEAREST").Int()
	if filter == backend.FilterLinear {
		glFilter = e.gl.Get("LINEAR").Int()
	}
	e.gl.Call("texParameteri", tex2D,
		e.gl.Get("TEXTURE_MIN_FILTER").Int(), glFilter)
	e.gl.Call("texParameteri", tex2D,
		e.gl.Get("TEXTURE_MAG_FILTER").Int(), glFilter)
}

// SetStencil configures stencil test state.
func (e *Encoder) SetStencil(enable bool, desc backend.StencilDescriptor) {
	if enable {
		e.gl.Call("enable", e.gl.Get("STENCIL_TEST").Int())
	} else {
		e.gl.Call("disable", e.gl.Get("STENCIL_TEST").Int())
	}
}

// SetColorWrite enables or disables color writing.
func (e *Encoder) SetColorWrite(enabled bool) {
	e.gl.Call("colorMask", enabled, enabled, enabled, enabled)
}

// SetViewport sets the rendering viewport.
func (e *Encoder) SetViewport(vp backend.Viewport) {
	e.gl.Call("viewport", vp.X, vp.Y, vp.Width, vp.Height)
}

// SetScissor sets the scissor rectangle.
func (e *Encoder) SetScissor(rect *backend.ScissorRect) {
	if rect == nil {
		e.gl.Call("disable", e.gl.Get("SCISSOR_TEST").Int())
		return
	}
	e.gl.Call("enable", e.gl.Get("SCISSOR_TEST").Int())
	e.gl.Call("scissor", rect.X, rect.Y, rect.Width, rect.Height)
}

// Draw issues a non-indexed draw call.
func (e *Encoder) Draw(vertexCount, instanceCount, firstVertex int) {
	if instanceCount <= 1 {
		e.gl.Call("drawArrays", e.gl.Get("TRIANGLES").Int(), firstVertex, vertexCount)
	} else {
		e.gl.Call("drawArraysInstanced", e.gl.Get("TRIANGLES").Int(),
			firstVertex, vertexCount, instanceCount)
	}
}

// DrawIndexed issues an indexed draw call.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	idxType := e.gl.Get("UNSIGNED_SHORT").Int()
	byteOffset := firstIndex * 2
	if e.indexFormat == backend.IndexUint32 {
		idxType = e.gl.Get("UNSIGNED_INT").Int()
		byteOffset = firstIndex * 4
	}
	if instanceCount <= 1 {
		e.gl.Call("drawElements", e.gl.Get("TRIANGLES").Int(),
			indexCount, idxType, byteOffset)
	} else {
		e.gl.Call("drawElementsInstanced", e.gl.Get("TRIANGLES").Int(),
			indexCount, idxType, byteOffset, instanceCount)
	}
}

// Flush is a no-op for WebGL2 — presentation happens automatically.
func (e *Encoder) Flush() {}
