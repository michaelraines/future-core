package backend

import "errors"

// ShaderLanguage identifies the source language of a shader pair.
//
// The default Device.NewShader path always assumes Kage source: the
// engine compiles Kage to GLSL 330 (via shaderir) and the active
// backend translates GLSL to its native form (WGSL for WebGPU, MSL
// for Metal, SPIR-V for Vulkan, ...). NativeShaderDevice lets a
// caller skip that pipeline by passing source already in the
// backend's native language.
type ShaderLanguage int

// ShaderLanguage constants.
const (
	// ShaderLanguageKage is the Kage language used by the built-in
	// translator; the default source language for Device.NewShader.
	ShaderLanguageKage ShaderLanguage = iota

	// ShaderLanguageGLSL is plain GLSL 330, the engine's intermediate
	// form between Kage and the per-backend translators.
	ShaderLanguageGLSL

	// ShaderLanguageGLSLES is GLSL ES 300 (WebGL2 native source).
	ShaderLanguageGLSLES

	// ShaderLanguageWGSL is WebGPU Shading Language (WebGPU native).
	ShaderLanguageWGSL

	// ShaderLanguageMSL is Metal Shading Language (Metal native).
	ShaderLanguageMSL

	// ShaderLanguageSPIRV is the SPIR-V binary format (Vulkan native).
	// Bytes are passed straight to vkCreateShaderModule without
	// going through shaderc.
	ShaderLanguageSPIRV

	// ShaderLanguageHLSL is High-Level Shading Language (DirectX 12 native).
	ShaderLanguageHLSL
)

// String returns a stable identifier for the language. Useful in error
// messages and tracing — e.g. when ErrUnsupportedShaderLanguage fires
// and the caller wants to report which language was rejected.
func (l ShaderLanguage) String() string {
	switch l {
	case ShaderLanguageKage:
		return "kage"
	case ShaderLanguageGLSL:
		return "glsl"
	case ShaderLanguageGLSLES:
		return "glsles"
	case ShaderLanguageWGSL:
		return "wgsl"
	case ShaderLanguageMSL:
		return "msl"
	case ShaderLanguageSPIRV:
		return "spirv"
	case ShaderLanguageHLSL:
		return "hlsl"
	}
	return "unknown"
}

// NativeUniformField describes one uniform's position in the
// combined-stage uniform struct of a NativeShaderDescriptor. Authors
// of native shaders must declare the layout explicitly because the
// framework cannot infer offsets from arbitrary native source — this
// is the data Device.NewShader's translator computes for itself when
// going through Kage→GLSL.
//
// std140 alignment rules (which every native target except SPIR-V
// must follow as well):
//
//	float, int                       → align 4,  size 4
//	vec2, ivec2                      → align 8,  size 8
//	vec3                             → align 16, size 12 (tail packs a scalar)
//	vec4, ivec4                      → align 16, size 16
//	mat4                             → align 16, size 64
//
// shadertranslate.ExtractUniformLayout in this repo computes the same
// layout from GLSL — running it on the Kage-translated GLSL of a
// shader yields a layout an author can copy into their native variant
// declaration.
type NativeUniformField struct {
	Name   string
	Offset int
	Size   int
}

// NativeShaderDescriptor describes a shader pair already in the active
// device's preferred source language. The Vertex and Fragment fields
// hold the source — UTF-8 text for WGSL/MSL/GLSL/HLSL, raw bytecode
// for SPIR-V. Uniforms declares the layout the shader expects so the
// framework can pack SetUniform* values into the GPU buffer at draw
// time; a layout mismatch silently corrupts uniform values without
// any compile-time signal.
type NativeShaderDescriptor struct {
	// Language must match the device's PreferredShaderLanguage.
	// Mismatches return ErrUnsupportedShaderLanguage.
	Language ShaderLanguage

	// Vertex and Fragment are the shader source for each stage.
	Vertex   []byte
	Fragment []byte

	// Uniforms declares the combined vertex+fragment uniform struct
	// layout. Order, names, offsets and sizes must match the native
	// shader's @group(0) @binding(0) uniform block (or the equivalent
	// in non-WGSL languages). The framework uses this for per-draw
	// uniform packing — see internal/backend/<name>/shader_*.go's
	// packUniforms for the consumer.
	Uniforms []NativeUniformField

	// Attributes declares the vertex attribute layout. For shaders
	// consuming the engine's standard Vertex2D stream, pass
	// batch.Vertex2DFormat().Attributes (pos@0, uv@8, color@16).
	Attributes []VertexAttribute
}

// NativeShaderDevice is the optional capability that lets a Device
// accept shader pairs already in its native source language without
// going through the Kage→GLSL→target translator pipeline.
//
// Detection by callers:
//
//	if nsd, ok := dev.(backend.NativeShaderDevice); ok {
//	    shader, err := nsd.NewShaderNative(backend.NativeShaderDescriptor{...})
//	}
//
// A device that does not implement NativeShaderDevice signals to the
// caller that it has no native-language shortcut: the caller must
// fall back to Device.NewShader with Kage-translated GLSL. This is
// the case for the soft rasterizer (no shader compilation at all)
// and any backend whose native path has not yet been wired up.
type NativeShaderDevice interface {
	// PreferredShaderLanguage reports the language this device accepts
	// natively. Callers use it to pick which entry from a multi-language
	// shader-source map to feed to NewShaderNative.
	PreferredShaderLanguage() ShaderLanguage

	// NewShaderNative compiles a shader pair in this device's preferred
	// language. Returns ErrUnsupportedShaderLanguage when desc.Language
	// doesn't match PreferredShaderLanguage.
	NewShaderNative(desc NativeShaderDescriptor) (Shader, error)
}

// ErrUnsupportedShaderLanguage is returned by NativeShaderDevice
// implementations when the descriptor's Language doesn't match the
// device's preferred native language. This case should be prevented
// at build time by the caller's compatibility-check matrix; a
// runtime occurrence usually indicates the build's tag combination
// is inconsistent.
var ErrUnsupportedShaderLanguage = errors.New("future-core: shader language not supported by this backend")
