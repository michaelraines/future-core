package shadertranslate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExtractUniformLayout_StdAlignment pins std140 arithmetic against
// hand-computed offsets for the layouts that matter in practice.
// The vec3-followed-by-float rows are the critical regression guards
// — those are where the Vulkan and MSL copies of this math used to
// diverge.
func TestExtractUniformLayout_StdAlignment(t *testing.T) {
	cases := []struct {
		name   string
		glsl   string
		expect []UniformField
	}{
		{
			name: "single float",
			glsl: `uniform float A;`,
			expect: []UniformField{
				{Name: "A", Type: "float", Offset: 0, Size: 4},
			},
		},
		{
			name: "vec2 then vec3 pads vec3 to offset 16",
			glsl: `uniform vec2 Center;
uniform vec3 LightColor;`,
			expect: []UniformField{
				{Name: "Center", Type: "vec2", Offset: 0, Size: 8},
				{Name: "LightColor", Type: "vec3", Offset: 16, Size: 12},
			},
		},
		{
			name: "vec3 then float packs float into 4-byte tail at offset+12",
			glsl: `uniform vec3 LightColor;
uniform float Intensity;`,
			expect: []UniformField{
				{Name: "LightColor", Type: "vec3", Offset: 0, Size: 12},
				{Name: "Intensity", Type: "float", Offset: 12, Size: 4},
			},
		},
		{
			name: "vec3 then vec3 pads second vec3 to offset 16",
			glsl: `uniform vec3 A;
uniform vec3 B;`,
			expect: []UniformField{
				{Name: "A", Type: "vec3", Offset: 0, Size: 12},
				{Name: "B", Type: "vec3", Offset: 16, Size: 12},
			},
		},
		{
			name: "mat4 always 16-aligned, consumes 64",
			glsl: `uniform mat4 uProjection;
uniform float After;`,
			expect: []UniformField{
				{Name: "uProjection", Type: "mat4", Offset: 0, Size: 64},
				{Name: "After", Type: "float", Offset: 64, Size: 4},
			},
		},
		{
			name: "samplers are filtered out entirely",
			glsl: `uniform sampler2D uTexture0;
uniform sampler2D uTexture1;
uniform vec2 Center;`,
			expect: []UniformField{
				{Name: "Center", Type: "vec2", Offset: 0, Size: 8},
			},
		},
		{
			name: "ivec types share the 4-byte scalar slot",
			glsl: `uniform int Count;
uniform ivec2 Pair;`,
			// ivec2 isn't in uniformSize's table → size 0 → skipped;
			// this case documents that gap. If/when ivec2 support lands,
			// update uniformSize + this expectation together.
			expect: []UniformField{
				{Name: "Count", Type: "int", Offset: 0, Size: 4},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractUniformLayout(tc.glsl)
			require.NoError(t, err)
			require.Equal(t, tc.expect, got)
		})
	}
}

// TestExtractUniformLayout_PointLight pins the full std140 layout of
// the lighting demo's point_light fragment shader. These offsets are
// the ground truth — shaderc's SPIR-V reads from exactly these
// positions (verified via spirv-dis during the vec3 bug hunt).
// If this test fails, either shaderc changed its packing or a new
// uniform was inserted; both cases warrant re-validating the whole
// layout path before merging.
func TestExtractUniformLayout_PointLight(t *testing.T) {
	// Synthesized from point_light.kage's emitted fragment GLSL: image
	// metadata uniforms come first (injected by the Kage compiler),
	// then Kage's `var` declarations in source order.
	glsl := `uniform sampler2D uTexture0;
uniform sampler2D uTexture1;
uniform sampler2D uTexture2;
uniform sampler2D uTexture3;
uniform vec2 uImageDstOrigin;
uniform vec2 uImageDstSize;
uniform vec2 uImageSrc0Origin;
uniform vec2 uImageSrc0Size;
uniform vec2 uImageSrc1Origin;
uniform vec2 uImageSrc1Size;
uniform vec2 uImageSrc2Origin;
uniform vec2 uImageSrc2Size;
uniform vec2 uImageSrc3Origin;
uniform vec2 uImageSrc3Size;
uniform vec2 Center;
uniform vec3 LightColor;
uniform float Intensity;
uniform float Radius;
uniform float FalloffType;
uniform float NormalEnabled;
uniform float LightHeight;
`

	expect := []UniformField{
		{Name: "uImageDstOrigin", Type: "vec2", Offset: 0, Size: 8},
		{Name: "uImageDstSize", Type: "vec2", Offset: 8, Size: 8},
		{Name: "uImageSrc0Origin", Type: "vec2", Offset: 16, Size: 8},
		{Name: "uImageSrc0Size", Type: "vec2", Offset: 24, Size: 8},
		{Name: "uImageSrc1Origin", Type: "vec2", Offset: 32, Size: 8},
		{Name: "uImageSrc1Size", Type: "vec2", Offset: 40, Size: 8},
		{Name: "uImageSrc2Origin", Type: "vec2", Offset: 48, Size: 8},
		{Name: "uImageSrc2Size", Type: "vec2", Offset: 56, Size: 8},
		{Name: "uImageSrc3Origin", Type: "vec2", Offset: 64, Size: 8},
		{Name: "uImageSrc3Size", Type: "vec2", Offset: 72, Size: 8},
		{Name: "Center", Type: "vec2", Offset: 80, Size: 8},
		// LightColor: offset jumps 88 → 96 for vec3's 16-byte alignment.
		{Name: "LightColor", Type: "vec3", Offset: 96, Size: 12},
		// Intensity packs into vec3's 4-byte std140 tail at 108.
		{Name: "Intensity", Type: "float", Offset: 108, Size: 4},
		{Name: "Radius", Type: "float", Offset: 112, Size: 4},
		{Name: "FalloffType", Type: "float", Offset: 116, Size: 4},
		{Name: "NormalEnabled", Type: "float", Offset: 120, Size: 4},
		{Name: "LightHeight", Type: "float", Offset: 124, Size: 4},
	}

	got, err := ExtractUniformLayout(glsl)
	require.NoError(t, err)
	require.Equal(t, expect, got)
}
