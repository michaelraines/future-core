package webgpu

import "github.com/michaelraines/future-render/internal/backend"

func init() {
	backend.Register("webgpu", func() backend.Device { return New() })
}
