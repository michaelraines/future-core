package vulkan

import "github.com/michaelraines/future-render/internal/backend"

func init() {
	backend.Register("vulkan", func() backend.Device { return New() })
}
