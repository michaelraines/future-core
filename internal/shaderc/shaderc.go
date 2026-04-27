//go:build (darwin || linux || freebsd || windows || android) && !soft

// Package shaderc provides pure-Go bindings to libshaderc for compiling
// GLSL to SPIR-V bytecode. Loaded at runtime via purego — no CGo required.
package shaderc

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Shader stage constants matching shaderc_shader_kind enum.
const (
	StageVertex   = 0 // shaderc_vertex_shader
	StageFragment = 1 // shaderc_fragment_shader
)

// Compilation status constants.
const (
	StatusSuccess          = 0
	StatusInvalidStage     = 1
	StatusCompilationError = 2
	StatusInternalError    = 3
)

// Uniform kind constants for set_binding_base_for_stage.
const (
	UniformKindImage   = 0
	UniformKindSampler = 1
	UniformKindTexture = 2
	UniformKindBuffer  = 3 // UBO
	UniformKindSSBO    = 4
	UniformKindUAV     = 5
)

// Function pointers loaded from libshaderc.
var (
	fnCompilerInit    func() uintptr
	fnCompilerRelease func(compiler uintptr)
	fnCompileIntoSpv  func(compiler uintptr, source *byte, sourceLen uint64, kind int32, inputFile *byte, entryPoint *byte, options uintptr) uintptr
	fnResultRelease   func(result uintptr)
	fnResultGetStatus func(result uintptr) int32
	fnResultGetLength func(result uintptr) uint64
	fnResultGetBytes  func(result uintptr) uintptr
	fnResultGetError  func(result uintptr) uintptr

	fnOptionsInit              func() uintptr
	fnOptionsRelease           func(options uintptr)
	fnOptionsSetAutoMapLocs    func(options uintptr, enable int32)
	fnOptionsSetAutoCombineSmp func(options uintptr, enable int32)
	fnOptionsSetAutoBindUni    func(options uintptr, enable int32)
	fnOptionsSetVulkanRelaxed  func(options uintptr, enable int32)
	fnOptionsSetBindBaseStage  func(options uintptr, shaderKind int32, uniformKind int32, base uint32)

	initialized bool
	compiler    uintptr
)

// Init loads libshaderc and initializes the compiler.
func Init() error {
	if initialized {
		return nil
	}

	lib, err := openLib()
	if err != nil {
		return fmt.Errorf("shaderc: %w", err)
	}

	resolve := func(fn any, name string) error {
		sym, e := purego.Dlsym(lib, name)
		if e != nil {
			return fmt.Errorf("shaderc: resolve %s: %w", name, e)
		}
		purego.RegisterFunc(fn, sym)
		return nil
	}

	for _, e := range []struct {
		fn   any
		name string
	}{
		{&fnCompilerInit, "shaderc_compiler_initialize"},
		{&fnCompilerRelease, "shaderc_compiler_release"},
		{&fnCompileIntoSpv, "shaderc_compile_into_spv"},
		{&fnResultRelease, "shaderc_result_release"},
		{&fnResultGetStatus, "shaderc_result_get_compilation_status"},
		{&fnResultGetLength, "shaderc_result_get_length"},
		{&fnResultGetBytes, "shaderc_result_get_bytes"},
		{&fnResultGetError, "shaderc_result_get_error_message"},
		{&fnOptionsInit, "shaderc_compile_options_initialize"},
		{&fnOptionsRelease, "shaderc_compile_options_release"},
		{&fnOptionsSetAutoMapLocs, "shaderc_compile_options_set_auto_map_locations"},
		{&fnOptionsSetAutoCombineSmp, "shaderc_compile_options_set_auto_combined_image_sampler"},
		{&fnOptionsSetAutoBindUni, "shaderc_compile_options_set_auto_bind_uniforms"},
		{&fnOptionsSetVulkanRelaxed, "shaderc_compile_options_set_vulkan_rules_relaxed"},
		{&fnOptionsSetBindBaseStage, "shaderc_compile_options_set_binding_base_for_stage"},
	} {
		if err := resolve(e.fn, e.name); err != nil {
			return err
		}
	}

	compiler = fnCompilerInit()
	if compiler == 0 {
		return fmt.Errorf("shaderc: failed to initialize compiler")
	}

	initialized = true
	return nil
}

// CompileGLSL compiles GLSL source to SPIR-V bytecode.
// stage is StageVertex or StageFragment.
func CompileGLSL(source string, stage int) ([]byte, error) {
	if !initialized {
		if err := Init(); err != nil {
			return nil, err
		}
	}

	srcBytes := cstr(source)
	fileName := cstr("shader")
	entryPoint := cstr("main")

	// Create options with auto-mapping for uniform locations and
	// combined image samplers (GLSL sampler2D → Vulkan combined sampler).
	options := fnOptionsInit()
	if options != 0 {
		fnOptionsSetAutoMapLocs(options, 1)
		fnOptionsSetAutoCombineSmp(options, 1)
		fnOptionsSetAutoBindUni(options, 1)
		fnOptionsSetVulkanRelaxed(options, 1)
		// Separate binding indices to avoid conflicts between stages:
		//   binding 0: combined image sampler (fragment)
		//   binding 1: fragment UBO
		//   binding 2: vertex UBO
		fnOptionsSetBindBaseStage(options, int32(StageFragment), int32(UniformKindBuffer), 1)
		fnOptionsSetBindBaseStage(options, int32(StageVertex), int32(UniformKindBuffer), 2)
		defer fnOptionsRelease(options)
	}

	result := fnCompileIntoSpv(compiler, srcBytes, uint64(len(source)),
		int32(stage), fileName, entryPoint, options)
	if result == 0 {
		return nil, fmt.Errorf("shaderc: compilation returned nil")
	}
	defer fnResultRelease(result)

	runtime.KeepAlive(srcBytes)
	runtime.KeepAlive(fileName)
	runtime.KeepAlive(entryPoint)

	status := fnResultGetStatus(result)
	if status != StatusSuccess {
		errPtr := fnResultGetError(result)
		errMsg := ""
		if errPtr != 0 {
			errMsg = goString(errPtr)
		}
		return nil, fmt.Errorf("shaderc: compilation failed (status %d): %s", status, errMsg)
	}

	length := fnResultGetLength(result)
	bytesPtr := fnResultGetBytes(result)
	if bytesPtr == 0 || length == 0 {
		return nil, fmt.Errorf("shaderc: empty result")
	}

	// Copy SPIR-V bytes out before result is released.
	spirv := make([]byte, length)
	copy(spirv, unsafe.Slice((*byte)(unsafe.Pointer(bytesPtr)), length))

	return spirv, nil
}

// cstr converts a Go string to a null-terminated C string.
func cstr(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// goString converts a C string pointer to a Go string.
func goString(p uintptr) string {
	if p == 0 {
		return ""
	}
	var length int
	for {
		b := *(*byte)(unsafe.Pointer(p + uintptr(length)))
		if b == 0 {
			break
		}
		length++
		if length > 4096 {
			break
		}
	}
	buf := make([]byte, length)
	copy(buf, unsafe.Slice((*byte)(unsafe.Pointer(p)), length))
	return string(buf)
}

// openLib loads the shaderc shared library.
func openLib() (uintptr, error) {
	var names []string
	switch runtime.GOOS {
	case "darwin":
		names = []string{
			"/opt/homebrew/lib/libshaderc_shared.1.dylib",
			"/opt/homebrew/lib/libshaderc_shared.dylib",
			"/usr/local/lib/libshaderc_shared.1.dylib",
			"/usr/local/lib/libshaderc_shared.dylib",
			"libshaderc_shared.1.dylib",
			"libshaderc_shared.dylib",
		}
	case "linux", "freebsd":
		// Two soname conventions seen in the wild:
		//   - upstream wgpu / Vulkan SDK release tarballs ship as
		//     `libshaderc_shared.so{,.1}`
		//   - Debian/Ubuntu's `libshaderc1` package installs as
		//     `libshaderc.so{,.1}` (no `_shared` suffix). Honour
		//     both so the package-managed install on stock distros
		//     works without a manual symlink.
		names = []string{
			"libshaderc_shared.so.1",
			"libshaderc_shared.so",
			"libshaderc.so.1",
			"libshaderc.so",
		}
	case "windows":
		names = []string{"shaderc_shared.dll"}
	}
	for _, name := range names {
		h, err := purego.Dlopen(name, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err == nil {
			return h, nil
		}
	}
	return 0, fmt.Errorf("failed to load libshaderc (tried %v)", names)
}
