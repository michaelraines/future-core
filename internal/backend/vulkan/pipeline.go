//go:build !(darwin || linux || freebsd || windows || android) || soft

package vulkan

import "github.com/michaelraines/future-core/internal/backend"

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
