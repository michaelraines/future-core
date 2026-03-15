package futurerender

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// ShaderType specifies the shader source language.
type ShaderType int

const (
	// ShaderTypeKage indicates Kage (Ebitengine-compatible) shader source.
	ShaderTypeKage ShaderType = iota
	// ShaderTypeGLSL indicates raw GLSL shader source.
	ShaderTypeGLSL
)

// ShaderReloader watches shader source files for changes and recompiles
// automatically. On recompile error the old shader is kept and the error
// is returned from Update. This is intended for development-time hot-reload
// workflows.
type ShaderReloader struct {
	mu sync.Mutex

	shaderType ShaderType

	// Kage uses a single path; GLSL uses vertPath + fragPath.
	path     string
	vertPath string
	fragPath string

	shader *Shader

	lastModTime     time.Time // single-file (Kage)
	lastVertModTime time.Time // GLSL vertex
	lastFragModTime time.Time // GLSL fragment

	closed bool
}

// NewShaderReloader creates a ShaderReloader that watches the file at path
// for changes and compiles it as a Kage shader. The shader is compiled
// immediately; an error is returned if the initial compilation fails.
func NewShaderReloader(path string, shaderType ShaderType) (*ShaderReloader, error) {
	if shaderType == ShaderTypeGLSL {
		return nil, fmt.Errorf("shader_reload: use NewGLSLShaderReloader for GLSL shaders")
	}

	src, modTime, err := readFileWithModTime(path)
	if err != nil {
		return nil, fmt.Errorf("shader_reload: read %s: %w", path, err)
	}

	shader, err := NewShader(src)
	if err != nil {
		return nil, fmt.Errorf("shader_reload: compile %s: %w", path, err)
	}

	return &ShaderReloader{
		shaderType:  shaderType,
		path:        path,
		shader:      shader,
		lastModTime: modTime,
	}, nil
}

// NewGLSLShaderReloader creates a ShaderReloader that watches vertex and
// fragment GLSL source files for changes. Both files are read and compiled
// immediately; an error is returned if the initial compilation fails.
func NewGLSLShaderReloader(vertPath, fragPath string) (*ShaderReloader, error) {
	vertSrc, vertMod, err := readFileWithModTime(vertPath)
	if err != nil {
		return nil, fmt.Errorf("shader_reload: read %s: %w", vertPath, err)
	}

	fragSrc, fragMod, err := readFileWithModTime(fragPath)
	if err != nil {
		return nil, fmt.Errorf("shader_reload: read %s: %w", fragPath, err)
	}

	shader, err := NewShaderFromGLSL(vertSrc, fragSrc)
	if err != nil {
		return nil, fmt.Errorf("shader_reload: compile GLSL: %w", err)
	}

	return &ShaderReloader{
		shaderType:      ShaderTypeGLSL,
		vertPath:        vertPath,
		fragPath:        fragPath,
		shader:          shader,
		lastVertModTime: vertMod,
		lastFragModTime: fragMod,
	}, nil
}

// Shader returns the current compiled shader. The returned pointer may
// change after a successful Update call that detects file modifications.
func (r *ShaderReloader) Shader() *Shader {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shader
}

// Update checks whether the watched source files have been modified (by
// comparing os.Stat mod times) and recompiles the shader if needed. If
// recompilation fails the previous shader is kept and the error is returned.
// Update is safe to call from the game loop.
func (r *ShaderReloader) Update() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	switch r.shaderType {
	case ShaderTypeKage:
		return r.updateKage()
	case ShaderTypeGLSL:
		return r.updateGLSL()
	default:
		return fmt.Errorf("shader_reload: unknown shader type %d", r.shaderType)
	}
}

func (r *ShaderReloader) updateKage() error {
	info, err := os.Stat(r.path)
	if err != nil {
		return fmt.Errorf("shader_reload: stat %s: %w", r.path, err)
	}

	if !info.ModTime().After(r.lastModTime) {
		return nil
	}

	src, err := os.ReadFile(r.path)
	if err != nil {
		return fmt.Errorf("shader_reload: read %s: %w", r.path, err)
	}

	newShader, err := NewShader(src)
	if err != nil {
		// Keep the old shader, return the compile error.
		return fmt.Errorf("shader_reload: recompile %s: %w", r.path, err)
	}

	r.shader.Deallocate()
	r.shader = newShader
	r.lastModTime = info.ModTime()
	return nil
}

func (r *ShaderReloader) updateGLSL() error {
	vertInfo, err := os.Stat(r.vertPath)
	if err != nil {
		return fmt.Errorf("shader_reload: stat %s: %w", r.vertPath, err)
	}

	fragInfo, err := os.Stat(r.fragPath)
	if err != nil {
		return fmt.Errorf("shader_reload: stat %s: %w", r.fragPath, err)
	}

	vertChanged := vertInfo.ModTime().After(r.lastVertModTime)
	fragChanged := fragInfo.ModTime().After(r.lastFragModTime)

	if !vertChanged && !fragChanged {
		return nil
	}

	vertSrc, err := os.ReadFile(r.vertPath)
	if err != nil {
		return fmt.Errorf("shader_reload: read %s: %w", r.vertPath, err)
	}

	fragSrc, err := os.ReadFile(r.fragPath)
	if err != nil {
		return fmt.Errorf("shader_reload: read %s: %w", r.fragPath, err)
	}

	newShader, err := NewShaderFromGLSL(vertSrc, fragSrc)
	if err != nil {
		return fmt.Errorf("shader_reload: recompile GLSL: %w", err)
	}

	r.shader.Deallocate()
	r.shader = newShader
	r.lastVertModTime = vertInfo.ModTime()
	r.lastFragModTime = fragInfo.ModTime()
	return nil
}

// Close stops the reloader. After Close, Update is a no-op. Close does not
// deallocate the current shader — the caller is responsible for calling
// Shader().Deallocate() when the shader is no longer needed.
func (r *ShaderReloader) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
}

// readFileWithModTime reads a file and returns its contents along with its
// modification time.
func readFileWithModTime(path string) ([]byte, time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, time.Time{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, err
	}

	return data, info.ModTime(), nil
}
