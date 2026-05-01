//go:build darwin && !soft

package metal

import (
	"fmt"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/shadertranslate"
)

// PreferredShaderLanguage reports MSL — the language Metal's
// MTLDevice.newLibraryWithSource consumes directly. Implements
// backend.NativeShaderDevice.
func (d *Device) PreferredShaderLanguage() backend.ShaderLanguage {
	return backend.ShaderLanguageMSL
}

// NewShaderNative compiles an MSL shader pair without going through
// the Kage → GLSL → MSL translation that Device.NewShader uses. The
// source bytes are stored as the shader's MSL source and handed
// straight to mtl.DeviceNewLibraryWithSource on first use.
//
// desc.Uniforms must declare the layout the MSL uniform struct uses.
// The Metal backend's per-draw packer uses byte offsets to write
// SetUniform* values into the buffer; a layout mismatch silently
// corrupts uniform values without any compile-time signal. Both
// vertex and fragment stages use the same layout so the packer
// produces a single buffer that satisfies both stages — authors
// declaring per-stage MSL structs should write the SAME struct
// (matching the union of vertex+fragment uniforms) in both files.
//
// Implements backend.NativeShaderDevice.
func (d *Device) NewShaderNative(desc backend.NativeShaderDescriptor) (backend.Shader, error) {
	if desc.Language != backend.ShaderLanguageMSL {
		return nil, fmt.Errorf("%w: metal accepts %s, got %s",
			backend.ErrUnsupportedShaderLanguage,
			backend.ShaderLanguageMSL, desc.Language)
	}
	layout := nativeUniformsToLayout(desc.Uniforms)
	return &Shader{
		dev:                   d,
		vertexSource:          string(desc.Vertex),
		fragmentSource:        string(desc.Fragment),
		attributes:            desc.Attributes,
		uniforms:              make(map[string]any),
		vertexUniformLayout:   layout,
		fragmentUniformLayout: layout,
		nativeMode:            true,
	}, nil
}

// nativeUniformsToLayout converts the public NativeUniformField slice
// into the internal shadertranslate.UniformField slice that Shader's
// packUniformBuffer consumes. Type is left empty because
// packUniformBuffer doesn't read it; only Offset+Size matter for
// byte-level packing.
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
