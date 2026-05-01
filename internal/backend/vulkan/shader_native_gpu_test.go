//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
)

// TestVulkanPreferredShaderLanguage confirms the native-shader entry
// reports GLSL. Pure accessor — no Vulkan instance required.
func TestVulkanPreferredShaderLanguage(t *testing.T) {
	dev := New()
	require.Equal(t, backend.ShaderLanguageGLSL, dev.PreferredShaderLanguage())
}

// TestVulkanNewShaderNativeRejectsMismatch verifies that asking the
// Vulkan backend to compile a non-GLSL native source returns
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
