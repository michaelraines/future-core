package builtin

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/shadertranslate"
)

// TestSpriteAccessorsReturnEmbeddedAssets confirms the public
// accessors on this package return the embedded GLSL + SPIR-V assets
// (i.e. //go:embed wired correctly and the accessors don't return
// placeholder zero values). Cheap sanity check against asset
// regressions.
func TestSpriteAccessorsReturnEmbeddedAssets(t *testing.T) {
	require.NotEmpty(t, SpriteVertexGLSL(), "vertex GLSL must be embedded")
	require.NotEmpty(t, SpriteFragmentGLSL(), "fragment GLSL must be embedded")
	require.Contains(t, SpriteVertexGLSL(), "#version 450", "vertex GLSL header")
	require.Contains(t, SpriteFragmentGLSL(), "#version 450", "fragment GLSL header")

	require.NotEmpty(t, SpriteVertexWGSL(), "vertex WGSL must be embedded")
	require.NotEmpty(t, SpriteFragmentWGSL(), "fragment WGSL must be embedded")
	// Both stages must declare the matching Uniforms struct so the
	// engine's combined uniform packer fills the buffer once and both
	// shader modules read the same bytes — the symptom of mismatch is
	// uColorBody/uColorTranslation reading garbage and the texture
	// rendering as black.
	require.Contains(t, string(SpriteVertexWGSL()), "uniforms.uProjection")
	require.Contains(t, string(SpriteFragmentWGSL()), "uniforms.uColorBody")
	require.Contains(t, string(SpriteFragmentWGSL()), "uniforms.uColorTranslation")

	require.NotEmpty(t, SpriteVertexSPIRV(), "vertex SPIR-V must be embedded")
	require.NotEmpty(t, SpriteFragmentSPIRV(), "fragment SPIR-V must be embedded")
	// SPIR-V magic number 0x07230203, little-endian => 03 02 23 07.
	require.GreaterOrEqual(t, len(SpriteVertexSPIRV()), 4, "vertex SPIR-V header")
	require.Equal(t, []byte{0x03, 0x02, 0x23, 0x07}, SpriteVertexSPIRV()[:4],
		"vertex SPIR-V magic")
	require.GreaterOrEqual(t, len(SpriteFragmentSPIRV()), 4, "fragment SPIR-V header")
	require.Equal(t, []byte{0x03, 0x02, 0x23, 0x07}, SpriteFragmentSPIRV()[:4],
		"fragment SPIR-V magic")

	// SPIR-V byte length must be a multiple of 4 (vkCreateShaderModule
	// requires this; createShaderModuleFromSPIRV in the Vulkan backend
	// returns an error otherwise).
	require.Zero(t, len(SpriteVertexSPIRV())%4, "vertex SPIR-V length divisible by 4")
	require.Zero(t, len(SpriteFragmentSPIRV())%4, "fragment SPIR-V length divisible by 4")

	layout := SpriteUniformLayout()
	require.NotEmpty(t, layout, "uniform layout must declare at least one field")
}

// TestSpriteUniformLayoutMatchesGLSL guards the hand-coded SPIR-V
// uniform layout against drift from the GLSL declarations. The Vulkan
// backend's GLSL-via-shaderc path derives layout via
// shadertranslate.ExtractUniformLayout at compile time; the SPIR-V
// path uses the layout supplied at NewShaderNative time. They MUST
// match — if the offsets diverge, the per-draw uniform packer writes
// to the wrong byte ranges and the shader reads garbage.
func TestSpriteUniformLayoutMatchesGLSL(t *testing.T) {
	combined := SpriteVertexGLSL() + "\n" + SpriteFragmentGLSL()
	derived, err := shadertranslate.ExtractUniformLayout(combined)
	require.NoError(t, err)
	require.NotEmpty(t, derived, "ExtractUniformLayout should find uniforms")

	declared := SpriteUniformLayout()

	// Build name -> (offset, size) maps for order-independent comparison.
	// ExtractUniformLayout merges both stages; SpriteUniformLayout is the
	// authored truth — every authored entry must agree with the derived
	// values for the matching uniform name.
	derivedByName := make(map[string]shadertranslate.UniformField, len(derived))
	for _, f := range derived {
		derivedByName[f.Name] = f
	}

	for _, want := range declared {
		got, ok := derivedByName[want.Name]
		require.Truef(t, ok, "uniform %q present in SpriteUniformLayout but not derived from GLSL", want.Name)
		require.Equalf(t, want.Offset, got.Offset, "uniform %q offset mismatch", want.Name)
		require.Equalf(t, want.Size, got.Size, "uniform %q size mismatch", want.Name)
	}
}
