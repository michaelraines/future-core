//go:build !metal

package metal

import "github.com/michaelraines/future-render/internal/backend"

// Buffer implements backend.Buffer for Metal.
// Models an MTLBuffer object.
type Buffer struct {
	backend.Buffer     // delegates all Buffer methods to inner
	storageMode    int // MTLStorageMode
}

// InnerBuffer returns the wrapped soft buffer for encoder unwrapping.
func (b *Buffer) InnerBuffer() backend.Buffer { return b.Buffer }
