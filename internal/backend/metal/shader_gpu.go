//go:build metal

package metal

import "github.com/michaelraines/future-render/internal/backend"

// Shader implements backend.Shader for Metal.
// Stores MSL source for later compilation into MTLLibrary + MTLFunction.
type Shader struct {
	dev            *Device
	vertexSource   string
	fragmentSource string
	attributes     []backend.VertexAttribute
	uniforms       map[string]interface{}
}

// SetUniformFloat sets a float uniform.
func (s *Shader) SetUniformFloat(name string, v float32) { s.uniforms[name] = v }

// SetUniformVec2 sets a vec2 uniform.
func (s *Shader) SetUniformVec2(name string, v [2]float32) { s.uniforms[name] = v }

// SetUniformVec4 sets a vec4 uniform.
func (s *Shader) SetUniformVec4(name string, v [4]float32) { s.uniforms[name] = v }

// SetUniformMat4 sets a mat4 uniform.
func (s *Shader) SetUniformMat4(name string, v [16]float32) { s.uniforms[name] = v }

// SetUniformInt sets an int uniform.
func (s *Shader) SetUniformInt(name string, v int32) { s.uniforms[name] = v }

// SetUniformBlock sets a uniform block.
func (s *Shader) SetUniformBlock(name string, data []byte) { s.uniforms[name] = data }

// Dispose releases shader resources.
func (s *Shader) Dispose() {
	s.uniforms = nil
}
