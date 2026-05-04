//go:build windows && !soft

package dx12

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
)

// TestDX12PreferredShaderLanguage confirms the native-shader entry
// reports HLSL — what callers in the future framework type-assert and
// then use to pick which native variant of a multi-language shader to
// feed in.
func TestDX12PreferredShaderLanguage(t *testing.T) {
	dev := New()
	require.Equal(t, backend.ShaderLanguageHLSL, dev.PreferredShaderLanguage())
}

// TestDX12NewShaderNativeRejectsMismatch verifies that asking the DX12
// backend to compile a non-HLSL native source returns
// ErrUnsupportedShaderLanguage. Build-tag misconfigurations should be
// caught at compile time by the future-side compat package; this is
// the runtime safety net.
func TestDX12NewShaderNativeRejectsMismatch(t *testing.T) {
	dev := New()
	_, err := dev.NewShaderNative(backend.NativeShaderDescriptor{
		Language: backend.ShaderLanguageWGSL,
		Vertex:   []byte("// not hlsl"),
		Fragment: []byte("// not hlsl"),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, backend.ErrUnsupportedShaderLanguage),
		"got %v, want wraps ErrUnsupportedShaderLanguage", err)
}

// TestDX12NewShaderNativeHLSL confirms the native HLSL path stores the
// source bytes and uniform layout on the resulting Shader, with
// nativeMode=true so the (future) D3DCompile path skips Kage→GLSL→HLSL
// translation.
func TestDX12NewShaderNativeHLSL(t *testing.T) {
	dev := New()

	const vertHLSL = `
struct VSInput  { float2 aPosition : TEXCOORD0; };
struct VSOutput { float4 position : SV_POSITION; };

cbuffer UniformsCB : register(b0) {
    row_major float4x4 uProjection;
};

VSOutput VSMain(VSInput input) {
    VSOutput output;
    output.position = mul(uProjection, float4(input.aPosition, 0.0, 1.0));
    return output;
}
`
	const fragHLSL = `
struct PSInput { float4 position : SV_POSITION; };

float4 PSMain(PSInput input) : SV_TARGET {
    return float4(1.0, 1.0, 1.0, 1.0);
}
`

	sh, err := dev.NewShaderNative(backend.NativeShaderDescriptor{
		Language: backend.ShaderLanguageHLSL,
		Vertex:   []byte(vertHLSL),
		Fragment: []byte(fragHLSL),
		Uniforms: []backend.NativeUniformField{
			{Name: "uProjection", Offset: 0, Size: 64},
		},
		Attributes: batch.Vertex2DFormat().Attributes,
	})
	require.NoError(t, err)
	defer sh.Dispose()

	s := sh.(*Shader)
	require.True(t, s.nativeMode, "nativeMode should be set so compile skips translation")
	require.Equal(t, vertHLSL, s.vertexSource)
	require.Equal(t, fragHLSL, s.fragmentSource)
	require.Len(t, s.nativeUniforms, 1, "uniform layout must come from the descriptor")
	require.Equal(t, "uProjection", s.nativeUniforms[0].Name)
}
