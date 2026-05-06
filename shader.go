package futurerender

import (
	"fmt"
	"sync/atomic"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
	"github.com/michaelraines/future-core/internal/shaderir"
	"github.com/michaelraines/future-core/internal/shadertranslate"
)

// Shader represents a compiled shader program. Shaders can be created from
// Kage source (Ebitengine-compatible) via NewShader, or from raw GLSL via
// NewShaderFromGLSL.
type Shader struct {
	id       uint32
	backend  backend.Shader
	pipeline backend.Pipeline
	uniforms []shaderir.Uniform
	disposed bool
}

// ID returns the shader's unique identifier for batcher sorting.
func (s *Shader) ID() uint32 {
	return s.id
}

// nextShaderID generates unique shader IDs. ID 0 is reserved for the default
// sprite shader.
var nextShaderID atomic.Uint32

func init() {
	nextShaderID.Store(1) // Reserve 0 for default.
}

// NewShader compiles a Kage shader program and returns a Shader.
// This is the Ebitengine-compatible entry point.
func NewShader(src []byte) (*Shader, error) {
	result, err := shaderir.Compile(src)
	if err != nil {
		return nil, err
	}
	return newShaderFromGLSLInternal(
		[]byte(result.VertexShader),
		[]byte(result.FragmentShader),
		result.Uniforms,
	)
}

// NewShaderFromGLSL compiles a shader from raw GLSL vertex and fragment
// source. This is for users who prefer GLSL over Kage.
func NewShaderFromGLSL(vertSrc, fragSrc []byte) (*Shader, error) {
	return newShaderFromGLSLInternal(vertSrc, fragSrc, nil)
}

// ShaderLanguage is the public alias of backend.ShaderLanguage. It
// names the source language of a shader pair so the future framework
// (and other consumers outside the internal/ tree) can talk about
// language selection without importing the internal backend package.
type ShaderLanguage = backend.ShaderLanguage

// ShaderLanguage constants re-exported for use outside future-core's
// internal tree. The values track backend.ShaderLanguage* exactly —
// they're aliases, not copies — so a backend.ShaderLanguage and a
// futurerender.ShaderLanguage are the same type.
const (
	ShaderLanguageKage   = backend.ShaderLanguageKage
	ShaderLanguageGLSL   = backend.ShaderLanguageGLSL
	ShaderLanguageGLSLES = backend.ShaderLanguageGLSLES
	ShaderLanguageWGSL   = backend.ShaderLanguageWGSL
	ShaderLanguageMSL    = backend.ShaderLanguageMSL
	ShaderLanguageSPIRV  = backend.ShaderLanguageSPIRV
	ShaderLanguageHLSL   = backend.ShaderLanguageHLSL
)

// NativeUniformField is the public alias of backend.NativeUniformField.
// Authors of native shader variants declare the std140 uniform layout
// using this type when calling NewShaderNative — see backend's
// NativeUniformField doc comment for the alignment rules.
type NativeUniformField = backend.NativeUniformField

// PreferredShaderLanguage reports the active rendering device's preferred
// native shader source language. Callers use it to pick which native
// variant of a multi-language shader to feed to NewShaderNative.
//
// Returns backend.ShaderLanguageKage when no rendering device is
// active, when the device does not implement backend.NativeShaderDevice
// (i.e. has no native-shader path — soft, or backends not yet wired
// up), or when the active backend prefers Kage. Callers should treat
// "Kage" as "no native variant; use the universal Kage fallback."
func PreferredShaderLanguage() backend.ShaderLanguage {
	rend := getRenderer()
	if rend == nil || rend.device == nil {
		return backend.ShaderLanguageKage
	}
	if nsd, ok := rend.device.(backend.NativeShaderDevice); ok {
		return nsd.PreferredShaderLanguage()
	}
	return backend.ShaderLanguageKage
}

// NewShaderNative compiles a shader pair already in the active device's
// preferred native source language, skipping the Kage → GLSL → target
// translation pipeline that NewShader and NewShaderFromGLSL go through.
//
// Use this when the active backend supports a native source path and a
// hand-written native variant exists for the shader (e.g. WGSL for
// WebGPU, MSL for Metal, SPIR-V for Vulkan). Cross-backend authors
// typically pair this with the matching ShaderSource registry on the
// future side, where build tags select which native variant gets
// embedded for the active backend.
//
// uniforms declares the combined vertex+fragment uniform struct
// layout — see backend.NativeUniformField for the std140 packing rules
// and how to derive a layout from a known-good GLSL form via
// shadertranslate.ExtractUniformLayout.
//
// Returns an error if no rendering device is available, if the active
// device does not implement backend.NativeShaderDevice (e.g. soft
// rasterizer), or if the descriptor's Language doesn't match the
// device's preferred native language. Builds that ship native variants
// should layer a compile-time compatibility check on top of this so
// the runtime case never fires.
func NewShaderNative(lang backend.ShaderLanguage, vertSrc, fragSrc []byte, uniforms []backend.NativeUniformField) (*Shader, error) {
	rend := getRenderer()
	if rend == nil || rend.device == nil {
		return nil, fmt.Errorf("shader: no rendering device available")
	}

	nsd, ok := rend.device.(backend.NativeShaderDevice)
	if !ok {
		return nil, fmt.Errorf("shader: active backend does not support native shader sources")
	}

	desc := backend.NativeShaderDescriptor{
		Language:   lang,
		Vertex:     vertSrc,
		Fragment:   fragSrc,
		Uniforms:   uniforms,
		Attributes: batch.Vertex2DFormat().Attributes,
	}
	sh, err := nsd.NewShaderNative(desc)
	if err != nil {
		return nil, fmt.Errorf("shader: native compile: %w", err)
	}

	pip, err := rend.device.NewPipeline(backend.PipelineDescriptor{
		Shader:       sh,
		VertexFormat: batch.Vertex2DFormat(),
		BlendMode:    backend.BlendSourceOver,
		DepthTest:    false,
		DepthWrite:   false,
		CullMode:     backend.CullNone,
		Primitive:    backend.PrimitiveTriangles,
	})
	if err != nil {
		sh.Dispose()
		return nil, fmt.Errorf("shader: create pipeline: %w", err)
	}

	id := nextShaderID.Add(1)
	s := &Shader{
		id:       id,
		backend:  sh,
		pipeline: pip,
	}

	if rend.registerShader != nil {
		rend.registerShader(id, s)
	}

	// Native shaders skip context-loss tracking: the recovery path
	// would need to re-translate Kage→GLSL→target, which doesn't
	// apply here. Authors using native variants opt into the
	// less-resilient lifecycle in exchange for skipping translation.

	return s, nil
}

// DeriveKageUniformLayout returns the std140-aligned uniform layout
// derived from a Kage shader source. Pure-Go pipeline:
//
//	Kage → GLSL via shaderir
//	GLSL → uniform field offsets via shadertranslate.ExtractUniformLayout
//
// Runs anywhere — including platforms without shaderc (Android Vulkan).
// Useful when pairing an externally-compiled SPIR-V binary with a
// runtime-derived layout, e.g. via NewShaderFromKageAndSPIRV.
//
// The returned []NativeUniformField has Offset/Size matching what
// shaderc emits when compiling the GLSL form against std140 — so the
// engine's per-draw uniform packer fills the same byte ranges the
// SPIR-V's gl_DefaultUniformBlock (or explicit-UBO) members read from.
func DeriveKageUniformLayout(kage []byte) ([]NativeUniformField, error) {
	result, err := shaderir.Compile(kage)
	if err != nil {
		return nil, fmt.Errorf("kage layout: shaderir compile: %w", err)
	}
	combined := result.VertexShader + "\n" + result.FragmentShader
	layout, err := shadertranslate.ExtractUniformLayout(combined)
	if err != nil {
		return nil, fmt.Errorf("kage layout: extract: %w", err)
	}
	out := make([]NativeUniformField, 0, len(layout))
	for _, f := range layout {
		out = append(out, NativeUniformField{
			Name:   f.Name,
			Offset: f.Offset,
			Size:   f.Size,
		})
	}
	return out, nil
}

// NewShaderFromKageAndSPIRV builds a *Shader from Kage source paired
// with externally-compiled SPIR-V binaries. Skips the runtime
// shaderc.CompileGLSL call entirely — the SPIR-V is supplied; the
// uniform layout is derived from the Kage source via
// DeriveKageUniformLayout.
//
// Required on Android Vulkan, where libshaderc is not bundled in the
// AAR. Beneficial on every other Vulkan host too: skipping shaderc
// at startup eliminates a one-time link cost and removes a runtime
// dependency on whatever shaderc version the host happens to have
// installed.
//
// Build-time companion: cmd/precompile-kage-spirv walks a directory
// of .kage shader sources and emits .vert.spv + .frag.spv siblings.
// libs/shaders.LoadFromFile in the future framework prefers those
// blobs when present; otherwise it falls through to NewShader (which
// runs shaderc).
func NewShaderFromKageAndSPIRV(kage, vertexSPIRV, fragmentSPIRV []byte) (*Shader, error) {
	if len(vertexSPIRV) == 0 || len(fragmentSPIRV) == 0 {
		return nil, fmt.Errorf("shader: precompiled SPIR-V required for both stages")
	}
	uniforms, err := DeriveKageUniformLayout(kage)
	if err != nil {
		return nil, err
	}
	return NewShaderNative(backend.ShaderLanguageSPIRV, vertexSPIRV, fragmentSPIRV, uniforms)
}

func newShaderFromGLSLInternal(vertSrc, fragSrc []byte, uniforms []shaderir.Uniform) (*Shader, error) {
	rend := getRenderer()
	if rend == nil || rend.device == nil {
		return nil, fmt.Errorf("shader: no rendering device available")
	}

	sh, err := rend.device.NewShader(backend.ShaderDescriptor{
		VertexSource:   string(vertSrc),
		FragmentSource: string(fragSrc),
		Attributes:     batch.Vertex2DFormat().Attributes,
	})
	if err != nil {
		return nil, fmt.Errorf("shader: compile: %w", err)
	}

	pip, err := rend.device.NewPipeline(backend.PipelineDescriptor{
		Shader:       sh,
		VertexFormat: batch.Vertex2DFormat(),
		BlendMode:    backend.BlendSourceOver,
		DepthTest:    false,
		DepthWrite:   false,
		CullMode:     backend.CullNone,
		Primitive:    backend.PrimitiveTriangles,
	})
	if err != nil {
		sh.Dispose()
		return nil, fmt.Errorf("shader: create pipeline: %w", err)
	}

	id := nextShaderID.Add(1)
	s := &Shader{
		id:       id,
		backend:  sh,
		pipeline: pip,
		uniforms: uniforms,
	}

	// Register in renderer for SpritePass lookup.
	if rend.registerShader != nil {
		rend.registerShader(id, s)
	}

	// Track for context loss recovery.
	if tracker := getTracker(); tracker != nil {
		tracker.TrackShader(s, string(vertSrc), string(fragSrc), uniforms)
	}

	return s, nil
}

// Deallocate releases the shader's GPU resources.
func (s *Shader) Deallocate() {
	if s.disposed {
		return
	}
	s.disposed = true

	// Untrack from context loss recovery.
	if tracker := getTracker(); tracker != nil {
		tracker.UntrackShader(s)
	}

	if s.pipeline != nil {
		s.pipeline.Dispose()
	}
	if s.backend != nil {
		s.backend.Dispose()
	}
}

// SetUniformFloat sets a float uniform on this shader.
func (s *Shader) SetUniformFloat(name string, v float32) {
	if s.backend != nil {
		s.backend.SetUniformFloat(name, v)
	}
}

// SetUniformVec2 sets a vec2 uniform.
func (s *Shader) SetUniformVec2(name string, v [2]float32) {
	if s.backend != nil {
		s.backend.SetUniformVec2(name, v)
	}
}

// SetUniformVec4 sets a vec4 uniform.
func (s *Shader) SetUniformVec4(name string, v [4]float32) {
	if s.backend != nil {
		s.backend.SetUniformVec4(name, v)
	}
}

// SetUniformMat4 sets a mat4 uniform.
func (s *Shader) SetUniformMat4(name string, v [16]float32) {
	if s.backend != nil {
		s.backend.SetUniformMat4(name, v)
	}
}

// applyUniforms applies uniforms from a map[string]any (Ebitengine-compatible).
func (s *Shader) applyUniforms(uniforms map[string]any) {
	if s.backend == nil || uniforms == nil {
		return
	}
	for name, val := range uniforms {
		switch v := val.(type) {
		case float32:
			s.backend.SetUniformFloat(name, v)
		case float64:
			s.backend.SetUniformFloat(name, float32(v))
		case int:
			s.backend.SetUniformInt(name, int32(v))
		case int32:
			s.backend.SetUniformInt(name, v)
		case []float32:
			applyFloatSliceUniform(s.backend, name, v)
		}
	}
}

// applyUniformValue sets a single uniform value on a backend shader,
// handling type dispatch. Exported for use by the sprite pass's
// ApplyUniforms callback.
func applyUniformValue(sh backend.Shader, name string, val any) {
	switch v := val.(type) {
	case float32:
		sh.SetUniformFloat(name, v)
	case float64:
		sh.SetUniformFloat(name, float32(v))
	case int:
		sh.SetUniformInt(name, int32(v))
	case int32:
		sh.SetUniformInt(name, v)
	case []float32:
		applyFloatSliceUniform(sh, name, v)
	}
}

// applyFloatSliceUniform sets a uniform from a float32 slice, inferring the
// type from the slice length.
func applyFloatSliceUniform(sh backend.Shader, name string, v []float32) {
	switch len(v) {
	case 1:
		sh.SetUniformFloat(name, v[0])
	case 2:
		sh.SetUniformVec2(name, [2]float32{v[0], v[1]})
	case 3:
		// vec3<f32> has SizeOf=12 and AlignOf=16 in WGSL. The 16-byte
		// alignment is handled by the struct-layout logic (pads offsets
		// before the vec3) — the value itself must occupy exactly 12
		// bytes, because when a scalar follows the vec3 it packs at
		// offset+12, NOT offset+16. Writing 16 bytes here would clobber
		// the following field (e.g. overwrite `Intensity` with 0 and
		// make every light invisible in the lighting demo).
		sh.SetUniformVec3(name, [3]float32{v[0], v[1], v[2]})
	case 4:
		sh.SetUniformVec4(name, [4]float32{v[0], v[1], v[2], v[3]})
	case 16:
		var m [16]float32
		copy(m[:], v)
		sh.SetUniformMat4(name, m)
	}
}
