//go:build wgpunative

package webgpu

import "github.com/michaelraines/future-render/internal/backend"

// Pipeline implements backend.Pipeline for WebGPU.
// Stores the descriptor for deferred GPURenderPipeline creation.
type Pipeline struct {
	dev  *Device
	desc backend.PipelineDescriptor
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

// Dispose releases pipeline resources.
func (p *Pipeline) Dispose() {}
