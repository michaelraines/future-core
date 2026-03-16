//go:build !metal

package metal

import "github.com/michaelraines/future-render/internal/backend/softdelegate"

// Encoder implements backend.CommandEncoder for Metal.
// Models an MTLRenderCommandEncoder. Delegates all commands to the
// soft rasterizer via the embedded softdelegate.Encoder.
type Encoder struct {
	softdelegate.Encoder
}
