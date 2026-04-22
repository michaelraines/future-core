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
// Known remaining bug (2026-04-22): on the lighting-demo scene, even
// after this rewrite, the custom Kage shader's per-fragment
// `dist = distance(dstPos.xy, Center)` evaluates as though every
// fragment shared the same `dstPos` (i.e. `dist > Radius` fires for
// every fragment of every light quad). Probes confirm the Center /
// Radius uniforms arrive at the correct SPIR-V offsets and that the
// rewrite *is* applied to every point_light / spot_light fragment
// source. WOODLAND does render correctly (small firefly quads show
// the expected 40 lit pixels matching WebGPU byte-for-byte) — the
// divergence is specific to the lighting-demo's larger quads + the
// shadow-stamping sequence. Narrowed to: not uniform layout, not
// vertex-input bindings, not pipeline-pair mis-wiring, not the
// extra vDstPos varying. Next investigation angle (not done here):
// step into MoltenVK's SPIR-V→MSL transpilation via the MoltenVK
// dumper, or run lavapipe+validation on the same workload in the
// Docker container to isolate MoltenVK-specific vs spec-level.
func rewriteVDstPosToFragCoord(src string) string {
	src = strings.Replace(src,
		"vec4 dstPos = vDstPos;",
		"vec4 dstPos = vec4(gl_FragCoord.xy, 0.0, 1.0);", 1)
	src = strings.Replace(src,
		"vec2 srcPos = vTexCoord;",
		"vec2 srcPos = gl_FragCoord.xy;", 1)
	return src
}
