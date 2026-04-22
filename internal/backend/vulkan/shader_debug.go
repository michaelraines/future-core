//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/michaelraines/future-core/internal/shadertranslate"
)

// Env-gated diagnostic helpers for custom Kage shaders. Each is zero-cost
// when its env var is unset (one string lookup per shader compile / per
// pack, no hot-path cost). See future-core/CLAUDE.md for the full table
// of FUTURE_CORE_* debug knobs.

// FUTURE_CORE_VK_DUMP_SHADERS=DIR writes each compiled shader's GLSL
// source into the directory as `<sha1>.vert.glsl` and `<sha1>.fragment.glsl`
// files. Pass `1` or `true` to write into $TMPDIR (or /tmp). Useful when a
// Kage shader produces wrong output on Vulkan and you want to confirm
// what shaderc actually sees — the Kage compiler's GLSL emission should
// be identical to what gets passed to the WGSL/MSL translators, and this
// captures it at the exact moment Vulkan ingests it. Also handy for
// piping into `spirv-dis` after running through shaderc manually.
//
// Each shader only dumps once per process (by content hash), so repeat
// compiles of the same source don't keep overwriting the file.
var vkShaderDumpDir = resolveShaderDumpDir()

var dumpedShaderHashes = map[string]bool{}

func resolveShaderDumpDir() string {
	v := os.Getenv("FUTURE_CORE_VK_DUMP_SHADERS")
	if v == "" {
		return ""
	}
	if v == "1" || v == "true" {
		return os.TempDir()
	}
	return v
}

// dumpShaderSource writes `src` to `<dir>/<sha1>.<suffix>` unless that
// hash has already been written this process. Silently no-ops on any
// filesystem error — it's diagnostic, not load-bearing.
func dumpShaderSource(src, suffix string) {
	if vkShaderDumpDir == "" || src == "" {
		return
	}
	sum := sha1.Sum([]byte(src))
	hash := hex.EncodeToString(sum[:])[:12]
	if dumpedShaderHashes[hash] {
		return
	}
	dumpedShaderHashes[hash] = true
	path := filepath.Join(vkShaderDumpDir, hash+"."+suffix)
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		return
	}
	fmt.Fprintf(os.Stderr, "vulkan: dumped %s shader → %s\n", suffix, path)
}

// FUTURE_CORE_VK_UNIFORM_PROBE=NAME logs the packed bytes written for the
// named uniform each time a shader is packed. Useful for confirming that
// a uniform's value reaches the GPU at the offset the SPIR-V expects.
// Bytes are printed as hex so they can be decoded to float32/int32 by
// eye (`cd cc cc 3e` → 0x3ECCCCCD → 0.4).
//
// Left as a thin boolean-gated helper rather than a ring buffer because
// the caller can redirect stderr to a file when a large capture is
// needed.
var vkUniformProbe = os.Getenv("FUTURE_CORE_VK_UNIFORM_PROBE")

func probePackedUniform(layout []shadertranslate.UniformField, buf []byte) {
	if vkUniformProbe == "" {
		return
	}
	for _, f := range layout {
		if f.Name != vkUniformProbe {
			continue
		}
		fmt.Fprintf(os.Stderr, "vulkan: probe[%s] offset=%d size=%d bytes=% x\n",
			f.Name, f.Offset, f.Size, buf[f.Offset:f.Offset+f.Size])
		return
	}
}
