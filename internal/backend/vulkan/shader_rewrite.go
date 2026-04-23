//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import "strings"

// prepareFragmentForShaderc applies Vulkan-specific rewrites to the
// Kage-emitted fragment GLSL before shaderc compilation.
//
// Unused-sampler stripping: the Kage → GLSL emitter unconditionally
// declares `uTexture0..3` (one for each possible Images[0..3] slot).
// Fragment shaders that only sample from `uTexture0` (e.g. point_light,
// multiply_dither) still carry the unused uTexture1/2/3 declarations.
// shaderc's `auto_bind_uniforms` then assigns EVERY sampler the same
// binding (Binding=0), producing SPIR-V that decorates four separate
// OpVariable<UniformConstant> with the identical (set=0, binding=0)
// tuple. That's invalid per the Vulkan spec but MoltenVK (via shaderc
// → spirv-cross → MSL) silently accepts it and synthesises pathological
// MSL. On the lighting demo we observed: stripping the unused samplers
// restored the scene from an all-black image to one that shows the
// ambient fill and scene geometry. The lights themselves remain a
// separate MoltenVK-specific interpolation issue (see below); the
// sampler strip is a necessary-but-not-sufficient step toward parity
// and a strict bugfix regardless.
//
// Why string-level stripping rather than emitter-level: the emitter
// lives in shaderir/kage.go and is shared across every backend; the
// WGSL translator and the Metal translator don't have the same
// binding-collision behavior. Doing the surgery here keeps the fix
// Vulkan-local until we can teach the Kage emitter to emit only the
// sampler slots its shader actually uses. A future refactor should
// pull usage-analysis up into shaderir.Compile so every backend
// receives the minimal declaration set.
//
// Lighting-demo residual: on Vulkan+MoltenVK the point_light and
// spot_light shaders still produce near-uniform output within their
// quad regions — consistent with `vDstPos.xy` being flat-interpolated
// to the primitive's provoking vertex. multiply_dither (drawn as a
// full-screen quad to the scene RT) reads vDstPos correctly. We
// haven't found a MoltenVK shader or pipeline option that selectively
// triggers flat interpolation for one custom-shader pipeline and not
// another, and spirv-cross's generated MSL declares `[[stage_in]]`
// inputs without flat qualifiers. The next investigation angle is
// running the same build under lavapipe (Docker) to confirm whether
// this is MoltenVK-specific or a spec-level Vulkan issue.
func prepareFragmentForShaderc(src string) string {
	return stripUnusedSamplers(src)
}

// stripUnusedSamplers removes `uniform sampler2D uTextureN;` declarations
// AND the matching imageSrcNAt / imageSrcNUnsafeAt / imageSrcNOrigin /
// imageSrcNSize helper functions whose sampler is never invoked from the
// user's fragment body. The Kage emitter unconditionally emits all four
// slots (see future-core/internal/shaderir/kage.go:emitImageHelpers), so
// the unused slots' helpers still reference their uTextureN sampler —
// which means a naive "strip only the declaration" pass leaves the
// reference dangling and GLSL won't compile. We detect a slot as "in
// use" iff the source calls `imageSrcNAt(` or `imageSrcNUnsafeAt(` at
// least once. Slot 0 is never stripped (the Kage pipeline's convention
// binds the primary texture there and most shaders that bother to use
// an image use slot 0).
func stripUnusedSamplers(src string) string {
	// The Kage emitter writes helper definitions before main(), so an
	// `imageSrc1At(` substring match would always hit the helper's own
	// signature even when the slot is unused. Detect real call sites by
	// restricting the usage search to the region AFTER `void main() {`,
	// which is where user code lives.
	mainStart := strings.Index(src, "void main()")
	if mainStart < 0 {
		return src
	}
	body := src[mainStart:]
	for i := 1; i <= 3; i++ {
		n := string(rune('0' + i))
		if strings.Contains(body, "imageSrc"+n+"At(") ||
			strings.Contains(body, "imageSrc"+n+"UnsafeAt(") ||
			strings.Contains(body, "imageSrc"+n+"Origin(") ||
			strings.Contains(body, "imageSrc"+n+"Size(") {
			continue
		}
		src = stripSamplerSlot(src, n)
	}
	return src
}

// stripSamplerSlot removes the sampler declaration AND the helper
// function bodies that reference it. Leaves the `uImageSrcNOrigin` /
// `uImageSrcNSize` uniform declarations in place so the UBO layout
// stays stable (our ExtractUniformLayout is a separate pass that runs
// on the same stripped source, so offsets remain aligned between our
// packed buffer and shaderc's SPIR-V).
//
// Either every edit lands or none of them do. A partial strip — e.g.
// declaration removed but the helper body referencing it kept because
// we failed to find the closing brace — would leave the GLSL in a state
// where it references a now-undeclared sampler and shaderc fails with
// an unhelpful error at the call site rather than at our rewrite. If
// any piece can't be located (Kage emitter changed its format, helper
// signatures drifted, etc.) we return the original source unchanged.
func stripSamplerSlot(src, n string) string {
	decl := "uniform sampler2D uTexture" + n + ";\n"
	declIdx := strings.Index(src, decl)
	if declIdx < 0 {
		// Nothing to strip — slot's declaration isn't present.
		return src
	}

	type region struct{ start, end int }
	regions := []region{{declIdx, declIdx + len(decl)}}

	for _, sig := range []string{
		"vec4 imageSrc" + n + "At(vec2 pos) {",
		"vec4 imageSrc" + n + "UnsafeAt(vec2 pos) {",
	} {
		start := strings.Index(src, sig)
		if start < 0 {
			// Helper isn't present — nothing to remove for this sig,
			// but since we haven't committed the edit yet that's fine.
			continue
		}
		// Helpers end with "}\n\n" from the emitter (closing brace
		// followed by the blank-line separator it appends). If we can't
		// find the terminator the source format has changed in a way
		// we can't safely rewrite — bail out and leave the shader
		// untouched rather than leave a half-rewritten body behind.
		rel := strings.Index(src[start:], "}\n\n")
		if rel < 0 {
			return src
		}
		regions = append(regions, region{start, start + rel + len("}\n\n")})
	}

	// Apply removals back-to-front so earlier offsets stay valid.
	// Stable sort by start offset descending.
	for i := 1; i < len(regions); i++ {
		for j := i; j > 0 && regions[j].start > regions[j-1].start; j-- {
			regions[j], regions[j-1] = regions[j-1], regions[j]
		}
	}
	for _, r := range regions {
		src = src[:r.start] + src[r.end:]
	}
	return src
}

// rewriteVDstPosToFragCoord is kept as a pass-through wrapper for backward
// compatibility with shader_gpu.go. It now just invokes the broader
// preparation pass.
func rewriteVDstPosToFragCoord(src string) string {
	return prepareFragmentForShaderc(src)
}
