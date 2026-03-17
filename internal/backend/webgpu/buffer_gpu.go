//go:build wgpunative

package webgpu

import (
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/wgpu"
)

// Buffer implements backend.Buffer for WebGPU via wgpu-native.
type Buffer struct {
	dev    *Device
	handle wgpu.Buffer
	size   int
}

// InnerBuffer returns nil for GPU buffers (no soft delegation).
func (b *Buffer) InnerBuffer() backend.Buffer { return nil }

// Upload replaces the entire buffer data.
func (b *Buffer) Upload(data []byte) {
	if len(data) == 0 || b.dev.queue == 0 {
		return
	}
	wgpu.QueueWriteBuffer(b.dev.queue, b.handle, 0,
		unsafe.Pointer(&data[0]), uint64(len(data)))
	runtime.KeepAlive(data)
}

// UploadRegion uploads a sub-region of buffer data.
func (b *Buffer) UploadRegion(data []byte, offset int) {
	if len(data) == 0 || b.dev.queue == 0 {
		return
	}
	wgpu.QueueWriteBuffer(b.dev.queue, b.handle, uint64(offset),
		unsafe.Pointer(&data[0]), uint64(len(data)))
	runtime.KeepAlive(data)
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() int { return b.size }

// Dispose releases the buffer.
func (b *Buffer) Dispose() {
	if b.handle != 0 {
		wgpu.BufferDestroy(b.handle)
		wgpu.BufferRelease(b.handle)
		b.handle = 0
	}
}
