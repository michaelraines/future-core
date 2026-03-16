//go:build !wgpunative

package webgpu

import "github.com/michaelraines/future-render/internal/backend"

// Buffer implements backend.Buffer for WebGPU.
// Models a GPUBuffer object.
type Buffer struct {
	backend.Buffer     // delegates all Buffer methods to inner
	usage          int // WGPUBufferUsage
}

// InnerBuffer returns the wrapped soft buffer for encoder unwrapping.
func (b *Buffer) InnerBuffer() backend.Buffer { return b.Buffer }
