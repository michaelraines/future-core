//go:build (darwin || linux || freebsd || windows) && !soft

package webgpu

import (
	"fmt"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/shadertranslate"
)

// PreferredShaderLanguage reports WGSL — the language WebGPU's
// CreateShaderModule API consumes directly. Implements
// backend.NativeShaderDevice; callers that detect the interface use
// this to pick which native shader variant from a multi-language
// source map to feed to NewShaderNative.
func (d *Device) PreferredShaderLanguage() backend.ShaderLanguage {
	return backend.ShaderLanguageWGSL
}

// NewShaderNative compiles a WGSL shader pair without going through
// the Kage → GLSL → WGSL translation that Device.NewShader uses.
// The bytes in desc.Vertex/Fragment are stored as the shader's WGSL
// source and handed straight to wgpu.DeviceCreateShaderModuleWGSL on
// first use (compilation is deferred to compile() the same way the
// Kage path defers, so shaders can be created before Device.Init has
// completed).
//
// desc.Uniforms must declare the layout the WGSL uniform struct uses
// — the framework copies it into the shader's combinedUniformLayout
// so the existing per-draw uniform-packing path works with native
// variants too. Mismatches between this layout and the actual WGSL
// struct silently corrupt uniform values; authors should derive the
// layout from a known-good GLSL form via shadertranslate.ExtractUniformLayout
// (the same code the Kage path uses), then commit it next to the
// hand-written .wgsl files.
//
// Implements backend.NativeShaderDevice.
func (d *Device) NewShaderNative(desc backend.NativeShaderDescriptor) (backend.Shader, error) {
	if desc.Language != backend.ShaderLanguageWGSL {
		return nil, fmt.Errorf("%w: webgpu accepts %s, got %s",
			backend.ErrUnsupportedShaderLanguage,
			backend.ShaderLanguageWGSL, desc.Language)
	}
	return &Shader{
		dev:                   d,
		vertexSource:          string(desc.Vertex),
		fragmentSource:        string(desc.Fragment),
		attributes:            desc.Attributes,
		uniforms:              make(map[string]any),
		combinedUniformLayout: nativeUniformsToLayout(desc.Uniforms),
		nativeMode:            true,
	}, nil
}

// nativeUniformsToLayout converts the public NativeUniformField slice
// into the internal shadertranslate.UniformField slice that Shader's
// packUniforms consumes. Type is left empty because packUniforms
// doesn't read it; only Offset+Size matter for byte-level packing.
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
