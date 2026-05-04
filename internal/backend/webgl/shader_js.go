//go:build js

package webgl

import (
	"syscall/js"

	"github.com/michaelraines/future-core/internal/backend"
)

// Shader implements backend.Shader for WebGL2.
//
// WebGL2 uniforms can only be uploaded against a currently-bound program
// (no glProgramUniform* like desktop OpenGL 3.3+), so the shader caches
// uniform values and applies them lazily — apply() pushes every cached
// value via gl.uniform* before each draw, with the assumption that
// SetPipeline has already glUseProgram'd this shader's program.
//
// Compile happens eagerly in NewShader so location lookups are valid
// before the first SetUniform* call.
type Shader struct {
	gl             js.Value
	vertexSource   string
	fragmentSource string
	attributes     []backend.VertexAttribute

	// Compiled GL program + uniform location cache.
	program   js.Value
	compiled  bool
	locations map[string]js.Value
	lastError string

	// Cached uniform values — pushed via apply().
	uFloat map[string]float32
	uVec2  map[string][2]float32
	uVec3  map[string][3]float32
	uVec4  map[string][4]float32
	uMat4  map[string][16]float32
	uInt   map[string]int32
}

// newShader constructs an empty shader; the device calls compile()
// after assignment so failures bubble up cleanly.
func newShader(gl js.Value, vert, frag string, attrs []backend.VertexAttribute) *Shader {
	return &Shader{
		gl:             gl,
		vertexSource:   vert,
		fragmentSource: frag,
		attributes:     attrs,
		locations:      make(map[string]js.Value),
		uFloat:         make(map[string]float32),
		uVec2:          make(map[string][2]float32),
		uVec3:          make(map[string][3]float32),
		uVec4:          make(map[string][4]float32),
		uMat4:          make(map[string][16]float32),
		uInt:           make(map[string]int32),
	}
}

// dumpShaderSource prints the shader source with 1-based line numbers
// to the JS console so the line number in the GL info log can be
// resolved against the actual source. Routed through console.warn so
// it surfaces in browser devtools without being suppressed by the
// engine's stderr-redirect plumbing.
func dumpShaderSource(stage, source string) {
	console := js.Global().Get("console")
	if console.IsNull() || console.IsUndefined() {
		return
	}
	console.Call("groupCollapsed", "[webgl] failing "+stage+" shader source:")
	lineNo := 1
	start := 0
	for i := 0; i <= len(source); i++ {
		if i == len(source) || source[i] == '\n' {
			console.Call("log", lineNo, source[start:i])
			lineNo++
			start = i + 1
		}
	}
	console.Call("groupEnd")
}

// compileError stashes the most recent compile/link diagnostic so the
// caller (NewShader) can wrap it into the returned error. Without this
// every shader compile failure surfaces as a generic "compile/link
// failed" message at the call site, hiding the actual GLSL error and
// the source line numbers that would let you fix it.
//
// Stored on the shader (rather than returned) because compile() is
// called lazily and from the same caller that cares — keeping it as
// state keeps the public bool-returning signature compatible with
// the existing call sites.
func (s *Shader) compileError() string { return s.lastError }

// compile compiles and links the vertex/fragment shaders into a GL program.
func (s *Shader) compile() bool {
	if s.compiled {
		return !s.program.IsNull() && !s.program.IsUndefined()
	}
	s.compiled = true

	if s.vertexSource == "" || s.fragmentSource == "" {
		s.lastError = "vertex or fragment source is empty"
		return false
	}

	vertShader := s.compileShader(s.gl.Get("VERTEX_SHADER").Int(), s.vertexSource, "vertex")
	if vertShader.IsNull() || vertShader.IsUndefined() {
		return false
	}

	fragShader := s.compileShader(s.gl.Get("FRAGMENT_SHADER").Int(), s.fragmentSource, "fragment")
	if fragShader.IsNull() || fragShader.IsUndefined() {
		s.gl.Call("deleteShader", vertShader)
		return false
	}

	prog := s.gl.Call("createProgram")
	s.gl.Call("attachShader", prog, vertShader)
	s.gl.Call("attachShader", prog, fragShader)

	// Bind attribute locations to the slots the pipeline will configure
	// (i.e. the order they appear in the VertexFormat). This guarantees
	// the slots used in vertexAttribPointer match what the linked program
	// expects — without this, drivers free-assign locations and slot 0
	// may not be aPosition.
	for i, attr := range s.attributes {
		s.gl.Call("bindAttribLocation", prog, i, attr.Name)
	}

	s.gl.Call("linkProgram", prog)

	s.gl.Call("detachShader", prog, vertShader)
	s.gl.Call("detachShader", prog, fragShader)
	s.gl.Call("deleteShader", vertShader)
	s.gl.Call("deleteShader", fragShader)

	linkStatus := s.gl.Call("getProgramParameter", prog,
		s.gl.Get("LINK_STATUS").Int())
	if !linkStatus.Bool() {
		log := s.gl.Call("getProgramInfoLog", prog).String()
		s.lastError = "link: " + log
		s.gl.Call("deleteProgram", prog)
		return false
	}

	s.program = prog
	return true
}

// compileShader compiles a single shader stage. On failure the GL
// info log is captured into s.lastError so callers can surface the
// actual GLSL diagnostic.
func (s *Shader) compileShader(shaderType int, source, stage string) js.Value {
	shader := s.gl.Call("createShader", shaderType)
	s.gl.Call("shaderSource", shader, source)
	s.gl.Call("compileShader", shader)

	compileStatus := s.gl.Call("getShaderParameter", shader,
		s.gl.Get("COMPILE_STATUS").Int())
	if !compileStatus.Bool() {
		log := s.gl.Call("getShaderInfoLog", shader).String()
		// Mirror the offending source to console with line numbers so
		// the failing line is actually visible alongside the diagnostic.
		// The driver-supplied error log only references "0:N" line
		// numbers without the source itself, which makes triage
		// painful when the source has been through a translator.
		dumpShaderSource(stage, source)
		s.lastError = stage + " compile: " + log
		s.gl.Call("deleteShader", shader)
		return js.Null()
	}
	return shader
}

// location returns the cached uniform location, looking it up on first use.
// Returns a null js.Value if the uniform is missing — gl.uniform* is a
// silent no-op for null locations, matching desktop driver behaviour.
func (s *Shader) location(name string) js.Value {
	if loc, ok := s.locations[name]; ok {
		return loc
	}
	if s.program.IsNull() || s.program.IsUndefined() {
		return js.Null()
	}
	loc := s.gl.Call("getUniformLocation", s.program, name)
	s.locations[name] = loc
	return loc
}

// SetUniformFloat records a float uniform.
func (s *Shader) SetUniformFloat(name string, v float32) { s.uFloat[name] = v }

// SetUniformVec2 records a vec2 uniform.
func (s *Shader) SetUniformVec2(name string, v [2]float32) { s.uVec2[name] = v }

// SetUniformVec3 records a vec3 uniform.
func (s *Shader) SetUniformVec3(name string, v [3]float32) { s.uVec3[name] = v }

// SetUniformVec4 records a vec4 uniform.
func (s *Shader) SetUniformVec4(name string, v [4]float32) { s.uVec4[name] = v }

// SetUniformMat4 records a mat4 uniform. The Y-flip for offscreen
// framebuffers is applied at upload time in apply() — recording the raw
// matrix here keeps the cache consistent regardless of which target the
// next draw lands on.
func (s *Shader) SetUniformMat4(name string, v [16]float32) { s.uMat4[name] = v }

// SetUniformInt records an int uniform.
func (s *Shader) SetUniformInt(name string, v int32) { s.uInt[name] = v }

// SetUniformBlock is a no-op for the current WebGL2 sprite shader path —
// uniform blocks aren't used by built-in shaders. UBOs would attach via
// glBindBufferBase + glUniformBlockBinding in a future revision.
func (s *Shader) SetUniformBlock(_ string, _ []byte) {}

// PackCurrentUniforms returns nil — WebGL2 uses individual gl.uniform* calls,
// not a packed UBO.
func (s *Shader) PackCurrentUniforms() []byte { return nil }

// apply pushes every cached uniform to the currently-bound program.
// Caller is responsible for glUseProgram'ing this shader first
// (Pipeline.bind handles that). yFlip controls the row-1 negation for
// uProjection — set true when rendering to an offscreen FBO so the
// engine's Y-down ortho lands the right way up; see Encoder.applyShader.
func (s *Shader) apply(yFlip bool) {
	if s.program.IsNull() || s.program.IsUndefined() {
		return
	}
	for name, v := range s.uFloat {
		loc := s.location(name)
		if loc.IsNull() {
			continue
		}
		s.gl.Call("uniform1f", loc, v)
	}
	for name, v := range s.uVec2 {
		loc := s.location(name)
		if loc.IsNull() {
			continue
		}
		s.gl.Call("uniform2f", loc, v[0], v[1])
	}
	for name, v := range s.uVec3 {
		loc := s.location(name)
		if loc.IsNull() {
			continue
		}
		s.gl.Call("uniform3f", loc, v[0], v[1], v[2])
	}
	for name, v := range s.uVec4 {
		loc := s.location(name)
		if loc.IsNull() {
			continue
		}
		s.gl.Call("uniform4f", loc, v[0], v[1], v[2], v[3])
	}
	for name, v := range s.uMat4 {
		loc := s.location(name)
		if loc.IsNull() {
			continue
		}
		m := v
		if yFlip && name == "uProjection" {
			m[1] = -m[1]
			m[5] = -m[5]
			m[9] = -m[9]
			m[13] = -m[13]
		}
		// uniformMatrix4fv wants a Float32Array.
		arr := js.Global().Get("Float32Array").New(16)
		for i := 0; i < 16; i++ {
			arr.SetIndex(i, m[i])
		}
		s.gl.Call("uniformMatrix4fv", loc, false, arr)
	}
	for name, v := range s.uInt {
		loc := s.location(name)
		if loc.IsNull() {
			continue
		}
		s.gl.Call("uniform1i", loc, v)
	}
}

// Dispose releases shader resources.
func (s *Shader) Dispose() {
	if !s.program.IsNull() && !s.program.IsUndefined() {
		s.gl.Call("deleteProgram", s.program)
		s.program = js.Null()
	}
	s.locations = nil
}
