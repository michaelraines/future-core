package webgl

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
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
	tests := []struct {
		name    string
		in      string
		wantHas []string
	}{
		{
			name: "330 core vertex",
			in: "#version 330 core\n" +
				"layout(location = 0) in vec2 aPosition;\n" +
				"void main() { gl_Position = vec4(aPosition, 0.0, 1.0); }\n",
			wantHas: []string{"#version 300 es"},
		},
		{
			name: "330 core fragment gets precision",
			in: "#version 330 core\n" +
				"in vec2 vTexCoord;\nout vec4 fragColor;\n" +
				"uniform sampler2D uTexture;\n" +
				"void main() { fragColor = texture(uTexture, vTexCoord); }\n",
			wantHas: []string{"#version 300 es", "precision highp float"},
		},
		{
			name: "already es 300 untouched",
			in: "#version 300 es\nprecision highp float;\n" +
				"out vec4 fragColor;\nvoid main() { fragColor = vec4(1.0); }\n",
			wantHas: []string{"#version 300 es", "precision highp float"},
		},
		{
			name:    "missing version gets prepended",
			in:      "void main() {}\n",
			wantHas: []string{"#version 300 es"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translateGLSLES(tt.in)
			for _, sub := range tt.wantHas {
				require.Contains(t, got, sub)
			}
		})
	}
}
