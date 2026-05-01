//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"fmt"

	"github.com/michaelraines/future-core/internal/backend"
)

// PreferredShaderLanguage reports GLSL — the Vulkan backend's preferred
// non-Kage source. Vulkan can also accept SPIR-V binaries directly via
// vkCreateShaderModule, which would skip shaderc entirely; that path is
// a follow-up. Reporting GLSL here lets the future framework pick up
// hand-written GLSL native variants today; switching this return value
// to ShaderLanguageSPIRV later is a one-line change once SPIR-V
// variants land.
//
// Implements backend.NativeShaderDevice.
func (d *Device) PreferredShaderLanguage() backend.ShaderLanguage {
	return backend.ShaderLanguageGLSL
}

// NewShaderNative compiles a GLSL shader pair without going through the
// Kage → GLSL translation that the framework's NewShader([]byte) entry
// performs upstream. Vulkan's existing Device.NewShader already accepts
// GLSL 330 source via ShaderDescriptor.VertexSource/FragmentSource and
// runs shaderc to produce SPIR-V at compile() time; the native path is
// a thin wrapper that validates desc.Language and dispatches to NewShader.
//
// The desc.Uniforms slice is unused: Vulkan's per-draw packer extracts
// the std140 layout from the GLSL itself (via shadertranslate.ExtractUniformLayout
// in shader_gpu.go), so the author's layout declaration is redundant for
// this backend. We accept it for API symmetry with the WGSL/MSL paths so
// callers in the future framework use the same registration shape across
// backends.
//
// Implements backend.NativeShaderDevice.
func (d *Device) NewShaderNative(desc backend.NativeShaderDescriptor) (backend.Shader, error) {
	if desc.Language != backend.ShaderLanguageGLSL {
		return nil, fmt.Errorf("%w: vulkan accepts %s, got %s",
			backend.ErrUnsupportedShaderLanguage,
			backend.ShaderLanguageGLSL, desc.Language)
	}
	return d.NewShader(backend.ShaderDescriptor{
		VertexSource:   string(desc.Vertex),
		FragmentSource: string(desc.Fragment),
		Attributes:     desc.Attributes,
	})
}
