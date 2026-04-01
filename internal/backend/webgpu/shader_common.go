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
		// Apply WGSL/std140 alignment rules.
		align := 4
		if f.Size >= 16 {
			align = 16
		} else if f.Size == 8 {
			align = 8
		}
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
