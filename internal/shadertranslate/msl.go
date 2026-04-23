// Package shadertranslate provides shader cross-compilation utilities.
// Currently supports GLSL 330 core → Metal Shading Language (MSL).
package shadertranslate

import (
	"fmt"
	"regexp"
	"strings"
)

// UniformField describes a uniform variable's layout in the packed buffer.
type UniformField struct {
	Name   string
	Type   string // MSL type: "float", "float2", "float4", "float4x4"
	Offset int    // byte offset in uniform buffer
	Size   int    // byte size
}

// MSLResult holds the translated MSL source and uniform layout.
type MSLResult struct {
	Source   string
	Uniforms []UniformField
}

// attribute represents a parsed vertex attribute.
type attribute struct {
	location int
	typ      string // GLSL type
	name     string
}

// uniform represents a parsed uniform declaration.
type uniform struct {
	typ  string // GLSL type
	name string
}

// varying represents a parsed varying (in/out) declaration.
type varying struct {
	typ  string // GLSL type
	name string
}

// Regex patterns for GLSL parsing.
var (
	reVersion   = regexp.MustCompile(`^\s*#version\s+`)
	reAttribute = regexp.MustCompile(`^\s*layout\s*\(\s*location\s*=\s*(\d+)\s*\)\s*in\s+(\w+)\s+(\w+)\s*;`)
	reUniform   = regexp.MustCompile(`^\s*uniform\s+(\w+)\s+(\w+)\s*;`)
	// reVaryingOut and reVaryingIn accept an optional `layout(location=N)`
	// prefix — Vulkan-targeted GLSL needs explicit varying locations or
	// glslang auto-assigns them (which is fine for isolated shaders but
	// the Kage path benefits from deterministic numbering across
	// vertex+fragment pairs). The WGSL/MSL translators assign their own
	// locations by declaration order, so the captured prefix is ignored.
	reVaryingOut = regexp.MustCompile(`^\s*(?:layout\s*\(\s*location\s*=\s*\d+\s*\)\s*)?out\s+(\w+)\s+(\w+)\s*;`)
	reVaryingIn  = regexp.MustCompile(`^\s*(?:layout\s*\(\s*location\s*=\s*\d+\s*\)\s*)?in\s+(\w+)\s+(\w+)\s*;`)
	reFragOut    = regexp.MustCompile(`^\s*out\s+vec4\s+(\w+)\s*;`)
	reMainStart  = regexp.MustCompile(`^\s*void\s+main\s*\(\s*\)\s*\{?\s*$`)
)

// glslToMSLType maps GLSL type names to MSL equivalents.
var glslToMSLType = map[string]string{
	"float":     "float",
	"vec2":      "float2",
	"vec3":      "float3",
	"vec4":      "float4",
	"mat3":      "float3x3",
	"mat4":      "float4x4",
	"int":       "int",
	"ivec2":     "int2",
	"ivec3":     "int3",
	"ivec4":     "int4",
	"sampler2D": "texture2d<float>",
}

// uniformSize returns the byte size for a GLSL uniform type.
func uniformSize(glslType string) int {
	switch glslType {
	case "float":
		return 4
	case "vec2":
		return 8
	case "vec3":
		// WGSL/std140 `SizeOf(vec3<f32>)` is 12 (three tightly packed
		// floats). The alignment is 16, but that only requires padding
		// BEFORE the vec3 if the preceding offset isn't already 16-aligned.
		// A following scalar (f32) packs at offset+12 with no extra pad.
		// Returning 16 here made every field after a vec3 land 4 bytes
		// past the WGSL layout position, which is why `Intensity` in
		// the lighting shader read 0.0.
		return 12
	case "vec4":
		return 16
	case "mat3":
		return 48 // 3 × float4 (std140 padding)
	case "mat4":
		return 64
	case "int":
		return 4
	default:
		return 0
	}
}

// GLSLToMSLVertex translates a GLSL 330 vertex shader to MSL.
func GLSLToMSLVertex(glsl string) (MSLResult, error) {
	lines := strings.Split(glsl, "\n")

	var attrs []attribute
	var uniforms []uniform
	var varyings []varying
	var bodyLines []string
	inBody := false
	braceDepth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if reVersion.MatchString(trimmed) {
			continue
		}

		if m := reAttribute.FindStringSubmatch(trimmed); m != nil {
			loc := 0
			if _, err := fmt.Sscanf(m[1], "%d", &loc); err != nil {
				return MSLResult{}, fmt.Errorf("invalid attribute location %q: %w", m[1], err)
			}
			attrs = append(attrs, attribute{location: loc, typ: m[2], name: m[3]})
			continue
		}

		if m := reUniform.FindStringSubmatch(trimmed); m != nil {
			uniforms = append(uniforms, uniform{typ: m[1], name: m[2]})
			continue
		}

		if m := reVaryingOut.FindStringSubmatch(trimmed); m != nil {
			varyings = append(varyings, varying{typ: m[1], name: m[2]})
			continue
		}

		if reMainStart.MatchString(trimmed) {
			inBody = true
			if strings.Contains(trimmed, "{") {
				braceDepth = 1
			}
			continue
		}

		if inBody {
			for _, ch := range trimmed {
				switch ch {
				case '{':
					braceDepth++
				case '}':
					braceDepth--
				}
			}
			if braceDepth <= 0 {
				inBody = false
				continue
			}
			bodyLines = append(bodyLines, trimmed)
		}
	}

	// Build MSL source.
	var b strings.Builder
	b.WriteString("#include <metal_stdlib>\nusing namespace metal;\n\n")

	// VertexIn struct.
	b.WriteString("struct VertexIn {\n")
	for _, a := range attrs {
		fmt.Fprintf(&b, "    %s %s [[attribute(%d)]];\n", mslType(a.typ), a.name, a.location)
	}
	b.WriteString("};\n\n")

	// VertexOut struct.
	b.WriteString("struct VertexOut {\n")
	b.WriteString("    float4 position [[position]];\n")
	for _, v := range varyings {
		fmt.Fprintf(&b, "    %s %s;\n", mslType(v.typ), v.name)
	}
	b.WriteString("};\n\n")

	// Uniform buffer struct (non-sampler uniforms only).
	var bufUniforms []uniform
	for _, u := range uniforms {
		if u.typ != "sampler2D" {
			bufUniforms = append(bufUniforms, u)
		}
	}

	uniformLayout := buildUniformLayout(bufUniforms)

	if len(bufUniforms) > 0 {
		b.WriteString("struct VertexUniforms {\n")
		for _, u := range bufUniforms {
			fmt.Fprintf(&b, "    %s %s;\n", mslType(u.typ), u.name)
		}
		b.WriteString("};\n\n")
	}

	// Vertex function signature.
	b.WriteString("vertex VertexOut vertexMain(\n")
	b.WriteString("    VertexIn in [[stage_in]]")
	if len(bufUniforms) > 0 {
		b.WriteString(",\n    constant VertexUniforms& uniforms [[buffer(1)]]")
	}
	b.WriteString("\n) {\n")
	b.WriteString("    VertexOut out;\n")

	// Translate body.
	for _, line := range bodyLines {
		translated := translateVertexLine(line, attrs, uniforms, varyings)
		b.WriteString("    " + translated + "\n")
	}

	b.WriteString("    return out;\n")
	b.WriteString("}\n")

	return MSLResult{Source: b.String(), Uniforms: uniformLayout}, nil
}

// GLSLToMSLFragment translates a GLSL 330 fragment shader to MSL.
func GLSLToMSLFragment(glsl string) (MSLResult, error) {
	lines := strings.Split(glsl, "\n")

	var uniforms []uniform
	var varyings []varying
	var fragOutName string
	var bodyLines []string
	inBody := false
	braceDepth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if reVersion.MatchString(trimmed) {
			continue
		}

		if m := reUniform.FindStringSubmatch(trimmed); m != nil {
			uniforms = append(uniforms, uniform{typ: m[1], name: m[2]})
			continue
		}

		if m := reVaryingIn.FindStringSubmatch(trimmed); m != nil {
			varyings = append(varyings, varying{typ: m[1], name: m[2]})
			continue
		}

		if m := reFragOut.FindStringSubmatch(trimmed); m != nil {
			fragOutName = m[1]
			continue
		}

		if reMainStart.MatchString(trimmed) {
			inBody = true
			if strings.Contains(trimmed, "{") {
				braceDepth = 1
			}
			continue
		}

		if inBody {
			for _, ch := range trimmed {
				switch ch {
				case '{':
					braceDepth++
				case '}':
					braceDepth--
				}
			}
			if braceDepth <= 0 {
				inBody = false
				continue
			}
			bodyLines = append(bodyLines, trimmed)
		}
	}

	// Build MSL source.
	var b strings.Builder
	b.WriteString("#include <metal_stdlib>\nusing namespace metal;\n\n")

	// FragmentIn struct (matches VertexOut).
	b.WriteString("struct FragmentIn {\n")
	b.WriteString("    float4 position [[position]];\n")
	for _, v := range varyings {
		fmt.Fprintf(&b, "    %s %s;\n", mslType(v.typ), v.name)
	}
	b.WriteString("};\n\n")

	// Collect sampler and non-sampler uniforms.
	var samplerUniforms []uniform
	var bufUniforms []uniform
	for _, u := range uniforms {
		if u.typ == "sampler2D" {
			samplerUniforms = append(samplerUniforms, u)
		} else {
			bufUniforms = append(bufUniforms, u)
		}
	}

	uniformLayout := buildUniformLayout(bufUniforms)

	if len(bufUniforms) > 0 {
		b.WriteString("struct FragmentUniforms {\n")
		for _, u := range bufUniforms {
			fmt.Fprintf(&b, "    %s %s;\n", mslType(u.typ), u.name)
		}
		b.WriteString("};\n\n")
	}

	// Fragment function signature.
	b.WriteString("fragment float4 fragmentMain(\n")
	b.WriteString("    FragmentIn in [[stage_in]]")
	for i, s := range samplerUniforms {
		fmt.Fprintf(&b, ",\n    texture2d<float> %s [[texture(%d)]]", s.name, i)
		fmt.Fprintf(&b, ",\n    sampler %s_sampler [[sampler(%d)]]", s.name, i)
	}
	if len(bufUniforms) > 0 {
		b.WriteString(",\n    constant FragmentUniforms& uniforms [[buffer(0)]]")
	}
	b.WriteString("\n) {\n")

	// Translate body.
	for _, line := range bodyLines {
		translated := translateFragmentLine(line, uniforms, varyings, samplerUniforms, fragOutName)
		b.WriteString("    " + translated + "\n")
	}

	b.WriteString("}\n")

	return MSLResult{Source: b.String(), Uniforms: uniformLayout}, nil
}

// translateVertexLine translates a single line of vertex shader body.
func translateVertexLine(line string, attrs []attribute, uniforms []uniform, varyings []varying) string {
	s := line

	// Type names.
	s = replaceTypes(s)

	// gl_Position → out.position
	s = strings.ReplaceAll(s, "gl_Position", "out.position")

	// Attribute references: aPosition → in.aPosition
	for _, a := range attrs {
		s = replaceIdentifier(s, a.name, "in."+a.name)
	}

	// Varying assignments: vTexCoord → out.vTexCoord
	for _, v := range varyings {
		s = replaceIdentifier(s, v.name, "out."+v.name)
	}

	// Uniform references: uProjection → uniforms.uProjection
	for _, u := range uniforms {
		if u.typ != "sampler2D" {
			s = replaceIdentifier(s, u.name, "uniforms."+u.name)
		}
	}

	return s
}

// translateFragmentLine translates a single line of fragment shader body.
func translateFragmentLine(line string, uniforms []uniform, varyings []varying, samplers []uniform, fragOutName string) string {
	s := line

	// Type names.
	s = replaceTypes(s)

	// texture(sampler, uv) → sampler.sample(sampler_sampler, uv)
	for _, samp := range samplers {
		s = replaceTextureCall(s, samp.name)
	}

	// Varying references: vTexCoord → in.vTexCoord
	for _, v := range varyings {
		s = replaceIdentifier(s, v.name, "in."+v.name)
	}

	// Uniform references: uColorBody → uniforms.uColorBody
	for _, u := range uniforms {
		if u.typ != "sampler2D" {
			s = replaceIdentifier(s, u.name, "uniforms."+u.name)
		}
	}

	// fragColor = expr → return expr
	if fragOutName != "" && strings.Contains(s, fragOutName) {
		s = strings.Replace(s, fragOutName+" =", "return", 1)
		s = strings.Replace(s, fragOutName+"=", "return", 1)
	}

	return s
}

// replaceTypes replaces GLSL type constructors with MSL equivalents.
func replaceTypes(s string) string {
	// Must be function-call-style: vec4( → float4(, etc.
	// Use word-boundary-aware replacement.
	s = replaceTypeConstructor(s, "vec2", "float2")
	s = replaceTypeConstructor(s, "vec3", "float3")
	s = replaceTypeConstructor(s, "vec4", "float4")
	s = replaceTypeConstructor(s, "mat3", "float3x3")
	s = replaceTypeConstructor(s, "mat4", "float4x4")
	s = replaceTypeConstructor(s, "ivec2", "int2")
	s = replaceTypeConstructor(s, "ivec3", "int3")
	s = replaceTypeConstructor(s, "ivec4", "int4")
	return s
}

// replaceTypeConstructor replaces a GLSL type name at word boundaries.
func replaceTypeConstructor(s, from, to string) string {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(from) + `\b`)
	return re.ReplaceAllString(s, to)
}

// replaceIdentifier replaces a standalone identifier (not part of a longer name).
// Avoids matching when preceded by '.' (e.g., "in.vTexCoord" won't re-match "vTexCoord").
func replaceIdentifier(s, from, to string) string {
	re := regexp.MustCompile(`(^|[^.\w])` + regexp.QuoteMeta(from) + `\b`)
	return re.ReplaceAllStringFunc(s, func(m string) string {
		// Preserve the leading non-identifier character.
		prefix := ""
		if len(m) > len(from) {
			prefix = m[:len(m)-len(from)]
		}
		return prefix + to
	})
}

// replaceTextureCall replaces texture(sampler, uv) with sampler.sample(sampler_sampler, uv).
func replaceTextureCall(s, samplerName string) string {
	re := regexp.MustCompile(`texture\s*\(\s*` + regexp.QuoteMeta(samplerName) + `\s*,\s*`)
	return re.ReplaceAllString(s, samplerName+".sample("+samplerName+"_sampler, ")
}

// mslType converts a GLSL type name to MSL.
func mslType(glslType string) string {
	if t, ok := glslToMSLType[glslType]; ok {
		return t
	}
	return glslType
}

// buildUniformLayout delegates std140 arithmetic to buildStd140Layout
// (layout.go) and then rewrites each field's Type from the GLSL name
// to its MSL equivalent. The old inline math here had the same vec3
// bug the Vulkan parser did (used `size >= 16` as the align-16
// trigger, which misses vec3 because SizeOf(vec3)=12), so every
// uniform after a vec3 would land at a CPU offset one slot past
// where the generated MSL struct expected it. Sharing the layout
// with ExtractUniformLayout makes that class of bug a single-point
// fix.
func buildUniformLayout(uniforms []uniform) []UniformField {
	fields := buildStd140Layout(uniforms)
	for i := range fields {
		fields[i].Type = mslType(fields[i].Type)
	}
	return fields
}
