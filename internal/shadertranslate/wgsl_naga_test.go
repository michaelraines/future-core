package shadertranslate

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/shaderir"
)

// nagaAvailable checks if the naga WGSL validator CLI is on PATH.
// Tests that require naga are skipped if it's not installed.
func nagaAvailable() bool {
	_, err := exec.LookPath("naga")
	return err == nil
}

// validateWGSL runs the naga WGSL validator on the given source.
// Returns nil on success, the validation error otherwise.
func validateWGSL(t *testing.T, source, label string) {
	t.Helper()
	if !nagaAvailable() {
		t.Skip("naga CLI not available; install with: cargo install naga-cli")
	}

	cmd := exec.Command("naga", "--stdin-file-path", label+".wgsl")
	cmd.Stdin = strings.NewReader(source)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("naga validation failed for %s:\n%s\n\nWGSL source:\n%s", label, string(out), source)
	}
}

// TestNagaValidateDefaultSpriteShader validates that the default sprite
// shader's WGSL translation compiles through the real naga WGSL parser.
func TestNagaValidateDefaultSpriteShader(t *testing.T) {
	vResult, err := GLSLToWGSLVertex(spriteVertexGLSL)
	require.NoError(t, err)
	validateWGSL(t, vResult.Source, "sprite_vertex")

	fResult, err := GLSLToWGSLFragment(spriteFragmentGLSL)
	require.NoError(t, err)
	validateWGSL(t, fResult.Source, "sprite_fragment")
}

// TestNagaValidateKageShaders validates the full Kage → GLSL → WGSL
// pipeline for real shader patterns used by the framework. Each test
// compiles Kage source, translates to WGSL, and validates with naga.
func TestNagaValidateKageShaders(t *testing.T) {
	tests := []struct {
		name string
		kage string
	}{
		{
			name: "simple_passthrough",
			kage: `//kage:unit pixels
package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return color
}
`,
		},
		{
			name: "texture_sample",
			kage: `//kage:unit pixels
package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	c := imageSrc0At(srcPos)
	return c * color
}
`,
		},
		{
			name: "normalize_vec3",
			kage: `//kage:unit pixels
package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	sample := imageSrc0At(srcPos)
	normal := normalize(sample.rgb*2.0 - 1.0)
	return vec4(normal, 1.0)
}
`,
		},
		{
			name: "uninitialized_var",
			kage: `//kage:unit pixels
package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	var attenuation float
	attenuation = 1.0
	return vec4(attenuation)
}
`,
		},
		{
			name: "uniform_vec3_access",
			kage: `//kage:unit pixels
package main

var LightPos vec3
var LightColor vec3
var Intensity float
var Radius float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	lightDir := normalize(LightPos)
	dist := length(LightPos.xy - srcPos)
	var attenuation float
	if dist < Radius {
		attenuation = 1.0 - dist/Radius
	}
	result := LightColor * Intensity * attenuation
	return vec4(result, 1.0)
}
`,
		},
		{
			name: "early_return",
			kage: `//kage:unit pixels
package main

var Radius float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	dist := length(srcPos)
	if dist > Radius {
		return vec4(0.0)
	}
	return color
}
`,
		},
		{
			name: "imageSrc_origin_size",
			kage: `//kage:unit pixels
package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	origin := imageSrc0Origin()
	size := imageSrc0Size()
	uv := (srcPos - origin) / size
	return imageSrc0At(srcPos) * vec4(uv, 0.0, 1.0)
}
`,
		},
		{
			name: "clamp_and_mix",
			kage: `//kage:unit pixels
package main

var Blend float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	a := imageSrc0At(srcPos)
	b := vec4(1.0, 0.0, 0.0, 1.0)
	t := clamp(Blend, 0.0, 1.0)
	return mix(a, b, t)
}
`,
		},
		{
			name: "mod_function",
			kage: `//kage:unit pixels
package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	x := mod(srcPos.x, 1.0)
	return vec4(x, 0.0, 0.0, 1.0)
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Kage → GLSL.
			result, err := shaderir.Compile([]byte(tt.kage))
			require.NoError(t, err, "Kage compilation failed")

			// GLSL → WGSL (fragment shader is where the interesting translations happen).
			wgslResult, err := GLSLToWGSLFragment(result.FragmentShader)
			require.NoError(t, err, "GLSL→WGSL translation failed")

			// Validate with naga.
			validateWGSL(t, wgslResult.Source, tt.name+"_fragment")

			// Also validate vertex shader.
			vResult, err := GLSLToWGSLVertex(result.VertexShader)
			require.NoError(t, err, "Vertex GLSL→WGSL translation failed")
			validateWGSL(t, vResult.Source, tt.name+"_vertex")
		})
	}
}

// TestNagaValidateInvalidWGSLDetected confirms naga catches bad WGSL,
// ensuring our test infrastructure actually validates.
func TestNagaValidateInvalidWGSLDetected(t *testing.T) {
	if !nagaAvailable() {
		t.Skip("naga CLI not available")
	}

	cmd := exec.Command("naga", "--stdin-file-path", "invalid.wgsl")
	cmd.Stdin = strings.NewReader("float x = 1.0;")
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "naga should reject invalid WGSL")
	require.Contains(t, string(out), "error")
}

// TestNagaValidateFromKageFiles validates WGSL translation for actual
// .kage shader files from the workspace, if they can be found.
func TestNagaValidateFromKageFiles(t *testing.T) {
	if !nagaAvailable() {
		t.Skip("naga CLI not available")
	}

	// Try to find actual Kage files from the future framework's lighting package.
	kageFiles := []string{
		"../../../future/libs/comp/lighting/point_light.kage",
		"../../../future/libs/comp/lighting/directional_light.kage",
		"../../../future/libs/comp/lighting/spot_light.kage",
		"../../../future/libs/comp/lighting/bloom_blur.kage",
	}

	found := 0
	for _, path := range kageFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // File not available (e.g., different workspace layout).
		}
		found++

		name := strings.TrimSuffix(strings.Replace(path, "../", "", -1), ".kage")
		name = strings.ReplaceAll(name, "/", "_")

		t.Run(name, func(t *testing.T) {
			result, err := shaderir.Compile(data)
			require.NoError(t, err, "Kage compilation failed for %s", path)

			wgslResult, err := GLSLToWGSLFragment(result.FragmentShader)
			require.NoError(t, err, "GLSL→WGSL failed for %s", path)

			validateWGSL(t, wgslResult.Source, name+"_fragment")
		})
	}

	if found == 0 {
		t.Skip("no .kage files found in expected paths")
	}
}
