package webgpu

import "github.com/michaelraines/future-render/internal/backend"

// Pipeline implements backend.Pipeline for WebGPU.
// Models a GPURenderPipeline object.
type Pipeline struct {
	backend.Pipeline // delegates Dispose to inner
	desc             backend.PipelineDescriptor
}

// InnerPipeline returns the wrapped soft pipeline for encoder unwrapping.
func (p *Pipeline) InnerPipeline() backend.Pipeline { return p.Pipeline }
