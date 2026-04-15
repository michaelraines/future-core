//go:build darwin && !soft

package metal

import (
	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/mtl"
)

// Pipeline implements backend.Pipeline for Metal.
// Stores the PipelineDescriptor and lazily creates MTLRenderPipelineState.
type Pipeline struct {
	dev           *Device
	desc          backend.PipelineDescriptor
	pipelineState mtl.RenderPipelineState
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

// createPipelineState lazily compiles the shader and creates the pipeline state.
func (p *Pipeline) createPipelineState() error {
	if p.pipelineState != 0 {
		return nil
	}

	shader, ok := p.desc.Shader.(*Shader)
	if !ok || shader == nil {
		return nil
	}

	if err := shader.compile(); err != nil {
		return err
	}

	if shader.vertexFn == 0 || shader.fragmentFn == 0 {
		return nil
	}

	blendEnabled, srcRGB, dstRGB, srcAlpha, dstAlpha, opRGB, opAlpha := mtlBlendConfig(p.desc.BlendMode)

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
		return err
	}
	p.pipelineState = pso
	return nil
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

// Dispose releases pipeline resources.
func (p *Pipeline) Dispose() {
	if p.pipelineState != 0 {
		mtl.RenderPipelineStateRelease(p.pipelineState)
		p.pipelineState = 0
	}
}
