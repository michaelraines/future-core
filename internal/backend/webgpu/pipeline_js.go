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

	// Cached bind group layouts.
	uniformBGL js.Value
	textureBGL js.Value
	layout     js.Value
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

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
	// Use the preferred canvas format when rendering to the surface, otherwise rgba8unorm.
	targetFormat := "rgba8unorm"
	if p.dev.hasContext && p.dev.preferredFormat != "" {
		targetFormat = p.dev.preferredFormat
	}
	target.Set("format", targetFormat)
	target.Set("writeMask", 0xF)

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

	// Depth/stencil.
	if p.desc.DepthTest {
		depthStencil := js.Global().Get("Object").New()
		depthStencil.Set("format", "depth24plus")
		depthStencil.Set("depthWriteEnabled", p.desc.DepthWrite)
		depthStencil.Set("depthCompare", jsCompareFunc(p.desc.DepthFunc))
		pipeDesc.Set("depthStencil", depthStencil)
	}

	// Multisample.
	multisample := js.Global().Get("Object").New()
	multisample.Set("count", 1)
	pipeDesc.Set("multisample", multisample)

	p.handle = p.dev.device.Call("createRenderPipeline", pipeDesc)
}

// createBindGroupLayouts creates uniform (group 0) and texture (group 1) layouts.
func (p *Pipeline) createBindGroupLayouts() {
	// Group 0: Uniform buffer.
	uniformEntry := js.Global().Get("Object").New()
	uniformEntry.Set("binding", 0)
	uniformEntry.Set("visibility", 3) // VERTEX | FRAGMENT
	bufferLayout := js.Global().Get("Object").New()
	bufferLayout.Set("type", "uniform")
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

func jsBlendState(mode backend.BlendMode) js.Value {
	switch mode {
	case backend.BlendSourceOver:
		return jsBlend("src-alpha", "one-minus-src-alpha", "one", "one-minus-src-alpha")
	case backend.BlendAdditive:
		return jsBlend("src-alpha", "one", "one", "one")
	case backend.BlendMultiplicative:
		return jsBlend("dst", "zero", "dst-alpha", "zero")
	case backend.BlendPremultiplied:
		return jsBlend("one", "one-minus-src-alpha", "one", "one-minus-src-alpha")
	default:
		return js.Undefined()
	}
}

func jsBlend(colorSrc, colorDst, alphaSrc, alphaDst string) js.Value {
	color := js.Global().Get("Object").New()
	color.Set("operation", "add")
	color.Set("srcFactor", colorSrc)
	color.Set("dstFactor", colorDst)

	alpha := js.Global().Get("Object").New()
	alpha.Set("operation", "add")
	alpha.Set("srcFactor", alphaSrc)
	alpha.Set("dstFactor", alphaDst)

	blend := js.Global().Get("Object").New()
	blend.Set("color", color)
	blend.Set("alpha", alpha)
	return blend
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
