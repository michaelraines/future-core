//go:build windows && !soft

package dx12

import (
	"fmt"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/d3d12"
)

// HLSL entry-point + shader-model defaults for hand-written HLSL
// variants. The future-side native shaders all use these names; if a
// caller wants different conventions they can pre-compile via
// d3d12.D3DCompile and stash bytecode another way.
const (
	hlslVertexEntry  = "VSMain"
	hlslPixelEntry   = "PSMain"
	hlslVertexTarget = "vs_5_0"
	hlslPixelTarget  = "ps_5_0"
)

// compileNativeHLSL compiles a NativeShaderDevice-produced Shader's
// vertex and pixel HLSL sources to DXBC bytecode via D3DCompile. The
// returned slices are independent allocations safe to outlive the
// Shader (D3DCompile copies the bytecode out of the ID3DBlob before
// releasing it).
//
// Errors are wrapped with the stage name so a failure in either pass
// is unambiguous. The returned diagnostic includes the compiler's
// own message text — D3DCompile populates a separate error blob that
// the d3d12 wrapper reads + releases.
func compileNativeHLSL(sh *Shader) (vertex, pixel []byte, err error) {
	vertex, err = d3d12.D3DCompile(
		[]byte(sh.vertexSource),
		"vertex.hlsl",
		hlslVertexEntry, hlslVertexTarget, 0,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("dx12: vertex shader compile: %w", err)
	}
	pixel, err = d3d12.D3DCompile(
		[]byte(sh.fragmentSource),
		"pixel.hlsl",
		hlslPixelEntry, hlslPixelTarget, 0,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("dx12: pixel shader compile: %w", err)
	}
	return vertex, pixel, nil
}

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
