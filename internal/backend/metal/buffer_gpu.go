//go:build metal

package metal

import (
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/mtl"
)

// Buffer implements backend.Buffer for Metal via MTLBuffer.
type Buffer struct {
	dev         *Device
	handle      mtl.Buffer
	size        int
	storageMode int
}

// InnerBuffer returns nil for GPU buffers (no soft delegation).
func (b *Buffer) InnerBuffer() backend.Buffer { return nil }

// Upload replaces the entire buffer data via contents pointer.
func (b *Buffer) Upload(data []byte) {
	if len(data) == 0 || b.handle == 0 {
		return
	}
	ptr := mtl.BufferContents(b.handle)
	if ptr == 0 {
		return
	}
	n := len(data)
	if n > b.size {
		n = b.size
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), n)
	copy(dst, data[:n])
}

// UploadRegion uploads a sub-region of buffer data.
func (b *Buffer) UploadRegion(data []byte, offset int) {
	if len(data) == 0 || b.handle == 0 {
		return
	}
	ptr := mtl.BufferContents(b.handle)
	if ptr == 0 {
		return
	}
	n := len(data)
	if offset+n > b.size {
		n = b.size - offset
	}
	if n <= 0 {
		return
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(ptr+uintptr(offset))), n)
	copy(dst, data[:n])
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() int { return b.size }

// Dispose releases the MTLBuffer.
func (b *Buffer) Dispose() {
	if b.handle != 0 {
		mtl.BufferRelease(b.handle)
		b.handle = 0
	}
}
