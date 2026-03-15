package webgl

import "github.com/michaelraines/future-render/internal/backend"

func init() {
	backend.Register("webgl", func() backend.Device { return New() })
}
