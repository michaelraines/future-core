package futurerender

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// validGLSLVert is a minimal GLSL vertex shader for testing.
var validGLSLVert = []byte("#version 330 core\nvoid main() { gl_Position = vec4(0); }")

// validGLSLFrag is a minimal GLSL fragment shader for testing.
var validGLSLFrag = []byte("#version 330 core\nout vec4 c; void main() { c = vec4(1); }")

// validKageSrc is a minimal Kage shader for testing.
var validKageSrc = []byte(`//go:build ignore

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return imageSrc0At(srcPos)
}
`)

func TestNewShaderReloaderKage(t *testing.T) {
	_ = withShaderRenderer(t)

	path := filepath.Join(t.TempDir(), "test.kage")
	require.NoError(t, os.WriteFile(path, validKageSrc, 0o644))

	r, err := NewShaderReloader(path, ShaderTypeKage)
	require.NoError(t, err)
	require.NotNil(t, r)
	require.NotNil(t, r.Shader())
	require.Greater(t, r.Shader().ID(), uint32(0))

	r.Close()
}

func TestNewShaderReloaderGLSLErrors(t *testing.T) {
	_ = withShaderRenderer(t)

	// ShaderTypeGLSL should be rejected by NewShaderReloader.
	_, err := NewShaderReloader("/nonexistent", ShaderTypeGLSL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "use NewGLSLShaderReloader")
}

func TestNewShaderReloaderFileNotFound(t *testing.T) {
	_ = withShaderRenderer(t)

	_, err := NewShaderReloader("/nonexistent/shader.kage", ShaderTypeKage)
	require.Error(t, err)
}

func TestNewShaderReloaderInvalidSource(t *testing.T) {
	_ = withShaderRenderer(t)

	path := filepath.Join(t.TempDir(), "bad.kage")
	require.NoError(t, os.WriteFile(path, []byte("not valid kage"), 0o644))

	_, err := NewShaderReloader(path, ShaderTypeKage)
	require.Error(t, err)
	require.Contains(t, err.Error(), "compile")
}

func TestNewGLSLShaderReloader(t *testing.T) {
	_ = withShaderRenderer(t)

	dir := t.TempDir()
	vertPath := filepath.Join(dir, "test.vert")
	fragPath := filepath.Join(dir, "test.frag")
	require.NoError(t, os.WriteFile(vertPath, validGLSLVert, 0o644))
	require.NoError(t, os.WriteFile(fragPath, validGLSLFrag, 0o644))

	r, err := NewGLSLShaderReloader(vertPath, fragPath)
	require.NoError(t, err)
	require.NotNil(t, r)
	require.NotNil(t, r.Shader())

	r.Close()
}

func TestNewGLSLShaderReloaderVertNotFound(t *testing.T) {
	_ = withShaderRenderer(t)

	fragPath := filepath.Join(t.TempDir(), "test.frag")
	require.NoError(t, os.WriteFile(fragPath, validGLSLFrag, 0o644))

	_, err := NewGLSLShaderReloader("/nonexistent/test.vert", fragPath)
	require.Error(t, err)
}

func TestNewGLSLShaderReloaderFragNotFound(t *testing.T) {
	_ = withShaderRenderer(t)

	vertPath := filepath.Join(t.TempDir(), "test.vert")
	require.NoError(t, os.WriteFile(vertPath, validGLSLVert, 0o644))

	_, err := NewGLSLShaderReloader(vertPath, "/nonexistent/test.frag")
	require.Error(t, err)
}

func TestShaderReloaderUpdateNoChange(t *testing.T) {
	_ = withShaderRenderer(t)

	dir := t.TempDir()
	vertPath := filepath.Join(dir, "test.vert")
	fragPath := filepath.Join(dir, "test.frag")
	require.NoError(t, os.WriteFile(vertPath, validGLSLVert, 0o644))
	require.NoError(t, os.WriteFile(fragPath, validGLSLFrag, 0o644))

	r, err := NewGLSLShaderReloader(vertPath, fragPath)
	require.NoError(t, err)

	originalShader := r.Shader()

	// Update without modifying files — shader should remain the same.
	require.NoError(t, r.Update())
	require.Equal(t, originalShader, r.Shader())

	r.Close()
}

func TestShaderReloaderUpdateGLSLRecompile(t *testing.T) {
	dev := withShaderRenderer(t)

	dir := t.TempDir()
	vertPath := filepath.Join(dir, "test.vert")
	fragPath := filepath.Join(dir, "test.frag")
	require.NoError(t, os.WriteFile(vertPath, validGLSLVert, 0o644))
	require.NoError(t, os.WriteFile(fragPath, validGLSLFrag, 0o644))

	r, err := NewGLSLShaderReloader(vertPath, fragPath)
	require.NoError(t, err)

	originalShader := r.Shader()
	originalID := originalShader.ID()
	initialShaderCount := len(dev.shaders)

	// Modify the fragment file with a future mod time to ensure change is detected.
	futureTime := time.Now().Add(2 * time.Second)
	newFrag := []byte("#version 330 core\nout vec4 c; void main() { c = vec4(0.5); }")
	require.NoError(t, os.WriteFile(fragPath, newFrag, 0o644))
	require.NoError(t, os.Chtimes(fragPath, futureTime, futureTime))

	require.NoError(t, r.Update())

	// Should have created a new shader.
	require.NotEqual(t, originalID, r.Shader().ID())
	require.Greater(t, len(dev.shaders), initialShaderCount)

	r.Close()
}

func TestShaderReloaderUpdateKageRecompile(t *testing.T) {
	dev := withShaderRenderer(t)

	path := filepath.Join(t.TempDir(), "test.kage")
	require.NoError(t, os.WriteFile(path, validKageSrc, 0o644))

	r, err := NewShaderReloader(path, ShaderTypeKage)
	require.NoError(t, err)

	originalID := r.Shader().ID()
	initialShaderCount := len(dev.shaders)

	// Modify source with new content and a future mod time.
	newSrc := []byte(`//go:build ignore

package main

var Time float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return imageSrc0At(srcPos)
}
`)
	futureTime := time.Now().Add(2 * time.Second)
	require.NoError(t, os.WriteFile(path, newSrc, 0o644))
	require.NoError(t, os.Chtimes(path, futureTime, futureTime))

	require.NoError(t, r.Update())
	require.NotEqual(t, originalID, r.Shader().ID())
	require.Greater(t, len(dev.shaders), initialShaderCount)

	r.Close()
}

func TestShaderReloaderUpdateInvalidKeepsOldShader(t *testing.T) {
	_ = withShaderRenderer(t)

	path := filepath.Join(t.TempDir(), "test.kage")
	require.NoError(t, os.WriteFile(path, validKageSrc, 0o644))

	r, err := NewShaderReloader(path, ShaderTypeKage)
	require.NoError(t, err)

	originalShader := r.Shader()
	originalID := originalShader.ID()

	// Write invalid source with future mod time.
	futureTime := time.Now().Add(2 * time.Second)
	require.NoError(t, os.WriteFile(path, []byte("not valid kage"), 0o644))
	require.NoError(t, os.Chtimes(path, futureTime, futureTime))

	err = r.Update()
	require.Error(t, err)
	require.Contains(t, err.Error(), "recompile")

	// Old shader should be preserved.
	require.Equal(t, originalID, r.Shader().ID())

	r.Close()
}

func TestShaderReloaderUpdateGLSLInvalidKeepsOldShader(t *testing.T) {
	_ = withShaderRenderer(t)

	dir := t.TempDir()
	vertPath := filepath.Join(dir, "test.vert")
	fragPath := filepath.Join(dir, "test.frag")
	require.NoError(t, os.WriteFile(vertPath, validGLSLVert, 0o644))
	require.NoError(t, os.WriteFile(fragPath, validGLSLFrag, 0o644))

	r, err := NewGLSLShaderReloader(vertPath, fragPath)
	require.NoError(t, err)

	originalID := r.Shader().ID()

	// The mock device always succeeds, so we test by removing the file
	// to cause a read error instead.
	futureTime := time.Now().Add(2 * time.Second)
	require.NoError(t, os.WriteFile(fragPath, validGLSLFrag, 0o644))
	require.NoError(t, os.Chtimes(fragPath, futureTime, futureTime))

	// Remove frag file after stat succeeds but before read — we simulate
	// by removing the vert file so ReadFile fails.
	require.NoError(t, os.Remove(vertPath))

	err = r.Update()
	require.Error(t, err)

	// Old shader preserved.
	require.Equal(t, originalID, r.Shader().ID())

	r.Close()
}

func TestShaderReloaderCloseStopsUpdates(t *testing.T) {
	_ = withShaderRenderer(t)

	path := filepath.Join(t.TempDir(), "test.kage")
	require.NoError(t, os.WriteFile(path, validKageSrc, 0o644))

	r, err := NewShaderReloader(path, ShaderTypeKage)
	require.NoError(t, err)

	originalID := r.Shader().ID()

	r.Close()

	// Modify the file.
	newSrc := []byte(`//go:build ignore

package main

var Time float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return imageSrc0At(srcPos)
}
`)
	futureTime := time.Now().Add(2 * time.Second)
	require.NoError(t, os.WriteFile(path, newSrc, 0o644))
	require.NoError(t, os.Chtimes(path, futureTime, futureTime))

	// Update after Close should be a no-op.
	require.NoError(t, r.Update())
	require.Equal(t, originalID, r.Shader().ID())
}

func TestShaderReloaderDoubleClose(t *testing.T) {
	_ = withShaderRenderer(t)

	path := filepath.Join(t.TempDir(), "test.kage")
	require.NoError(t, os.WriteFile(path, validKageSrc, 0o644))

	r, err := NewShaderReloader(path, ShaderTypeKage)
	require.NoError(t, err)

	// Double close should be safe.
	r.Close()
	r.Close()
}

func TestShaderReloaderUpdateKageStatError(t *testing.T) {
	_ = withShaderRenderer(t)

	path := filepath.Join(t.TempDir(), "test.kage")
	require.NoError(t, os.WriteFile(path, validKageSrc, 0o644))

	r, err := NewShaderReloader(path, ShaderTypeKage)
	require.NoError(t, err)

	// Remove the file so Stat fails.
	require.NoError(t, os.Remove(path))

	err = r.Update()
	require.Error(t, err)
	require.Contains(t, err.Error(), "stat")

	r.Close()
}

func TestShaderReloaderUpdateGLSLStatErrors(t *testing.T) {
	_ = withShaderRenderer(t)

	dir := t.TempDir()
	vertPath := filepath.Join(dir, "test.vert")
	fragPath := filepath.Join(dir, "test.frag")
	require.NoError(t, os.WriteFile(vertPath, validGLSLVert, 0o644))
	require.NoError(t, os.WriteFile(fragPath, validGLSLFrag, 0o644))

	r, err := NewGLSLShaderReloader(vertPath, fragPath)
	require.NoError(t, err)

	// Remove vert file — stat should fail.
	require.NoError(t, os.Remove(vertPath))
	err = r.Update()
	require.Error(t, err)
	require.Contains(t, err.Error(), "stat")

	// Restore vert, remove frag — stat should fail for frag.
	require.NoError(t, os.WriteFile(vertPath, validGLSLVert, 0o644))
	require.NoError(t, os.Remove(fragPath))
	err = r.Update()
	require.Error(t, err)
	require.Contains(t, err.Error(), "stat")

	r.Close()
}

func TestReadFileWithModTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	content := []byte("hello world")
	require.NoError(t, os.WriteFile(path, content, 0o644))

	data, modTime, err := readFileWithModTime(path)
	require.NoError(t, err)
	require.Equal(t, content, data)
	require.False(t, modTime.IsZero())
}

func TestReadFileWithModTimeNotFound(t *testing.T) {
	_, _, err := readFileWithModTime("/nonexistent/file.txt")
	require.Error(t, err)
}
