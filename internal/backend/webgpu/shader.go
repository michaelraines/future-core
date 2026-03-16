//go:build !wgpunative

package webgpu

import "github.com/michaelraines/future-render/internal/backend"

// Shader implements backend.Shader for WebGPU.
// Models a GPUShaderModule compiled from WGSL source.
type Shader struct {
	backend.Shader // delegates all Shader methods to inner
}
