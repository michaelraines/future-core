package soft

// Shader implements backend.Shader as a uniform value store.
type Shader struct {
	id       uint64
	uniforms map[string]any
	disposed bool
}

// SetUniformFloat sets a float uniform.
func (s *Shader) SetUniformFloat(name string, v float32) {
	if s.uniforms == nil {
		return
	}
	s.uniforms[name] = v
}

// SetUniformVec2 sets a vec2 uniform.
func (s *Shader) SetUniformVec2(name string, v [2]float32) {
	if s.uniforms == nil {
		return
	}
	s.uniforms[name] = v
}

// SetUniformVec4 sets a vec4 uniform.
func (s *Shader) SetUniformVec4(name string, v [4]float32) {
	if s.uniforms == nil {
		return
	}
	s.uniforms[name] = v
}

// SetUniformMat4 sets a mat4 uniform.
func (s *Shader) SetUniformMat4(name string, v [16]float32) {
	if s.uniforms == nil {
		return
	}
	s.uniforms[name] = v
}

// SetUniformInt sets an int uniform.
func (s *Shader) SetUniformInt(name string, v int32) {
	if s.uniforms == nil {
		return
	}
	s.uniforms[name] = v
}

// SetUniformBlock sets a uniform block's data.
func (s *Shader) SetUniformBlock(name string, data []byte) {
	if s.uniforms == nil {
		return
	}
	dst := make([]byte, len(data))
	copy(dst, data)
	s.uniforms[name] = dst
}

// Dispose releases the shader.
func (s *Shader) Dispose() {
	s.disposed = true
	s.uniforms = nil
}

// Uniform returns the value of a uniform by name. For testing only.
func (s *Shader) Uniform(name string) (any, bool) {
	v, ok := s.uniforms[name]
	return v, ok
}
