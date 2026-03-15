package webgl

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/backend"
)

func TestGLFormatFromTextureFormat(t *testing.T) {
	tests := []struct {
		name string
		in   backend.TextureFormat
		want int
	}{
		{"RGBA8", backend.TextureFormatRGBA8, glRGBA},
		{"RGB8", backend.TextureFormatRGB8, glRGB},
		{"R8", backend.TextureFormatR8, glRed},
		{"RGBA16F", backend.TextureFormatRGBA16F, glRGBA16F},
		{"RGBA32F", backend.TextureFormatRGBA32F, glRGBA32F},
		{"Depth24", backend.TextureFormatDepth24, glDepth24},
		{"Depth32F", backend.TextureFormatDepth32F, glDepth32F},
		{"unknown", backend.TextureFormat(99), glRGBA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, glFormatFromTextureFormat(tt.in))
		})
	}
}

func TestGLUsageFromBufferUsage(t *testing.T) {
	tests := []struct {
		name string
		in   backend.BufferUsage
		want int
	}{
		{"vertex", backend.BufferUsageVertex, glArrayBuffer},
		{"index", backend.BufferUsageIndex, glElementArrayBuffer},
		{"uniform", backend.BufferUsageUniform, glUniformBuffer},
		{"unknown", backend.BufferUsage(99), glArrayBuffer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, glUsageFromBufferUsage(tt.in))
		})
	}
}

func TestTranslateGLSLES(t *testing.T) {
	src := "#version 330\nvoid main() {}"
	require.Equal(t, src, translateGLSLES(src))
}
