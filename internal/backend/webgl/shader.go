package webgl

import "github.com/michaelraines/future-render/internal/backend"

// Shader implements backend.Shader for WebGL2.
// Wraps a soft.Shader and stores the GLSL ES 3.00 source that a real
// WebGL2 implementation would compile via gl.createProgram().
type Shader struct {
	inner        backend.Shader
	vertexSource string // GLSL ES 3.00 vertex shader source
	fragSource   string // GLSL ES 3.00 fragment shader source
}

// SetUniformFloat sets a float uniform.
func (s *Shader) SetUniformFloat(name string, v float32) {
	s.inner.SetUniformFloat(name, v)
}

// SetUniformVec2 sets a vec2 uniform.
func (s *Shader) SetUniformVec2(name string, v [2]float32) {
	s.inner.SetUniformVec2(name, v)
}

// SetUniformVec4 sets a vec4 uniform.
func (s *Shader) SetUniformVec4(name string, v [4]float32) {
	s.inner.SetUniformVec4(name, v)
}

// SetUniformMat4 sets a mat4 uniform.
func (s *Shader) SetUniformMat4(name string, v [16]float32) {
	s.inner.SetUniformMat4(name, v)
}

// SetUniformInt sets an int uniform.
func (s *Shader) SetUniformInt(name string, v int32) {
	s.inner.SetUniformInt(name, v)
}

// SetUniformBlock sets a uniform block's data.
func (s *Shader) SetUniformBlock(name string, data []byte) {
	s.inner.SetUniformBlock(name, data)
}

// Dispose releases the shader.
func (s *Shader) Dispose() {
	s.inner.Dispose()
}
