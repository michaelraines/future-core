//go:build metal

package metal

import (
	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/mtl"
)

// Shader implements backend.Shader for Metal.
// Stores MSL source and compiled MTLLibrary/MTLFunction handles.
type Shader struct {
	dev            *Device
	vertexSource   string
	fragmentSource string
	attributes     []backend.VertexAttribute
	uniforms       map[string]interface{}

	// Compiled Metal objects (lazily created).
	vertexLib    mtl.Library
	fragmentLib  mtl.Library
	vertexFn     mtl.Function
	fragmentFn   mtl.Function
	compiled     bool
	compileError error
}

// compile compiles the MSL source into MTLLibrary + MTLFunction.
func (s *Shader) compile() error {
	if s.compiled {
		return s.compileError
	}
	s.compiled = true

	if s.vertexSource != "" {
		lib, err := mtl.DeviceNewLibraryWithSource(s.dev.device, s.vertexSource)
		if err != nil {
			s.compileError = err
			return err
		}
		s.vertexLib = lib
		s.vertexFn = mtl.LibraryNewFunctionWithName(lib, "vertexMain")
		if s.vertexFn == 0 {
			// Try common alternative names.
			s.vertexFn = mtl.LibraryNewFunctionWithName(lib, "vertex_main")
		}
	}

	if s.fragmentSource != "" {
		lib, err := mtl.DeviceNewLibraryWithSource(s.dev.device, s.fragmentSource)
		if err != nil {
			s.compileError = err
			return err
		}
		s.fragmentLib = lib
		s.fragmentFn = mtl.LibraryNewFunctionWithName(lib, "fragmentMain")
		if s.fragmentFn == 0 {
			s.fragmentFn = mtl.LibraryNewFunctionWithName(lib, "fragment_main")
		}
	}

	return nil
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
