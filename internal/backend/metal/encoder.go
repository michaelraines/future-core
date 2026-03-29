//go:build !darwin || soft

package metal

import "github.com/michaelraines/future-core/internal/backend/softdelegate"

// Encoder implements backend.CommandEncoder for Metal.
// Models an MTLRenderCommandEncoder. Delegates all commands to the
// soft rasterizer via the embedded softdelegate.Encoder.
type Encoder struct {
	softdelegate.Encoder
}
