//go:build (darwin || linux || freebsd || windows) && !soft

// precompile-glsl-spirv walks a directory tree for `*.vert.glsl` and
// `*.frag.glsl` files, compiles each to SPIR-V via internal/shaderc,
// and writes the bytes alongside as `*.vert.spv` / `*.frag.spv`.
//
// Driven by the future-side scripts/regen-spirv.sh — runs whenever a
// `.glsl` source changes. The Vulkan backend's NewShaderNative SPIR-V
// path consumes the resulting bytes directly via vkCreateShaderModule,
// skipping shaderc at runtime — required on Android, where libshaderc
// is not available, and a startup-time win on every other Vulkan-capable
// platform.
//
// Why this can't be a simple `glslc` wrapper: the source GLSL files
// across libs/shaders + libs/comp/lighting are written in plain
// desktop-GL dialect (bare `uniform mat4 uProjection;`, no UBO blocks,
// no auto-binding qualifiers). glslc's Vulkan target rejects that
// shape; only shaderc's `set_vulkan_rules_relaxed` + auto-bind /
// auto-map / auto-combine options accept it. internal/shaderc already
// turns those on, so we drive compilation through it.
//
// Usage:
//
//	go run github.com/michaelraines/future-core/cmd/precompile-glsl-spirv \
//	    -dir libs
//
// Re-runs are idempotent — the .spv blob is rewritten only when the
// source GLSL has changed.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/michaelraines/future-core/internal/shaderc"
)

func main() {
	dir := flag.String("dir", ".", "directory to walk for *.{vert,frag}.glsl files")
	verbose := flag.Bool("v", false, "print every compile, not just changes")
	flag.Parse()

	count, changed := 0, 0
	err := filepath.WalkDir(*dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		stage, ok := stageOf(path)
		if !ok {
			return nil
		}
		count++

		updated, err := compileOne(path, stage, *verbose)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if updated {
			changed++
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d shader source(s) scanned, %d .spv blob(s) updated\n", count, changed)
}

func stageOf(path string) (int, bool) {
	switch {
	case strings.HasSuffix(path, ".vert.glsl"):
		return shaderc.StageVertex, true
	case strings.HasSuffix(path, ".frag.glsl"):
		return shaderc.StageFragment, true
	}
	return 0, false
}

func compileOne(path string, stage int, verbose bool) (bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	spirv, err := shaderc.CompileGLSL(string(src), stage)
	if err != nil {
		return false, err
	}

	out := strings.TrimSuffix(path, ".glsl") + ".spv"
	prev, _ := os.ReadFile(out)
	if bytes.Equal(prev, spirv) {
		if verbose {
			fmt.Printf("  unchanged: %s (%d bytes)\n", out, len(spirv))
		}
		return false, nil
	}
	if err := os.WriteFile(out, spirv, 0o644); err != nil {
		return false, err
	}
	fmt.Printf("  %s (%d bytes)\n", out, len(spirv))
	return true, nil
}
