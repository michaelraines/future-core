//go:build (darwin || linux || freebsd || windows) && !soft

package webgpu

import (
	"encoding/binary"
	"math"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/shadertranslate"
	"github.com/michaelraines/future-core/internal/wgpu"
)

// Shader implements backend.Shader for WebGPU.
// Stores WGSL source and compiled WGPUShaderModule handle.
type Shader struct {
	dev            *Device
	vertexSource   string
	fragmentSource string
	attributes     []backend.VertexAttribute
	uniforms       map[string]interface{}

	// Uniform layout from GLSL→WGSL translation.
	vertexUniformLayout   []shadertranslate.UniformField
	fragmentUniformLayout []shadertranslate.UniformField
	combinedUniformLayout []shadertranslate.UniformField

	// Compiled shader modules (lazily created).
	vertexModule   wgpu.ShaderModule
	fragmentModule wgpu.ShaderModule
	compiled       bool
}

// compile translates GLSL to WGSL and compiles the source into shader modules.
// Both stages share a single combined uniform struct at @group(0) @binding(0)
// so that vertex and fragment uniforms occupy non-overlapping offsets.
func (s *Shader) compile() {
	if s.compiled || s.dev.device == 0 {
		return
	}
	s.compiled = true

	var vertexWGSL, fragmentWGSL string

	if s.vertexSource != "" {
		result, err := shadertranslate.GLSLToWGSLVertex(s.vertexSource)
		if err == nil {
			vertexWGSL = result.Source
			s.vertexUniformLayout = result.Uniforms
		}
	}
	if s.fragmentSource != "" {
		result, err := shadertranslate.GLSLToWGSLFragment(s.fragmentSource)
		if err == nil {
			fragmentWGSL = result.Source
			s.fragmentUniformLayout = result.Uniforms
		}
	}

	// Build a combined uniform layout so both stages share one buffer
	// with non-overlapping offsets.
	s.combinedUniformLayout = buildCombinedUniformLayout(
		s.vertexUniformLayout, s.fragmentUniformLayout)

	if len(s.combinedUniformLayout) > 0 {
		structSrc := buildUniformStructWGSL("Uniforms", s.combinedUniformLayout)
		if vertexWGSL != "" && len(s.vertexUniformLayout) > 0 {
			vertexWGSL = replaceUniformStruct(vertexWGSL, "VertexUniforms", "Uniforms", structSrc)
		}
		if fragmentWGSL != "" && len(s.fragmentUniformLayout) > 0 {
			fragmentWGSL = replaceUniformStruct(fragmentWGSL, "FragmentUniforms", "Uniforms", structSrc)
		}
	}

	if vertexWGSL != "" {
		s.vertexModule = wgpu.DeviceCreateShaderModuleWGSL(s.dev.device, vertexWGSL)
	}
	if fragmentWGSL != "" {
		s.fragmentModule = wgpu.DeviceCreateShaderModuleWGSL(s.dev.device, fragmentWGSL)
	}
}

// packUniforms packs recorded uniforms into a byte buffer using the given layout.
func (s *Shader) packUniforms(layout []shadertranslate.UniformField) []byte {
	if len(layout) == 0 {
		return nil
	}
	// Calculate total buffer size (last field offset + size, aligned to 16).
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
		writeUniformValue(buf[f.Offset:], v)
	}
	return buf
}

// writeUniformValue writes a uniform value into a byte slice.
func writeUniformValue(dst []byte, v interface{}) {
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
	if s.vertexModule != 0 {
		wgpu.ShaderModuleRelease(s.vertexModule)
		s.vertexModule = 0
	}
	if s.fragmentModule != 0 {
		wgpu.ShaderModuleRelease(s.fragmentModule)
		s.fragmentModule = 0
	}
}
