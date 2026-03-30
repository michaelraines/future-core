//go:build !darwin || soft

package metal

import "github.com/michaelraines/future-core/internal/backend"

// Shader implements backend.Shader for Metal.
// Models an MTLLibrary containing compiled MSL functions.
type Shader struct {
	backend.Shader // delegates all Shader methods to inner
}
