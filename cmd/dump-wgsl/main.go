// dump-wgsl: one-shot tool that runs Kage source through future-core's
// shaderir → shadertranslate pipeline and emits the post-merge WGSL
// pair (vertex + fragment) plus the combined uniform layout. Used as
// the starting point for hand-authored WGSL variants in the
// multi-language shader-source migration.
//
// This file is not committed to main — it lives inside future-core
// only long enough to run against each shader's .kage source, then
// the dumps are checked in alongside the per-language sibling .go
// files in the future repo.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/michaelraines/future-core/internal/shaderir"
	"github.com/michaelraines/future-core/internal/shadertranslate"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: dump-wgsl path/to/shader.kage\n")
		os.Exit(2)
	}
	src, err := os.ReadFile(os.Args[1])
	must(err)

	compiled, err := shaderir.Compile(src)
	must(err)

	vRes, err := shadertranslate.GLSLToWGSLVertex(compiled.VertexShader)
	must(err)
	fRes, err := shadertranslate.GLSLToWGSLFragment(compiled.FragmentShader)
	must(err)

	combined := buildCombinedLayout(vRes.Uniforms, fRes.Uniforms)

	fmt.Println("=== COMBINED uniform layout (sent to WebGPU) ===")
	for _, u := range combined {
		fmt.Printf("  %-24s %-16s offset=%-4d size=%d\n", u.Name, u.Type, u.Offset, u.Size)
	}
	if len(combined) > 0 {
		last := combined[len(combined)-1]
		fmt.Printf("  total = %d bytes\n", last.Offset+last.Size)
	}

	structSrc := buildUniformStruct("Uniforms", combined)
	finalVertex := replaceUniformStruct(vRes.Source, "VertexUniforms", "Uniforms", structSrc)
	finalFragment := replaceUniformStruct(fRes.Source, "FragmentUniforms", "Uniforms", structSrc)

	fmt.Println("\n=== FINAL WGSL VERTEX ===")
	fmt.Println(finalVertex)
	fmt.Println("\n=== FINAL WGSL FRAGMENT ===")
	fmt.Println(finalFragment)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

// --- helpers (mirror internal/backend/webgpu/shader_common.go) ---

func wgslAlign(t string) int {
	switch t {
	case "f32", "i32", "u32":
		return 4
	case "vec2<f32>", "vec2<i32>", "vec2<u32>":
		return 8
	case "vec3<f32>", "vec3<i32>", "vec3<u32>",
		"vec4<f32>", "vec4<i32>", "vec4<u32>",
		"mat3x3<f32>", "mat4x4<f32>":
		return 16
	}
	return 4
}

func buildCombinedLayout(vertex, fragment []shadertranslate.UniformField) []shadertranslate.UniformField {
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
	last := combined[len(combined)-1]
	cursor := last.Offset + last.Size
	for _, f := range fragment {
		if seen[f.Name] {
			continue
		}
		align := wgslAlign(f.Type)
		if cursor%align != 0 {
			cursor += align - (cursor % align)
		}
		f.Offset = cursor
		cursor += f.Size
		combined = append(combined, f)
		seen[f.Name] = true
	}
	return combined
}

func buildUniformStruct(name string, fields []shadertranslate.UniformField) string {
	var b strings.Builder
	b.WriteString("struct ")
	b.WriteString(name)
	b.WriteString(" {\n")
	for _, f := range fields {
		b.WriteString("    ")
		b.WriteString(f.Name)
		b.WriteString(": ")
		b.WriteString(f.Type)
		b.WriteString(",\n")
	}
	b.WriteString("};")
	return b.String()
}

func replaceUniformStruct(src, oldName, newName, structSrc string) string {
	out := src
	if start := strings.Index(out, "struct "+oldName+" {"); start >= 0 {
		end := indexAfterStructEnd(out, start)
		if end > start {
			out = out[:start] + structSrc + out[end:]
		}
	}
	out = strings.ReplaceAll(out, ": "+oldName, ": "+newName)
	return out
}

func indexAfterStructEnd(src string, start int) int {
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				if i+1 < len(src) && src[i+1] == ';' {
					return i + 2
				}
				return i + 1
			}
		}
	}
	return -1
}
