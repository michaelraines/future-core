//go:build darwin && !soft

package metal

import (
	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/mtl"
)

// Pipeline implements backend.Pipeline for Metal.
// Holds a map of MTLRenderPipelineState variants keyed by BlendMode —
// Metal pipelines are immutable in the dimensions of color-attachment
// format and blend state, so a single Pipeline instance must be able
// to serve multiple blend modes (the engine's sprite-pass uses one
// shared default Pipeline and switches blend per-batch via
// SetBlendMode + SetPipeline). Without per-blend variants, every batch
// renders with the descriptor's original blend (typically
// SourceOver) — silently turning every BlendLighter / BlendMultiply /
// custom-blend draw into SourceOver. Symptoms: sprite-demo's
// 3-stacked additive cyan orbs render as a single dim cyan instead
// of oversaturating to white; iso-combat's lightmap multiply replaces
// the scene instead of multiplying. WebGPU and Vulkan have the same
// per-blend variant cache (see webgpu/encoder_gpu.go ensurePipelineForVariant).
type Pipeline struct {
	dev      *Device
	desc     backend.PipelineDescriptor
	variants map[backend.BlendMode]mtl.RenderPipelineState
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

// stateForBlend returns the MTLRenderPipelineState compiled for the
// given blend mode, creating it on first use. Returns 0 on failure.
func (p *Pipeline) stateForBlend(blend backend.BlendMode) mtl.RenderPipelineState {
	if p.variants == nil {
		p.variants = make(map[backend.BlendMode]mtl.RenderPipelineState)
	}
	if pso, ok := p.variants[blend]; ok {
		return pso
	}

	shader, ok := p.desc.Shader.(*Shader)
	if !ok || shader == nil {
		return 0
	}
	if err := shader.compile(); err != nil {
		return 0
	}
	if shader.vertexFn == 0 || shader.fragmentFn == 0 {
		return 0
	}

	blendEnabled, srcRGB, dstRGB, srcAlpha, dstAlpha, opRGB, opAlpha := mtlBlendConfig(blend)

	// Build vertex descriptor from pipeline's vertex format.
	var vertexAttrs []mtl.VertexAttr
	stride := 0
	if p.desc.VertexFormat.Stride > 0 {
		stride = p.desc.VertexFormat.Stride
		for i, attr := range p.desc.VertexFormat.Attributes {
			vertexAttrs = append(vertexAttrs, mtl.VertexAttr{
				Format: backendAttrToMTL(attr.Format),
				Offset: attr.Offset,
				Index:  i,
			})
		}
	}

	pso, err := mtl.CreateRenderPipelineState(
		p.dev.device,
		shader.vertexFn, shader.fragmentFn,
		mtl.PixelFormatRGBA8Unorm,
		blendEnabled,
		srcRGB, dstRGB, srcAlpha, dstAlpha,
		opRGB, opAlpha,
		vertexAttrs, stride,
	)
	if err != nil {
		return 0
	}
	p.variants[blend] = pso
	return pso
}

// mtlBlendConfig returns Metal blend parameters for a backend blend mode.
// Honours arbitrary factor/operation combinations by mapping each struct
// field to the corresponding MTLBlendFactor / MTLBlendOperation constant.
func mtlBlendConfig(mode backend.BlendMode) (enabled bool, srcRGB, dstRGB, srcAlpha, dstAlpha, opRGB, opAlpha int) {
	if !mode.Enabled {
		return false, 0, 0, 0, 0, 0, 0
	}
	return true,
		mtlBlendFactor(mode.SrcFactorRGB), mtlBlendFactor(mode.DstFactorRGB),
		mtlBlendFactor(mode.SrcFactorAlpha), mtlBlendFactor(mode.DstFactorAlpha),
		mtlBlendOp(mode.OpRGB), mtlBlendOp(mode.OpAlpha)
}

// mtlBlendFactor maps a backend BlendFactor to the MTLBlendFactor constant.
func mtlBlendFactor(f backend.BlendFactor) int {
	switch f {
	case backend.BlendFactorZero:
		return mtl.BlendFactorZero
	case backend.BlendFactorOne:
		return mtl.BlendFactorOne
	case backend.BlendFactorSrcAlpha:
		return mtl.BlendFactorSourceAlpha
	case backend.BlendFactorOneMinusSrcAlpha:
		return mtl.BlendFactorOneMinusSourceAlpha
	case backend.BlendFactorDstAlpha:
		return mtl.BlendFactorDestinationAlpha
	case backend.BlendFactorOneMinusDstAlpha:
		return mtl.BlendFactorOneMinusDestinationAlpha
	case backend.BlendFactorSrcColor:
		return mtl.BlendFactorSourceColor
	case backend.BlendFactorOneMinusSrcColor:
		return mtl.BlendFactorOneMinusSourceColor
	case backend.BlendFactorDstColor:
		return mtl.BlendFactorDestinationColor
	case backend.BlendFactorOneMinusDstColor:
		return mtl.BlendFactorOneMinusDestinationColor
	default:
		return mtl.BlendFactorOne
	}
}

// mtlBlendOp maps a backend BlendOperation to the MTLBlendOperation constant.
func mtlBlendOp(op backend.BlendOperation) int {
	switch op {
	case backend.BlendOpAdd:
		return mtl.BlendOperationAdd
	case backend.BlendOpSubtract:
		return mtl.BlendOperationSubtract
	case backend.BlendOpReverseSubtract:
		return mtl.BlendOperationReverseSubtract
	case backend.BlendOpMin:
		return mtl.BlendOperationMin
	case backend.BlendOpMax:
		return mtl.BlendOperationMax
	default:
		return mtl.BlendOperationAdd
	}
}

// backendAttrToMTL maps backend vertex attribute formats to MTLVertexFormat.
func backendAttrToMTL(f backend.AttributeFormat) int {
	switch f {
	case backend.AttributeFloat2:
		return mtl.VertexFormatFloat2
	case backend.AttributeFloat3:
		return mtl.VertexFormatFloat3
	case backend.AttributeFloat4:
		return mtl.VertexFormatFloat4
	default:
		return mtl.VertexFormatFloat4
	}
}

// Dispose releases pipeline resources for every cached blend variant.
func (p *Pipeline) Dispose() {
	for _, pso := range p.variants {
		if pso != 0 {
			mtl.RenderPipelineStateRelease(pso)
		}
	}
	p.variants = nil
}
