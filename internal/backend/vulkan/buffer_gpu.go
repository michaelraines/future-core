//go:build vulkan

package vulkan

import (
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/vk"
)

// Buffer implements backend.Buffer for Vulkan using VkBuffer + VkDeviceMemory.
type Buffer struct {
	dev    *Device
	buffer vk.Buffer
	memory vk.DeviceMemory
	size   int

	vkUsage int
}

// InnerBuffer returns nil for GPU buffers (no soft delegation).
func (b *Buffer) InnerBuffer() backend.Buffer { return nil }

// Upload uploads data to the buffer via mapped memory.
func (b *Buffer) Upload(data []byte) {
	if len(data) == 0 || b.memory == 0 {
		return
	}
	ptr, err := vk.MapMemory(b.dev.device, b.memory, 0, uint64(len(data)))
	if err != nil {
		return
	}
	dst := unsafe.Slice((*byte)(ptr), len(data))
	copy(dst, data)
	vk.UnmapMemory(b.dev.device, b.memory)
}

// UploadRegion uploads data to a region of the buffer.
func (b *Buffer) UploadRegion(data []byte, offset int) {
	if len(data) == 0 || b.memory == 0 {
		return
	}
	ptr, err := vk.MapMemory(b.dev.device, b.memory, uint64(offset), uint64(len(data)))
	if err != nil {
		return
	}
	dst := unsafe.Slice((*byte)(ptr), len(data))
	copy(dst, data)
	vk.UnmapMemory(b.dev.device, b.memory)
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() int { return b.size }

// Dispose releases the VkBuffer and VkDeviceMemory.
func (b *Buffer) Dispose() {
	if b.dev == nil || b.dev.device == 0 {
		return
	}
	if b.buffer != 0 {
		vk.DestroyBuffer(b.dev.device, b.buffer)
	}
	if b.memory != 0 {
		vk.FreeMemory(b.dev.device, b.memory)
	}
}
