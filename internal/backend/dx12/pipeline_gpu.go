//go:build dx12native

package dx12

import "github.com/michaelraines/future-render/internal/backend"

// Pipeline implements backend.Pipeline for DX12.
// Stores the PipelineDescriptor for future ID3D12PipelineState creation.
type Pipeline struct {
	dev  *Device
	desc backend.PipelineDescriptor
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

// Dispose releases pipeline resources.
func (p *Pipeline) Dispose() {}
