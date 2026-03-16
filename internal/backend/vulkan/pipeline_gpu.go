//go:build vulkan

package vulkan

import "github.com/michaelraines/future-render/internal/backend"

// Pipeline implements backend.Pipeline for Vulkan.
// Stores the pipeline descriptor for deferred VkPipeline creation.
type Pipeline struct {
	dev  *Device
	desc backend.PipelineDescriptor
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

// Dispose releases pipeline resources.
func (p *Pipeline) Dispose() {}
