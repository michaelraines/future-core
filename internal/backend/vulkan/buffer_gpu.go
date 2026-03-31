//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/vk"
)

// Buffer implements backend.Buffer for Vulkan using VkBuffer + VkDeviceMemory.
// Uses a ring-buffer write strategy: each Upload appends at an increasing offset
// so that deferred draw commands reference distinct data. The write cursor resets
// when it would overflow. Memory is persistently mapped to avoid per-frame
// map/unmap overhead.
type Buffer struct {
	dev    *Device
	buffer vk.Buffer
	memory vk.DeviceMemory
	mapped unsafe.Pointer // persistently mapped pointer
	size   int

	vkUsage int

	// Ring-buffer write cursor for deferred command buffers.
	writeOffset     int // next write position
	lastWriteOffset int // start of the most recent Upload
}

// InnerBuffer returns nil for GPU buffers (no soft delegation).
func (b *Buffer) InnerBuffer() backend.Buffer { return nil }

// Upload appends data to the buffer at an increasing offset via the
// persistently mapped pointer (no per-frame map/unmap syscalls).
func (b *Buffer) Upload(data []byte) {
	if len(data) == 0 || b.mapped == nil {
		return
	}
	// Wrap to start if remaining space is insufficient.
	if b.writeOffset+len(data) > b.size {
		b.writeOffset = 0
	}
	b.lastWriteOffset = b.writeOffset

	dst := unsafe.Slice((*byte)(b.mapped), b.size)
	copy(dst[b.writeOffset:b.writeOffset+len(data)], data)

	b.writeOffset += len(data)
}

// UploadRegion uploads data to a region of the buffer.
func (b *Buffer) UploadRegion(data []byte, offset int) {
	if len(data) == 0 || b.mapped == nil {
		return
	}
	dst := unsafe.Slice((*byte)(b.mapped), b.size)
	copy(dst[offset:offset+len(data)], data)
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() int { return b.size }

// Dispose releases the VkBuffer and VkDeviceMemory.
func (b *Buffer) Dispose() {
	if b.dev == nil || b.dev.device == 0 {
		return
	}
	if b.mapped != nil {
		vk.UnmapMemory(b.dev.device, b.memory)
		b.mapped = nil
	}
	if b.buffer != 0 {
		vk.DestroyBuffer(b.dev.device, b.buffer)
	}
	if b.memory != 0 {
		vk.FreeMemory(b.dev.device, b.memory)
	}
}
