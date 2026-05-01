// dump-glsl: one-shot tool that runs Kage source through future-core's
// shaderir compiler and emits the GLSL 330 source pair (vertex +
// fragment). Used to bootstrap hand-authored GLSL variants for the
// multi-language shader-source migration on the OpenGL backend.
//
// Usage:
//
//	go run ./cmd/dump-glsl path/to/shader.kage
//
// Companion to cmd/dump-wgsl and cmd/dump-msl. The OpenGL backend
// already accepts GLSL 330 directly via ShaderDescriptor.VertexSource,
// so the GLSL output here is exactly what runs at compile time today
// — no per-stage struct rewrite, no uniform-layout merge. The
// translator simply emits the vertex and fragment GLSL source the
// runtime path consumes; authoring a hand-written variant is just
// "edit this output and commit it."
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/michaelraines/future-core/internal/shaderir"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: dump-glsl path/to/shader.kage\n")
		os.Exit(2)
	}
	src, err := os.ReadFile(os.Args[1])
	must(err)

	compiled, err := shaderir.Compile(src)
	must(err)

	fmt.Println("=== Kage uniforms (each becomes a top-level uniform in GLSL) ===")
	names := make([]string, 0, len(compiled.Uniforms))
	for _, u := range compiled.Uniforms {
		names = append(names, fmt.Sprintf("  %s %s", u.Type, u.Name))
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Println(n)
	}

	fmt.Println("\n=== GLSL VERTEX ===")
	fmt.Println(compiled.VertexShader)
	fmt.Println("\n=== GLSL FRAGMENT ===")
	fmt.Println(compiled.FragmentShader)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}
