//go:build !vulkan

package vulkan

import "github.com/michaelraines/future-render/internal/backend"

// Pipeline implements backend.Pipeline for Vulkan.
// Models a VkPipeline (graphics pipeline state object). In Vulkan, pipeline
// state is baked into an immutable PSO, unlike OpenGL/WebGL2 where state
// is set imperatively.
type Pipeline struct {
	backend.Pipeline // delegates Dispose to inner
	desc             backend.PipelineDescriptor
}

// InnerPipeline returns the wrapped soft pipeline for encoder unwrapping.
func (p *Pipeline) InnerPipeline() backend.Pipeline { return p.Pipeline }
