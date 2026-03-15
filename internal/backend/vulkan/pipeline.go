package vulkan

import "github.com/michaelraines/future-render/internal/backend"

// Pipeline implements backend.Pipeline for Vulkan.
// Models a VkPipeline (graphics pipeline state object). In Vulkan, pipeline
// state is baked into an immutable PSO, unlike OpenGL/WebGL2 where state
// is set imperatively.
type Pipeline struct {
	inner backend.Pipeline
	desc  backend.PipelineDescriptor
}

// Dispose releases the pipeline.
func (p *Pipeline) Dispose() {
	p.inner.Dispose()
}
