//go:build wgpunative

package webgpu

import (
	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/wgpu"
)

// Shader implements backend.Shader for WebGPU.
// Stores WGSL source and compiled WGPUShaderModule handle.
type Shader struct {
	dev            *Device
	vertexSource   string
	fragmentSource string
	attributes     []backend.VertexAttribute
	uniforms       map[string]interface{}

	// Compiled shader modules (lazily created).
	vertexModule   wgpu.ShaderModule
	fragmentModule wgpu.ShaderModule
	compiled       bool
}

// compile compiles the WGSL source into shader modules.
func (s *Shader) compile() {
	if s.compiled || s.dev.device == 0 {
		return
	}
	s.compiled = true

	if s.vertexSource != "" {
		s.vertexModule = wgpu.DeviceCreateShaderModuleWGSL(s.dev.device, s.vertexSource)
	}
	if s.fragmentSource != "" {
		s.fragmentModule = wgpu.DeviceCreateShaderModuleWGSL(s.dev.device, s.fragmentSource)
	}
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
func (s *Shader) Dispose() {
	s.uniforms = nil
	if s.vertexModule != 0 {
		wgpu.ShaderModuleRelease(s.vertexModule)
		s.vertexModule = 0
	}
	if s.fragmentModule != 0 {
		wgpu.ShaderModuleRelease(s.fragmentModule)
		s.fragmentModule = 0
	}
}
