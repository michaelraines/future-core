// DIAGNOSTIC TOOL — not exercised by CI. Dumps every stage of the Kage →
// GLSL → WGSL translation pipeline for a given Kage source so that
// translator output can be diffed against a hand-written WGSL baseline.
//
// Companion diagnostics (see those files for details):
//
//	internal/pipeline/trace.go            — FUTURE_CORE_TRACE_BATCHES / _PASSES
//	internal/backend/webgpu/trace_js.go   — FUTURE_CORE_TRACE_WEBGPU
//
// Usage:
//
//	go test -v -run TestDumpPointLightWGSL ./internal/shadertranslate/
//
// When to use: the trace env vars show what the engine submits to the GPU,
// but if pixels look wrong you often need to know whether the shader
// source itself is correct. This tool prints the shader in full.

package shadertranslate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/shaderir"
)

// pointLightKage mirrors future/libs/comp/lighting/point_light.kage and exists
// so we can compile the real lighting Kage shader end-to-end without
// importing the future/ tree. Useful whenever we need to diff translator
// output against a hand-written WGSL baseline (e.g. when chasing a lighting
// parity regression).
const pointLightKage = `//kage:unit pixels

package lighting

var Center vec2
var LightColor vec3
var Intensity float
var Radius float
var FalloffType float
var NormalEnabled float
var LightHeight float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	dist := distance(dstPos.xy, Center)

	if dist > Radius {
		return vec4(0)
	}

	nd := dist / Radius

	var attenuation float
	if FalloffType > 1.5 {
		attenuation = 1.0 / (1.0 + 0.7*nd + 1.8*nd*nd)
		attenuation *= 1.0 - smoothstep(0.9, 1.0, nd)
	} else if FalloffType > 0.5 {
		attenuation = 1.0 - nd
	} else {
		attenuation = 1.0 / (1.0 + nd*nd*4.0)
		attenuation *= 1.0 - smoothstep(0.8, 1.0, nd)
	}

	normalMod := 1.0
	if NormalEnabled > 0.5 {
		normalSample := imageSrc0At(srcPos)
		if normalSample.a > 0 {
			normal := normalize(normalSample.rgb*2.0 - 1.0)
			lightDir := normalize(vec3(Center.x-dstPos.x, Center.y-dstPos.y, LightHeight))
			ndotl := max(dot(normal, lightDir), 0.0)
			normalMod = ndotl
		}
	}

	result := LightColor * Intensity * attenuation * normalMod

	n1 := fract(sin(dot(dstPos.xy, vec2(12.9898, 78.233))) * 43758.5453)
	n2 := fract(sin(dot(dstPos.xy, vec2(93.9898, 67.345))) * 28461.6421)
	result += vec3((n1 + n2 - 1.0) / 128.0)

	return vec4(result, attenuation*Intensity*normalMod)
}
`

// TestDumpPointLightWGSL compiles the lighting demo's point-light Kage
// shader through the full Kage → GLSL → WGSL pipeline and prints every
// intermediate form, including the combined uniform layout and the final
// WGSL that the WebGPU shader module actually receives after the
// VertexUniforms / FragmentUniforms → Uniforms struct substitution done
// in internal/backend/webgpu/shader_common.go.
//
// Use this to diff translator output against a hand-written WGSL baseline
// (e.g. parity-tests/wgsl-lighting-demo/lighting.js). Run:
//
//	go test -v -run TestDumpPointLightWGSL ./internal/shadertranslate/
func TestDumpPointLightWGSL(t *testing.T) {
	compiled, err := shaderir.Compile([]byte(pointLightKage))
	require.NoError(t, err)

	t.Log("=========== Kage → GLSL (vertex) ===========")
	t.Log("\n" + compiled.VertexShader)

	t.Log("=========== Kage → GLSL (fragment) ===========")
	t.Log("\n" + compiled.FragmentShader)

	t.Log("=========== Kage uniforms ===========")
	names := make([]string, 0, len(compiled.Uniforms))
	for _, u := range compiled.Uniforms {
		names = append(names, fmt.Sprintf("%s %s", u.Type, u.Name))
	}
	sort.Strings(names)
	t.Log("\n" + strings.Join(names, "\n"))

	vResult, err := GLSLToWGSLVertex(compiled.VertexShader)
	require.NoError(t, err)
	t.Log("=========== GLSL → WGSL (vertex) ===========")
	t.Log("\n" + vResult.Source)

	t.Log("=========== WGSL vertex uniform fields (name / type / offset / size) ===========")
	for _, f := range vResult.Uniforms {
		t.Logf("  %-24s %-16s offset=%-4d size=%d", f.Name, f.Type, f.Offset, f.Size)
	}

	fResult, err := GLSLToWGSLFragment(compiled.FragmentShader)
	require.NoError(t, err)
	t.Log("=========== GLSL → WGSL (fragment) ===========")
	t.Log("\n" + fResult.Source)

	t.Log("=========== WGSL fragment uniform fields (name / type / offset / size) ===========")
	for _, f := range fResult.Uniforms {
		t.Logf("  %-24s %-16s offset=%-4d size=%d", f.Name, f.Type, f.Offset, f.Size)
	}

	combined := buildCombinedUniformLayoutForTest(vResult.Uniforms, fResult.Uniforms)
	structSrc := buildUniformStructForTest("Uniforms", combined)
	finalVertex := replaceUniformStructForTest(vResult.Source, "VertexUniforms", "Uniforms", structSrc)
	finalFragment := replaceUniformStructForTest(fResult.Source, "FragmentUniforms", "Uniforms", structSrc)

	t.Log("=========== COMBINED uniform layout (post-merge, sent to WebGPU) ===========")
	for _, f := range combined {
		t.Logf("  %-24s %-16s offset=%-4d size=%d", f.Name, f.Type, f.Offset, f.Size)
	}
	if len(combined) > 0 {
		last := combined[len(combined)-1]
		t.Logf("  total size before padding = %d", last.Offset+last.Size)
	}

	t.Log("=========== FINAL WGSL (vertex, after combined-struct replacement) ===========")
	t.Log("\n" + finalVertex)
	t.Log("=========== FINAL WGSL (fragment, after combined-struct replacement) ===========")
	t.Log("\n" + finalFragment)
}

// The helpers below mirror internal/backend/webgpu/shader_common.go's
// private funcs. They're duplicated here instead of exported from that
// package because the webgpu package carries the three-mode build-tag
// complexity (soft/GPU/js) and this diagnostic only needs the pure-Go
// layout math.

// wgslFieldAlignForTest returns the WGSL alignment (bytes) for a field
// type. Mirrors internal/backend/webgpu/shader_common.go wgslFieldAlign.
func wgslFieldAlignForTest(typeName string) int {
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

// buildCombinedUniformLayoutForTest mirrors
// internal/backend/webgpu/shader_common.go buildCombinedUniformLayout.
func buildCombinedUniformLayoutForTest(vertex, fragment []UniformField) []UniformField {
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
	combined := make([]UniformField, len(vertex))
	copy(combined, vertex)
	for _, f := range vertex {
		seen[f.Name] = true
	}
	offset := 0
	if len(vertex) > 0 {
		last := vertex[len(vertex)-1]
		offset = last.Offset + last.Size
	}
	for _, f := range fragment {
		if seen[f.Name] {
			continue
		}
		align := wgslFieldAlignForTest(f.Type)
		if offset%align != 0 {
			offset += align - (offset % align)
		}
		combined = append(combined, UniformField{Name: f.Name, Type: f.Type, Offset: offset, Size: f.Size})
		seen[f.Name] = true
		offset += f.Size
	}
	return combined
}

// buildUniformStructForTest mirrors
// internal/backend/webgpu/shader_common.go buildUniformStructWGSL.
func buildUniformStructForTest(name string, fields []UniformField) string {
	var b strings.Builder
	fmt.Fprintf(&b, "struct %s {\n", name)
	for _, f := range fields {
		fmt.Fprintf(&b, "    %s: %s,\n", f.Name, f.Type)
	}
	b.WriteString("};\n")
	return b.String()
}

// replaceUniformStructForTest mirrors
// internal/backend/webgpu/shader_common.go replaceUniformStruct.
func replaceUniformStructForTest(source, oldName, newName, newStruct string) string {
	re := regexp.MustCompile(`struct ` + regexp.QuoteMeta(oldName) + ` \{[^}]*\};\n`)
	source = re.ReplaceAllString(source, newStruct)
	source = strings.ReplaceAll(source, "uniforms: "+oldName, "uniforms: "+newName)
	return source
}
