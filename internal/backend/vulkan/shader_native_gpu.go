//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"fmt"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/shadertranslate"
)

// PreferredShaderLanguage reports the Vulkan backend's preferred native
// source language. SPIR-V is preferred — it is the native format
// vkCreateShaderModule consumes, with no runtime compilation step. GLSL
// is also accepted via NewShaderNative; the framework picks SPIR-V
// when a SPIR-V variant is registered for the active shader and falls
// back to GLSL (compiled via shaderc) otherwise.
//
// On Android shaderc is not available — the AAR doesn't ship
// libshaderc.so and the system has no system copy. Callers MUST register
// a SPIR-V variant for every shader they hit on Android, or the GLSL
// fallback will fail with a "shaderc: failed to load libshaderc"
// pipeline-creation error and the engine renders a black frame.
//
// Implements backend.NativeShaderDevice.
func (d *Device) PreferredShaderLanguage() backend.ShaderLanguage {
	return backend.ShaderLanguageSPIRV
}

// NewShaderNative compiles a shader pair in either GLSL or SPIR-V form
// without going through the Kage → GLSL translation that the framework's
// NewShader([]byte) entry performs upstream.
//
//   - GLSL: dispatches to Device.NewShader, which runs shaderc internally.
//     desc.Uniforms is unused — the Vulkan compile path extracts the
//     std140 layout from the GLSL via shadertranslate.ExtractUniformLayout.
//   - SPIR-V: bytes are stored on the Shader and fed straight to
//     vkCreateShaderModule at compile() time. Skips shaderc entirely —
//     this is the path that makes Vulkan run on Android. desc.Uniforms
//     IS required: there is no GLSL to derive the layout from, so the
//     caller declares the std140 byte offsets explicitly.
//
// Implements backend.NativeShaderDevice.
func (d *Device) NewShaderNative(desc backend.NativeShaderDescriptor) (backend.Shader, error) {
	switch desc.Language {
	case backend.ShaderLanguageGLSL:
		return d.NewShader(backend.ShaderDescriptor{
			VertexSource:   string(desc.Vertex),
			FragmentSource: string(desc.Fragment),
			Attributes:     desc.Attributes,
		})
	case backend.ShaderLanguageSPIRV:
		layout := nativeUniformsToLayout(desc.Uniforms)
		return &Shader{
			dev:                   d,
			vertexSPIRV:           desc.Vertex,
			fragmentSPIRV:         desc.Fragment,
			attributes:            desc.Attributes,
			uniforms:              make(map[string]interface{}),
			nativeMode:            true,
			vertexUniformLayout:   layout,
			fragmentUniformLayout: layout,
		}, nil
	default:
		return nil, fmt.Errorf("%w: vulkan accepts %s or %s, got %s",
			backend.ErrUnsupportedShaderLanguage,
			backend.ShaderLanguageGLSL, backend.ShaderLanguageSPIRV, desc.Language)
	}
}

// nativeUniformsToLayout converts the public NativeUniformField slice
// into the internal shadertranslate.UniformField slice that Shader's
// per-draw packer consumes. Type is left empty because the packer reads
// only Offset+Size for byte-level packing.
func nativeUniformsToLayout(in []backend.NativeUniformField) []shadertranslate.UniformField {
	if len(in) == 0 {
		return nil
	}
	out := make([]shadertranslate.UniformField, len(in))
	for i, f := range in {
		out[i] = shadertranslate.UniformField{
			Name:   f.Name,
			Offset: f.Offset,
			Size:   f.Size,
		}
	}
	return out
}
