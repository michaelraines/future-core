//go:build darwin || linux || freebsd

package opengl

import "github.com/michaelraines/future-render/internal/backend"

func init() {
	backend.Register("opengl", func() backend.Device { return New() })
}
