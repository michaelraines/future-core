package metal

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/backend"
)

func TestMtlPixelFormatFromBackend(t *testing.T) {
	tests := []struct {
		name string
		in   backend.TextureFormat
		want int
	}{
		{"RGBA8", backend.TextureFormatRGBA8, mtlPixelFormatRGBA8UNorm},
		{"RGB8", backend.TextureFormatRGB8, mtlPixelFormatRGBA8UNorm},
		{"R8", backend.TextureFormatR8, mtlPixelFormatR8UNorm},
		{"RGBA16F", backend.TextureFormatRGBA16F, mtlPixelFormatRGBA16Float},
		{"RGBA32F", backend.TextureFormatRGBA32F, mtlPixelFormatRGBA32Float},
		{"Depth24", backend.TextureFormatDepth24, mtlPixelFormatDepth24UNormStencil8},
		{"Depth32F", backend.TextureFormatDepth32F, mtlPixelFormatDepth32Float},
		{"unknown", backend.TextureFormat(99), mtlPixelFormatRGBA8UNorm},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, mtlPixelFormatFromBackend(tt.in))
		})
	}
}
