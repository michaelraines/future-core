//go:build js

package webgl

import (
	"syscall/js"

	"github.com/michaelraines/future-core/internal/backend"
)

// Buffer implements backend.Buffer for WebGL2.
type Buffer struct {
	gl     js.Value
	handle js.Value
	size   int
	usage  backend.BufferUsage

	// Reusable JS Uint8Array to avoid creating new JS objects per Upload.
	// Without reuse, each Upload pins a js.Value holding a JS Uint8Array
	// alive until Go GC finalizes it. In WASM, GC may not keep up with
	// the per-frame allocation rate, causing unbounded JS heap growth.
	jsArr    js.Value
	jsArrCap int
}

// InnerBuffer returns nil for GPU buffers (no soft delegation).
func (b *Buffer) InnerBuffer() backend.Buffer { return nil }

// ensureJSArray ensures the reusable Uint8Array is at least n bytes.
func (b *Buffer) ensureJSArray(n int) js.Value {
	if n > b.jsArrCap {
		b.jsArr = js.Global().Get("Uint8Array").New(n)
		b.jsArrCap = n
	}
	return b.jsArr
}

// Upload replaces the entire buffer data.
func (b *Buffer) Upload(data []byte) {
	target := glBufferTarget(b.gl, b.usage)
	b.gl.Call("bindBuffer", target, b.handle)

	arr := b.ensureJSArray(len(data))
	js.CopyBytesToJS(arr, data)
	// Pass a subarray view of exact length to avoid uploading stale tail bytes.
	b.gl.Call("bufferSubData", target, 0, arr.Call("subarray", 0, len(data)))

	b.gl.Call("bindBuffer", target, js.Null())
}

// UploadRegion uploads a sub-region of buffer data.
func (b *Buffer) UploadRegion(data []byte, offset int) {
	target := glBufferTarget(b.gl, b.usage)
	b.gl.Call("bindBuffer", target, b.handle)

	arr := b.ensureJSArray(len(data))
	js.CopyBytesToJS(arr, data)
	b.gl.Call("bufferSubData", target, offset, arr.Call("subarray", 0, len(data)))

	b.gl.Call("bindBuffer", target, js.Null())
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() int { return b.size }

// Dispose releases the buffer.
func (b *Buffer) Dispose() {
	b.gl.Call("deleteBuffer", b.handle)
}
