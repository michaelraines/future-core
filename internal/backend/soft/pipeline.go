package soft

import "github.com/michaelraines/future-render/internal/backend"

// Pipeline implements backend.Pipeline as a stored descriptor.
type Pipeline struct {
	id       uint64
	desc     backend.PipelineDescriptor
	disposed bool
}

// Dispose releases the pipeline.
func (p *Pipeline) Dispose() {
	p.disposed = true
}

// Desc returns the pipeline descriptor. For testing only.
func (p *Pipeline) Desc() backend.PipelineDescriptor { return p.desc }
