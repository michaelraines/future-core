//go:build windows

package dx12

import "github.com/michaelraines/future-core/internal/backend"

func init() {
	backend.Register("dx12", func() backend.Device { return New() })
}
