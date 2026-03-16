//go:build !vulkan

package vulkan

import "github.com/michaelraines/future-render/internal/backend"

// Buffer implements backend.Buffer for Vulkan.
// Models a VkBuffer + VkDeviceMemory pair.
type Buffer struct {
	backend.Buffer     // delegates all Buffer methods to inner
	vkUsage        int // VkBufferUsageFlags
}

// InnerBuffer returns the wrapped soft buffer for encoder unwrapping.
func (b *Buffer) InnerBuffer() backend.Buffer { return b.Buffer }
