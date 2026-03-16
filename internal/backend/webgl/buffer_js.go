//go:build js

package webgl

import (
	"syscall/js"

	"github.com/michaelraines/future-render/internal/backend"
)

// Buffer implements backend.Buffer for WebGL2.
type Buffer struct {
	gl     js.Value
	handle js.Value
	size   int
	usage  backend.BufferUsage
}

// InnerBuffer returns nil for GPU buffers (no soft delegation).
func (b *Buffer) InnerBuffer() backend.Buffer { return nil }

// Upload replaces the entire buffer data.
func (b *Buffer) Upload(data []byte) {
	target := glBufferTarget(b.gl, b.usage)
	b.gl.Call("bindBuffer", target, b.handle)

	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	b.gl.Call("bufferSubData", target, 0, arr)

	b.gl.Call("bindBuffer", target, js.Null())
}

// UploadRegion uploads a sub-region of buffer data.
func (b *Buffer) UploadRegion(data []byte, offset int) {
	target := glBufferTarget(b.gl, b.usage)
	b.gl.Call("bindBuffer", target, b.handle)

	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	b.gl.Call("bufferSubData", target, offset, arr)

	b.gl.Call("bindBuffer", target, js.Null())
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() int { return b.size }

// Dispose releases the buffer.
func (b *Buffer) Dispose() {
	b.gl.Call("deleteBuffer", b.handle)
}
