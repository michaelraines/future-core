//go:build js

package webgl

import (
	"syscall/js"

	"github.com/michaelraines/future-core/internal/backend"
)

// Shader implements backend.Shader for WebGL2.
// Stores GLSL ES 3.00 source for compilation when a pipeline is created.
type Shader struct {
	gl             js.Value
	vertexSource   string
	fragmentSource string
	attributes     []backend.VertexAttribute
	uniforms       map[string]interface{}

	// Compiled GL program (lazily created).
	program  js.Value
	compiled bool
}

// compile compiles and links the vertex/fragment shaders into a GL program.
func (s *Shader) compile() bool {
	if s.compiled {
		return !s.program.IsNull() && !s.program.IsUndefined()
	}
	s.compiled = true

	if s.vertexSource == "" || s.fragmentSource == "" {
		return false
	}

	vertShader := s.compileShader(s.gl.Get("VERTEX_SHADER").Int(), s.vertexSource)
	if vertShader.IsNull() || vertShader.IsUndefined() {
		return false
	}

	fragShader := s.compileShader(s.gl.Get("FRAGMENT_SHADER").Int(), s.fragmentSource)
	if fragShader.IsNull() || fragShader.IsUndefined() {
		s.gl.Call("deleteShader", vertShader)
		return false
	}

	prog := s.gl.Call("createProgram")
	s.gl.Call("attachShader", prog, vertShader)
	s.gl.Call("attachShader", prog, fragShader)
	s.gl.Call("linkProgram", prog)

	// Shaders can be detached after linking.
	s.gl.Call("detachShader", prog, vertShader)
	s.gl.Call("detachShader", prog, fragShader)
	s.gl.Call("deleteShader", vertShader)
	s.gl.Call("deleteShader", fragShader)

	linkStatus := s.gl.Call("getProgramParameter", prog,
		s.gl.Get("LINK_STATUS").Int())
	if !linkStatus.Bool() {
		s.gl.Call("deleteProgram", prog)
		return false
	}

	s.program = prog
	return true
}

// compileShader compiles a single shader stage.
func (s *Shader) compileShader(shaderType int, source string) js.Value {
	shader := s.gl.Call("createShader", shaderType)
	s.gl.Call("shaderSource", shader, source)
	s.gl.Call("compileShader", shader)

	compileStatus := s.gl.Call("getShaderParameter", shader,
		s.gl.Get("COMPILE_STATUS").Int())
	if !compileStatus.Bool() {
		s.gl.Call("deleteShader", shader)
		return js.Null()
	}
	return shader
}

// SetUniformFloat records a float uniform.
func (s *Shader) SetUniformFloat(name string, v float32) { s.uniforms[name] = v }

// SetUniformVec2 records a vec2 uniform.
func (s *Shader) SetUniformVec2(name string, v [2]float32) { s.uniforms[name] = v }

// SetUniformVec4 records a vec4 uniform.
func (s *Shader) SetUniformVec4(name string, v [4]float32) { s.uniforms[name] = v }

// SetUniformMat4 records a mat4 uniform.
func (s *Shader) SetUniformMat4(name string, v [16]float32) { s.uniforms[name] = v }

// SetUniformInt records an int uniform.
func (s *Shader) SetUniformInt(name string, v int32) { s.uniforms[name] = v }

// SetUniformBlock records a uniform block.
func (s *Shader) SetUniformBlock(name string, data []byte) { s.uniforms[name] = data }

// Dispose releases shader resources.
// PackCurrentUniforms returns nil (not yet implemented for this GPU backend).
func (s *Shader) PackCurrentUniforms() []byte { return nil }

func (s *Shader) Dispose() {
	s.uniforms = nil
	if !s.program.IsNull() && !s.program.IsUndefined() {
		s.gl.Call("deleteProgram", s.program)
		s.program = js.Null()
	}
}
