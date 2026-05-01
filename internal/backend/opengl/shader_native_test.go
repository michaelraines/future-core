//go:build darwin || linux || freebsd || windows

package opengl

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
)

// TestOpenGLPreferredShaderLanguage confirms the native-shader entry
// reports GLSL — the language OpenGL's NewShader path consumes
// directly. Pure accessor; no GL context required.
func TestOpenGLPreferredShaderLanguage(t *testing.T) {
	dev := New()
	require.Equal(t, backend.ShaderLanguageGLSL, dev.PreferredShaderLanguage())
}

// TestOpenGLNewShaderNativeRejectsMismatch verifies that asking the
// OpenGL backend to compile a non-GLSL native source returns
// ErrUnsupportedShaderLanguage. The runtime safety net behind the
// future-side compat package's build-time guards.
//
// No GL context is needed because the rejection happens before any
// GL call is made.
func TestOpenGLNewShaderNativeRejectsMismatch(t *testing.T) {
	dev := New()
	_, err := dev.NewShaderNative(backend.NativeShaderDescriptor{
		Language: backend.ShaderLanguageWGSL,
		Vertex:   []byte("// not glsl"),
		Fragment: []byte("// not glsl"),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, backend.ErrUnsupportedShaderLanguage),
		"got %v, want wraps ErrUnsupportedShaderLanguage", err)
}
