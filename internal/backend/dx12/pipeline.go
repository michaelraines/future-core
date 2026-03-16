//go:build !dx12native

package dx12

import "github.com/michaelraines/future-render/internal/backend"

// Pipeline implements backend.Pipeline for DirectX 12.
// Models an ID3D12PipelineState (graphics PSO) with an associated
// root signature.
type Pipeline struct {
	backend.Pipeline // delegates Dispose to inner
	desc             backend.PipelineDescriptor
}

// InnerPipeline returns the wrapped soft pipeline for encoder unwrapping.
func (p *Pipeline) InnerPipeline() backend.Pipeline { return p.Pipeline }
