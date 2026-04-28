//go:build (darwin || linux || freebsd || windows) && !soft

package webgpu

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
)

// TestWebGPUPreferredShaderLanguage confirms the native-shader entry
// reports WGSL — used by callers in the future framework to decide
// which native variant of a multi-language shader to feed in.
func TestWebGPUPreferredShaderLanguage(t *testing.T) {
	dev := New()
	// PreferredShaderLanguage is a pure accessor — no Init needed.
	require.Equal(t, backend.ShaderLanguageWGSL, dev.PreferredShaderLanguage())
}

// TestWebGPUNewShaderNativeRejectsMismatch verifies that asking the
// WebGPU backend to compile a non-WGSL native source returns
// ErrUnsupportedShaderLanguage. This is the runtime safety net that
// catches build-tag matrix bugs that the future-side compat package
// missed.
func TestWebGPUNewShaderNativeRejectsMismatch(t *testing.T) {
	dev, _ := initGPUDevice(t)
	_, err := dev.NewShaderNative(backend.NativeShaderDescriptor{
		Language: backend.ShaderLanguageMSL,
		Vertex:   []byte("// not wgsl"),
		Fragment: []byte("// not wgsl"),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, backend.ErrUnsupportedShaderLanguage),
		"got %v, want wraps ErrUnsupportedShaderLanguage", err)
}

// TestWebGPUNewShaderNativeWGSL confirms the native WGSL path actually
// reaches wgpu.DeviceCreateShaderModuleWGSL — both shader modules end
// up populated and the GLSL→WGSL translator never runs (vertexUniformLayout
// stays nil because compile() short-circuits in nativeMode).
//
// The WGSL itself is the minimal viable pair: pos+uv+color vertex
// inputs at the engine's standard locations, a single uProjection
// uniform mat4, and a passthrough fragment that returns the
// interpolated vertex color modulated by a sampled texture.
func TestWebGPUNewShaderNativeWGSL(t *testing.T) {
	dev, _ := initGPUDevice(t)

	const vertWGSL = `
struct Uniforms {
    uProjection: mat4x4<f32>,
};
@group(0) @binding(0) var<uniform> u: Uniforms;

struct VertexInput {
    @location(0) pos: vec2<f32>,
    @location(1) uv: vec2<f32>,
    @location(2) color: vec4<f32>,
};
struct VertexOutput {
    @builtin(position) position: vec4<f32>,
    @location(0) uv: vec2<f32>,
    @location(1) color: vec4<f32>,
};

@vertex
fn main(in: VertexInput) -> VertexOutput {
    var out: VertexOutput;
    out.position = u.uProjection * vec4<f32>(in.pos, 0.0, 1.0);
    out.uv = in.uv;
    out.color = in.color;
    return out;
}
`
	const fragWGSL = `
@group(1) @binding(0) var tex: texture_2d<f32>;
@group(1) @binding(1) var samp: sampler;

@fragment
fn main(@location(0) uv: vec2<f32>, @location(1) color: vec4<f32>) -> @location(0) vec4<f32> {
    return textureSample(tex, samp, uv) * color;
}
`

	sh, err := dev.NewShaderNative(backend.NativeShaderDescriptor{
		Language: backend.ShaderLanguageWGSL,
		Vertex:   []byte(vertWGSL),
		Fragment: []byte(fragWGSL),
		Uniforms: []backend.NativeUniformField{
			{Name: "uProjection", Offset: 0, Size: 64},
		},
		Attributes: batch.Vertex2DFormat().Attributes,
	})
	require.NoError(t, err)
	defer sh.Dispose()

	s := sh.(*Shader)
	require.True(t, s.nativeMode, "nativeMode should be set so compile() skips translation")
	require.Len(t, s.combinedUniformLayout, 1, "uniform layout must come from the descriptor, not the translator")
	require.Equal(t, "uProjection", s.combinedUniformLayout[0].Name)

	s.compile()
	require.NotZero(t, s.vertexModule, "vertex shader module should be created via DeviceCreateShaderModuleWGSL")
	require.NotZero(t, s.fragmentModule, "fragment shader module should be created via DeviceCreateShaderModuleWGSL")

	// Native path must NOT populate the per-stage uniform layouts —
	// those are GLSL-translator artifacts, not part of the native flow.
	require.Nil(t, s.vertexUniformLayout, "native mode should not run GLSLToWGSLVertex")
	require.Nil(t, s.fragmentUniformLayout, "native mode should not run GLSLToWGSLFragment")
}
