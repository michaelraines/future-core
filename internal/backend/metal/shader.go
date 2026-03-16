//go:build !metal

package metal

import "github.com/michaelraines/future-render/internal/backend"

// Shader implements backend.Shader for Metal.
// Models an MTLLibrary containing compiled MSL functions.
type Shader struct {
	backend.Shader // delegates all Shader methods to inner
}
