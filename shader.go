package futurerender

import (
	"fmt"
	"sync/atomic"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
	"github.com/michaelraines/future-core/internal/shaderir"
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
