package soft

import "github.com/michaelraines/future-core/internal/backend"

func init() {
	backend.Register("soft", func() backend.Device { return New() })
}
