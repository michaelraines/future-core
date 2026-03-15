package webgl

import "github.com/michaelraines/future-render/internal/backend"

// Pipeline implements backend.Pipeline for WebGL2.
// In WebGL2, pipeline state is set imperatively (glEnable, glBlendFunc, etc.)
// rather than baked into a PSO. This type stores the descriptor so the
// encoder can apply the correct GL state before draw calls.
type Pipeline struct {
	inner backend.Pipeline
	desc  backend.PipelineDescriptor
}

// Dispose releases the pipeline.
func (p *Pipeline) Dispose() {
	p.inner.Dispose()
}
