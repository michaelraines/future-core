//go:build darwin && !soft

package metal

import (
	"encoding/binary"
	"fmt"
	"math"
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/mtl"
	"github.com/michaelraines/future-core/internal/shadertranslate"
)

// Shader implements backend.Shader for Metal.
// GLSL source is translated to MSL at compile time.
type Shader struct {
	dev            *Device
	vertexSource   string
	fragmentSource string
	attributes     []backend.VertexAttribute
	uniforms       map[string]any

	// Compiled Metal objects (lazily created).
	vertexLib    mtl.Library
	fragmentLib  mtl.Library
	vertexFn     mtl.Function
	fragmentFn   mtl.Function
	compiled     bool
	compileError error

	// Uniform buffer layout from the GLSL→MSL translator.
	vertexUniformLayout   []shadertranslate.UniformField
	fragmentUniformLayout []shadertranslate.UniformField
}

// compile translates GLSL to MSL and compiles to MTLLibrary + MTLFunction.
func (s *Shader) compile() error {
	if s.compiled {
		return s.compileError
	}
	s.compiled = true

	if s.vertexSource != "" {
		result, err := shadertranslate.GLSLToMSLVertex(s.vertexSource)
		if err != nil {
			s.compileError = fmt.Errorf("metal: vertex GLSL→MSL: %w", err)
			return s.compileError
		}
		s.vertexUniformLayout = result.Uniforms

		lib, err := mtl.DeviceNewLibraryWithSource(s.dev.device, result.Source)
		if err != nil {
			s.compileError = fmt.Errorf("metal: compile vertex MSL: %w", err)
			return s.compileError
		}
		s.vertexLib = lib
		s.vertexFn = mtl.LibraryNewFunctionWithName(lib, "vertexMain")
	}

	if s.fragmentSource != "" {
		result, err := shadertranslate.GLSLToMSLFragment(s.fragmentSource)
		if err != nil {
			s.compileError = fmt.Errorf("metal: fragment GLSL→MSL: %w", err)
			return s.compileError
		}
		s.fragmentUniformLayout = result.Uniforms

		lib, err := mtl.DeviceNewLibraryWithSource(s.dev.device, result.Source)
		if err != nil {
			s.compileError = fmt.Errorf("metal: compile fragment MSL: %w", err)
			return s.compileError
		}
		s.fragmentLib = lib
		s.fragmentFn = mtl.LibraryNewFunctionWithName(lib, "fragmentMain")
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
	return buf
}

// writeUniformValue writes a uniform value to a byte slice.
func writeUniformValue(dst []byte, v any) {
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
// PackCurrentUniforms returns nil (not yet implemented for this GPU backend).
func (s *Shader) PackCurrentUniforms() []byte { return nil }

func (s *Shader) Dispose() {
	s.uniforms = nil
	if s.vertexLib != 0 {
		mtl.LibraryRelease(s.vertexLib)
		s.vertexLib = 0
	}
	if s.fragmentLib != 0 {
		mtl.LibraryRelease(s.fragmentLib)
		s.fragmentLib = 0
	}
	s.vertexFn = 0
	s.fragmentFn = 0
}

// Keep the compiler happy.
var _ = unsafe.Pointer(nil)
