package dx12

import "github.com/michaelraines/future-render/internal/backend"

// Pipeline implements backend.Pipeline for DirectX 12.
// Models an ID3D12PipelineState (graphics PSO) with an associated
// root signature.
type Pipeline struct {
	inner backend.Pipeline
	desc  backend.PipelineDescriptor
}

// Dispose releases the pipeline.
func (p *Pipeline) Dispose() { p.inner.Dispose() }
