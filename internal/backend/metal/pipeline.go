package metal

import "github.com/michaelraines/future-render/internal/backend"

// Pipeline implements backend.Pipeline for Metal.
// Models an MTLRenderPipelineState object.
type Pipeline struct {
	inner backend.Pipeline
	desc  backend.PipelineDescriptor
}

// Dispose releases the pipeline.
func (p *Pipeline) Dispose() { p.inner.Dispose() }
