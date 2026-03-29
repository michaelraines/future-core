//go:build darwin || linux || freebsd || windows || js

package webgpu

import "github.com/michaelraines/future-core/internal/backend"

func init() {
	backend.Register("webgpu", func() backend.Device { return New() })
}
