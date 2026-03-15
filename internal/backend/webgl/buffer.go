package webgl

import "github.com/michaelraines/future-render/internal/backend"

// Buffer implements backend.Buffer for WebGL2.
// Wraps a soft.Buffer and adds the WebGL2 buffer target.
type Buffer struct {
	inner   backend.Buffer
	glUsage int // GL buffer target (e.g. GL_ARRAY_BUFFER)
}

// Upload replaces the entire buffer data.
func (b *Buffer) Upload(data []byte) {
	b.inner.Upload(data)
}

// UploadRegion uploads data to a region of the buffer.
func (b *Buffer) UploadRegion(data []byte, offset int) {
	b.inner.UploadRegion(data, offset)
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() int { return b.inner.Size() }

// Dispose releases the buffer.
func (b *Buffer) Dispose() {
	b.inner.Dispose()
}
