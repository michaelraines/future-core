//go:build dx12native

package dx12

import (
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/d3d12"
)

// Buffer implements backend.Buffer for DX12 via ID3D12Resource.
type Buffer struct {
	dev      *Device
	resource d3d12.Resource
	size     int
	mapped   uintptr
	gpuAddr  uintptr
	heapType int
}

// InnerBuffer returns nil for GPU buffers (no soft delegation).
func (b *Buffer) InnerBuffer() backend.Buffer { return nil }

// Upload replaces the entire buffer data via mapped pointer.
func (b *Buffer) Upload(data []byte) {
	if len(data) == 0 || b.mapped == 0 {
		return
	}
	n := len(data)
	if n > b.size {
		n = b.size
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(b.mapped)), n)
	copy(dst, data[:n])
}

// UploadRegion uploads a sub-region of buffer data.
func (b *Buffer) UploadRegion(data []byte, offset int) {
	if len(data) == 0 || b.mapped == 0 {
		return
	}
	n := len(data)
	if offset+n > b.size {
		n = b.size - offset
	}
	if n <= 0 {
		return
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(b.mapped+uintptr(offset))), n)
	copy(dst, data[:n])
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() int { return b.size }

// Dispose releases the ID3D12Resource.
func (b *Buffer) Dispose() {
	if b.resource != 0 {
		if b.mapped != 0 {
			d3d12.ResourceUnmap(b.resource)
			b.mapped = 0
		}
		d3d12.Release(uintptr(b.resource))
		b.resource = 0
	}
}
