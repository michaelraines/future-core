package vulkan

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/backend"
)

func TestVkFormatFromTextureFormat(t *testing.T) {
	tests := []struct {
		name string
		in   backend.TextureFormat
		want int
	}{
		{"RGBA8", backend.TextureFormatRGBA8, vkFormatR8G8B8A8UNorm},
		{"RGB8", backend.TextureFormatRGB8, vkFormatR8G8B8UNorm},
		{"R8", backend.TextureFormatR8, vkFormatR8UNorm},
		{"RGBA16F", backend.TextureFormatRGBA16F, vkFormatR16G16B16A16SFloat},
		{"RGBA32F", backend.TextureFormatRGBA32F, vkFormatR32G32B32A32SFloat},
		{"Depth24", backend.TextureFormatDepth24, vkFormatD24UNormS8UInt},
		{"Depth32F", backend.TextureFormatDepth32F, vkFormatD32SFloat},
		{"unknown", backend.TextureFormat(99), vkFormatR8G8B8A8UNorm},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, vkFormatFromTextureFormat(tt.in))
		})
	}
}

func TestVkBufferUsageFromBackend(t *testing.T) {
	tests := []struct {
		name string
		in   backend.BufferUsage
		want int
	}{
		{"vertex", backend.BufferUsageVertex, vkBufferUsageVertexBuf | vkBufferUsageTransferDst},
		{"index", backend.BufferUsageIndex, vkBufferUsageIndexBuf | vkBufferUsageTransferDst},
		{"uniform", backend.BufferUsageUniform, vkBufferUsageUniformBuf | vkBufferUsageTransferDst},
		{"unknown", backend.BufferUsage(99), vkBufferUsageVertexBuf | vkBufferUsageTransferDst},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, vkBufferUsageFromBackend(tt.in))
		})
	}
}

func TestAPIVersionConstants(t *testing.T) {
	require.Greater(t, vkAPIVersion1_1, vkAPIVersion1_0)
	require.Greater(t, vkAPIVersion1_2, vkAPIVersion1_1)
	require.Greater(t, vkAPIVersion1_3, vkAPIVersion1_2)
}
