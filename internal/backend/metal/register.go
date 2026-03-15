package metal

import "github.com/michaelraines/future-render/internal/backend"

func init() {
	backend.Register("metal", func() backend.Device { return New() })
}
