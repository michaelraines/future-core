package futurerender

import (
	"fmt"
	"sync/atomic"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/batch"
	"github.com/michaelraines/future-render/internal/shaderir"
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

// applyFloatSliceUniform sets a uniform from a float32 slice, inferring the
// type from the slice length.
func applyFloatSliceUniform(sh backend.Shader, name string, v []float32) {
	switch len(v) {
	case 1:
		sh.SetUniformFloat(name, v[0])
	case 2:
		sh.SetUniformVec2(name, [2]float32{v[0], v[1]})
	case 4:
		sh.SetUniformVec4(name, [4]float32{v[0], v[1], v[2], v[3]})
	case 16:
		var m [16]float32
		copy(m[:], v)
		sh.SetUniformMat4(name, m)
	}
}
