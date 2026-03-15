package dx12

import "github.com/michaelraines/future-render/internal/backend"

// Buffer implements backend.Buffer for DirectX 12.
// Models an ID3D12Resource (committed buffer resource).
type Buffer struct {
	inner    backend.Buffer
	heapType int // D3D12_HEAP_TYPE
}

// Upload replaces the entire buffer data.
func (b *Buffer) Upload(data []byte) { b.inner.Upload(data) }

// UploadRegion uploads data to a region of the buffer.
func (b *Buffer) UploadRegion(data []byte, offset int) { b.inner.UploadRegion(data, offset) }

// Size returns the buffer size in bytes.
func (b *Buffer) Size() int { return b.inner.Size() }

// Dispose releases the buffer.
func (b *Buffer) Dispose() { b.inner.Dispose() }
