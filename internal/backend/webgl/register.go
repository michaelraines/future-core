//go:build js

package webgl

import "github.com/michaelraines/future-core/internal/backend"

func init() {
	backend.Register("webgl", func() backend.Device { return New() })
}
