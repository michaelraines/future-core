//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"encoding/binary"
	"fmt"
	"math"
	"regexp"
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/shaderc"
	"github.com/michaelraines/future-core/internal/vk"
)

// uniformField describes a uniform variable's layout in a packed buffer.
type uniformField struct {
	Name   string
	Type   string // GLSL type: "float", "vec2", "vec4", "mat4", etc.
	Offset int
	Size   int
}

// Shader implements backend.Shader for Vulkan.
// Stores GLSL source for SPIR-V compilation when a pipeline is created.
// Uniform values are recorded and applied when the shader is bound.
type Shader struct {
	dev            *Device
	vertexSource   string
	fragmentSource string
	attributes     []backend.VertexAttribute
	uniforms       map[string]interface{}

	// Compiled SPIR-V modules (lazily created).
	vertexModule   vk.ShaderModule
	fragmentModule vk.ShaderModule
	compiled       bool
	compileError   error

	// Uniform buffer layout parsed from GLSL source.
	vertexUniformLayout   []uniformField
	fragmentUniformLayout []uniformField
}

// compile compiles GLSL source to SPIR-V and creates VkShaderModules.
func (s *Shader) compile() error {
	if s.compiled {
		return s.compileError
	}
	s.compiled = true

	if s.vertexSource != "" {
		s.vertexUniformLayout = parseUniformLayout(s.vertexSource)

		spirv, err := shaderc.CompileGLSL(s.vertexSource, shaderc.StageVertex)
		if err != nil {
			s.compileError = fmt.Errorf("vulkan: vertex GLSL→SPIR-V: %w", err)
			return s.compileError
		}
		info := vk.ShaderModuleCreateInfo{
			SType:    vk.StructureTypeShaderModuleCreateInfo,
			CodeSize: uint64(len(spirv)),
			PCode:    uintptr(unsafe.Pointer(&spirv[0])),
		}
		mod, err := vk.CreateShaderModule(s.dev.device, &info)
		if err != nil {
			s.compileError = fmt.Errorf("vulkan: create vertex shader module: %w", err)
			return s.compileError
		}
		s.vertexModule = mod
	}

	if s.fragmentSource != "" {
		s.fragmentUniformLayout = parseUniformLayout(s.fragmentSource)

		spirv, err := shaderc.CompileGLSL(s.fragmentSource, shaderc.StageFragment)
		if err != nil {
			s.compileError = fmt.Errorf("vulkan: fragment GLSL→SPIR-V: %w", err)
			return s.compileError
		}
		info := vk.ShaderModuleCreateInfo{
			SType:    vk.StructureTypeShaderModuleCreateInfo,
			CodeSize: uint64(len(spirv)),
			PCode:    uintptr(unsafe.Pointer(&spirv[0])),
		}
		mod, err := vk.CreateShaderModule(s.dev.device, &info)
		if err != nil {
			s.compileError = fmt.Errorf("vulkan: create fragment shader module: %w", err)
			return s.compileError
		}
		s.fragmentModule = mod
	}

	return nil
}

// packUniformBuffer builds a byte buffer from the uniform map using the given layout.
func (s *Shader) packUniformBuffer(layout []uniformField) []byte {
	if len(layout) == 0 {
		return nil
	}
	// Calculate total size.
	totalSize := 0
	for _, f := range layout {
		end := f.Offset + f.Size
		if end > totalSize {
			totalSize = end
		}
	}
	buf := make([]byte, totalSize)

	for _, f := range layout {
		v, ok := s.uniforms[f.Name]
		if !ok {
			continue
		}
		writeUniformValue(buf[f.Offset:f.Offset+f.Size], v)
	}
	return buf
}

// writeUniformValue writes a uniform value to a byte slice.
func writeUniformValue(dst []byte, v interface{}) {
	switch val := v.(type) {
	case float32:
		binary.LittleEndian.PutUint32(dst, math.Float32bits(val))
	case [2]float32:
		binary.LittleEndian.PutUint32(dst[0:4], math.Float32bits(val[0]))
		binary.LittleEndian.PutUint32(dst[4:8], math.Float32bits(val[1]))
	case [4]float32:
		for i := 0; i < 4; i++ {
			binary.LittleEndian.PutUint32(dst[i*4:(i+1)*4], math.Float32bits(val[i]))
		}
	case [16]float32:
		for i := 0; i < 16; i++ {
			binary.LittleEndian.PutUint32(dst[i*4:(i+1)*4], math.Float32bits(val[i]))
		}
	case int32:
		binary.LittleEndian.PutUint32(dst, uint32(val))
	}
}

// reGLSLUniform matches uniform declarations in GLSL source.
var reGLSLUniform = regexp.MustCompile(`^\s*uniform\s+(\w+)\s+(\w+)\s*;`)

// parseUniformLayout parses GLSL source for uniform declarations and computes
// std140-aligned byte offsets. Sampler uniforms (sampler2D) are excluded.
func parseUniformLayout(glsl string) []uniformField {
	var fields []uniformField
	offset := 0
	for _, line := range splitLines(glsl) {
		m := reGLSLUniform.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		typ, name := m[1], m[2]
		if typ == "sampler2D" {
			continue
		}
		size := glslTypeSize(typ)
		if size == 0 {
			continue
		}
		// std140 alignment: 16 for mat4/vec4/mat3, 8 for vec2, 4 otherwise.
		align := 4
		if size >= 16 {
			align = 16
		} else if size == 8 {
			align = 8
		}
		if offset%align != 0 {
			offset += align - (offset % align)
		}
		fields = append(fields, uniformField{
			Name:   name,
			Type:   typ,
			Offset: offset,
			Size:   size,
		})
		offset += size
	}
	return fields
}

// glslTypeSize returns the byte size for a GLSL uniform type.
func glslTypeSize(typ string) int {
	switch typ {
	case "float":
		return 4
	case "vec2":
		return 8
	case "vec3":
		return 12
	case "vec4":
		return 16
	case "mat3":
		return 48 // 3 x float4 (std140 padding)
	case "mat4":
		return 64
	case "int":
		return 4
	default:
		return 0
	}
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// uniformLayoutSize returns the total byte size of a uniform layout.
func uniformLayoutSize(layout []uniformField) int {
	if len(layout) == 0 {
		return 0
	}
	totalSize := 0
	for _, f := range layout {
		end := f.Offset + f.Size
		if end > totalSize {
			totalSize = end
		}
	}
	return totalSize
}

// SetUniformFloat records a float uniform.
func (s *Shader) SetUniformFloat(name string, v float32) { s.uniforms[name] = v }

// SetUniformVec2 records a vec2 uniform.
func (s *Shader) SetUniformVec2(name string, v [2]float32) { s.uniforms[name] = v }

// SetUniformVec4 records a vec4 uniform.
func (s *Shader) SetUniformVec4(name string, v [4]float32) { s.uniforms[name] = v }

// SetUniformMat4 records a mat4 uniform.
// For the projection matrix, negates row 1 (Y) to account for Vulkan's Y-down
// clip space vs OpenGL's Y-up.
func (s *Shader) SetUniformMat4(name string, v [16]float32) {
	if name == "uProjection" {
		v[1] = -v[1]
		v[5] = -v[5]
		v[9] = -v[9]
		v[13] = -v[13]
	}
	s.uniforms[name] = v
}

// SetUniformInt records an int uniform.
func (s *Shader) SetUniformInt(name string, v int32) { s.uniforms[name] = v }

// SetUniformBlock records a uniform block.
func (s *Shader) SetUniformBlock(name string, data []byte) { s.uniforms[name] = data }

// Dispose releases shader resources.
func (s *Shader) Dispose() {
	if s.dev != nil && s.dev.device != 0 {
		if s.vertexModule != 0 {
			vk.DestroyShaderModule(s.dev.device, s.vertexModule)
			s.vertexModule = 0
		}
		if s.fragmentModule != 0 {
			vk.DestroyShaderModule(s.dev.device, s.fragmentModule)
			s.fragmentModule = 0
		}
	}
	s.uniforms = nil
}

// Keep the compiler happy for unsafe import.
var _ = unsafe.Pointer(nil)
