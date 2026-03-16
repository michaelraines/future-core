//go:build !wgpunative

package webgpu

import "github.com/michaelraines/future-render/internal/backend/softdelegate"

// Encoder implements backend.CommandEncoder for WebGPU.
// Models a GPUCommandEncoder + GPURenderPassEncoder. Delegates all commands to the
// soft rasterizer via the embedded softdelegate.Encoder.
type Encoder struct {
	softdelegate.Encoder
}
