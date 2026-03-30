//go:build js && !soft

package webgpu

import (
	"syscall/js"

	"github.com/michaelraines/future-core/internal/backend"
)

// Buffer implements backend.Buffer for WebGPU via the browser JS API.
type Buffer struct {
	dev    *Device
	handle js.Value
	size   int
}

// InnerBuffer returns nil for GPU buffers (no soft delegation).
func (b *Buffer) InnerBuffer() backend.Buffer { return nil }

// Upload replaces the entire buffer data.
func (b *Buffer) Upload(data []byte) {
	if len(data) == 0 {
		return
	}
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	b.dev.queue.Call("writeBuffer", b.handle, 0, arr)
}

// UploadRegion uploads a sub-region of buffer data.
func (b *Buffer) UploadRegion(data []byte, offset int) {
	if len(data) == 0 {
		return
	}
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	b.dev.queue.Call("writeBuffer", b.handle, offset, arr)
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() int { return b.size }

// Dispose releases the buffer.
func (b *Buffer) Dispose() {
	if !b.handle.IsUndefined() && !b.handle.IsNull() {
		b.handle.Call("destroy")
	}
}
