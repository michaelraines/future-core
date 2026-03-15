package dx12

import "github.com/michaelraines/future-render/internal/backend"

func init() {
	backend.Register("dx12", func() backend.Device { return New() })
}
