//go:build windows && !soft

package dx12

import (
	"fmt"

	"github.com/michaelraines/future-core/internal/backend"
)

// PreferredShaderLanguage reports HLSL — the language D3D12's
// D3DCompile consumes directly. Implements backend.NativeShaderDevice.
func (d *Device) PreferredShaderLanguage() backend.ShaderLanguage {
	return backend.ShaderLanguageHLSL
}

// NewShaderNative accepts an HLSL shader pair without going through
// the Kage → GLSL → HLSL translation that Device.NewShader would use
// once that translator lands. The source bytes are stored as the
// shader's HLSL source and handed to D3DCompile when the pipeline
// state object is built.
//
// desc.Uniforms must declare the layout the HLSL cbuffer uses. The
// per-draw packer writes SetUniform* values into the constant buffer
// at the declared byte offsets — a layout mismatch silently corrupts
// uniform values without any compile-time signal. Both vertex and
// pixel stages share the same layout so the packer produces a single
// buffer that satisfies both stages.
//
// Implements backend.NativeShaderDevice.
func (d *Device) NewShaderNative(desc backend.NativeShaderDescriptor) (backend.Shader, error) {
	if desc.Language != backend.ShaderLanguageHLSL {
		return nil, fmt.Errorf("%w: dx12 accepts %s, got %s",
			backend.ErrUnsupportedShaderLanguage,
			backend.ShaderLanguageHLSL, desc.Language)
	}
	return &Shader{
		dev:            d,
		vertexSource:   string(desc.Vertex),
		fragmentSource: string(desc.Fragment),
		attributes:     desc.Attributes,
		uniforms:       make(map[string]interface{}),
		nativeMode:     true,
		nativeUniforms: desc.Uniforms,
	}, nil
}
