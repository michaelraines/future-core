//go:build (darwin || linux || freebsd || windows) && !soft

package shaderc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const testVertexGLSL = `#version 330 core
layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;
uniform mat4 uProjection;
out vec2 vTexCoord;
out vec4 vColor;
void main() {
    vTexCoord = aTexCoord;
    vColor = aColor;
    gl_Position = uProjection * vec4(aPosition, 0.0, 1.0);
}
`

const testFragmentGLSL = `#version 330 core
in vec2 vTexCoord;
in vec4 vColor;
uniform sampler2D uTexture;
uniform mat4 uColorBody;
uniform vec4 uColorTranslation;
out vec4 fragColor;
void main() {
    vec4 c = texture(uTexture, vTexCoord) * vColor;
    fragColor = uColorBody * c + uColorTranslation;
}
`

func TestCompileVertex(t *testing.T) {
	spirv, err := CompileGLSL(testVertexGLSL, StageVertex)
	require.NoError(t, err)
	require.NotEmpty(t, spirv)
	// SPIR-V magic number: 0x07230203
	require.Equal(t, byte(0x03), spirv[0])
	require.Equal(t, byte(0x02), spirv[1])
	require.Equal(t, byte(0x23), spirv[2])
	require.Equal(t, byte(0x07), spirv[3])
	t.Logf("Vertex SPIR-V: %d bytes", len(spirv))
}

func TestCompileFragment(t *testing.T) {
	spirv, err := CompileGLSL(testFragmentGLSL, StageFragment)
	require.NoError(t, err)
	require.NotEmpty(t, spirv)
	require.Equal(t, byte(0x03), spirv[0])
	t.Logf("Fragment SPIR-V: %d bytes", len(spirv))
}

func TestCompileInvalidGLSL(t *testing.T) {
	_, err := CompileGLSL("this is not valid GLSL", StageVertex)
	require.Error(t, err)
	require.Contains(t, err.Error(), "compilation failed")
}
