//go:build darwin && !soft

package metal

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
)

// TestMetalPreferredShaderLanguage confirms the native-shader entry
// reports MSL — what callers in the future framework type-assert and
// then use to pick which native variant of a multi-language shader
// to feed in.
func TestMetalPreferredShaderLanguage(t *testing.T) {
	dev := New()
	require.Equal(t, backend.ShaderLanguageMSL, dev.PreferredShaderLanguage())
}

// TestMetalNewShaderNativeRejectsMismatch verifies that asking the
// Metal backend to compile a non-MSL native source returns
// ErrUnsupportedShaderLanguage. Build-tag misconfigurations should be
// caught at compile time by the future-side compat package; this is
// the runtime safety net.
func TestMetalNewShaderNativeRejectsMismatch(t *testing.T) {
	dev, _ := initGPUDevice(t)
	_, err := dev.NewShaderNative(backend.NativeShaderDescriptor{
		Language: backend.ShaderLanguageWGSL,
		Vertex:   []byte("// not msl"),
		Fragment: []byte("// not msl"),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, backend.ErrUnsupportedShaderLanguage),
		"got %v, want wraps ErrUnsupportedShaderLanguage", err)
}

// TestMetalNewShaderNativeMSL confirms the native MSL path actually
// reaches mtl.DeviceNewLibraryWithSource — both shader libraries end
// up populated and the GLSL→MSL translator never runs (vertexUniformLayout
// stays unset because compile() short-circuits in nativeMode).
func TestMetalNewShaderNativeMSL(t *testing.T) {
	dev, _ := initGPUDevice(t)

	const vertMSL = `
#include <metal_stdlib>
using namespace metal;

struct VertexInput {
    float2 aPosition [[attribute(0)]];
    float2 aTexCoord [[attribute(1)]];
    float4 aColor    [[attribute(2)]];
};

struct VertexOutput {
    float4 position [[position]];
    float2 vTexCoord;
    float4 vColor;
};

struct Uniforms {
    float4x4 uProjection;
};

vertex VertexOutput vertexMain(VertexInput in [[stage_in]],
                                constant Uniforms& u [[buffer(1)]]) {
    VertexOutput out;
    out.position = u.uProjection * float4(in.aPosition, 0.0, 1.0);
    out.vTexCoord = in.aTexCoord;
    out.vColor = in.aColor;
    return out;
}
`
	const fragMSL = `
#include <metal_stdlib>
using namespace metal;

struct FragmentInput {
    float2 vTexCoord;
    float4 vColor;
};

fragment float4 fragmentMain(FragmentInput in [[stage_in]]) {
    return in.vColor;
}
`

	sh, err := dev.NewShaderNative(backend.NativeShaderDescriptor{
		Language: backend.ShaderLanguageMSL,
		Vertex:   []byte(vertMSL),
		Fragment: []byte(fragMSL),
		Uniforms: []backend.NativeUniformField{
			{Name: "uProjection", Offset: 0, Size: 64},
		},
		Attributes: batch.Vertex2DFormat().Attributes,
	})
	require.NoError(t, err)
	defer sh.Dispose()

	s := sh.(*Shader)
	require.True(t, s.nativeMode, "nativeMode should be set so compile() skips translation")
	require.Len(t, s.vertexUniformLayout, 1, "uniform layout must come from the descriptor, not the translator")
	require.Equal(t, "uProjection", s.vertexUniformLayout[0].Name)
	require.Equal(t, s.vertexUniformLayout, s.fragmentUniformLayout,
		"native path uses the same union layout for both stages")

	require.NoError(t, s.compile())
	require.NotZero(t, s.vertexLib, "vertex library should be created via DeviceNewLibraryWithSource")
	require.NotZero(t, s.fragmentLib, "fragment library should be created via DeviceNewLibraryWithSource")
	require.NotZero(t, s.vertexFn, "vertex function should be looked up from the library")
	require.NotZero(t, s.fragmentFn, "fragment function should be looked up from the library")
}
