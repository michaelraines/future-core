//go:build darwin

package metal

import "github.com/michaelraines/future-core/internal/backend"

func init() {
	backend.Register("metal", func() backend.Device { return New() })
}
