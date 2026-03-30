package shadertranslate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGLSLToWGSLVertex(t *testing.T) {
	result, err := GLSLToWGSLVertex(spriteVertexGLSL)
	require.NoError(t, err)

	src := result.Source
	t.Logf("Translated vertex WGSL:\n%s", src)

	// VertexInput struct with attributes.
	require.Contains(t, src, "@location(0) aPosition: vec2<f32>")
	require.Contains(t, src, "@location(1) aTexCoord: vec2<f32>")
	require.Contains(t, src, "@location(2) aColor: vec4<f32>")

	// VertexOutput struct with varyings.
	require.Contains(t, src, "@builtin(position) position: vec4<f32>")
	require.Contains(t, src, "@location(0) vTexCoord: vec2<f32>")
	require.Contains(t, src, "@location(1) vColor: vec4<f32>")

	// Uniform struct.
	require.Contains(t, src, "struct VertexUniforms")
	require.Contains(t, src, "uProjection: mat4x4<f32>")

	// Binding declaration.
	require.Contains(t, src, "@group(0) @binding(0) var<uniform> uniforms: VertexUniforms")

	// Function signature.
	require.Contains(t, src, "@vertex")
	require.Contains(t, src, "fn vs_main(in: VertexInput) -> VertexOutput")

	// Body translation.
	require.Contains(t, src, "out.vTexCoord = in.aTexCoord")
	require.Contains(t, src, "out.vColor = in.aColor")
	require.Contains(t, src, "out.position = uniforms.uProjection * vec4<f32>(in.aPosition, 0.0, 1.0)")
	require.Contains(t, src, "return out;")

	// No GLSL remnants.
	require.NotContains(t, src, "#version")
	require.NotContains(t, src, "gl_Position")

	// Uniform layout.
	require.Len(t, result.Uniforms, 1)
	require.Equal(t, "uProjection", result.Uniforms[0].Name)
	require.Equal(t, 0, result.Uniforms[0].Offset)
	require.Equal(t, 64, result.Uniforms[0].Size)
}

func TestGLSLToWGSLFragment(t *testing.T) {
	result, err := GLSLToWGSLFragment(spriteFragmentGLSL)
	require.NoError(t, err)

	src := result.Source
	t.Logf("Translated fragment WGSL:\n%s", src)

	// FragmentInput struct.
	require.Contains(t, src, "struct FragmentInput")
	require.Contains(t, src, "@builtin(position) position: vec4<f32>")
	require.Contains(t, src, "@location(0) vTexCoord: vec2<f32>")
	require.Contains(t, src, "@location(1) vColor: vec4<f32>")

	// Uniform struct (no sampler2D).
	require.Contains(t, src, "struct FragmentUniforms")
	require.Contains(t, src, "uColorBody: mat4x4<f32>")
	require.Contains(t, src, "uColorTranslation: vec4<f32>")
	require.NotContains(t, src, "sampler2D")

	// Texture and sampler bindings.
	require.Contains(t, src, "@group(1) @binding(0) var uTexture: texture_2d<f32>")
	require.Contains(t, src, "@group(1) @binding(1) var uTexture_sampler: sampler")

	// Uniform binding.
	require.Contains(t, src, "@group(0) @binding(0) var<uniform> uniforms: FragmentUniforms")

	// Function signature.
	require.Contains(t, src, "@fragment")
	require.Contains(t, src, "fn fs_main(in: FragmentInput) -> @location(0) vec4<f32>")

	// Body: local var declaration converted to WGSL syntax.
	require.Contains(t, src, "var c: vec4<f32> =")

	// Body: texture sampling.
	require.Contains(t, src, "textureSample(uTexture, uTexture_sampler, in.vTexCoord)")

	// Body: uniform struct access.
	require.Contains(t, src, "uniforms.uColorBody")
	require.Contains(t, src, "uniforms.uColorTranslation")

	// Body: fragColor → return.
	require.True(t, strings.Contains(src, "return uniforms.uColorBody"))

	// Uniform layout.
	require.Len(t, result.Uniforms, 2)
	require.Equal(t, "uColorBody", result.Uniforms[0].Name)
	require.Equal(t, 0, result.Uniforms[0].Offset)
	require.Equal(t, 64, result.Uniforms[0].Size)
	require.Equal(t, "uColorTranslation", result.Uniforms[1].Name)
	require.Equal(t, 64, result.Uniforms[1].Offset)
	require.Equal(t, 16, result.Uniforms[1].Size)
}

func TestWGSLTypeMapping(t *testing.T) {
	require.Equal(t, "vec2<f32>", wgslType("vec2"))
	require.Equal(t, "vec4<f32>", wgslType("vec4"))
	require.Equal(t, "mat4x4<f32>", wgslType("mat4"))
	require.Equal(t, "texture_2d<f32>", wgslType("sampler2D"))
	require.Equal(t, "f32", wgslType("float"))
	require.Equal(t, "i32", wgslType("int"))
	require.Equal(t, "vec2<i32>", wgslType("ivec2"))
}

func TestWGSLTextureCallReplace(t *testing.T) {
	input := "vec4 c = texture(uTexture, vTexCoord) * vColor;"
	result := replaceWGSLTextureCall(input, "uTexture")
	require.Contains(t, result, "textureSample(uTexture, uTexture_sampler, vTexCoord)")
}

func TestWGSLUniformLayout(t *testing.T) {
	uniforms := []uniform{
		{typ: "float", name: "a"},
		{typ: "vec4", name: "b"},
		{typ: "mat4", name: "c"},
	}
	layout := buildWGSLUniformLayout(uniforms)
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

func TestGLSLToWGSLVertexNoUniforms(t *testing.T) {
	glsl := `#version 330 core
layout(location = 0) in vec2 aPosition;
void main() {
    gl_Position = vec4(aPosition, 0.0, 1.0);
}
`
	result, err := GLSLToWGSLVertex(glsl)
	require.NoError(t, err)
	require.NotContains(t, result.Source, "VertexUniforms")
	require.NotContains(t, result.Source, "@group")
	require.Contains(t, result.Source, "out.position = vec4<f32>(in.aPosition, 0.0, 1.0)")
	require.Nil(t, result.Uniforms)
}

func TestGLSLToWGSLFragmentNoSamplers(t *testing.T) {
	glsl := `#version 330 core
in vec4 vColor;
out vec4 fragColor;
void main() {
    fragColor = vColor;
}
`
	result, err := GLSLToWGSLFragment(glsl)
	require.NoError(t, err)
	require.NotContains(t, result.Source, "texture_2d")
	require.Contains(t, result.Source, "return in.vColor")
}
