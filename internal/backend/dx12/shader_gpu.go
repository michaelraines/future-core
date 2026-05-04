//go:build windows && !soft

package dx12

import "github.com/michaelraines/future-core/internal/backend"

// Shader implements backend.Shader for DX12.
// Stores HLSL source for later compilation into DXBC/DXIL bytecode.
//
// nativeMode signals the source came in via NewShaderNative already in
// HLSL form (so when D3DCompile lands, no Kage→GLSL→HLSL translation
// step runs — the bytes go straight to D3DCompile). nativeUniforms
// carries the combined-stage uniform layout the framework's per-draw
// packer uses to write SetUniform* values into the constant buffer.
type Shader struct {
	dev            *Device
	vertexSource   string
	fragmentSource string
	attributes     []backend.VertexAttribute
	uniforms       map[string]interface{}
	nativeMode     bool
	nativeUniforms []backend.NativeUniformField
}

// SetUniformFloat sets a float uniform.
func (s *Shader) SetUniformFloat(name string, v float32) { s.uniforms[name] = v }

// SetUniformVec2 sets a vec2 uniform.
func (s *Shader) SetUniformVec2(name string, v [2]float32) { s.uniforms[name] = v }

// SetUniformVec3 sets a vec3 uniform.
func (s *Shader) SetUniformVec3(name string, v [3]float32) { s.uniforms[name] = v }

// SetUniformVec4 sets a vec4 uniform.
func (s *Shader) SetUniformVec4(name string, v [4]float32) { s.uniforms[name] = v }

// SetUniformMat4 sets a mat4 uniform.
func (s *Shader) SetUniformMat4(name string, v [16]float32) { s.uniforms[name] = v }

// SetUniformInt sets an int uniform.
func (s *Shader) SetUniformInt(name string, v int32) { s.uniforms[name] = v }

// SetUniformBlock sets a uniform block.
func (s *Shader) SetUniformBlock(name string, data []byte) { s.uniforms[name] = data }

// Dispose releases shader resources.
// PackCurrentUniforms returns nil (not yet implemented for this GPU backend).
func (s *Shader) PackCurrentUniforms() []byte { return nil }

func (s *Shader) Dispose() {
	s.uniforms = nil
}
