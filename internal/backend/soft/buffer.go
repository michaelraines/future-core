package soft

// Buffer implements backend.Buffer as a CPU-side byte slice.
type Buffer struct {
	id       uint64
	data     []byte
	disposed bool
}

// Upload replaces the entire buffer data.
func (b *Buffer) Upload(data []byte) {
	copy(b.data, data)
}

// UploadRegion uploads data to a region of the buffer.
func (b *Buffer) UploadRegion(data []byte, offset int) {
	copy(b.data[offset:], data)
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() int { return len(b.data) }

// Dispose releases the buffer.
func (b *Buffer) Dispose() {
	b.disposed = true
	b.data = nil
}

// Data returns the raw buffer data. For testing/conformance only.
func (b *Buffer) Data() []byte { return b.data }
