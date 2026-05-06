//go:build (darwin || linux || freebsd || windows) && !soft

// precompile-kage-shaders walks a directory tree for *.kage shader
// sources and emits per-backend native variants alongside each:
//
//	<name>.vert.glsl    + <name>.frag.glsl     (desktop GL / shaderc input)
//	<name>.vert.glsles  + <name>.frag.glsles   (WebGL2)
//	<name>.vert.wgsl    + <name>.frag.wgsl     (WebGPU)
//	<name>.vert.msl     + <name>.frag.msl      (Metal)
//	<name>.vert.spv     + <name>.frag.spv      (Vulkan, via shaderc)
//
// Why precompile: the Kage→target translation pipeline (shaderir +
// shadertranslate) is pure Go and runs anywhere — including Android —
// so in principle every backend could translate at runtime. But:
//
//   - Vulkan-on-Android specifically REQUIRES SPIR-V at vkCreateShaderModule
//     and libshaderc isn't bundled in the AAR (~5 MB per ABI). Without
//     a precompiled .spv, runtime Kage shaders fail silently on Android.
//
//   - On every other backend, the precompiled native variant lets the
//     engine skip the runtime translator step — faster startup, no
//     surprise translator divergence between dev/prod (e.g. a
//     translator bug-fix in future-core that would change a shader's
//     output won't ship until precompile runs again).
//
//   - The native variants become editable as hand-written sources for
//     backend-specific tuning. Re-running the tool keeps them in sync
//     with the Kage source unless `-skip-existing` preserves hand work.
//
// At runtime, libs/shaders.LoadFromFile (in the future framework)
// builds a rendering.ShaderSource with whichever native variants are
// present alongside the .kage source, and dispatches via
// rendering.GetShaderCompiler().NewShaderFromSource — the multi-
// language registry path that the built-in shaders already use. No
// runtime Kage parsing happens when a matching native variant exists.
//
// Usage:
//
//	go run github.com/michaelraines/future-core/cmd/precompile-kage-shaders \
//	    -dir examples
//
// Idempotent — output files are rewritten only when their content
// changes. Pass `-skip-existing` to preserve hand-written variants
// when re-running.
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
	"github.com/michaelraines/future-core/internal/shaderir"
	"github.com/michaelraines/future-core/internal/shadertranslate"
)

type stats struct {
	scanned int
	updated int
	skipped int
}

func main() {
	dir := flag.String("dir", ".", "directory to walk for *.kage files")
	verbose := flag.Bool("v", false, "print every compile, not just changes")
	skipExisting := flag.Bool("skip-existing", false, "preserve files that already exist on disk")
	skipSPIRV := flag.Bool("skip-spirv", false, "skip SPIR-V emit (set when shaderc isn't available)")
	flag.Parse()

	var s stats
	err := filepath.WalkDir(*dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".kage") {
			return nil
		}
		s.scanned++
		if err := compileOne(path, *verbose, *skipExisting, *skipSPIRV, &s); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d kage source(s) scanned, %d file(s) updated, %d preserved\n",
		s.scanned, s.updated, s.skipped)
}

// compileOne runs every available translator+compiler against a single
// .kage source, writing the per-language variants alongside.
func compileOne(path string, verbose, skipExisting, skipSPIRV bool, s *stats) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	result, err := shaderir.Compile(src)
	if err != nil {
		return fmt.Errorf("kage compile: %w", err)
	}
	base := strings.TrimSuffix(path, ".kage")

	// Phase 1 (this PR): SPIR-V only — that's the variant Android needs
	// (shaderc unavailable at runtime in the AAR). Other backends keep
	// going through the runtime Kage→native translator on
	// rendering.GetShaderCompiler().NewShader, which works fine because
	// those translators are pure Go and run everywhere.
	//
	// Phase 2 follow-up: extend to emit .wgsl / .msl / .glsl / .glsles
	// variants too, so each backend can skip its runtime translator
	// step. Requires precompile-side struct-combining to share a
	// single UBO across vertex+fragment stages — the bare translator
	// output in shadertranslate emits per-stage structs that
	// NewShaderNative can't consume directly. Tracked separately.

	// SPIR-V — Vulkan. Requires shaderc on the build host.
	//
	// The runtime engine's NewShaderNative SPIR-V path packs uniforms
	// using a single combined std140 layout shared by both stages
	// (matches the explicit-UBO pattern the built-in sprite shader
	// uses). shaderc with auto_bind_uniforms produces per-stage UBOs
	// at offsets that DON'T match std140-combined — fragment-only
	// uniforms start at offset 0 in the fragment UBO even when the
	// std140 combined layout puts them at offset 64+. To produce
	// SPIR-V the native path can read correctly, wrap both stages'
	// GLSL in an explicit `layout(std140, binding=0) uniform UBO {...}`
	// block before sending to shaderc.
	if !skipSPIRV {
		uboFields, err := shadertranslate.ExtractUniformLayout(result.VertexShader + "\n" + result.FragmentShader)
		if err != nil {
			return fmt.Errorf("uniform layout: %w", err)
		}
		vertGLSLForSPV := wrapLooseUniformsInUBO(result.VertexShader, uboFields)
		fragGLSLForSPV := wrapLooseUniformsInUBO(result.FragmentShader, uboFields)
		vSPV, err := shaderc.CompileGLSL(vertGLSLForSPV, shaderc.StageVertex)
		if err != nil {
			return fmt.Errorf("vertex shaderc: %w", err)
		}
		fSPV, err := shaderc.CompileGLSL(fragGLSLForSPV, shaderc.StageFragment)
		if err != nil {
			return fmt.Errorf("fragment shaderc: %w", err)
		}
		if err := writeIfChanged(base+".vert.spv", vSPV, verbose, skipExisting, s); err != nil {
			return err
		}
		if err := writeIfChanged(base+".frag.spv", fSPV, verbose, skipExisting, s); err != nil {
			return err
		}
	}
	return nil
}

func writeIfChanged(path string, data []byte, verbose, skipExisting bool, s *stats) error {
	if skipExisting {
		if _, err := os.Stat(path); err == nil {
			s.skipped++
			if verbose {
				fmt.Printf("  preserved: %s\n", path)
			}
			return nil
		}
	}
	prev, _ := os.ReadFile(path)
	if bytes.Equal(prev, data) {
		if verbose {
			fmt.Printf("  unchanged: %s\n", path)
		}
		return nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	s.updated++
	fmt.Printf("  %s (%d bytes)\n", path, len(data))
	return nil
}

// wrapLooseUniformsInUBO rewrites a GLSL fragment/vertex source so
// every non-sampler `uniform <type> <name>;` declaration is bundled
// into a single explicit `layout(std140, binding=0) uniform UBO {...}`
// block whose member order matches uboFields. Sampler declarations are
// preserved at file scope (they bind via descriptor sets, not the UBO).
//
// Why bundle: shaderc's auto_bind_uniforms packs each stage's loose
// uniforms into per-stage UBOs with stage-local offsets — fragment-
// only uniforms start at offset 0 in the fragment UBO, even when the
// engine expects a combined std140 layout where uProjection at offset
// 0 belongs to the vertex stage. With the combined layout the engine's
// per-draw uniform packer writes (e.g.) LightColor/Intensity at
// offsets the fragment shader can't read, producing visibly missing
// effects — same root cause as the Android-Adreno lighting bug fixed
// for built-in shaders by writing explicit UBO blocks by hand.
//
// Bumps the GLSL #version to 450 core if it wasn't already (the
// `binding=0` qualifier requires it). The Kage emitter targets 330
// by default.
func wrapLooseUniformsInUBO(glsl string, uboFields []shadertranslate.UniformField) string {
	if len(uboFields) == 0 {
		return glsl
	}
	out := upgradeGLSLVersionTo450(glsl)

	// Build the UBO block.
	var ubo strings.Builder
	ubo.WriteString("layout(std140, binding = 0) uniform UBO {\n")
	for _, f := range uboFields {
		fmt.Fprintf(&ubo, "    %s %s;\n", f.Type, f.Name)
	}
	ubo.WriteString("};\n")

	// Strip loose `uniform <type> <name>;` lines whose <name> is in the
	// UBO's member list. Samplers stay (they're not in uboFields). Walk
	// line-by-line so multi-line strips stay surgical.
	wanted := make(map[string]bool, len(uboFields))
	for _, f := range uboFields {
		wanted[f.Name] = true
	}
	lines := strings.Split(out, "\n")
	kept := make([]string, 0, len(lines))
	uboInjected := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Detect bare loose uniform: `uniform <type> <name>;`
		if strings.HasPrefix(trimmed, "uniform ") && strings.HasSuffix(trimmed, ";") {
			body := strings.TrimSuffix(strings.TrimPrefix(trimmed, "uniform "), ";")
			parts := strings.Fields(body)
			if len(parts) == 2 && wanted[parts[1]] {
				if !uboInjected {
					kept = append(kept, ubo.String())
					uboInjected = true
				}
				continue
			}
		}
		kept = append(kept, line)
	}
	// Edge case: file had no loose uniforms in uboFields (already-packed
	// or no UBO members). Still inject the block right after #version.
	if !uboInjected {
		insertAt := -1
		for i, line := range kept {
			if strings.HasPrefix(strings.TrimSpace(line), "#version") {
				insertAt = i + 1
				break
			}
		}
		if insertAt >= 0 {
			kept = append(kept[:insertAt], append([]string{ubo.String()}, kept[insertAt:]...)...)
		}
	}
	return strings.Join(kept, "\n")
}

func upgradeGLSLVersionTo450(glsl string) string {
	for _, old := range []string{"#version 330 core", "#version 330", "#version 450"} {
		if i := strings.Index(glsl, old); i >= 0 {
			return glsl[:i] + "#version 450 core" + glsl[i+len(old):]
		}
	}
	return glsl
}
