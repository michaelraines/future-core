//go:build metal

package metal

import (
	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/mtl"
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

	blendEnabled, srcRGB, dstRGB, srcAlpha, dstAlpha := mtlBlendConfig(p.desc.BlendMode)

	pso, err := mtl.CreateRenderPipelineState(
		p.dev.device,
		shader.vertexFn, shader.fragmentFn,
		mtl.PixelFormatRGBA8Unorm,
		blendEnabled,
		srcRGB, dstRGB, srcAlpha, dstAlpha,
	)
	if err != nil {
		return err
	}
	p.pipelineState = pso
	return nil
}

// mtlBlendConfig returns Metal blend parameters for a backend blend mode.
func mtlBlendConfig(mode backend.BlendMode) (enabled bool, srcRGB, dstRGB, srcAlpha, dstAlpha int) {
	switch mode {
	case backend.BlendSourceOver:
		return true,
			mtl.BlendFactorSourceAlpha, mtl.BlendFactorOneMinusSourceAlpha,
			mtl.BlendFactorOne, mtl.BlendFactorOneMinusSourceAlpha
	case backend.BlendAdditive:
		return true,
			mtl.BlendFactorSourceAlpha, mtl.BlendFactorOne,
			mtl.BlendFactorOne, mtl.BlendFactorOne
	case backend.BlendMultiplicative:
		return true,
			mtl.BlendFactorDestinationColor, mtl.BlendFactorZero,
			mtl.BlendFactorDestinationAlpha, mtl.BlendFactorZero
	case backend.BlendPremultiplied:
		return true,
			mtl.BlendFactorOne, mtl.BlendFactorOneMinusSourceAlpha,
			mtl.BlendFactorOne, mtl.BlendFactorOneMinusSourceAlpha
	default:
		return false, 0, 0, 0, 0
	}
}

// Dispose releases pipeline resources.
func (p *Pipeline) Dispose() {
	if p.pipelineState != 0 {
		mtl.RenderPipelineStateRelease(p.pipelineState)
		p.pipelineState = 0
	}
}
