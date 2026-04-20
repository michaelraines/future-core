//go:build js && !soft

package webgpu

import (
	"syscall/js"

	"github.com/michaelraines/future-core/internal/backend"
)

// Pipeline implements backend.Pipeline for WebGPU via the browser JS API.
type Pipeline struct {
	dev    *Device
	desc   backend.PipelineDescriptor
	handle js.Value

	// The texture format and blend mode this pipeline was compiled for.
	createdFormat string
	createdBlend  backend.BlendMode

	// Cached bind group layouts.
	uniformBGL js.Value
	textureBGL js.Value
	layout     js.Value
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

// ensurePipelineForFormat creates or recreates the pipeline if the target
// format has changed (e.g. switching between canvas and offscreen targets).
func (p *Pipeline) ensurePipelineForFormat(format string) {
	p.ensurePipeline(format, p.desc.BlendMode)
}

// ensurePipeline creates or recreates the pipeline if the target format
// or blend mode has changed. Custom shader draws may use different blend
// modes per-draw (e.g., additive for light accumulation, multiply for
// lightmap compositing).
func (p *Pipeline) ensurePipeline(format string, blend backend.BlendMode) {
	hasHandle := !p.handle.IsUndefined() && !p.handle.IsNull()
	if hasHandle && p.createdFormat == format && p.createdBlend == blend {
		return
	}
	p.createdFormat = format
	p.createdBlend = blend
	p.desc.BlendMode = blend
	p.createPipeline()
}

// createPipeline lazily compiles the shader and creates the render pipeline.
func (p *Pipeline) createPipeline() {
	shader, ok := p.desc.Shader.(*Shader)
	if !ok || shader == nil {
		return
	}

	shader.compile()
	if shader.vertexModule.IsUndefined() || shader.vertexModule.IsNull() {
		return
	}
	if shader.fragmentModule.IsUndefined() || shader.fragmentModule.IsNull() {
		return
	}

	p.createBindGroupLayouts()
	if p.layout.IsUndefined() || p.layout.IsNull() {
		return
	}

	// Build vertex buffer layout.
	var vertexBuffers js.Value
	if len(p.desc.VertexFormat.Attributes) > 0 {
		attrs := js.Global().Get("Array").New()
		stride := p.desc.VertexFormat.Stride
		for i, a := range p.desc.VertexFormat.Attributes {
			attr := js.Global().Get("Object").New()
			attr.Set("format", jsVertexFormat(a.Format))
			attr.Set("offset", a.Offset)
			attr.Set("shaderLocation", i)
			attrs.Call("push", attr)
		}
		if stride == 0 {
			for _, a := range p.desc.VertexFormat.Attributes {
				end := a.Offset + backend.AttributeFormatSize(a.Format)
				if end > stride {
					stride = end
				}
			}
		}
		bufLayout := js.Global().Get("Object").New()
		bufLayout.Set("arrayStride", stride)
		bufLayout.Set("stepMode", "vertex")
		bufLayout.Set("attributes", attrs)
		vertexBuffers = js.Global().Get("Array").New(bufLayout)
	} else {
		vertexBuffers = js.Global().Get("Array").New()
	}

	// Vertex state.
	vertex := js.Global().Get("Object").New()
	vertex.Set("module", shader.vertexModule)
	vertex.Set("entryPoint", "vs_main")
	vertex.Set("buffers", vertexBuffers)

	// Fragment state with blend.
	target := js.Global().Get("Object").New()
	// Use the format determined by the encoder's current render target.
	targetFormat := p.createdFormat
	if targetFormat == "" {
		targetFormat = "rgba8unorm"
	}
	target.Set("format", targetFormat)
	if p.desc.ColorWriteDisabled {
		target.Set("writeMask", 0)
	} else {
		target.Set("writeMask", 0xF)
	}

	blend := jsBlendState(p.desc.BlendMode)
	if !blend.IsUndefined() {
		target.Set("blend", blend)
	}

	fragment := js.Global().Get("Object").New()
	fragment.Set("module", shader.fragmentModule)
	fragment.Set("entryPoint", "fs_main")
	fragment.Set("targets", js.Global().Get("Array").New(target))

	// Primitive state.
	primitive := js.Global().Get("Object").New()
	primitive.Set("topology", jsTopology(p.desc.Primitive))
	primitive.Set("frontFace", "ccw")
	primitive.Set("cullMode", jsCullMode(p.desc.CullMode))

	// Pipeline descriptor.
	pipeDesc := js.Global().Get("Object").New()
	pipeDesc.Set("layout", p.layout)
	pipeDesc.Set("vertex", vertex)
	pipeDesc.Set("fragment", fragment)
	pipeDesc.Set("primitive", primitive)

	// Depth/stencil is declared unconditionally on the browser backend
	// because every render pass — including screen passes — attaches a
	// depth24plus-stencil8 view (device_js.go allocates a screen-wide one
	// and offscreen RTs always carry one). WebGPU requires the pipeline's
	// declared depth-stencil format to exactly match the render pass's
	// attachment, so pipelines that don't otherwise care about depth or
	// stencil get an "always"/"keep" no-op state here.
	depthStencil := js.Global().Get("Object").New()
	depthStencil.Set("format", "depth24plus-stencil8")
	depthStencil.Set("depthWriteEnabled", p.desc.DepthWrite)
	if p.desc.DepthTest {
		depthStencil.Set("depthCompare", jsCompareFunc(p.desc.DepthFunc))
	} else {
		depthStencil.Set("depthCompare", "always")
	}
	if p.desc.StencilEnable {
		jsApplyStencilState(depthStencil, p.desc.Stencil)
	} else {
		jsApplyStencilNoop(depthStencil)
	}
	pipeDesc.Set("depthStencil", depthStencil)

	// Multisample.
	multisample := js.Global().Get("Object").New()
	multisample.Set("count", 1)
	pipeDesc.Set("multisample", multisample)

	p.handle = p.dev.device.Call("createRenderPipeline", pipeDesc)
	if p.handle.IsUndefined() || p.handle.IsNull() {
		js.Global().Get("console").Call("error", "[webgpu] createRenderPipeline returned null/undefined")
	} else if diagLogAllow(&pipelineLogCount) {
		js.Global().Get("console").Call("log", "[webgpu] createRenderPipeline OK: format="+targetFormat+
			" depthTest="+boolStr(p.desc.DepthTest)+
			" stencilEnable="+boolStr(p.desc.StencilEnable)+
			" colorWriteDisabled="+boolStr(p.desc.ColorWriteDisabled))
	}
}

var pipelineLogCount int

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// createBindGroupLayouts creates uniform (group 0) and texture (group 1) layouts.
func (p *Pipeline) createBindGroupLayouts() {
	// Group 0: Uniform buffer.
	uniformEntry := js.Global().Get("Object").New()
	uniformEntry.Set("binding", 0)
	uniformEntry.Set("visibility", 3) // VERTEX | FRAGMENT
	bufferLayout := js.Global().Get("Object").New()
	bufferLayout.Set("type", "uniform")
	bufferLayout.Set("hasDynamicOffset", true)
	uniformEntry.Set("buffer", bufferLayout)

	uniformBGLDesc := js.Global().Get("Object").New()
	uniformBGLDesc.Set("entries", js.Global().Get("Array").New(uniformEntry))
	p.uniformBGL = p.dev.device.Call("createBindGroupLayout", uniformBGLDesc)

	// Group 1: Texture + sampler.
	texEntry := js.Global().Get("Object").New()
	texEntry.Set("binding", 0)
	texEntry.Set("visibility", 2) // FRAGMENT
	texLayout := js.Global().Get("Object").New()
	texLayout.Set("sampleType", "float")
	texLayout.Set("viewDimension", "2d")
	texEntry.Set("texture", texLayout)

	sampEntry := js.Global().Get("Object").New()
	sampEntry.Set("binding", 1)
	sampEntry.Set("visibility", 2) // FRAGMENT
	sampLayout := js.Global().Get("Object").New()
	sampLayout.Set("type", "filtering")
	sampEntry.Set("sampler", sampLayout)

	textureBGLDesc := js.Global().Get("Object").New()
	textureBGLDesc.Set("entries", js.Global().Get("Array").New(texEntry, sampEntry))
	p.textureBGL = p.dev.device.Call("createBindGroupLayout", textureBGLDesc)

	// Pipeline layout.
	plDesc := js.Global().Get("Object").New()
	plDesc.Set("bindGroupLayouts", js.Global().Get("Array").New(p.uniformBGL, p.textureBGL))
	p.layout = p.dev.device.Call("createPipelineLayout", plDesc)
}

// Dispose releases pipeline resources.
func (p *Pipeline) Dispose() {
	// GPU objects are garbage-collected by the browser.
}

// --- JS mapping helpers ---

func jsVertexFormat(f backend.AttributeFormat) string {
	switch f {
	case backend.AttributeFloat2:
		return "float32x2"
	case backend.AttributeFloat3:
		return "float32x3"
	case backend.AttributeFloat4:
		return "float32x4"
	case backend.AttributeByte4Norm:
		return "unorm8x4"
	default:
		return "float32x4"
	}
}

// jsApplyStencilNoop sets stencilFront/Back to compare=always, ops=keep so
// the stencil test is effectively transparent. Used by pipelines that
// declare a depth24plus-stencil8 attachment (required for format
// compatibility with the render pass) but don't actually use stencil.
func jsApplyStencilNoop(ds js.Value) {
	face := js.Global().Get("Object").New()
	face.Set("compare", "always")
	face.Set("failOp", "keep")
	face.Set("depthFailOp", "keep")
	face.Set("passOp", "keep")
	ds.Set("stencilFront", face)
	ds.Set("stencilBack", face)
	ds.Set("stencilReadMask", 0xFF)
	ds.Set("stencilWriteMask", 0)
}

// jsApplyStencilState writes front/back face ops and masks for a pipeline
// with stencil enabled. Reference value is dynamic; set via the encoder's
// SetStencilReference call. Matches the invariants baked into the
// sprite pass's stencil-write and color pipelines.
func jsApplyStencilState(ds js.Value, s backend.StencilDescriptor) {
	frontOps := s.Front
	backOps := s.Back
	if !s.TwoSided {
		backOps = s.Front
	}
	front := js.Global().Get("Object").New()
	front.Set("compare", jsCompareFunc(s.Func))
	front.Set("failOp", jsStencilOp(frontOps.SFail))
	front.Set("depthFailOp", jsStencilOp(frontOps.DPFail))
	front.Set("passOp", jsStencilOp(frontOps.DPPass))

	back := js.Global().Get("Object").New()
	back.Set("compare", jsCompareFunc(s.Func))
	back.Set("failOp", jsStencilOp(backOps.SFail))
	back.Set("depthFailOp", jsStencilOp(backOps.DPFail))
	back.Set("passOp", jsStencilOp(backOps.DPPass))

	ds.Set("stencilFront", front)
	ds.Set("stencilBack", back)
	readMask := uint32(s.Mask)
	if readMask == 0 {
		readMask = 0xFF
	}
	writeMask := s.WriteMask
	if writeMask == 0 {
		writeMask = 0xFF
	}
	ds.Set("stencilReadMask", int(readMask))
	ds.Set("stencilWriteMask", int(writeMask))
}

// jsStencilOp maps a backend StencilOp to the WebGPU string value.
// https://www.w3.org/TR/webgpu/#enumdef-gpustenciloperation
func jsStencilOp(op backend.StencilOp) string {
	switch op {
	case backend.StencilKeep:
		return "keep"
	case backend.StencilZero:
		return "zero"
	case backend.StencilReplace:
		return "replace"
	case backend.StencilInvert:
		return "invert"
	case backend.StencilIncr:
		return "increment-clamp"
	case backend.StencilDecr:
		return "decrement-clamp"
	case backend.StencilIncrWrap:
		return "increment-wrap"
	case backend.StencilDecrWrap:
		return "decrement-wrap"
	default:
		return "keep"
	}
}

// jsBlendState builds a WebGPU GPUBlendState object from the backend
// BlendMode struct. Returns js.Undefined() when blending is disabled so
// that the pipeline target descriptor omits the blend key (required by
// the WebGPU spec when Enabled=false).
func jsBlendState(mode backend.BlendMode) js.Value {
	if !mode.Enabled {
		return js.Undefined()
	}
	color := js.Global().Get("Object").New()
	color.Set("operation", jsBlendOp(mode.OpRGB))
	color.Set("srcFactor", jsBlendFactor(mode.SrcFactorRGB))
	color.Set("dstFactor", jsBlendFactor(mode.DstFactorRGB))

	alpha := js.Global().Get("Object").New()
	alpha.Set("operation", jsBlendOp(mode.OpAlpha))
	alpha.Set("srcFactor", jsBlendFactor(mode.SrcFactorAlpha))
	alpha.Set("dstFactor", jsBlendFactor(mode.DstFactorAlpha))

	blend := js.Global().Get("Object").New()
	blend.Set("color", color)
	blend.Set("alpha", alpha)
	return blend
}

// jsBlendFactor maps a backend BlendFactor to the WebGPU string value.
// https://www.w3.org/TR/webgpu/#enumdef-gpublendfactor
func jsBlendFactor(f backend.BlendFactor) string {
	switch f {
	case backend.BlendFactorZero:
		return "zero"
	case backend.BlendFactorOne:
		return "one"
	case backend.BlendFactorSrcAlpha:
		return "src-alpha"
	case backend.BlendFactorOneMinusSrcAlpha:
		return "one-minus-src-alpha"
	case backend.BlendFactorDstAlpha:
		return "dst-alpha"
	case backend.BlendFactorOneMinusDstAlpha:
		return "one-minus-dst-alpha"
	case backend.BlendFactorSrcColor:
		return "src"
	case backend.BlendFactorOneMinusSrcColor:
		return "one-minus-src"
	case backend.BlendFactorDstColor:
		return "dst"
	case backend.BlendFactorOneMinusDstColor:
		return "one-minus-dst"
	default:
		return "one"
	}
}

// jsBlendOp maps a backend BlendOperation to the WebGPU string value.
// https://www.w3.org/TR/webgpu/#enumdef-gpublendoperation
func jsBlendOp(op backend.BlendOperation) string {
	switch op {
	case backend.BlendOpAdd:
		return "add"
	case backend.BlendOpSubtract:
		return "subtract"
	case backend.BlendOpReverseSubtract:
		return "reverse-subtract"
	case backend.BlendOpMin:
		return "min"
	case backend.BlendOpMax:
		return "max"
	default:
		return "add"
	}
}

func jsTopology(p backend.PrimitiveType) string {
	switch p {
	case backend.PrimitiveTriangles:
		return "triangle-list"
	case backend.PrimitiveTriangleStrip:
		return "triangle-strip"
	case backend.PrimitiveLines:
		return "line-list"
	case backend.PrimitiveLineStrip:
		return "line-strip"
	case backend.PrimitivePoints:
		return "point-list"
	default:
		return "triangle-list"
	}
}

func jsCullMode(mode backend.CullMode) string {
	switch mode {
	case backend.CullFront:
		return "front"
	case backend.CullBack:
		return "back"
	default:
		return "none"
	}
}

func jsCompareFunc(cf backend.CompareFunc) string {
	switch cf {
	case backend.CompareNever:
		return "never"
	case backend.CompareLess:
		return "less"
	case backend.CompareLessEqual:
		return "less-equal"
	case backend.CompareEqual:
		return "equal"
	case backend.CompareGreaterEqual:
		return "greater-equal"
	case backend.CompareGreater:
		return "greater"
	case backend.CompareNotEqual:
		return "not-equal"
	case backend.CompareAlways:
		return "always"
	default:
		return "always"
	}
}
