//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"encoding/binary"
	"fmt"
	"math"
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/shaderc"
	"github.com/michaelraines/future-core/internal/shadertranslate"
	"github.com/michaelraines/future-core/internal/vk"
)

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

	// Uniform buffer layouts computed by the shared std140 extractor.
	// The per-backend regex parser this replaces used to diverge from
	// shaderc's SPIR-V layout on vec3 alignment (and silently dropped
	// [3]float32 values) — see shadertranslate/layout.go for rationale.
	vertexUniformLayout   []shadertranslate.UniformField
	fragmentUniformLayout []shadertranslate.UniformField
}

// compile compiles GLSL source to SPIR-V and creates VkShaderModules.
func (s *Shader) compile() error {
	if s.compiled {
		return s.compileError
	}
	s.compiled = true

	if s.vertexSource != "" {
		dumpShaderSource(s.vertexSource, "vert.glsl")
		layout, err := shadertranslate.ExtractUniformLayout(s.vertexSource)
		if err != nil {
			s.compileError = fmt.Errorf("vulkan: vertex uniform layout: %w", err)
			return s.compileError
		}
		s.vertexUniformLayout = layout

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
		dumpShaderSource(s.fragmentSource, "frag.glsl")
		layout, err := shadertranslate.ExtractUniformLayout(s.fragmentSource)
		if err != nil {
			s.compileError = fmt.Errorf("vulkan: fragment uniform layout: %w", err)
			return s.compileError
		}
		s.fragmentUniformLayout = layout

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
func (s *Shader) packUniformBuffer(layout []shadertranslate.UniformField) []byte {
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
	probePackedUniform(layout, buf)
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
	case [3]float32:
		// vec3 writes 12 bytes; the trailing 4 bytes of its 16-byte
		// std140 slot stay zero from the make([]byte, ...) above.
		// Without this case, SetUniformVec3 was a silent no-op on
		// Vulkan and every point-light LightColor rendered as (0,0,0).
		for i := 0; i < 3; i++ {
			binary.LittleEndian.PutUint32(dst[i*4:(i+1)*4], math.Float32bits(val[i]))
		}
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

// SetUniformFloat records a float uniform.
func (s *Shader) SetUniformFloat(name string, v float32) { s.uniforms[name] = v }

// SetUniformVec2 records a vec2 uniform.
func (s *Shader) SetUniformVec2(name string, v [2]float32) { s.uniforms[name] = v }

// SetUniformVec3 records a vec3 uniform.
func (s *Shader) SetUniformVec3(name string, v [3]float32) { s.uniforms[name] = v }

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
// PackCurrentUniforms returns nil (not yet implemented for this GPU backend).
func (s *Shader) PackCurrentUniforms() []byte { return nil }

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
