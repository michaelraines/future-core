//go:build darwin && !soft

package metal

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
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

	// nativeMode signals that vertexSource and fragmentSource hold
	// MSL directly, not Kage-translated GLSL. compile() skips the
	// GLSL→MSL translator block when this is true. Populated by
	// Device.NewShaderNative; the per-stage uniform layouts are set
	// up at construction time so packUniformBuffer works without a
	// translator round-trip.
	nativeMode bool
}

// compile translates GLSL to MSL and compiles to MTLLibrary + MTLFunction.
//
// When nativeMode is true the source fields already hold MSL —
// Device.NewShaderNative stored them — and the translator step is
// skipped. The vertex/fragment uniform layouts were populated at
// NewShaderNative time so packUniformBuffer works the same way.
func (s *Shader) compile() error {
	if s.compiled {
		return s.compileError
	}
	s.compiled = true

	if s.vertexSource != "" {
		var msl string
		if s.nativeMode {
			msl = s.vertexSource
		} else {
			result, err := shadertranslate.GLSLToMSLVertex(s.vertexSource)
			if err != nil {
				s.compileError = fmt.Errorf("metal: vertex GLSL→MSL: %w", err)
				return s.compileError
			}
			s.vertexUniformLayout = result.Uniforms
			msl = result.Source
		}

		lib, err := mtl.DeviceNewLibraryWithSource(s.dev.device, msl)
		if err != nil {
			// Diagnostic: emit the failed MSL source plus the
			// localizedDescription from the NSError so a translator
			// regression surfaces as a readable build error rather than
			// a silent black render. Compile failures should not happen
			// in steady state — every shader the framework ships
			// translates cleanly — but new game-side Kage shaders that
			// hit a translator gap will be caught here.
			fmt.Fprintf(os.Stderr, "metal: vertex MSL compile failed:\n%s\nerror: %v\n", msl, err)
			s.compileError = fmt.Errorf("metal: compile vertex MSL: %w", err)
			return s.compileError
		}
		s.vertexLib = lib
		s.vertexFn = mtl.LibraryNewFunctionWithName(lib, "vertexMain")
	}

	if s.fragmentSource != "" {
		var msl string
		if s.nativeMode {
			msl = s.fragmentSource
		} else {
			result, err := shadertranslate.GLSLToMSLFragment(s.fragmentSource)
			if err != nil {
				s.compileError = fmt.Errorf("metal: fragment GLSL→MSL: %w", err)
				return s.compileError
			}
			s.fragmentUniformLayout = result.Uniforms
			msl = result.Source
		}

		lib, err := mtl.DeviceNewLibraryWithSource(s.dev.device, msl)
		if err != nil {
			fmt.Fprintf(os.Stderr, "metal: fragment MSL compile failed:\n%s\nerror: %v\n", msl, err)
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
	case [3]float32:
		// vec3 is packed as three contiguous floats (size=12). The MSL
		// uniform struct uses explicit pad fields (e.g. `_pad152` in
		// lighting/point_light.frag.msl) so that the field following a
		// vec3 lands on a 16-byte boundary; the writer here just emits
		// the 12 raw bytes the layout table declares.
		binary.LittleEndian.PutUint32(dst[0:4], math.Float32bits(val[0]))
		binary.LittleEndian.PutUint32(dst[4:8], math.Float32bits(val[1]))
		binary.LittleEndian.PutUint32(dst[8:12], math.Float32bits(val[2]))
	case [4]float32:
		for i := range 4 {
			binary.LittleEndian.PutUint32(dst[i*4:(i+1)*4], math.Float32bits(val[i]))
		}
	case [16]float32:
		for i := range 16 {
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
