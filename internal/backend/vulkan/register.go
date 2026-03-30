//go:build darwin || linux || freebsd || windows || android

package vulkan

import "github.com/michaelraines/future-core/internal/backend"

func init() {
	backend.Register("vulkan", func() backend.Device { return New() })
}
