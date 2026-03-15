//go:build glfw

// Package gl provides pure Go OpenGL 3.3 core profile bindings loaded at
// runtime via purego. No CGo is required. The shared library (libGL.so on
// Linux, OpenGL.framework on macOS, opengl32.dll on Windows) must be
// available at runtime.
package gl

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// ---------------------------------------------------------------------------
// OpenGL constants (subset used by the engine)
// ---------------------------------------------------------------------------

const (
	FALSE = 0
	TRUE  = 1

	UNSIGNED_BYTE  = 0x1401
	UNSIGNED_SHORT = 0x1403
	UNSIGNED_INT   = 0x1405
	FLOAT          = 0x1406

	TRIANGLES      = 0x0004
	TRIANGLE_STRIP = 0x0005
	LINES          = 0x0001
	LINE_STRIP     = 0x0003
	POINTS         = 0x0000

	ARRAY_BUFFER         = 0x8892
	ELEMENT_ARRAY_BUFFER = 0x8893

	STATIC_DRAW  = 0x88E4
	DYNAMIC_DRAW = 0x88E8

	TEXTURE_2D = 0x0DE1

	TEXTURE_WRAP_S     = 0x2802
	TEXTURE_WRAP_T     = 0x2803
	TEXTURE_MIN_FILTER = 0x2801
	TEXTURE_MAG_FILTER = 0x2800

	NEAREST = 0x2600
	LINEAR  = 0x2601

	REPEAT          = 0x2901
	MIRRORED_REPEAT = 0x8370
	CLAMP_TO_EDGE   = 0x812F

	TEXTURE0 = 0x84C0

	R8      = 0x8229
	RGB8    = 0x8051
	RGBA8   = 0x8058
	RGBA16F = 0x881A
	RGBA32F = 0x8814

	RED  = 0x1903
	RGB  = 0x1907
	RGBA = 0x1908

	DEPTH_COMPONENT    = 0x1902
	DEPTH_COMPONENT24  = 0x81A6
	DEPTH_COMPONENT32F = 0x8CAC

	FRAMEBUFFER          = 0x8D40
	COLOR_ATTACHMENT0    = 0x8CE0
	DEPTH_ATTACHMENT     = 0x8D00
	FRAMEBUFFER_COMPLETE = 0x8CD5

	COLOR_BUFFER_BIT   = 0x00004000
	DEPTH_BUFFER_BIT   = 0x00000100
	STENCIL_BUFFER_BIT = 0x00000400

	BLEND        = 0x0BE2
	DEPTH_TEST   = 0x0B71
	SCISSOR_TEST = 0x0C11
	CULL_FACE    = 0x0B44
	STENCIL_TEST = 0x0B90

	ZERO                = 0
	ONE                 = 1
	SRC_ALPHA           = 0x0302
	ONE_MINUS_SRC_ALPHA = 0x0303
	DST_COLOR           = 0x0306

	FRONT = 0x0404
	BACK  = 0x0405

	// Stencil operations.
	KEEP      = 0x1E00
	REPLACE   = 0x1E01
	INCR      = 0x1E02
	DECR      = 0x1E03
	INVERT    = 0x150A
	INCR_WRAP = 0x8507
	DECR_WRAP = 0x8508

	NEVER    = 0x0200
	LESS     = 0x0201
	EQUAL    = 0x0202
	LEQUAL   = 0x0203
	GREATER  = 0x0204
	NOTEQUAL = 0x0205
	GEQUAL   = 0x0206
	ALWAYS   = 0x0207

	VERTEX_SHADER   = 0x8B31
	FRAGMENT_SHADER = 0x8B30

	COMPILE_STATUS  = 0x8B81
	LINK_STATUS     = 0x8B82
	INFO_LOG_LENGTH = 0x8B84

	VERSION = 0x1F02

	MAX_TEXTURE_SIZE = 0x0D33
)

// ---------------------------------------------------------------------------
// Internal function variables — populated by Init()
// ---------------------------------------------------------------------------

var (
	fnEnable  func(cap uint32)
	fnDisable func(cap uint32)

	fnBlendFunc func(sfactor, dfactor uint32)

	fnClearColor func(r, g, b, a float32)
	fnClearDepth func(d float64) // GL 1.0: glClearDepth takes GLdouble
	fnClear      func(mask uint32)

	fnViewport func(x, y, w, h int32)
	fnScissor  func(x, y, w, h int32)

	fnDepthFunc func(fn uint32)
	fnDepthMask func(flag uint8) // GLboolean = unsigned char
	fnCullFace  func(mode uint32)

	fnGenTextures    func(n int32, textures *uint32)
	fnDeleteTextures func(n int32, textures *uint32)
	fnBindTexture    func(target, texture uint32)
	fnActiveTexture  func(texture uint32)
	fnTexParameteri  func(target, pname uint32, param int32)
	fnTexImage2D     func(target uint32, level, internalformat int32, width, height, border int32, format, typ uint32, pixels uintptr)
	fnTexSubImage2D  func(target uint32, level, xoffset, yoffset, width, height int32, format, typ uint32, pixels uintptr)

	fnGenBuffers    func(n int32, buffers *uint32)
	fnDeleteBuffers func(n int32, buffers *uint32)
	fnBindBuffer    func(target, buffer uint32)
	fnBufferData    func(target uint32, size int, data uintptr, usage uint32)
	fnBufferSubData func(target uint32, offset, size int, data uintptr)

	fnCreateShader     func(shaderType uint32) uint32
	fnDeleteShader     func(shader uint32)
	fnShaderSource     func(shader uint32, count int32, source **byte, length *int32)
	fnCompileShader    func(shader uint32)
	fnGetShaderiv      func(shader, pname uint32, params *int32)
	fnGetShaderInfoLog func(shader uint32, maxLength int32, length *int32, infoLog *byte)

	fnCreateProgram     func() uint32
	fnDeleteProgram     func(program uint32)
	fnAttachShader      func(program, shader uint32)
	fnLinkProgram       func(program uint32)
	fnUseProgram        func(program uint32)
	fnGetProgramiv      func(program, pname uint32, params *int32)
	fnGetProgramInfoLog func(program uint32, maxLength int32, length *int32, infoLog *byte)

	fnGetUniformLocation      func(program uint32, name *byte) int32
	fnProgramUniform1f        func(program uint32, location int32, v0 float32)
	fnProgramUniform1i        func(program uint32, location int32, v0 int32)
	fnProgramUniform2fv       func(program uint32, location, count int32, value *float32)
	fnProgramUniform4fv       func(program uint32, location, count int32, value *float32)
	fnProgramUniformMatrix4fv func(program uint32, location, count int32, transpose uint8, value *float32)

	fnGenFramebuffers        func(n int32, framebuffers *uint32)
	fnDeleteFramebuffers     func(n int32, framebuffers *uint32)
	fnBindFramebuffer        func(target, framebuffer uint32)
	fnFramebufferTexture2D   func(target, attachment, textarget, texture uint32, level int32)
	fnCheckFramebufferStatus func(target uint32) uint32

	fnGetIntegerv func(pname uint32, data *int32)
	fnGetString   func(name uint32) uintptr

	fnDrawArrays            func(mode uint32, first, count int32)
	fnDrawArraysInstanced   func(mode uint32, first, count, instancecount int32)
	fnDrawElements          func(mode uint32, count int32, typ uint32, indices uintptr)
	fnDrawElementsInstanced func(mode uint32, count int32, typ uint32, indices uintptr, instancecount int32)

	fnFlush func()

	fnStencilFunc  func(fn uint32, ref int32, mask uint32)
	fnStencilOp    func(sfail, dpfail, dppass uint32)
	fnStencilMask  func(mask uint32)
	fnClearStencil func(s int32)
	fnColorMask    func(r, g, b, a uint8) // GLboolean

	fnGenSamplers       func(n int32, samplers *uint32)
	fnDeleteSamplers    func(n int32, samplers *uint32)
	fnBindSampler       func(unit, sampler uint32)
	fnSamplerParameteri func(sampler, pname uint32, param int32)

	fnGetTexImage func(target uint32, level int32, format, typ uint32, pixels uintptr)
)

// lib holds the loaded OpenGL library handle.
var lib uintptr

// ---------------------------------------------------------------------------
// Public wrappers
// ---------------------------------------------------------------------------

func Enable(cap uint32)                 { fnEnable(cap) }
func Disable(cap uint32)                { fnDisable(cap) }
func BlendFunc(sfactor, dfactor uint32) { fnBlendFunc(sfactor, dfactor) }
func ClearColor(r, g, b, a float32)     { fnClearColor(r, g, b, a) }
func ClearDepthf(d float32)             { fnClearDepth(float64(d)) }
func Clear(mask uint32)                 { fnClear(mask) }
func Viewport(x, y, w, h int32)         { fnViewport(x, y, w, h) }
func Scissor(x, y, w, h int32)          { fnScissor(x, y, w, h) }
func DepthFunc(fn uint32)               { fnDepthFunc(fn) }

func DepthMask(flag bool) {
	v := uint8(0)
	if flag {
		v = 1
	}
	fnDepthMask(v)
}

func CullFace(mode uint32)                          { fnCullFace(mode) }
func StencilFunc(fn uint32, ref int32, mask uint32) { fnStencilFunc(fn, ref, mask) }
func StencilOp(sfail, dpfail, dppass uint32)        { fnStencilOp(sfail, dpfail, dppass) }
func StencilMask(mask uint32)                       { fnStencilMask(mask) }
func ClearStencil(s int32)                          { fnClearStencil(s) }
func ColorMask(r, g, b, a bool)                     { fnColorMask(b2u(r), b2u(g), b2u(b), b2u(a)) }

// b2u converts a bool to a GLboolean (uint8).
func b2u(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}
func GenTextures(n int32, textures *uint32)           { fnGenTextures(n, textures) }
func DeleteTextures(n int32, textures *uint32)        { fnDeleteTextures(n, textures) }
func BindTexture(target, texture uint32)              { fnBindTexture(target, texture) }
func ActiveTexture(texture uint32)                    { fnActiveTexture(texture) }
func TexParameteri(target, pname uint32, param int32) { fnTexParameteri(target, pname, param) }

func TexImage2D(target uint32, level, internalformat int32, width, height, border int32, format, typ uint32, pixels unsafe.Pointer) {
	fnTexImage2D(target, level, internalformat, width, height, border, format, typ, uintptr(pixels))
}

func TexSubImage2D(target uint32, level, xoffset, yoffset, width, height int32, format, typ uint32, pixels unsafe.Pointer) {
	fnTexSubImage2D(target, level, xoffset, yoffset, width, height, format, typ, uintptr(pixels))
}

func GetTexImage(target uint32, level int32, format, typ uint32, pixels unsafe.Pointer) {
	fnGetTexImage(target, level, format, typ, uintptr(pixels))
}

func GenBuffers(n int32, buffers *uint32)    { fnGenBuffers(n, buffers) }
func DeleteBuffers(n int32, buffers *uint32) { fnDeleteBuffers(n, buffers) }
func BindBuffer(target, buffer uint32)       { fnBindBuffer(target, buffer) }

func BufferData(target uint32, size int, data unsafe.Pointer, usage uint32) {
	fnBufferData(target, size, uintptr(data), usage)
}

func BufferSubData(target uint32, offset, size int, data unsafe.Pointer) {
	fnBufferSubData(target, offset, size, uintptr(data))
}

func CreateShader(shaderType uint32) uint32             { return fnCreateShader(shaderType) }
func DeleteShader(shader uint32)                        { fnDeleteShader(shader) }
func CompileShader(shader uint32)                       { fnCompileShader(shader) }
func GetShaderiv(shader, pname uint32, params *int32)   { fnGetShaderiv(shader, pname, params) }
func CreateProgram() uint32                             { return fnCreateProgram() }
func DeleteProgram(program uint32)                      { fnDeleteProgram(program) }
func AttachShader(program, shader uint32)               { fnAttachShader(program, shader) }
func LinkProgram(program uint32)                        { fnLinkProgram(program) }
func UseProgram(program uint32)                         { fnUseProgram(program) }
func GetProgramiv(program, pname uint32, params *int32) { fnGetProgramiv(program, pname, params) }

func ShaderSource(shader uint32, count int32, source **byte, length *int32) {
	fnShaderSource(shader, count, source, length)
}

func GetShaderInfoLog(shader uint32, maxLength int32, length *int32, infoLog *byte) {
	fnGetShaderInfoLog(shader, maxLength, length, infoLog)
}

func GetProgramInfoLog(program uint32, maxLength int32, length *int32, infoLog *byte) {
	fnGetProgramInfoLog(program, maxLength, length, infoLog)
}

func GetUniformLocation(program uint32, name *byte) int32 {
	return fnGetUniformLocation(program, name)
}

func ProgramUniform1f(program uint32, location int32, v0 float32) {
	fnProgramUniform1f(program, location, v0)
}

func ProgramUniform1i(program uint32, location int32, v0 int32) {
	fnProgramUniform1i(program, location, v0)
}

func ProgramUniform2fv(program uint32, location, count int32, value *float32) {
	fnProgramUniform2fv(program, location, count, value)
}

func ProgramUniform4fv(program uint32, location, count int32, value *float32) {
	fnProgramUniform4fv(program, location, count, value)
}

func ProgramUniformMatrix4fv(program uint32, location, count int32, transpose bool, value *float32) {
	t := uint8(0)
	if transpose {
		t = 1
	}
	fnProgramUniformMatrix4fv(program, location, count, t, value)
}

func GenFramebuffers(n int32, fbs *uint32)    { fnGenFramebuffers(n, fbs) }
func DeleteFramebuffers(n int32, fbs *uint32) { fnDeleteFramebuffers(n, fbs) }
func BindFramebuffer(target, fb uint32)       { fnBindFramebuffer(target, fb) }

func FramebufferTexture2D(target, attachment, textarget, texture uint32, level int32) {
	fnFramebufferTexture2D(target, attachment, textarget, texture, level)
}

func CheckFramebufferStatus(target uint32) uint32 {
	return fnCheckFramebufferStatus(target)
}

func GetIntegerv(pname uint32, data *int32) { fnGetIntegerv(pname, data) }

func DrawArrays(mode uint32, first, count int32) { fnDrawArrays(mode, first, count) }

func DrawArraysInstanced(mode uint32, first, count, instancecount int32) {
	fnDrawArraysInstanced(mode, first, count, instancecount)
}

func DrawElements(mode uint32, count int32, typ uint32, indices unsafe.Pointer) {
	fnDrawElements(mode, count, typ, uintptr(indices))
}

func DrawElementsInstanced(mode uint32, count int32, typ uint32, indices unsafe.Pointer, instancecount int32) {
	fnDrawElementsInstanced(mode, count, typ, uintptr(indices), instancecount)
}

func Flush() { fnFlush() }

func GenSamplers(n int32, samplers *uint32)    { fnGenSamplers(n, samplers) }
func DeleteSamplers(n int32, samplers *uint32) { fnDeleteSamplers(n, samplers) }
func BindSampler(unit, sampler uint32)         { fnBindSampler(unit, sampler) }
func SamplerParameteri(sampler, pname uint32, param int32) {
	fnSamplerParameteri(sampler, pname, param)
}

// ---------------------------------------------------------------------------
// String helpers — replace go-gl/gl utility functions
// ---------------------------------------------------------------------------

// GetGoString returns the OpenGL string for the given name (e.g. VERSION).
func GetGoString(name uint32) string {
	ptr := fnGetString(name)
	if ptr == 0 {
		return ""
	}
	return cString(ptr)
}

// Str converts a Go string to a null-terminated C string pointer.
// The returned *byte keeps the backing []byte alive for the GC.
// When passed directly as a purego function argument, the pointer is
// pinned during the call. If stored in a local variable and passed
// indirectly (e.g. &csource), use runtime.KeepAlive after the call.
func Str(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// Ptr returns an unsafe.Pointer to the underlying data of a slice.
func Ptr(data interface{}) unsafe.Pointer {
	if data == nil {
		return nil
	}
	switch v := data.(type) {
	case *uint8:
		return unsafe.Pointer(v)
	case []byte:
		if len(v) == 0 {
			return nil
		}
		return unsafe.Pointer(&v[0])
	case []float32:
		if len(v) == 0 {
			return nil
		}
		return unsafe.Pointer(&v[0])
	case []uint16:
		if len(v) == 0 {
			return nil
		}
		return unsafe.Pointer(&v[0])
	case []uint32:
		if len(v) == 0 {
			return nil
		}
		return unsafe.Pointer(&v[0])
	default:
		panic(fmt.Sprintf("gl.Ptr: unsupported type %T", data))
	}
}

// Strs converts Go strings to null-terminated C strings. Returns a pointer
// to the first element and a free function (no-op since Go manages memory).
func Strs(strs ...string) (cstrs **byte, free func()) {
	ptrs := make([]*byte, len(strs))
	for i, s := range strs {
		ptrs[i] = Str(s)
	}
	return &ptrs[0], func() {}
}

// cString reads a null-terminated C string from a uintptr.
func cString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	var length int
	for {
		if *(*byte)(unsafe.Pointer(ptr + uintptr(length))) == 0 {
			break
		}
		length++
	}
	bs := make([]byte, length)
	for i := range bs {
		bs[i] = *(*byte)(unsafe.Pointer(ptr + uintptr(i)))
	}
	return string(bs)
}

// ---------------------------------------------------------------------------
// Initialization
// ---------------------------------------------------------------------------

// Init loads the OpenGL shared library and resolves all function pointers.
// Must be called after an OpenGL context has been made current.
func Init() error {
	var err error
	lib, err = openGLLib()
	if err != nil {
		return fmt.Errorf("gl: %w", err)
	}

	must := func(fn interface{}, name string) error {
		addr, serr := purego.Dlsym(lib, name)
		if serr != nil {
			return fmt.Errorf("gl: symbol %s: %w", name, serr)
		}
		purego.RegisterFunc(fn, addr)
		return nil
	}

	// GL 1.0–1.1 core functions.
	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnEnable, "glEnable"},
		{&fnDisable, "glDisable"},
		{&fnBlendFunc, "glBlendFunc"},
		{&fnClearColor, "glClearColor"},
		{&fnClearDepth, "glClearDepth"},
		{&fnClear, "glClear"},
		{&fnViewport, "glViewport"},
		{&fnScissor, "glScissor"},
		{&fnDepthFunc, "glDepthFunc"},
		{&fnDepthMask, "glDepthMask"},
		{&fnCullFace, "glCullFace"},
		{&fnGenTextures, "glGenTextures"},
		{&fnDeleteTextures, "glDeleteTextures"},
		{&fnBindTexture, "glBindTexture"},
		{&fnTexParameteri, "glTexParameteri"},
		{&fnTexImage2D, "glTexImage2D"},
		{&fnTexSubImage2D, "glTexSubImage2D"},
		{&fnGetTexImage, "glGetTexImage"},
		{&fnStencilFunc, "glStencilFunc"},
		{&fnStencilOp, "glStencilOp"},
		{&fnStencilMask, "glStencilMask"},
		{&fnClearStencil, "glClearStencil"},
		{&fnColorMask, "glColorMask"},
		{&fnGetIntegerv, "glGetIntegerv"},
		{&fnGetString, "glGetString"},
		{&fnDrawArrays, "glDrawArrays"},
		{&fnDrawElements, "glDrawElements"},
		{&fnFlush, "glFlush"},
	} {
		if ferr := must(e.fn, e.name); ferr != nil {
			return ferr
		}
	}

	// GL 1.3+ functions.
	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnActiveTexture, "glActiveTexture"},
	} {
		if ferr := must(e.fn, e.name); ferr != nil {
			return ferr
		}
	}

	// GL 1.5+ / GL 2.0+ functions (buffers, shaders).
	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnGenBuffers, "glGenBuffers"},
		{&fnDeleteBuffers, "glDeleteBuffers"},
		{&fnBindBuffer, "glBindBuffer"},
		{&fnBufferData, "glBufferData"},
		{&fnBufferSubData, "glBufferSubData"},
		{&fnCreateShader, "glCreateShader"},
		{&fnDeleteShader, "glDeleteShader"},
		{&fnShaderSource, "glShaderSource"},
		{&fnCompileShader, "glCompileShader"},
		{&fnGetShaderiv, "glGetShaderiv"},
		{&fnGetShaderInfoLog, "glGetShaderInfoLog"},
		{&fnCreateProgram, "glCreateProgram"},
		{&fnDeleteProgram, "glDeleteProgram"},
		{&fnAttachShader, "glAttachShader"},
		{&fnLinkProgram, "glLinkProgram"},
		{&fnUseProgram, "glUseProgram"},
		{&fnGetProgramiv, "glGetProgramiv"},
		{&fnGetProgramInfoLog, "glGetProgramInfoLog"},
		{&fnGetUniformLocation, "glGetUniformLocation"},
	} {
		if ferr := must(e.fn, e.name); ferr != nil {
			return ferr
		}
	}

	// GL 3.0+ functions (framebuffers, instancing).
	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnGenFramebuffers, "glGenFramebuffers"},
		{&fnDeleteFramebuffers, "glDeleteFramebuffers"},
		{&fnBindFramebuffer, "glBindFramebuffer"},
		{&fnFramebufferTexture2D, "glFramebufferTexture2D"},
		{&fnCheckFramebufferStatus, "glCheckFramebufferStatus"},
		{&fnDrawArraysInstanced, "glDrawArraysInstanced"},
		{&fnDrawElementsInstanced, "glDrawElementsInstanced"},
		{&fnGenSamplers, "glGenSamplers"},
		{&fnDeleteSamplers, "glDeleteSamplers"},
		{&fnBindSampler, "glBindSampler"},
		{&fnSamplerParameteri, "glSamplerParameteri"},
	} {
		if ferr := must(e.fn, e.name); ferr != nil {
			return ferr
		}
	}

	// GL 4.1 / ARB_separate_shader_objects functions.
	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnProgramUniform1f, "glProgramUniform1f"},
		{&fnProgramUniform1i, "glProgramUniform1i"},
		{&fnProgramUniform2fv, "glProgramUniform2fv"},
		{&fnProgramUniform4fv, "glProgramUniform4fv"},
		{&fnProgramUniformMatrix4fv, "glProgramUniformMatrix4fv"},
	} {
		if ferr := must(e.fn, e.name); ferr != nil {
			return ferr
		}
	}

	return nil
}

// openGLLib opens the platform-specific OpenGL shared library.
func openGLLib() (uintptr, error) {
	var names []string
	switch runtime.GOOS {
	case "darwin":
		names = []string{"/System/Library/Frameworks/OpenGL.framework/OpenGL"}
	case "windows":
		names = []string{"opengl32.dll"}
	default: // linux, freebsd, etc.
		names = []string{"libGL.so.1", "libGL.so"}
	}

	var firstErr error
	for _, name := range names {
		h, err := purego.Dlopen(name, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err == nil {
			return h, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return 0, fmt.Errorf("failed to load OpenGL: %w", firstErr)
}
