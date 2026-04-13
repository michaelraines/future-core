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

func TestWGSLModCallReplace(t *testing.T) {
	input := "float r = mod(x, 1.0);"
	result := replaceWGSLModCall(input)
	require.Equal(t, "float r = (x % 1.0);", result)
}

func TestWGSLModCallNested(t *testing.T) {
	input := "float r = mod(x + 0.5, 1.0) * 2.0;"
	result := replaceWGSLModCall(input)
	require.Contains(t, result, "(x + 0.5 % 1.0)")
}

func TestStripLineComment(t *testing.T) {
	require.Equal(t, "float x = 1.0;", stripLineComment("float x = 1.0; // initialize"))
	require.Equal(t, "float x = 1.0;", stripLineComment("float x = 1.0;"))
	require.Equal(t, "", stripLineComment("// full line comment"))
}

func TestGLSLToWGSLFragmentWithBuiltins(t *testing.T) {
	glsl := `#version 330 core
in vec2 vTexCoord;
uniform float uTime;
out vec4 fragColor;
void main() {
    float r = 0.5 + 0.5 * sin(uTime + vTexCoord.x * 6.2831); // red channel
    float g = clamp(cos(uTime), 0.0, 1.0);
    float b = mod(uTime, 1.0);
    fragColor = vec4(r, g, b, 1.0);
}
`
	result, err := GLSLToWGSLFragment(glsl)
	require.NoError(t, err)

	src := result.Source
	t.Logf("Translated fragment WGSL with builtins:\n%s", src)

	// sin/cos/clamp pass through unchanged.
	require.Contains(t, src, "sin(uniforms.uTime")
	require.Contains(t, src, "clamp(cos(uniforms.uTime)")

	// mod(x, y) → (x % y)
	require.Contains(t, src, "(uniforms.uTime % 1.0)")
	require.NotContains(t, src, "mod(")

	// Comment stripped.
	require.NotContains(t, src, "// red channel")

	// Local vars converted.
	require.Contains(t, src, "var r: f32 =")
	require.Contains(t, src, "var g: f32 =")
	require.Contains(t, src, "var b: f32 =")
}

func TestGLSLToWGSLFragmentWithControlFlow(t *testing.T) {
	glsl := `#version 330 core
in vec4 vColor;
out vec4 fragColor;
void main() {
    if (vColor.a < 0.01) {
        discard;
    }
    fragColor = vColor;
}
`
	result, err := GLSLToWGSLFragment(glsl)
	require.NoError(t, err)

	src := result.Source
	t.Logf("Translated fragment WGSL with control flow:\n%s", src)

	// if/discard pass through (identical in WGSL).
	require.Contains(t, src, "if (in.vColor.a < 0.01)")
	require.Contains(t, src, "discard;")
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

func TestWGSLUninitializedVarDecl(t *testing.T) {
	tests := []struct {
		name  string
		glsl  string
		want  string
	}{
		{"float", "    float attenuation;", "    var attenuation: f32;"},
		{"vec2", "    vec2 offset;", "    var offset: vec2<f32>;"},
		{"vec4", "    vec4 color;", "    var color: vec4<f32>;"},
		{"int", "    int count;", "    var count: i32;"},
		{"with init stays unchanged", "    float x = 1.0;", "    var x: f32 = 1.0;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceWGSLLocalVarDecl(tt.glsl)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestWGSLImageHelpers(t *testing.T) {
	// GLSL fragment shader with imageSrc0At calls (as generated by Kage compiler).
	glsl := `#version 330
uniform sampler2D uTexture0;
uniform vec2 uImageSrc0Origin;
uniform vec2 uImageSrc0Size;
in vec2 vTexCoord;
in vec4 vColor;
out vec4 fragColor;
vec4 imageSrc0At(vec2 pos) {
    vec2 origin = uImageSrc0Origin;
    vec2 size = uImageSrc0Size;
    if (pos.x < origin.x || pos.y < origin.y || pos.x >= origin.x + size.x || pos.y >= origin.y + size.y) {
        return vec4(0.0);
    }
    return texture(uTexture0, pos);
}
void main() {
    vec4 c = imageSrc0At(vTexCoord);
    fragColor = c * vColor;
}
`
	result, err := GLSLToWGSLFragment(glsl)
	require.NoError(t, err)

	src := result.Source
	t.Logf("Translated WGSL:\n%s", src)

	// The WGSL output should contain the imageSrc0At helper function.
	require.Contains(t, src, "fn imageSrc0At(pos: vec2<f32>) -> vec4<f32>")
	require.Contains(t, src, "textureSample(uTexture0, uTexture0_sampler, pos)")

	// The body should call imageSrc0At (pass-through, not translated).
	require.Contains(t, src, "imageSrc0At(in.vTexCoord)")

	// Origin/size helpers should also be emitted.
	require.Contains(t, src, "fn imageSrc0Origin() -> vec2<f32>")
	require.Contains(t, src, "fn imageSrc0Size() -> vec2<f32>")
}

func TestWGSLImageHelpersNotEmittedWhenUnused(t *testing.T) {
	// Plain fragment shader without any imageSrc calls.
	glsl := `#version 330
uniform sampler2D uTexture;
in vec2 vTexCoord;
in vec4 vColor;
out vec4 fragColor;
void main() {
    fragColor = texture(uTexture, vTexCoord) * vColor;
}
`
	result, err := GLSLToWGSLFragment(glsl)
	require.NoError(t, err)

	// No image helpers should appear.
	require.NotContains(t, result.Source, "fn imageSrc")
	require.NotContains(t, result.Source, "fn imageDst")
}
