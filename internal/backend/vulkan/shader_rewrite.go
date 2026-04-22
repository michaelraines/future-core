//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import "strings"

// rewriteVDstPosToFragCoord substitutes the Kage-generated
// `vec4 dstPos = vDstPos;` line in a fragment shader with
// `vec4 dstPos = vec4(gl_FragCoord.xy, 0.0, 1.0);`.
//
// Why this exists: custom Kage shaders on Vulkan (specifically through
// shaderc + MoltenVK on macOS) produced non-interpolated varying values
// in the fragment shader for Location-2 outputs. The SPIR-V validates,
// the vertex/fragment Location decorations match, spirv-val passes —
// but the fragment reads a constant (near-zero) value for `vDstPos.xy`
// instead of the interpolated per-pixel position. Built-in sprite
// shaders (which only use Location 0/1 varyings) are unaffected, and
// the same Kage source produces correctly-interpolated varyings on
// WebGPU. The lighting demo's point-light + spot-light shaders
// depend on `dstPos.xy` to compute `distance(dstPos.xy, Center)`, so
// a constant `dstPos` makes every fragment return the early
// `if dist > Radius { return vec4(0) }` and the lightmap stays dark.
//
// The Kage `dstPos` input is semantically the destination pixel
// coordinate (per the `kage:unit pixels` directive). Vulkan's
// gl_FragCoord.xy is defined as the framebuffer pixel coordinate
// with top-left origin, which matches — for any draw whose projection
// maps vertex position directly to framebuffer (which the sprite pass
// does: screen and offscreen RTs alike). Using gl_FragCoord bypasses
// the varying entirely, letting the rasterizer compute the value it
// already had to compute anyway.
//
// Kage emits exactly one `vec4 dstPos = vDstPos;` line per shader
// (in its generated main() prologue, see
// future-core/internal/shaderir/kage.go:emitFragmentShader), so a
// simple string substitution is sufficient and safe. The `in vec4
// vDstPos;` declaration at the top of the fragment stays — it
// becomes an unused input after the rewrite, which shaderc / glslang
// happily compile away.
//
// TODO: fold this into the Kage emitter itself once the equivalent
// substitution is wired into the WGSL/MSL translators too. Until
// then, this is a Vulkan-local workaround scoped to one function.
func rewriteVDstPosToFragCoord(src string) string {
	// dstPos: pixel-space fragment coordinate in the framebuffer.
	// Semantically identical to gl_FragCoord.xy on Vulkan for any
	// draw whose projection maps vertex pos 1:1 to framebuffer — the
	// sprite pass's common case, both for screen and offscreen RTs.
	src = strings.Replace(src,
		"vec4 dstPos = vDstPos;",
		"vec4 dstPos = vec4(gl_FragCoord.xy, 0.0, 1.0);", 1)
	// srcPos: pixel-space source-texture coordinate. For Kage shaders
	// whose source texture is framebuffer-sized and framebuffer-aligned
	// (the lightmap-apply path, bloom prefilter, any "pipeline post-
	// process over same-size texture"), srcPos == gl_FragCoord.xy.
	// Draws that transform the source sampling (e.g. scaled blits with
	// non-identity tex coords) would read the wrong region if this
	// substitution runs — we'd need to gate the rewrite on the pipeline
	// intent if that case appears. No current Kage shader hits it, and
	// without this substitution the lightmap-compose shader collapses
	// to one sampled pixel on Vulkan (the varying bug this file works
	// around).
	src = strings.Replace(src,
		"vec2 srcPos = vTexCoord;",
		"vec2 srcPos = gl_FragCoord.xy;", 1)
	return src
}
