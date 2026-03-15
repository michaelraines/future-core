package webgpu

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/backend"
)

func TestWgpuTextureFormatFromBackend(t *testing.T) {
	tests := []struct {
		name string
		in   backend.TextureFormat
		want int
	}{
		{"RGBA8", backend.TextureFormatRGBA8, wgpuTextureFormatRGBA8UNorm},
		{"RGB8", backend.TextureFormatRGB8, wgpuTextureFormatRGBA8UNorm},
		{"R8", backend.TextureFormatR8, wgpuTextureFormatR8UNorm},
		{"RGBA16F", backend.TextureFormatRGBA16F, wgpuTextureFormatRGBA16Float},
		{"RGBA32F", backend.TextureFormatRGBA32F, wgpuTextureFormatRGBA32Float},
		{"Depth24", backend.TextureFormatDepth24, wgpuTextureFormatDepth24Plus},
		{"Depth32F", backend.TextureFormatDepth32F, wgpuTextureFormatDepth32Float},
		{"unknown", backend.TextureFormat(99), wgpuTextureFormatRGBA8UNorm},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, wgpuTextureFormatFromBackend(tt.in))
		})
	}
}

func TestWgpuBufferUsageFromBackend(t *testing.T) {
	tests := []struct {
		name string
		in   backend.BufferUsage
		want int
	}{
		{"vertex", backend.BufferUsageVertex, wgpuBufferUsageVertex | wgpuBufferUsageCopyDst},
		{"index", backend.BufferUsageIndex, wgpuBufferUsageIndex | wgpuBufferUsageCopyDst},
		{"uniform", backend.BufferUsageUniform, wgpuBufferUsageUniform | wgpuBufferUsageCopyDst},
		{"unknown", backend.BufferUsage(99), wgpuBufferUsageVertex | wgpuBufferUsageCopyDst},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, wgpuBufferUsageFromBackend(tt.in))
		})
	}
}
