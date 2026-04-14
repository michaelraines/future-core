//go:build (darwin || linux || freebsd || windows || js) && !soft

package webgpu

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/michaelraines/future-core/internal/shadertranslate"
)

// buildCombinedUniformLayout merges vertex and fragment uniform layouts into a
// single layout where all fields have non-overlapping offsets. Vertex fields
// come first, then fragment-only fields are appended.
func buildCombinedUniformLayout(vertex, fragment []shadertranslate.UniformField) []shadertranslate.UniformField {
	if len(vertex) == 0 && len(fragment) == 0 {
		return nil
	}
	if len(fragment) == 0 {
		return vertex
	}
	if len(vertex) == 0 {
		return fragment
	}

	seen := make(map[string]bool, len(vertex))
	combined := make([]shadertranslate.UniformField, len(vertex))
	copy(combined, vertex)
	for _, f := range vertex {
		seen[f.Name] = true
	}

	// Next available offset after vertex fields.
	offset := 0
	if len(vertex) > 0 {
		last := vertex[len(vertex)-1]
		offset = last.Offset + last.Size
	}

	for _, f := range fragment {
		if seen[f.Name] {
			continue
		}
		// Apply WGSL/std140 alignment rules. We must key on the type
		// name rather than f.Size: vec3<f32> has SizeOf=12 but
		// AlignOf=16, so a size-based heuristic would give it the
		// wrong alignment (4) and every field placed after it in the
		// combined struct would land at a CPU offset that disagrees
		// with the WGSL layout.
		align := wgslFieldAlign(f.Type)
		if offset%align != 0 {
			offset += align - (offset % align)
		}
		combined = append(combined, shadertranslate.UniformField{
			Name:   f.Name,
			Type:   f.Type,
			Offset: offset,
			Size:   f.Size,
		})
		seen[f.Name] = true
		offset += f.Size
	}

	return combined
}

// wgslFieldAlign returns the WGSL/std140 alignment (in bytes) of a uniform
// struct field given its declared type name.
// https://www.w3.org/TR/WGSL/#alignment-and-size
func wgslFieldAlign(typeName string) int {
	switch typeName {
	case "f32", "i32", "u32", "int", "float":
		return 4
	case "vec2<f32>", "vec2<i32>", "vec2<u32>", "vec2":
		return 8
	case "vec3<f32>", "vec3<i32>", "vec3<u32>", "vec3",
		"vec4<f32>", "vec4<i32>", "vec4<u32>", "vec4",
		"mat3x3<f32>", "mat3", "mat4x4<f32>", "mat4":
		return 16
	default:
		return 4
	}
}

// buildUniformStructWGSL generates a WGSL struct declaration from uniform fields.
func buildUniformStructWGSL(name string, fields []shadertranslate.UniformField) string {
	var b strings.Builder
	fmt.Fprintf(&b, "struct %s {\n", name)
	for _, f := range fields {
		fmt.Fprintf(&b, "    %s: %s,\n", f.Name, f.Type)
	}
	b.WriteString("};\n")
	return b.String()
}

// reUniformStruct matches a WGSL uniform struct declaration block.
var reUniformStruct = regexp.MustCompile(`struct (\w+) \{[^}]*\};\n`)

// replaceUniformStruct patches WGSL source to replace a per-stage uniform
// struct with the combined struct.
func replaceUniformStruct(source, oldName, newName, newStruct string) string {
	// Replace the struct block for oldName with the combined struct.
	re := regexp.MustCompile(`struct ` + regexp.QuoteMeta(oldName) + ` \{[^}]*\};\n`)
	source = re.ReplaceAllString(source, newStruct)
	// Update the var<uniform> type reference.
	source = strings.ReplaceAll(source, "uniforms: "+oldName, "uniforms: "+newName)
	return source
}
