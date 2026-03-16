//go:build !vulkan

package vulkan

import "github.com/michaelraines/future-render/internal/backend/softdelegate"

// Encoder implements backend.CommandEncoder for Vulkan.
// Models a VkCommandBuffer recording. Delegates all commands to the
// soft rasterizer via the embedded softdelegate.Encoder.
type Encoder struct {
	softdelegate.Encoder
}
