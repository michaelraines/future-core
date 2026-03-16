//go:build !js

package webgl

import "github.com/michaelraines/future-render/internal/backend"

// Buffer implements backend.Buffer for WebGL2.
// Wraps a soft.Buffer and adds the WebGL2 buffer target.
type Buffer struct {
	backend.Buffer     // delegates all Buffer methods to inner
	glUsage        int // GL buffer target (e.g. GL_ARRAY_BUFFER)
}

// InnerBuffer returns the wrapped soft buffer for encoder unwrapping.
func (b *Buffer) InnerBuffer() backend.Buffer { return b.Buffer }
