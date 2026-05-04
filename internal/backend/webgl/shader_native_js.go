//go:build js

package webgl

import (
	"fmt"

	"github.com/michaelraines/future-core/internal/backend"
)

// PreferredShaderLanguage reports GLSL ES 3.00 — WebGL2's native
// source language. Implements backend.NativeShaderDevice.
//
// Declaring this lets the futurecore-side shader registry pick a
// hand-written `.glsles` variant when one exists for a given
// shader, bypassing the Kage→GLSL translator. The translator
// targets desktop GLSL 330 semantics; WebGL2 (GLSL ES 3.00) is
// stricter on int-vs-float comparisons and a handful of other
// conversions, so the per-backend hand-written variant model
// other backends already use is the right fit here too.
func (d *Device) PreferredShaderLanguage() backend.ShaderLanguage {
	return backend.ShaderLanguageGLSLES
}

// NewShaderNative compiles a GLSL ES 3.00 shader pair directly.
//
// The Kage→GLSL→ES translator path is bypassed — desc.Vertex /
// desc.Fragment already contain ES 3.00 source. Uniform layout is
// accepted but unused; WebGL2 binds uniforms via gl.uniform* against
// resolved locations, not std140 byte offsets, so the layout serves
// only as informational symmetry with WGSL/MSL/SPIR-V native paths.
//
// Implements backend.NativeShaderDevice.
func (d *Device) NewShaderNative(desc backend.NativeShaderDescriptor) (backend.Shader, error) {
	if desc.Language != backend.ShaderLanguageGLSLES {
		return nil, fmt.Errorf("%w: webgl accepts %s, got %s",
			backend.ErrUnsupportedShaderLanguage,
			backend.ShaderLanguageGLSLES, desc.Language)
	}
	return d.NewShader(backend.ShaderDescriptor{
		VertexSource:   string(desc.Vertex),
		FragmentSource: string(desc.Fragment),
		Attributes:     desc.Attributes,
	})
}
