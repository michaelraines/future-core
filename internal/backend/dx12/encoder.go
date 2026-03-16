//go:build !dx12native

package dx12

import "github.com/michaelraines/future-render/internal/backend/softdelegate"

// Encoder implements backend.CommandEncoder for DirectX 12.
// Models an ID3D12GraphicsCommandList. Delegates all commands to the
// soft rasterizer via the embedded softdelegate.Encoder.
type Encoder struct {
	softdelegate.Encoder
}
