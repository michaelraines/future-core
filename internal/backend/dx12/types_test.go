package dx12

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/backend"
)

func TestDxgiFormatFromBackend(t *testing.T) {
	tests := []struct {
		name string
		in   backend.TextureFormat
		want int
	}{
		{"RGBA8", backend.TextureFormatRGBA8, dxgiFormatR8G8B8A8UNorm},
		{"RGB8", backend.TextureFormatRGB8, dxgiFormatR8G8B8A8UNorm},
		{"R8", backend.TextureFormatR8, dxgiFormatR8UNorm},
		{"RGBA16F", backend.TextureFormatRGBA16F, dxgiFormatR16G16B16A16Float},
		{"RGBA32F", backend.TextureFormatRGBA32F, dxgiFormatR32G32B32A32Float},
		{"Depth24", backend.TextureFormatDepth24, dxgiFormatD24UNormS8UInt},
		{"Depth32F", backend.TextureFormatDepth32F, dxgiFormatD32Float},
		{"unknown", backend.TextureFormat(99), dxgiFormatR8G8B8A8UNorm},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, dxgiFormatFromBackend(tt.in))
		})
	}
}
