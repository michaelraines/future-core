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
	require.Contains(t, src, "textureSampleLevel(uTexture, uTexture_sampler, in.vTexCoord, 0.0)")

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
	require.Contains(t, result, "textureSampleLevel(uTexture, uTexture_sampler, vTexCoord, 0.0)")
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

	// sin passes through unchanged.
	require.Contains(t, src, "sin(uniforms.uTime")

	// clamp(x, 0.0, 1.0) is rewritten to saturate(x) so the call works
	// for both scalar and vector x under WGSL's strict typing — GLSL
	// auto-broadcasts the scalar min/max args, WGSL doesn't. See
	// replaceWGSLClampSaturate in wgsl.go for details.
	require.Contains(t, src, "saturate(cos(uniforms.uTime))")
	require.NotContains(t, src, "clamp(cos")

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

// TestGLSLToWGSLFragmentClampVec3 regresses the common Kage/GLSL pattern
// `clamp(vec_expr, 0, 1)` — which relies on scalar broadcast — compiling
// as valid WGSL. The translator rewrites it to `saturate(vec_expr)` so
// it works for both scalar and vector first args under WGSL's strict
// typing rules.
func TestGLSLToWGSLFragmentClampVec3(t *testing.T) {
	glsl := `#version 330 core
in vec4 vColor;
out vec4 fragColor;
void main() {
    vec3 rgb = clamp(vColor.rgb * 2.0, 0, 1);
    fragColor = vec4(rgb, 1.0);
}
`
	result, err := GLSLToWGSLFragment(glsl)
	require.NoError(t, err)

	src := result.Source
	t.Logf("Translated fragment WGSL with vec3 clamp:\n%s", src)

	require.Contains(t, src, "saturate(in.vColor.rgb * 2.0)")
	require.NotContains(t, src, "clamp(in.vColor.rgb * 2.0, 0, 1)")

	// Also accepts the explicit-float form `clamp(x, 0.0, 1.0)`.
	glslFloats := `#version 330 core
in vec4 vColor;
out vec4 fragColor;
void main() {
    fragColor = vec4(clamp(vColor.rgb, 0.0, 1.0), 1.0);
}
`
	result2, err := GLSLToWGSLFragment(glslFloats)
	require.NoError(t, err)
	require.Contains(t, result2.Source, "saturate(in.vColor.rgb)")
	require.NotContains(t, result2.Source, "clamp(in.vColor.rgb, 0.0, 1.0)")
}

// TestGLSLToWGSLFragmentClampPreserved confirms that non-saturate
// clamps (e.g. `clamp(x, 0.2, 0.8)`) are left as-is so downstream
// behavior matches GLSL exactly.
func TestGLSLToWGSLFragmentClampPreserved(t *testing.T) {
	glsl := `#version 330 core
in vec4 vColor;
out vec4 fragColor;
void main() {
    float v = clamp(vColor.a, 0.2, 0.8);
    fragColor = vec4(vColor.rgb, v);
}
`
	result, err := GLSLToWGSLFragment(glsl)
	require.NoError(t, err)
	require.Contains(t, result.Source, "clamp(in.vColor.a, 0.2, 0.8)")
	require.NotContains(t, result.Source, "saturate")
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
		name string
		glsl string
		want string
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

	// The WGSL output should contain the imageSrc0At helper function
	// using select() for uniform control flow (no if-branch around textureSample).
	require.Contains(t, src, "fn imageSrc0At(pos: vec2<f32>) -> vec4<f32>")
	// pos is in pixel coords (Kage `kage:unit pixels`); helper divides
	// by textureDimensions before passing to textureSampleLevel.
	require.Contains(t, src, "textureSampleLevel(uTexture0, uTexture0_sampler, uv, 0.0)")
	require.Contains(t, src, "textureDimensions(uTexture0)")
	require.Contains(t, src, "select(vec4<f32>(0.0), sampled, inBounds)")

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

// --- Comprehensive translation test suite ---

func TestWGSLLocalVarDecl(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// Initialized declarations.
		{"float init", "    float x = 1.0;", "    var x: f32 = 1.0;"},
		// When the same type appears as both the declaration type AND a
		// constructor on the same line, the constructor may not be replaced
		// (known limitation of replaceWGSLTypeConstructor's index check).
		{"vec2 init", "    vec2 v = vec2(1.0, 2.0);", "    var v: vec2<f32> = vec2(1.0, 2.0);"},
		{"vec3 init", "    vec3 n = normalize(v);", "    var n: vec3<f32> = normalize(v);"},
		{"vec4 init", "    vec4 c = vec4(0.0);", "    var c: vec4<f32> = vec4(0.0);"},
		{"int init", "    int i = 0;", "    var i: i32 = 0;"},
		{"mat4 init", "    mat4 m = mat4(1.0);", "    var m: mat4x4<f32> = mat4x4<f32>(1.0);"},
		{"mat3 init", "    mat3 m = mat3(1.0);", "    var m: mat3x3<f32> = mat3x3<f32>(1.0);"},
		{"bool init", "    bool b = true;", "    var b: bool = true;"},
		{"ivec2 init", "    ivec2 v = ivec2(1, 2);", "    var v: vec2<i32> = vec2<i32>(1, 2);"},

		// Uninitialized declarations.
		{"float no init", "    float x;", "    var x: f32;"},
		{"vec2 no init", "    vec2 v;", "    var v: vec2<f32>;"},
		{"vec3 no init", "    vec3 n;", "    var n: vec3<f32>;"},
		{"vec4 no init", "    vec4 c;", "    var c: vec4<f32>;"},
		{"int no init", "    int i;", "    var i: i32;"},
		{"bool no init", "    bool b;", "    var b: bool;"},
		{"ivec4 no init", "    ivec4 v;", "    var v: vec4<i32>;"},

		// Non-matching lines pass through unchanged.
		{"assignment", "    x = 1.0;", "    x = 1.0;"},
		{"function call", "    doSomething();", "    doSomething();"},
		{"return", "    return vec4(1.0);", "    return vec4(1.0);"},
		{"if statement", "    if (x > 0.0) {", "    if (x > 0.0) {"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceWGSLLocalVarDecl(tt.in)
			// Also apply type constructor replacement for init cases.
			if strings.Contains(tt.in, "=") {
				got = replaceWGSLTypes(got)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestWGSLModCall(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple", "mod(x, y)", "(x % y)"},
		{"with expr", "mod(a + b, 2.0)", "(a + b % 2.0)"},
		{"nested in expr", "float r = mod(x, 1.0);", "float r = (x % 1.0);"},
		{"no mod", "normalize(v)", "normalize(v)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceWGSLModCall(tt.in)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestWGSLTypeConstructors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"vec2", "vec2(1.0, 2.0)", "vec2<f32>(1.0, 2.0)"},
		{"vec3", "vec3(0.0)", "vec3<f32>(0.0)"},
		{"vec4", "vec4(1.0, 0.0, 0.0, 1.0)", "vec4<f32>(1.0, 0.0, 0.0, 1.0)"},
		{"mat4", "mat4(1.0)", "mat4x4<f32>(1.0)"},
		{"mat3", "mat3(m)", "mat3x3<f32>(m)"},
		{"ivec2", "ivec2(1, 2)", "vec2<i32>(1, 2)"},
		{"already wgsl", "vec2<f32>(1.0)", "vec2<f32>(1.0)"},
		{"no type", "normalize(v)", "normalize(v)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceWGSLTypes(tt.in)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestWGSLTextureCall(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		sampler string
		want    string
	}{
		{"basic", "texture(uTexture, uv)", "uTexture",
			"textureSampleLevel(uTexture, uTexture_sampler, uv, 0.0)"},
		{"numbered", "texture(uTexture0, pos)", "uTexture0",
			"textureSampleLevel(uTexture0, uTexture0_sampler, pos, 0.0)"},
		{"no match", "texture(other, uv)", "uTexture",
			"texture(other, uv)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceWGSLTextureCall(tt.in, tt.sampler)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestWGSLFragmentBareReturn(t *testing.T) {
	// Fragment shader with bare return inside an if block.
	// The fragColor assignment should become "return", and the bare
	// "return;" should be removed (not converted to return vec4).
	glsl := `#version 330
uniform sampler2D uTexture;
in vec2 vTexCoord;
in vec4 vColor;
out vec4 fragColor;
void main() {
    if (vColor.a <= 0.0) {
        fragColor = vec4(0.0);
        return;
    }
    fragColor = texture(uTexture, vTexCoord) * vColor;
}
`
	result, err := GLSLToWGSLFragment(glsl)
	require.NoError(t, err)

	src := result.Source
	t.Logf("WGSL:\n%s", src)

	// The first fragColor assignment should become a return.
	require.Contains(t, src, "return vec4<f32>(0.0)")

	// The bare "return;" should be removed, not produce unreachable code.
	require.NotContains(t, src, "return vec4<f32>(0.0);\n    return")
	// No bare "return;" should remain.
	for _, line := range strings.Split(src, "\n") {
		require.NotEqual(t, "    return;", strings.TrimRight(line, " "))
	}
}

func TestWGSLFragmentMultipleTextures(t *testing.T) {
	// Fragment shader using two textures (like a lightmap + scene composite).
	glsl := `#version 330
uniform sampler2D uTexture0;
uniform sampler2D uTexture1;
in vec2 vTexCoord;
in vec4 vColor;
out vec4 fragColor;
void main() {
    vec4 scene = texture(uTexture0, vTexCoord);
    vec4 light = texture(uTexture1, vTexCoord);
    fragColor = scene * light;
}
`
	result, err := GLSLToWGSLFragment(glsl)
	require.NoError(t, err)

	src := result.Source
	t.Logf("WGSL:\n%s", src)

	// Both textures should have bindings.
	require.Contains(t, src, "var uTexture0: texture_2d<f32>")
	require.Contains(t, src, "var uTexture0_sampler: sampler")
	require.Contains(t, src, "var uTexture1: texture_2d<f32>")
	require.Contains(t, src, "var uTexture1_sampler: sampler")

	// Both texture samples should be translated.
	require.Contains(t, src, "textureSampleLevel(uTexture0, uTexture0_sampler, in.vTexCoord, 0.0)")
	require.Contains(t, src, "textureSampleLevel(uTexture1, uTexture1_sampler, in.vTexCoord, 0.0)")
}

func TestWGSLImageHelpersMultipleIndices(t *testing.T) {
	// GLSL using both imageSrc0At and imageSrc1At.
	glsl := `#version 330
uniform sampler2D uTexture0;
uniform sampler2D uTexture1;
uniform vec2 uImageSrc0Origin;
uniform vec2 uImageSrc0Size;
uniform vec2 uImageSrc1Origin;
uniform vec2 uImageSrc1Size;
in vec2 vTexCoord;
in vec4 vColor;
out vec4 fragColor;
vec4 imageSrc0At(vec2 pos) { return texture(uTexture0, pos); }
vec4 imageSrc1At(vec2 pos) { return texture(uTexture1, pos); }
void main() {
    vec4 a = imageSrc0At(vTexCoord);
    vec4 b = imageSrc1At(vTexCoord);
    fragColor = a + b;
}
`
	result, err := GLSLToWGSLFragment(glsl)
	require.NoError(t, err)

	src := result.Source

	// Both image helpers should be emitted.
	require.Contains(t, src, "fn imageSrc0At(pos: vec2<f32>) -> vec4<f32>")
	require.Contains(t, src, "fn imageSrc1At(pos: vec2<f32>) -> vec4<f32>")
	require.Contains(t, src, "fn imageSrc0UnsafeAt")
	require.Contains(t, src, "fn imageSrc1UnsafeAt")
}

func TestWGSLImageDstHelpers(t *testing.T) {
	glsl := `#version 330
uniform vec2 uImageDstOrigin;
uniform vec2 uImageDstSize;
in vec2 vTexCoord;
in vec4 vColor;
out vec4 fragColor;
void main() {
    vec2 origin = imageDstOrigin();
    vec2 size = imageDstSize();
    fragColor = vec4(origin.x / size.x, 0.0, 0.0, 1.0);
}
`
	result, err := GLSLToWGSLFragment(glsl)
	require.NoError(t, err)

	src := result.Source

	require.Contains(t, src, "fn imageDstOrigin() -> vec2<f32>")
	require.Contains(t, src, "fn imageDstSize() -> vec2<f32>")
	require.Contains(t, src, "uniforms.uImageDstOrigin")
	require.Contains(t, src, "uniforms.uImageDstSize")
}

func TestWGSLFragmentComplexLighting(t *testing.T) {
	// Simulate a point light shader with the patterns that caused
	// real translation failures: uninitialized vars, uniform field
	// access, normalize of uniform vec3, imageSrc0At sampling.
	glsl := `#version 330
uniform sampler2D uTexture0;
uniform vec2 uImageSrc0Origin;
uniform vec2 uImageSrc0Size;
uniform vec3 LightPos;
uniform vec3 LightColor;
uniform float Intensity;
uniform float Radius;
in vec2 vTexCoord;
in vec4 vColor;
out vec4 fragColor;
vec4 imageSrc0At(vec2 pos) {
    vec2 origin = uImageSrc0Origin;
    vec2 size = uImageSrc0Size;
    if (pos.x < origin.x) { return vec4(0.0); }
    return texture(uTexture0, pos);
}
void main() {
    vec4 normalSample = imageSrc0At(vTexCoord);
    vec3 normal = normalize(((normalSample.rgb * 2.0) - 1.0));
    float dist = length(LightPos.xy - vTexCoord);
    float attenuation;
    if (dist < Radius) {
        attenuation = 1.0 - (dist / Radius);
    }
    vec3 result = ((LightColor * Intensity) * attenuation);
    fragColor = vec4(result, 1.0);
}
`
	result, err := GLSLToWGSLFragment(glsl)
	require.NoError(t, err)

	src := result.Source
	t.Logf("WGSL:\n%s", src)

	// imageSrc0At helper emitted with uniform-flow-safe select().
	require.Contains(t, src, "fn imageSrc0At(pos: vec2<f32>)")
	require.Contains(t, src, "select(")

	// Uninitialized var declaration converted.
	require.Contains(t, src, "var attenuation: f32;")

	// Initialized declarations converted.
	require.Contains(t, src, "var normalSample: vec4<f32>")
	require.Contains(t, src, "var normal: vec3<f32>")
	require.Contains(t, src, "var dist: f32")
	require.Contains(t, src, "var result: vec3<f32>")

	// Uniform references prefixed.
	require.Contains(t, src, "uniforms.LightPos")
	require.Contains(t, src, "uniforms.LightColor")
	require.Contains(t, src, "uniforms.Intensity")
	require.Contains(t, src, "uniforms.Radius")

	// No bare GLSL types remaining in var declarations.
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "var ") && strings.Contains(trimmed, "=") {
			require.NotContains(t, trimmed, ": f32 = normalize",
				"normalize should infer vec3, not f32")
		}
	}
}
