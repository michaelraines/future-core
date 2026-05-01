// dump-msl: one-shot tool that runs Kage source through future-core's
// shaderir → shadertranslate pipeline and emits the per-stage MSL
// source pair (vertex + fragment) plus the union uniform layout.
// Used to bootstrap hand-authored MSL variants for the
// multi-language shader-source migration.
//
// Usage:
//
//	go run ./cmd/dump-msl path/to/shader.kage
//
// Companion tool to cmd/dump-wgsl. Mirrors the per-stage parsing the
// Metal backend's GLSLToMSLVertex/Fragment translator does at
// runtime, so the output is byte-equivalent to what compiles in
// production. Authors copy the output as the starting point for
// their .vert.msl / .frag.msl files and hand-tune as needed
// (e.g. swapping the fract-of-sin dither for a deterministic Bayer
// matrix).
//
// Note: Metal uses two SEPARATE per-stage uniform buffers (vertex
// uses [[buffer(0)]], fragment uses [[buffer(0)]] in its own
// argument list). The translator emits separate `Uniforms` structs
// per stage. For hand-authored MSL, the framework's per-draw packer
// uses a single byte layout for both stages (set via
// NativeShaderDescriptor.Uniforms), so the safest pattern is to
// declare the same union struct in both .msl files. Uniforms not
// referenced by a given stage cost a few bytes in the buffer but
// nothing in shader execution.
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
		fmt.Fprintf(os.Stderr, "usage: dump-msl path/to/shader.kage\n")
		os.Exit(2)
	}
	src, err := os.ReadFile(os.Args[1])
	must(err)

	compiled, err := shaderir.Compile(src)
	must(err)

	vRes, err := shadertranslate.GLSLToMSLVertex(compiled.VertexShader)
	must(err)
	fRes, err := shadertranslate.GLSLToMSLFragment(compiled.FragmentShader)
	must(err)

	union := buildUnionLayout(vRes.Uniforms, fRes.Uniforms)

	fmt.Println("=== UNION uniform layout (declare this in both .vert.msl + .frag.msl) ===")
	for _, u := range union {
		fmt.Printf("  %-24s %-16s offset=%-4d size=%d\n", u.Name, u.Type, u.Offset, u.Size)
	}
	if len(union) > 0 {
		last := union[len(union)-1]
		fmt.Printf("  total = %d bytes\n", last.Offset+last.Size)
	}

	fmt.Println("\n=== MSL VERTEX (translator output, pre-rewrite) ===")
	fmt.Println(vRes.Source)
	fmt.Println("\n=== MSL FRAGMENT (translator output, pre-rewrite) ===")
	fmt.Println(fRes.Source)

	fmt.Println("\n=== Suggested combined Uniforms struct (copy into both .msl files) ===")
	fmt.Println(buildUnionStruct(union))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

// buildUnionLayout builds the union of vertex+fragment uniform
// fields with non-overlapping offsets, mirroring the layout the
// framework's per-draw packer assumes. Mirrors the same logic as
// internal/backend/webgpu/shader_common.go's buildCombinedUniformLayout.
func buildUnionLayout(vertex, fragment []shadertranslate.UniformField) []shadertranslate.UniformField {
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
	out := make([]shadertranslate.UniformField, len(vertex))
	copy(out, vertex)
	for _, f := range vertex {
		seen[f.Name] = true
	}
	last := out[len(out)-1]
	cursor := last.Offset + last.Size
	for _, f := range fragment {
		if seen[f.Name] {
			continue
		}
		align := mslAlign(f.Type)
		if cursor%align != 0 {
			cursor += align - (cursor % align)
		}
		f.Offset = cursor
		cursor += f.Size
		out = append(out, f)
		seen[f.Name] = true
	}
	return out
}

// mslAlign returns the std140-equivalent alignment for a Metal type.
// Note that the Metal translator emits the same MSL type names as
// WGSL would for our shader subset (float, float2, float3, float4,
// float4x4); the alignment is identical.
func mslAlign(t string) int {
	switch t {
	case "float", "int", "uint":
		return 4
	case "float2", "int2", "uint2":
		return 8
	case "float3", "float4", "int3", "int4", "uint3", "uint4",
		"float3x3", "float4x4":
		return 16
	}
	return 4
}

func buildUnionStruct(fields []shadertranslate.UniformField) string {
	var b strings.Builder
	b.WriteString("struct Uniforms {\n")
	for _, f := range fields {
		b.WriteString("    ")
		b.WriteString(f.Type)
		b.WriteString(" ")
		b.WriteString(f.Name)
		b.WriteString(";\n")
	}
	b.WriteString("};")
	return b.String()
}
