package shadertranslate

import "strings"

// ExtractUniformLayout scans GLSL 330 source for uniform declarations,
// drops samplers, and computes std140-aligned byte offsets.
//
// This is the single source of truth for Kage-shader uniform layout —
// every backend that needs to pack a Go-side uniform map into a GPU
// buffer should consume its output. Vulkan reads the returned slice
// directly (SPIR-V encodes the layout); the WGSL and MSL translators
// wrap this with target-language type conversion for their struct
// emitters. Previously each backend had its own layout computation:
// the Vulkan and MSL copies diverged from the reference (WGSL) on
// vec3 alignment (both used `size >= 16 → align 16`, which picks up
// vec4 and mat* but leaves vec3 at align=4). That silently moved
// every uniform after a vec3 four bytes past the SPIR-V-expected
// position — visible in the lighting demo as point-light LightColor
// rendering black.
//
// UniformField.Type is the GLSL type name ("vec3", "mat4", etc.).
// Backends that emit struct fields in a target language rename via
// wgslType / mslType at emission time.
func ExtractUniformLayout(glsl string) ([]UniformField, error) {
	uniforms := parseUniformDecls(glsl)
	return buildStd140Layout(uniforms), nil
}

// parseUniformDecls walks GLSL source line-by-line, matches bare
// `uniform <type> <name>;` declarations with reUniform, and drops
// samplers (which belong in descriptor sets, not UBOs). The regex
// is shared with the MSL / WGSL translators' parse loops.
func parseUniformDecls(glsl string) []uniform {
	var out []uniform
	for _, line := range strings.Split(glsl, "\n") {
		trimmed := strings.TrimSpace(line)
		m := reUniform.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		typ, name := m[1], m[2]
		if isSamplerType(typ) {
			continue
		}
		out = append(out, uniform{typ: typ, name: name})
	}
	return out
}

// buildStd140Layout turns a parsed uniform list into std140-aligned
// UniformFields. Type is kept as the GLSL name — translators convert
// at source-emission time.
//
// Alignment rules (std140):
//
//	float, int, ivec2..ivec4 scalars → align 4
//	vec2                             → align 8
//	vec3, vec4, mat3, mat4           → align 16
//
// vec3 consumes only 12 bytes despite its 16-byte alignment; a
// following scalar packs into the 4-byte tail. Returning size 16
// for vec3 — which the old MSL / Vulkan copies effectively did by
// using `size >= 16` as the alignment trigger — would leave every
// subsequent scalar one full slot past the shaderc-emitted SPIR-V
// offset and blank out any shader with vec3 uniforms.
func buildStd140Layout(uniforms []uniform) []UniformField {
	if len(uniforms) == 0 {
		return nil
	}
	fields := make([]UniformField, 0, len(uniforms))
	offset := 0
	for _, u := range uniforms {
		size := uniformSize(u.typ)
		if size == 0 {
			// Unknown type — skip rather than pack a zero-size hole.
			// Matches the old per-backend parsers; extend uniformSize
			// when a new type genuinely needs UBO space.
			continue
		}
		align := std140Align(u.typ)
		if offset%align != 0 {
			offset += align - (offset % align)
		}
		fields = append(fields, UniformField{
			Name:   u.name,
			Type:   u.typ,
			Offset: offset,
			Size:   size,
		})
		offset += size
	}
	return fields
}

// std140Align returns the std140 alignment for a GLSL type. Keep this
// table co-located with uniformSize (msl.go) so the two stay in sync.
// Unknown / zero-size types return 4 (matches the base scalar rule;
// parseUniformDecls skips them anyway).
func std140Align(glslType string) int {
	switch glslType {
	case "vec2":
		return 8
	case "vec3", "vec4", "mat3", "mat4":
		return 16
	default:
		return 4
	}
}

// isSamplerType returns true for GLSL sampler types that belong in
// descriptor sets rather than UBOs. Extend this as new sampler
// variants appear in Kage output.
func isSamplerType(glslType string) bool {
	switch glslType {
	case "sampler2D", "sampler3D", "samplerCube", "sampler2DArray":
		return true
	default:
		return false
	}
}
