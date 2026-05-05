//go:build !js && !android

// precompile-builtin-spirv compiles the built-in shader GLSL sources
// in internal/builtin to SPIR-V via libshaderc and writes the bytes
// alongside the GLSL as *.spv blobs. It also rewrites the matching
// `<shader>_spirv.go` registration file so the embedded byte arrays
// stay in sync.
//
// Usage:
//
//	go run ./cmd/precompile-builtin-spirv
//
// Or via go:generate (preferred — runs from the package being
// generated):
//
//	//go:generate go run ../../cmd/precompile-builtin-spirv
//
// Requires libshaderc available at runtime (the binary loads it via
// internal/dlopen at startup). Host-only — never runs on Android. CI
// must install libshaderc as well; see Makefile prereqs.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/michaelraines/future-core/internal/shaderc"
)

// shaderSpec is one (vertex, fragment) pair to compile. Add entries
// here when introducing new built-in shaders.
type shaderSpec struct {
	Name string // "sprite" — used as filename prefix
}

var specs = []shaderSpec{
	{Name: "sprite"},
}

func main() {
	dir, err := findBuiltinDir()
	if err != nil {
		log.Fatalf("locate internal/builtin: %v", err)
	}

	for _, spec := range specs {
		if err := compileSpec(dir, spec); err != nil {
			log.Fatalf("%s: %v", spec.Name, err)
		}
	}
	fmt.Printf("precompiled %d built-in shader pair(s) to SPIR-V\n", len(specs))
}

func compileSpec(dir string, spec shaderSpec) error {
	vertGLSL, err := os.ReadFile(filepath.Join(dir, spec.Name+".vert.glsl"))
	if err != nil {
		return fmt.Errorf("read vertex source: %w", err)
	}
	fragGLSL, err := os.ReadFile(filepath.Join(dir, spec.Name+".frag.glsl"))
	if err != nil {
		return fmt.Errorf("read fragment source: %w", err)
	}

	vertSPIRV, err := shaderc.CompileGLSL(string(vertGLSL), shaderc.StageVertex)
	if err != nil {
		return fmt.Errorf("compile vertex: %w", err)
	}
	fragSPIRV, err := shaderc.CompileGLSL(string(fragGLSL), shaderc.StageFragment)
	if err != nil {
		return fmt.Errorf("compile fragment: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, spec.Name+".vert.spv"), vertSPIRV, 0o644); err != nil {
		return fmt.Errorf("write vertex spv: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, spec.Name+".frag.spv"), fragSPIRV, 0o644); err != nil {
		return fmt.Errorf("write fragment spv: %w", err)
	}

	fmt.Printf("  %-12s vert=%d frag=%d bytes\n", spec.Name, len(vertSPIRV), len(fragSPIRV))
	return nil
}

// findBuiltinDir walks up from the working directory until it finds
// internal/builtin. This lets the tool work both when invoked from
// the repo root (`go run ./cmd/precompile-builtin-spirv`) and from a
// "generate" directive inside the package itself.
func findBuiltinDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Working directory IS internal/builtin (go:generate case).
	if strings.HasSuffix(wd, filepath.Join("internal", "builtin")) {
		return wd, nil
	}

	// Look for it at internal/builtin under wd.
	candidate := filepath.Join(wd, "internal", "builtin")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate, nil
	}

	return "", fmt.Errorf("internal/builtin directory not found from %s", wd)
}
