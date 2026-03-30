//go:build !(darwin || linux || freebsd || windows) || soft

package vulkan

import "github.com/michaelraines/future-core/internal/backend"

// Texture implements backend.Texture for Vulkan.
// Models a VkImage + VkImageView + VkDeviceMemory triple.
type Texture struct {
	backend.Texture     // delegates all Texture methods to inner
	vkFormat        int // VkFormat
	vkUsage         int // VkImageUsageFlags
	mipLevels       int
}

// InnerTexture returns the wrapped soft texture for encoder unwrapping.
func (t *Texture) InnerTexture() backend.Texture { return t.Texture }
