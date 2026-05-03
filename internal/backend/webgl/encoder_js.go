//go:build js

package webgl

import (
	"syscall/js"

	"github.com/michaelraines/future-core/internal/backend"
)

// Encoder implements backend.CommandEncoder for WebGL2 using syscall/js.
type Encoder struct {
	gl  js.Value
	dev *Device

	inRenderPass    bool
	currentPipeline *Pipeline
	indexFormat     backend.IndexFormat

	// targetIsOffscreen is set by BeginRenderPass and read by the
	// pre-draw shader.apply() to decide whether to flip uProjection.
	// WebGL's NDC is Y-up; the engine's ortho is Y-down. The window FB
	// gets a free flip on display, but offscreen FBOs don't — sampling
	// them as textures otherwise reads upside-down. Mirrors the
	// OpenGL desktop and Vulkan handling.
	targetIsOffscreen bool

	// pendingBlend is the sticky SetBlendMode override. The engine
	// calls SetBlendMode before SetPipeline per-batch so blend changes
	// don't require pipeline rebuilds; the pipeline picks up the
	// override here instead of using its descriptor default. Mirrors
	// the WebGPU/Vulkan/Metal/OpenGL contract — without it,
	// BlendLighter / multiply / additive batches silently render as
	// SourceOver. has==false means "use pipeline default".
	pendingBlend    backend.BlendMode
	pendingBlendHas bool

	// Cached stencil state from the most recently bound pipeline. WebGL2
	// stencil ops are mutable per-draw (unlike WebGPU where they're
	// baked into the pipeline), so SetPipeline applies the full state
	// eagerly here. SetStencilReference re-issues glStencilFuncSeparate
	// with the cached compare func + read mask and a new reference.
	// stencilWriteMask is cached so BeginRenderPass's stencil-clear
	// path can restore it after temporarily forcing writeMask=0xFF.
	stencilEnabled   bool
	stencilFunc      int
	stencilReadMask  int
	stencilWriteMask int
}

// BeginRenderPass begins a WebGL2 render pass.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	if desc.Target != nil {
		if rt, ok := desc.Target.(*RenderTarget); ok {
			e.gl.Call("bindFramebuffer", e.gl.Get("FRAMEBUFFER").Int(), rt.fbo)
			e.gl.Call("viewport", 0, 0, rt.w, rt.h)
			e.targetIsOffscreen = true
		}
	} else {
		e.gl.Call("bindFramebuffer", e.gl.Get("FRAMEBUFFER").Int(), js.Null())
		e.gl.Call("viewport", 0, 0, e.dev.width, e.dev.height)
		e.targetIsOffscreen = false
	}

	// Reset per-pass dynamic state that leaks across glBindFramebuffer
	// the same way OpenGL desktop does (see opengl/encoder.go for the
	// full writeup): a previous stencil-write pipeline can leave
	// ColorMask=(F,F,F,F) and a stale scissor enabled. Either silently
	// suppresses the gl.Clear below — and on subsequent draws,
	// suppresses the draw itself. SetPipeline / SetScissor will
	// re-establish the correct state for the new pass.
	e.gl.Call("colorMask", true, true, true, true)
	e.gl.Call("disable", e.gl.Get("SCISSOR_TEST").Int())

	if desc.LoadAction == backend.LoadActionClear {
		c := desc.ClearColor
		e.gl.Call("clearColor", c[0], c[1], c[2], c[3])
		e.gl.Call("clear", e.gl.Get("COLOR_BUFFER_BIT").Int()|e.gl.Get("DEPTH_BUFFER_BIT").Int())
	}
	if desc.StencilLoadAction == backend.LoadActionClear {
		e.gl.Call("clearStencil", int(desc.ClearStencil))
		// A previous glStencilMask(front, 0) from a no-op stencil
		// pipeline can suppress the clear; re-enable full-mask writes
		// around the clear to guarantee the buffer is zeroed.
		e.gl.Call("stencilMask", 0xFF)
		e.gl.Call("clear", e.gl.Get("STENCIL_BUFFER_BIT").Int())
		// Restore the pipeline's cached writeMask. Without this the
		// next draw issued before SetPipeline rebinds would write
		// stencil through the wide mask left over from the clear,
		// even though the pipeline's baked state says narrower.
		if e.stencilEnabled {
			front := e.gl.Get("FRONT").Int()
			back := e.gl.Get("BACK").Int()
			e.gl.Call("stencilMaskSeparate", front, e.stencilWriteMask)
			e.gl.Call("stencilMaskSeparate", back, e.stencilWriteMask)
		}
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

// SetPipeline applies pipeline state (blend mode, shader program, vertex
// attributes). Also eagerly applies pipeline-baked color write and
// stencil state: on WebGL2 these are mutable encoder state rather than
// compiled into the pipeline object, so we re-issue them every time a
// pipeline is bound.
//
// Honours the sticky SetBlendMode override (pendingBlend) before falling
// back to the pipeline's descriptor blend. The override clears after
// being consumed so the next pipeline rebind without an intervening
// SetBlendMode picks up the descriptor default — same contract as
// every other backend.
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	p, ok := pipeline.(*Pipeline)
	if !ok {
		return
	}
	e.currentPipeline = p
	if e.pendingBlendHas {
		e.applyBlendMode(e.pendingBlend)
		e.pendingBlendHas = false
	} else {
		e.applyBlendMode(p.desc.BlendMode)
	}
	p.bind()

	// Color write mask baked into the pipeline. Stencil-only passes set
	// ColorWriteDisabled=true so the draw updates the stencil buffer
	// without touching color. Non-stencil pipelines default back to
	// writing all channels.
	if p.desc.ColorWriteDisabled {
		e.gl.Call("colorMask", false, false, false, false)
	} else {
		e.gl.Call("colorMask", true, true, true, true)
	}

	// Pipeline-baked stencil state. The reference value stays dynamic
	// and is updated via SetStencilReference; here we cache the compare
	// func and read mask so re-issuing glStencilFuncSeparate uses the
	// same test against a new ref.
	if p.desc.StencilEnable {
		sd := p.desc.Stencil
		e.gl.Call("enable", e.gl.Get("STENCIL_TEST").Int())
		e.stencilEnabled = true
		e.stencilFunc = glCompareFunc(e.gl, sd.Func)
		e.stencilReadMask = int(sd.Mask)
		front := e.gl.Get("FRONT").Int()
		back := e.gl.Get("BACK").Int()
		backOps := sd.Front
		if sd.TwoSided {
			backOps = sd.Back
		}
		e.gl.Call("stencilFuncSeparate", front,
			e.stencilFunc, 0, e.stencilReadMask)
		e.gl.Call("stencilFuncSeparate", back,
			e.stencilFunc, 0, e.stencilReadMask)
		e.gl.Call("stencilOpSeparate", front,
			glStencilOp(e.gl, sd.Front.SFail),
			glStencilOp(e.gl, sd.Front.DPFail),
			glStencilOp(e.gl, sd.Front.DPPass))
		e.gl.Call("stencilOpSeparate", back,
			glStencilOp(e.gl, backOps.SFail),
			glStencilOp(e.gl, backOps.DPFail),
			glStencilOp(e.gl, backOps.DPPass))
		e.stencilWriteMask = int(sd.WriteMask)
		e.gl.Call("stencilMaskSeparate", front, e.stencilWriteMask)
		e.gl.Call("stencilMaskSeparate", back, e.stencilWriteMask)
	} else if e.stencilEnabled {
		e.gl.Call("disable", e.gl.Get("STENCIL_TEST").Int())
		e.stencilEnabled = false
	}
}

// glCompareFunc returns the WebGL2 compare constant for a backend
// CompareFunc. Shared between stencil and (future) depth compare.
func glCompareFunc(gl js.Value, cf backend.CompareFunc) int {
	switch cf {
	case backend.CompareNever:
		return gl.Get("NEVER").Int()
	case backend.CompareLess:
		return gl.Get("LESS").Int()
	case backend.CompareLessEqual:
		return gl.Get("LEQUAL").Int()
	case backend.CompareEqual:
		return gl.Get("EQUAL").Int()
	case backend.CompareGreaterEqual:
		return gl.Get("GEQUAL").Int()
	case backend.CompareGreater:
		return gl.Get("GREATER").Int()
	case backend.CompareNotEqual:
		return gl.Get("NOTEQUAL").Int()
	case backend.CompareAlways:
		return gl.Get("ALWAYS").Int()
	default:
		return gl.Get("ALWAYS").Int()
	}
}

// glStencilOp returns the WebGL2 constant for a backend StencilOp.
func glStencilOp(gl js.Value, op backend.StencilOp) int {
	switch op {
	case backend.StencilZero:
		return gl.Get("ZERO").Int()
	case backend.StencilReplace:
		return gl.Get("REPLACE").Int()
	case backend.StencilIncr:
		return gl.Get("INCR").Int()
	case backend.StencilDecr:
		return gl.Get("DECR").Int()
	case backend.StencilInvert:
		return gl.Get("INVERT").Int()
	case backend.StencilIncrWrap:
		return gl.Get("INCR_WRAP").Int()
	case backend.StencilDecrWrap:
		return gl.Get("DECR_WRAP").Int()
	default: // StencilKeep
		return gl.Get("KEEP").Int()
	}
}

// applyBlendMode sets WebGL2 blend state from a backend blend mode.
// Honours arbitrary factor/operation combinations via
// blendFuncSeparate + blendEquationSeparate.
func (e *Encoder) applyBlendMode(mode backend.BlendMode) {
	if !mode.Enabled {
		e.gl.Call("disable", e.gl.Get("BLEND").Int())
		return
	}
	e.gl.Call("enable", e.gl.Get("BLEND").Int())
	e.gl.Call("blendFuncSeparate",
		glBlendFactor(e.gl, mode.SrcFactorRGB),
		glBlendFactor(e.gl, mode.DstFactorRGB),
		glBlendFactor(e.gl, mode.SrcFactorAlpha),
		glBlendFactor(e.gl, mode.DstFactorAlpha))
	e.gl.Call("blendEquationSeparate",
		glBlendOp(e.gl, mode.OpRGB),
		glBlendOp(e.gl, mode.OpAlpha))
}

// glBlendFactor returns the WebGL2 constant for a backend BlendFactor.
func glBlendFactor(gl js.Value, f backend.BlendFactor) int {
	switch f {
	case backend.BlendFactorZero:
		return gl.Get("ZERO").Int()
	case backend.BlendFactorOne:
		return gl.Get("ONE").Int()
	case backend.BlendFactorSrcAlpha:
		return gl.Get("SRC_ALPHA").Int()
	case backend.BlendFactorOneMinusSrcAlpha:
		return gl.Get("ONE_MINUS_SRC_ALPHA").Int()
	case backend.BlendFactorDstAlpha:
		return gl.Get("DST_ALPHA").Int()
	case backend.BlendFactorOneMinusDstAlpha:
		return gl.Get("ONE_MINUS_DST_ALPHA").Int()
	case backend.BlendFactorSrcColor:
		return gl.Get("SRC_COLOR").Int()
	case backend.BlendFactorOneMinusSrcColor:
		return gl.Get("ONE_MINUS_SRC_COLOR").Int()
	case backend.BlendFactorDstColor:
		return gl.Get("DST_COLOR").Int()
	case backend.BlendFactorOneMinusDstColor:
		return gl.Get("ONE_MINUS_DST_COLOR").Int()
	default:
		return gl.Get("ONE").Int()
	}
}

// glBlendOp returns the WebGL2 constant for a backend BlendOperation.
func glBlendOp(gl js.Value, op backend.BlendOperation) int {
	switch op {
	case backend.BlendOpAdd:
		return gl.Get("FUNC_ADD").Int()
	case backend.BlendOpSubtract:
		return gl.Get("FUNC_SUBTRACT").Int()
	case backend.BlendOpReverseSubtract:
		return gl.Get("FUNC_REVERSE_SUBTRACT").Int()
	case backend.BlendOpMin:
		return gl.Get("MIN").Int()
	case backend.BlendOpMax:
		return gl.Get("MAX").Int()
	default:
		return gl.Get("FUNC_ADD").Int()
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

// SetStencilReference updates the dynamic stencil reference value by
// re-issuing glStencilFuncSeparate for both faces with the cached
// compare func + read mask. A no-op when the current pipeline didn't
// enable the stencil test.
func (e *Encoder) SetStencilReference(ref uint32) {
	if !e.stencilEnabled {
		return
	}
	front := e.gl.Get("FRONT").Int()
	back := e.gl.Get("BACK").Int()
	e.gl.Call("stencilFuncSeparate", front,
		e.stencilFunc, int(ref), e.stencilReadMask)
	e.gl.Call("stencilFuncSeparate", back,
		e.stencilFunc, int(ref), e.stencilReadMask)
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
	e.applyShaderUniforms()
	if instanceCount <= 1 {
		e.gl.Call("drawArrays", e.gl.Get("TRIANGLES").Int(), firstVertex, vertexCount)
	} else {
		e.gl.Call("drawArraysInstanced", e.gl.Get("TRIANGLES").Int(),
			firstVertex, vertexCount, instanceCount)
	}
}

// DrawIndexed issues an indexed draw call.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	e.applyShaderUniforms()
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

// SetBlendMode records a sticky blend-mode override that the next
// SetPipeline picks up. The engine calls SetBlendMode BEFORE
// SetPipeline per-batch; without this override the pipeline's
// descriptor blend wins and BlendLighter/multiply/etc silently render
// as SourceOver. Mirrors the contract on every other backend.
func (e *Encoder) SetBlendMode(mode backend.BlendMode) {
	e.pendingBlend = mode
	e.pendingBlendHas = true
}

// applyShaderUniforms pushes the current shader's cached uniforms to
// the GL program. Called before every draw — the cached map covers
// uProjection + per-batch uniforms (uColorBody, uColorTranslation,
// uTexture); pushing them per-draw is cheap relative to the JS bridge
// crossing already happening per draw.
func (e *Encoder) applyShaderUniforms() {
	if e.currentPipeline == nil {
		return
	}
	sh, ok := e.currentPipeline.desc.Shader.(*Shader)
	if !ok || sh == nil {
		return
	}
	sh.apply(e.targetIsOffscreen)
}

// Flush is a no-op for WebGL2 — presentation happens automatically as
// the canvas is composited each frame.
func (e *Encoder) Flush() {}
