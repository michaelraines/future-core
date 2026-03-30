//go:build js && !soft

package webgpu

import (
	"encoding/binary"
	"math"
	"syscall/js"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/shadertranslate"
)

// Shader implements backend.Shader for WebGPU via the browser JS API.
type Shader struct {
	dev            *Device
	vertexSource   string
	fragmentSource string
	attributes     []backend.VertexAttribute
	uniforms       map[string]interface{}

	// Uniform layout from GLSL→WGSL translation.
	vertexUniformLayout   []shadertranslate.UniformField
	fragmentUniformLayout []shadertranslate.UniformField

	// Compiled shader modules (lazily created).
	vertexModule   js.Value
	fragmentModule js.Value
	compiled       bool
}

// compile translates GLSL to WGSL and creates shader modules.
func (s *Shader) compile() {
	if s.compiled {
		return
	}
	s.compiled = true

	if s.vertexSource != "" {
		result, err := shadertranslate.GLSLToWGSLVertex(s.vertexSource)
		if err == nil {
			desc := js.Global().Get("Object").New()
			desc.Set("code", result.Source)
			s.vertexModule = s.dev.device.Call("createShaderModule", desc)
			s.vertexUniformLayout = result.Uniforms
		}
	}
	if s.fragmentSource != "" {
		result, err := shadertranslate.GLSLToWGSLFragment(s.fragmentSource)
		if err == nil {
			desc := js.Global().Get("Object").New()
			desc.Set("code", result.Source)
			s.fragmentModule = s.dev.device.Call("createShaderModule", desc)
			s.fragmentUniformLayout = result.Uniforms
		}
	}
}

// packUniforms packs recorded uniforms into a byte buffer using the given layout.
func (s *Shader) packUniforms(layout []shadertranslate.UniformField) []byte {
	if len(layout) == 0 {
		return nil
	}
	last := layout[len(layout)-1]
	totalSize := last.Offset + last.Size
	if totalSize%16 != 0 {
		totalSize += 16 - (totalSize % 16)
	}
	buf := make([]byte, totalSize)
	for _, f := range layout {
		v, ok := s.uniforms[f.Name]
		if !ok {
			continue
		}
		writeUniformValueJS(buf[f.Offset:], v)
	}
	return buf
}

// writeUniformValueJS writes a uniform value into a byte slice.
func writeUniformValueJS(dst []byte, v interface{}) {
	switch val := v.(type) {
	case float32:
		binary.LittleEndian.PutUint32(dst, math.Float32bits(val))
	case int32:
		binary.LittleEndian.PutUint32(dst, uint32(val))
	case [2]float32:
		binary.LittleEndian.PutUint32(dst[0:], math.Float32bits(val[0]))
		binary.LittleEndian.PutUint32(dst[4:], math.Float32bits(val[1]))
	case [4]float32:
		for i := 0; i < 4; i++ {
			binary.LittleEndian.PutUint32(dst[i*4:], math.Float32bits(val[i]))
		}
	case [16]float32:
		for i := 0; i < 16; i++ {
			binary.LittleEndian.PutUint32(dst[i*4:], math.Float32bits(val[i]))
		}
	case []byte:
		copy(dst, val)
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
}
