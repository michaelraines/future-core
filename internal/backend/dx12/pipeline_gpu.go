//go:build windows && !soft

package dx12

import "github.com/michaelraines/future-core/internal/backend"

// Pipeline implements backend.Pipeline for DX12.
// Stores the PipelineDescriptor and (when the descriptor's Shader was
// produced via NewShaderNative with HLSL source) the DXBC bytecode
// pair compiled at NewPipeline time.
type Pipeline struct {
	dev  *Device
	desc backend.PipelineDescriptor

	// vertexBytecode and pixelBytecode hold the DXBC produced by
	// D3DCompile from the Shader's HLSL source. Both are populated
	// together: nil on both means the pipeline is descriptor-only
	// (the legacy stub path) and an actual ID3D12PipelineState
	// creation will need to re-compile or fall back to the
	// translator. Non-nil on both is the steady-state once the
	// translator path lands too.
	vertexBytecode []byte
	pixelBytecode  []byte
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

// Dispose releases pipeline resources.
func (p *Pipeline) Dispose() {}
