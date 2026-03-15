package soft

import "github.com/michaelraines/future-render/internal/backend"

func init() {
	backend.Register("soft", func() backend.Device { return New() })
}
