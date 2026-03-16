//go:build !dx12native

package dx12

import "github.com/michaelraines/future-render/internal/backend"

// Buffer implements backend.Buffer for DirectX 12.
// Models an ID3D12Resource (committed buffer resource).
type Buffer struct {
	backend.Buffer     // delegates all Buffer methods to inner
	heapType       int // D3D12_HEAP_TYPE
}

// InnerBuffer returns the wrapped soft buffer for encoder unwrapping.
func (b *Buffer) InnerBuffer() backend.Buffer { return b.Buffer }
