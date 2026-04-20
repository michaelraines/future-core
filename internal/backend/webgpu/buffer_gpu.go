//go:build (darwin || linux || freebsd || windows) && !soft

package webgpu

import (
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/wgpu"
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
	src, size := alignedBufferCopy(data)
	wgpu.QueueWriteBuffer(b.dev.queue, b.handle, 0, src, size)
	runtime.KeepAlive(data)
}

// UploadRegion uploads a sub-region of buffer data.
func (b *Buffer) UploadRegion(data []byte, offset int) {
	if len(data) == 0 || b.dev.queue == 0 {
		return
	}
	src, size := alignedBufferCopy(data)
	wgpu.QueueWriteBuffer(b.dev.queue, b.handle, uint64(offset), src, size)
	runtime.KeepAlive(data)
}

// alignedBufferCopy returns a pointer + size for data that respects wgpu's
// 4-byte COPY_BUFFER_ALIGNMENT (equivalent to Vulkan/WebGPU's
// WGPU_COPY_BUFFER_ALIGNMENT). When the input's length is already a
// multiple of 4, we return the original backing storage to avoid a copy
// — common case for Vertex2D (32 bytes) and uint32 index streams.
// Otherwise a short zero-padded copy is allocated. Required at every
// entry point into QueueWriteBuffer because wgpu-native raises an
// uncaptured validation error otherwise (previously crashed the native
// WebGPU conformance suite with "Copy size 6 does not respect
// COPY_BUFFER_ALIGNMENT" on any uint16 index buffer with an odd triangle
// count).
func alignedBufferCopy(data []byte) (unsafe.Pointer, uint64) {
	n := len(data)
	if n&3 == 0 {
		return unsafe.Pointer(&data[0]), uint64(n)
	}
	padded := make([]byte, (n+3)&^3)
	copy(padded, data)
	return unsafe.Pointer(&padded[0]), uint64(len(padded))
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
