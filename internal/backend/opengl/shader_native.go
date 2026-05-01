//go:build darwin || linux || freebsd || windows

package opengl

import (
	"fmt"

	"github.com/michaelraines/future-core/internal/backend"
)

// PreferredShaderLanguage reports GLSL — OpenGL's native source
// language. Implements backend.NativeShaderDevice.
func (d *Device) PreferredShaderLanguage() backend.ShaderLanguage {
	return backend.ShaderLanguageGLSL
}

// NewShaderNative compiles a GLSL shader pair without going through
// the Kage → GLSL translation that Device.NewShader's caller does.
//
// The OpenGL backend's existing NewShader already accepts GLSL 330
// directly via ShaderDescriptor.VertexSource/FragmentSource, so the
// native path is a thin wrapper that validates desc.Language and
// dispatches to NewShader. The desc.Uniforms layout is unused —
// OpenGL programs use uniform locations resolved via
// glGetUniformLocation, not byte offsets — but we accept it for API
// symmetry with the WGSL/MSL/SPIR-V native paths so callers can use
// the same registration pattern across backends.
//
// Implements backend.NativeShaderDevice.
func (d *Device) NewShaderNative(desc backend.NativeShaderDescriptor) (backend.Shader, error) {
	if desc.Language != backend.ShaderLanguageGLSL {
		return nil, fmt.Errorf("%w: opengl accepts %s, got %s",
			backend.ErrUnsupportedShaderLanguage,
			backend.ShaderLanguageGLSL, desc.Language)
	}
	return d.NewShader(backend.ShaderDescriptor{
		VertexSource:   string(desc.Vertex),
		FragmentSource: string(desc.Fragment),
		Attributes:     desc.Attributes,
	})
}
