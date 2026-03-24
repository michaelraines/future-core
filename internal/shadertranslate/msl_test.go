package shadertranslate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const spriteVertexGLSL = `#version 330 core

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

const spriteFragmentGLSL = `#version 330 core

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

func TestGLSLToMSLVertex(t *testing.T) {
	result, err := GLSLToMSLVertex(spriteVertexGLSL)
	require.NoError(t, err)

	src := result.Source
	t.Logf("Translated vertex MSL:\n%s", src)

	// Header.
	require.Contains(t, src, "#include <metal_stdlib>")
	require.Contains(t, src, "using namespace metal;")

	// VertexIn struct with attributes.
	require.Contains(t, src, "float2 aPosition [[attribute(0)]]")
	require.Contains(t, src, "float2 aTexCoord [[attribute(1)]]")
	require.Contains(t, src, "float4 aColor [[attribute(2)]]")

	// VertexOut struct with varyings.
	require.Contains(t, src, "float4 position [[position]]")
	require.Contains(t, src, "float2 vTexCoord;")
	require.Contains(t, src, "float4 vColor;")

	// Uniform struct.
	require.Contains(t, src, "struct VertexUniforms")
	require.Contains(t, src, "float4x4 uProjection")

	// Function signature.
	require.Contains(t, src, "vertex VertexOut vertexMain(")
	require.Contains(t, src, "VertexIn in [[stage_in]]")
	require.Contains(t, src, "constant VertexUniforms& uniforms [[buffer(1)]]")

	// Body translation.
	require.Contains(t, src, "out.vTexCoord = in.aTexCoord")
	require.Contains(t, src, "out.vColor = in.aColor")
	require.Contains(t, src, "out.position = uniforms.uProjection * float4(in.aPosition, 0.0, 1.0)")
	require.Contains(t, src, "return out;")

	// No GLSL remnants.
	require.NotContains(t, src, "#version")
	require.NotContains(t, src, "gl_Position")
	require.NotContains(t, src, "vec2")
	require.NotContains(t, src, "vec4")
	require.NotContains(t, src, "mat4")

	// Uniform layout.
	require.Len(t, result.Uniforms, 1)
	require.Equal(t, "uProjection", result.Uniforms[0].Name)
	require.Equal(t, 0, result.Uniforms[0].Offset)
	require.Equal(t, 64, result.Uniforms[0].Size)
}

func TestGLSLToMSLFragment(t *testing.T) {
	result, err := GLSLToMSLFragment(spriteFragmentGLSL)
	require.NoError(t, err)

	src := result.Source
	t.Logf("Translated fragment MSL:\n%s", src)

	// FragmentIn struct.
	require.Contains(t, src, "struct FragmentIn")
	require.Contains(t, src, "float4 position [[position]]")
	require.Contains(t, src, "float2 vTexCoord;")
	require.Contains(t, src, "float4 vColor;")

	// Uniform struct (no sampler2D in the buffer struct).
	require.Contains(t, src, "struct FragmentUniforms")
	require.Contains(t, src, "float4x4 uColorBody")
	require.Contains(t, src, "float4 uColorTranslation")
	// sampler2D should not appear in the uniform struct.
	require.NotContains(t, src, "sampler2D")

	// Function signature with texture + sampler.
	require.Contains(t, src, "fragment float4 fragmentMain(")
	require.Contains(t, src, "texture2d<float> uTexture [[texture(0)]]")
	require.Contains(t, src, "sampler uTexture_sampler [[sampler(0)]]")
	require.Contains(t, src, "constant FragmentUniforms& uniforms [[buffer(0)]]")

	// Body: texture sampling.
	require.Contains(t, src, "uTexture.sample(uTexture_sampler, in.vTexCoord)")

	// Body: uniform struct access.
	require.Contains(t, src, "uniforms.uColorBody")
	require.Contains(t, src, "uniforms.uColorTranslation")

	// Body: fragColor → return.
	require.True(t, strings.Contains(src, "return uniforms.uColorBody"))

	// No GLSL remnants.
	require.NotContains(t, src, "#version")
	require.NotContains(t, src, "sampler2D")

	// Uniform layout.
	require.Len(t, result.Uniforms, 2)
	require.Equal(t, "uColorBody", result.Uniforms[0].Name)
	require.Equal(t, 0, result.Uniforms[0].Offset)
	require.Equal(t, 64, result.Uniforms[0].Size)
	require.Equal(t, "uColorTranslation", result.Uniforms[1].Name)
	require.Equal(t, 64, result.Uniforms[1].Offset)
	require.Equal(t, 16, result.Uniforms[1].Size)
}

func TestUniformLayout(t *testing.T) {
	uniforms := []uniform{
		{typ: "float", name: "a"},
		{typ: "vec4", name: "b"},
		{typ: "mat4", name: "c"},
	}
	layout := buildUniformLayout(uniforms)
	require.Len(t, layout, 3)

	// float a: offset 0, size 4
	require.Equal(t, 0, layout[0].Offset)
	require.Equal(t, 4, layout[0].Size)

	// vec4 b: aligned to 16, offset 16, size 16
	require.Equal(t, 16, layout[1].Offset)
	require.Equal(t, 16, layout[1].Size)

	// mat4 c: aligned to 16, offset 32, size 64
	require.Equal(t, 32, layout[2].Offset)
	require.Equal(t, 64, layout[2].Size)
}

func TestReplaceTextureCall(t *testing.T) {
	input := "vec4 c = texture(uTexture, vTexCoord) * vColor;"
	result := replaceTextureCall(input, "uTexture")
	require.Contains(t, result, "uTexture.sample(uTexture_sampler, vTexCoord)")
}

func TestMSLTypeMapping(t *testing.T) {
	require.Equal(t, "float2", mslType("vec2"))
	require.Equal(t, "float4", mslType("vec4"))
	require.Equal(t, "float4x4", mslType("mat4"))
	require.Equal(t, "texture2d<float>", mslType("sampler2D"))
	require.Equal(t, "float", mslType("float"))
}
