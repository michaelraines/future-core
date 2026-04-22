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
// after this rewrite, the fragment shader's `gl_FragCoord.xy` reads
// as a near-constant value at every fragment of every custom-shader
// (Kage) light-quad draw. Narrowed quantitatively:
//
//   - range-probe shader (vec4 color by dstPos.x bucket):
//     every fragment's `dstPos.x ∈ [1020, 1024)` on Vulkan+MoltenVK.
//     The RT is 1024 wide, so gl_FragCoord.x is at or very near the
//     right edge for every covered pixel.
//   - on WebGPU (no rewrite; uses `vec4 dstPos = vDstPos`) the
//     distribution is broad across all buckets — vDstPos interpolates
//     correctly.
//   - woodland parity is 0.03% vs WebGPU (essentially byte-identical);
//     the bug is specific to the lighting demo's larger-radius quads,
//     not to the Kage pipeline path in general.
//
// Ruled out:
//   - Uniform layout. Probes + spirv-dis confirm byte-for-byte
//     correct offsets for Center/LightColor/Radius/Intensity.
//   - Vertex input bindings, stride, attributes. Match sprite pipe.
//   - Pipeline module pairing. Trace confirms custom Kage vertex
//     always pairs with its matching custom Kage fragment.
//   - Retina 2x physical scaling. Dividing gl_FragCoord.xy by 2
//     didn't shift the constant value — it's still at ~1023.
//   - Dropping the unused vDstPos varying entirely (to match the
//     sprite pipeline's two-varying interface). No change.
//   - Forcing the non-shadow BlendLighter branch (bypassing
//     stampShadowAlpha). No change.
//
// What the MSL looks like (from MVK_CONFIG_LOG_LEVEL=4 dump):
//   fragment main0_out main0(main0_in in [[stage_in]],
//                            constant spvDescriptorSetBuffer0& ...,
//                            float4 gl_FragCoord [[position]])
//   {
//       float4 dstPos = float4(gl_FragCoord.xy, 0.0, 1.0);
//       ...
//   }
// The `[[position]]` builtin should give per-fragment framebuffer
// coords. For this specific pipeline under specific scene state,
// it returns a primitive-constant value near (RT.width-1, y).
//
// Next investigation angle: step into whether MVK pipeline state
// leaks across the stampShadowAlpha DrawImage→custom-shader
// transition in a way that clobbers `[[position]]` interpolation
// (a Metal-side primitive-flat interpolation getting set somewhere).
// Or run lavapipe + validation layers via docker-compose to isolate
// MoltenVK-specific vs spec-level Vulkan bug.
//
// The rewrite below is the minimum that makes the multiply_dither
// full-screen compositing path correct; it's necessary but not
// sufficient for the lighting demo.
func rewriteVDstPosToFragCoord(src string) string {
	src = strings.Replace(src,
		"vec4 dstPos = vDstPos;",
		"vec4 dstPos = vec4(gl_FragCoord.xy, 0.0, 1.0);", 1)
	src = strings.Replace(src,
		"vec2 srcPos = vTexCoord;",
		"vec2 srcPos = gl_FragCoord.xy;", 1)
	return src
}
