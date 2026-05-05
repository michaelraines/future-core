//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
)

// TestVulkanPreferredShaderLanguage confirms the native-shader entry
// reports SPIR-V. Pure accessor — no Vulkan instance required. Vulkan
// also accepts GLSL via NewShaderNative; SPIR-V is preferred because
// it skips shaderc entirely (required on Android, where libshaderc
// is not available).
func TestVulkanPreferredShaderLanguage(t *testing.T) {
	dev := New()
	require.Equal(t, backend.ShaderLanguageSPIRV, dev.PreferredShaderLanguage())
}

// TestVulkanNewShaderNativeRejectsMismatch verifies that asking the
// Vulkan backend to compile a non-GLSL/non-SPIR-V native source returns
// ErrUnsupportedShaderLanguage. The rejection happens before any
// Vulkan call, so no instance/device init is required.
func TestVulkanNewShaderNativeRejectsMismatch(t *testing.T) {
	dev := New()
	_, err := dev.NewShaderNative(backend.NativeShaderDescriptor{
		Language: backend.ShaderLanguageMSL,
		Vertex:   []byte("// not glsl"),
		Fragment: []byte("// not glsl"),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, backend.ErrUnsupportedShaderLanguage),
		"got %v, want wraps ErrUnsupportedShaderLanguage", err)
}

// TestVulkanNewShaderNativeSPIRVStashesBytes confirms the SPIR-V
// language path stores the bytes + uniform layout on the resulting
// Shader without invoking shaderc, so callers shipping pre-compiled
// SPIR-V (e.g. on Android) avoid the libshaderc runtime dependency.
// Doesn't init a Vulkan device — only checks the descriptor-handling
// branch in NewShaderNative.
func TestVulkanNewShaderNativeSPIRVStashesBytes(t *testing.T) {
	dev := New()

	// Minimal valid SPIR-V magic + version header. Real bytes would
	// be a full module; this test only asserts construction state,
	// not vkCreateShaderModule (which needs a real device).
	header := []byte{
		0x03, 0x02, 0x23, 0x07, // SPIR-V magic 0x07230203 (LE)
		0x00, 0x00, 0x01, 0x00, // version 1.0
	}

	sh, err := dev.NewShaderNative(backend.NativeShaderDescriptor{
		Language: backend.ShaderLanguageSPIRV,
		Vertex:   header,
		Fragment: header,
		Uniforms: []backend.NativeUniformField{
			{Name: "uProjection", Offset: 0, Size: 64},
		},
	})
	require.NoError(t, err)
	defer sh.Dispose()

	s := sh.(*Shader)
	require.True(t, s.nativeMode, "nativeMode must be set so compile() skips shaderc")
	require.Equal(t, header, s.vertexSPIRV)
	require.Equal(t, header, s.fragmentSPIRV)
	require.Empty(t, s.vertexSource, "no GLSL source on the SPIR-V path")
	require.Empty(t, s.fragmentSource, "no GLSL source on the SPIR-V path")
	require.Len(t, s.vertexUniformLayout, 1, "uniform layout came from the descriptor")
	require.Equal(t, "uProjection", s.vertexUniformLayout[0].Name)
}
