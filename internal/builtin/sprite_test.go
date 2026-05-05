package builtin

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/shadertranslate"
)

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
