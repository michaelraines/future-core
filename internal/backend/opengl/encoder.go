//go:build darwin || linux || freebsd || windows

package opengl

import (
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/gl"
)

// commandEncoder implements backend.CommandEncoder for OpenGL.
type commandEncoder struct {
	// Cached sampler objects: one per TextureFilter value.
	samplerNearest uint32
	samplerLinear  uint32
	samplersReady  bool

	// indexFormat is the format set by the most recent SetIndexBuffer call.
	// Used by DrawIndexed to select between gl.UNSIGNED_SHORT and gl.UNSIGNED_INT.
	indexFormat backend.IndexFormat

	// currentFormat holds the vertex format from the most recent SetPipeline call.
	currentFormat backend.VertexFormat

	// prevAttribCount tracks how many vertex attributes were enabled by the
	// previous pipeline, so stale slots can be disabled on switch.
	prevAttribCount int

	// Stencil state cached from the most recent SetPipeline call. OpenGL
	// stencil ops change per-draw (not per-pipeline); we apply them
	// eagerly in SetPipeline and then re-issue glStencilFunc with the
	// same (func, mask) and a new reference value when
	// SetStencilReference is called.
	stencilEnabled  bool
	stencilFunc     uint32
	stencilReadMask uint32
}

// BeginRenderPass begins a render pass.
func (e *commandEncoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	if desc.Target != nil {
		rt := desc.Target.(*renderTarget)
		gl.BindFramebuffer(gl.FRAMEBUFFER, rt.fbo)
		gl.Viewport(0, 0, int32(rt.rtWidth), int32(rt.rtHeight))
	} else {
		gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	}

	if desc.LoadAction == backend.LoadActionClear {
		var mask uint32
		c := desc.ClearColor
		gl.ClearColor(c[0], c[1], c[2], c[3])
		mask |= gl.COLOR_BUFFER_BIT

		gl.ClearDepthf(desc.ClearDepth)
		mask |= gl.DEPTH_BUFFER_BIT

		gl.Clear(mask)
	}
}

// EndRenderPass ends the current render pass.
func (e *commandEncoder) EndRenderPass() {
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

// SetPipeline binds a render pipeline.
func (e *commandEncoder) SetPipeline(pipeline backend.Pipeline) {
	ps := pipeline.(*pipelineState)
	e.currentFormat = ps.desc.VertexFormat

	// Bind shader.
	s := ps.desc.Shader.(*shader)
	gl.UseProgram(s.program)

	// Blend mode.
	applyBlendMode(ps.desc.BlendMode)

	// Depth.
	if ps.desc.DepthTest {
		gl.Enable(gl.DEPTH_TEST)
		gl.DepthFunc(compareFuncToGL(ps.desc.DepthFunc))
	} else {
		gl.Disable(gl.DEPTH_TEST)
	}
	if ps.desc.DepthWrite {
		gl.DepthMask(true)
	} else {
		gl.DepthMask(false)
	}

	// Cull mode.
	switch ps.desc.CullMode {
	case backend.CullFront:
		gl.Enable(gl.CULL_FACE)
		gl.CullFace(gl.FRONT)
	case backend.CullBack:
		gl.Enable(gl.CULL_FACE)
		gl.CullFace(gl.BACK)
	default:
		gl.Disable(gl.CULL_FACE)
	}

	// Pipeline-baked color write mask. Stencil-only passes set this so
	// they populate the stencil buffer without touching color; the
	// cross-backend contract is "color write state lives on the pipeline
	// on WebGPU/Vulkan/Metal/DX12, and GL applies it eagerly here".
	// Runtime SetColorWrite callers can still override after SetPipeline.
	if ps.desc.ColorWriteDisabled {
		gl.ColorMask(false, false, false, false)
	} else {
		gl.ColorMask(true, true, true, true)
	}

	// Stencil state is baked into the pipeline on pipeline-native APIs
	// (WebGPU/Vulkan/Metal/DX12); on GL it's mutable state so we apply
	// it eagerly here from the pipeline's StencilDescriptor. Only the
	// Front face ops are applied here — OpenGL two-sided stencil
	// (glStencil*Separate) is follow-up work; the `internal/gl` package
	// currently only exposes the monolithic entry points.
	if ps.desc.StencilEnable {
		sd := ps.desc.Stencil
		gl.Enable(gl.STENCIL_TEST)
		e.stencilFunc = compareFuncToGL(sd.Func)
		e.stencilReadMask = sd.Mask
		e.stencilEnabled = true
		gl.StencilFunc(e.stencilFunc, 0, e.stencilReadMask)
		gl.StencilOp(
			stencilOpToGL(sd.Front.SFail),
			stencilOpToGL(sd.Front.DPFail),
			stencilOpToGL(sd.Front.DPPass),
		)
		gl.StencilMask(sd.WriteMask)
	} else if e.stencilEnabled {
		gl.Disable(gl.STENCIL_TEST)
		e.stencilEnabled = false
	}
}

// SetVertexBuffer binds a vertex buffer and configures vertex attribute
// pointers based on the current pipeline's vertex format. Attribute pointers
// snap to the currently-bound VBO, so this must be called after binding.
func (e *commandEncoder) SetVertexBuffer(buf backend.Buffer, slot int) {
	b := buf.(*buffer)
	gl.BindBuffer(gl.ARRAY_BUFFER, b.id)
	e.applyVertexFormat()
}

// applyVertexFormat configures vertex attribute pointers from the current
// pipeline's vertex format and disables any stale attribute slots left
// over from a previous pipeline with more attributes.
func (e *commandEncoder) applyVertexFormat() {
	count := len(e.currentFormat.Attributes)
	stride := int32(e.currentFormat.Stride)
	for i, attr := range e.currentFormat.Attributes {
		idx := uint32(i)
		size, typ := attributeFormatToGL(attr.Format)
		normalized := attr.Format == backend.AttributeByte4Norm
		gl.EnableVertexAttribArray(idx)
		gl.VertexAttribPointer(idx, size, typ, normalized, stride, uintptr(attr.Offset))
	}
	// Disable attribute slots that were enabled by the previous pipeline
	// but are no longer used by the current one.
	for i := count; i < e.prevAttribCount; i++ {
		gl.DisableVertexAttribArray(uint32(i))
	}
	e.prevAttribCount = count
}

// SetIndexBuffer binds an index buffer and records the index format
// for use in subsequent DrawIndexed calls.
func (e *commandEncoder) SetIndexBuffer(buf backend.Buffer, format backend.IndexFormat) {
	b := buf.(*buffer)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, b.id)
	e.indexFormat = format
}

// SetTexture binds a texture to a slot.
func (e *commandEncoder) SetTexture(tex backend.Texture, slot int) {
	t := tex.(*texture)
	gl.ActiveTexture(uint32(gl.TEXTURE0 + slot))
	gl.BindTexture(gl.TEXTURE_2D, t.id)
}

// SetTextureFilter overrides the texture filter for the given slot using
// a GL sampler object, decoupling filter state from the texture object.
func (e *commandEncoder) SetTextureFilter(slot int, filter backend.TextureFilter) {
	e.ensureSamplers()
	var sampler uint32
	switch filter {
	case backend.FilterLinear:
		sampler = e.samplerLinear
	default:
		sampler = e.samplerNearest
	}
	gl.BindSampler(uint32(slot), sampler)
}

// ensureSamplers lazily creates the cached sampler objects.
func (e *commandEncoder) ensureSamplers() {
	if e.samplersReady {
		return
	}
	gl.GenSamplers(1, &e.samplerNearest)
	gl.SamplerParameteri(e.samplerNearest, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.SamplerParameteri(e.samplerNearest, gl.TEXTURE_MAG_FILTER, gl.NEAREST)

	gl.GenSamplers(1, &e.samplerLinear)
	gl.SamplerParameteri(e.samplerLinear, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.SamplerParameteri(e.samplerLinear, gl.TEXTURE_MAG_FILTER, gl.LINEAR)

	e.samplersReady = true
}

// SetStencilReference updates the dynamic stencil reference value by
// re-issuing glStencilFunc with the pipeline's cached compare-func and
// read mask. A no-op when the current pipeline doesn't enable the
// stencil test.
func (e *commandEncoder) SetStencilReference(ref uint32) {
	if !e.stencilEnabled {
		return
	}
	gl.StencilFunc(e.stencilFunc, int32(ref), e.stencilReadMask)
}

// SetColorWrite enables or disables writing to the color buffer.
func (e *commandEncoder) SetColorWrite(enabled bool) {
	gl.ColorMask(enabled, enabled, enabled, enabled)
}

// SetViewport sets the rendering viewport.
func (e *commandEncoder) SetViewport(vp backend.Viewport) {
	gl.Viewport(int32(vp.X), int32(vp.Y), int32(vp.Width), int32(vp.Height))
}

// SetScissor sets the scissor rectangle.
func (e *commandEncoder) SetScissor(rect *backend.ScissorRect) {
	if rect == nil {
		gl.Disable(gl.SCISSOR_TEST)
		return
	}
	gl.Enable(gl.SCISSOR_TEST)
	gl.Scissor(int32(rect.X), int32(rect.Y), int32(rect.Width), int32(rect.Height))
}

// Draw issues a non-indexed draw call.
func (e *commandEncoder) Draw(vertexCount, instanceCount, firstVertex int) {
	if instanceCount <= 1 {
		gl.DrawArrays(gl.TRIANGLES, int32(firstVertex), int32(vertexCount))
	} else {
		gl.DrawArraysInstanced(gl.TRIANGLES, int32(firstVertex), int32(vertexCount), int32(instanceCount))
	}
}

// DrawIndexed issues an indexed draw call, using the index format
// set by the most recent SetIndexBuffer call.
func (e *commandEncoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	glType, stride := e.indexTypeAndStride()
	offset := glOffset(firstIndex * stride)
	if instanceCount <= 1 {
		gl.DrawElements(gl.TRIANGLES, int32(indexCount), glType, offset)
	} else {
		gl.DrawElementsInstanced(gl.TRIANGLES, int32(indexCount), glType, offset, int32(instanceCount))
	}
}

// indexTypeAndStride returns the GL index type enum and byte stride
// for the current index format.
func (e *commandEncoder) indexTypeAndStride() (uint32, int) {
	if e.indexFormat == backend.IndexUint32 {
		return gl.UNSIGNED_INT, 4
	}
	return gl.UNSIGNED_SHORT, 2
}

// glOffset converts a byte offset to an unsafe.Pointer for OpenGL
// buffer offset parameters (e.g. DrawElements index offset).
//
//nolint:govet // This is the standard pattern for OpenGL buffer offsets.
func glOffset(offset int) unsafe.Pointer {
	return unsafe.Pointer(uintptr(offset))
}

// Flush submits all recorded commands. For OpenGL this is a no-op since
// commands execute immediately.
// SetBlendMode is a no-op for this backend.
func (e *commandEncoder) SetBlendMode(_ backend.BlendMode) {}

func (e *commandEncoder) Flush() {
	gl.Flush()
}

// --- helpers ---

func applyBlendMode(mode backend.BlendMode) {
	if !mode.Enabled {
		gl.Disable(gl.BLEND)
		return
	}
	gl.Enable(gl.BLEND)
	gl.BlendFuncSeparate(
		glBlendFactor(mode.SrcFactorRGB),
		glBlendFactor(mode.DstFactorRGB),
		glBlendFactor(mode.SrcFactorAlpha),
		glBlendFactor(mode.DstFactorAlpha),
	)
	gl.BlendEquationSeparate(
		glBlendOp(mode.OpRGB),
		glBlendOp(mode.OpAlpha),
	)
}

// glBlendFactor maps a backend BlendFactor to its OpenGL enum.
func glBlendFactor(f backend.BlendFactor) uint32 {
	switch f {
	case backend.BlendFactorZero:
		return gl.ZERO
	case backend.BlendFactorOne:
		return gl.ONE
	case backend.BlendFactorSrcAlpha:
		return gl.SRC_ALPHA
	case backend.BlendFactorOneMinusSrcAlpha:
		return gl.ONE_MINUS_SRC_ALPHA
	case backend.BlendFactorDstAlpha:
		return gl.DST_ALPHA
	case backend.BlendFactorOneMinusDstAlpha:
		return gl.ONE_MINUS_DST_ALPHA
	case backend.BlendFactorSrcColor:
		return gl.SRC_COLOR
	case backend.BlendFactorOneMinusSrcColor:
		return gl.ONE_MINUS_SRC_COLOR
	case backend.BlendFactorDstColor:
		return gl.DST_COLOR
	case backend.BlendFactorOneMinusDstColor:
		return gl.ONE_MINUS_DST_COLOR
	default:
		return gl.ONE
	}
}

// glBlendOp maps a backend BlendOperation to its OpenGL enum.
func glBlendOp(op backend.BlendOperation) uint32 {
	switch op {
	case backend.BlendOpAdd:
		return gl.FUNC_ADD
	case backend.BlendOpSubtract:
		return gl.FUNC_SUBTRACT
	case backend.BlendOpReverseSubtract:
		return gl.FUNC_REVERSE_SUBTRACT
	case backend.BlendOpMin:
		return gl.MIN
	case backend.BlendOpMax:
		return gl.MAX
	default:
		return gl.FUNC_ADD
	}
}

func compareFuncToGL(f backend.CompareFunc) uint32 {
	switch f {
	case backend.CompareNever:
		return gl.NEVER
	case backend.CompareLess:
		return gl.LESS
	case backend.CompareLessEqual:
		return gl.LEQUAL
	case backend.CompareEqual:
		return gl.EQUAL
	case backend.CompareGreaterEqual:
		return gl.GEQUAL
	case backend.CompareGreater:
		return gl.GREATER
	case backend.CompareNotEqual:
		return gl.NOTEQUAL
	case backend.CompareAlways:
		return gl.ALWAYS
	default:
		return gl.LESS
	}
}

func stencilOpToGL(op backend.StencilOp) uint32 {
	switch op {
	case backend.StencilKeep:
		return gl.KEEP
	case backend.StencilZero:
		return gl.ZERO
	case backend.StencilReplace:
		return gl.REPLACE
	case backend.StencilIncr:
		return gl.INCR
	case backend.StencilDecr:
		return gl.DECR
	case backend.StencilInvert:
		return gl.INVERT
	case backend.StencilIncrWrap:
		return gl.INCR_WRAP
	case backend.StencilDecrWrap:
		return gl.DECR_WRAP
	default:
		return gl.KEEP
	}
}
