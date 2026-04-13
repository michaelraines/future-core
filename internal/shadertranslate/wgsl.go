package shadertranslate

import (
	"fmt"
	"regexp"
	"strings"
)

// WGSLResult holds the translated WGSL source and uniform layout.
type WGSLResult struct {
	Source   string
	Uniforms []UniformField
}

// glslToWGSLType maps GLSL type names to WGSL equivalents.
var glslToWGSLType = map[string]string{
	"float":     "f32",
	"vec2":      "vec2<f32>",
	"vec3":      "vec3<f32>",
	"vec4":      "vec4<f32>",
	"mat3":      "mat3x3<f32>",
	"mat4":      "mat4x4<f32>",
	"int":       "i32",
	"ivec2":     "vec2<i32>",
	"ivec3":     "vec3<i32>",
	"ivec4":     "vec4<i32>",
	"bool":      "bool",
	"sampler2D": "texture_2d<f32>",
}

// wgslType converts a GLSL type name to WGSL.
func wgslType(glslType string) string {
	if t, ok := glslToWGSLType[glslType]; ok {
		return t
	}
	return glslType
}

// GLSLToWGSLVertex translates a GLSL 330 vertex shader to WGSL.
func GLSLToWGSLVertex(glsl string) (WGSLResult, error) {
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
				return WGSLResult{}, fmt.Errorf("invalid attribute location %q: %w", m[1], err)
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

	var b strings.Builder

	// VertexInput struct.
	b.WriteString("struct VertexInput {\n")
	for _, a := range attrs {
		fmt.Fprintf(&b, "    @location(%d) %s: %s,\n", a.location, a.name, wgslType(a.typ))
	}
	b.WriteString("};\n\n")

	// VertexOutput struct.
	b.WriteString("struct VertexOutput {\n")
	b.WriteString("    @builtin(position) position: vec4<f32>,\n")
	for i, v := range varyings {
		fmt.Fprintf(&b, "    @location(%d) %s: %s,\n", i, v.name, wgslType(v.typ))
	}
	b.WriteString("};\n\n")

	// Uniform buffer struct (non-sampler uniforms only).
	var bufUniforms []uniform
	for _, u := range uniforms {
		if u.typ != "sampler2D" {
			bufUniforms = append(bufUniforms, u)
		}
	}

	uniformLayout := buildWGSLUniformLayout(bufUniforms)

	if len(bufUniforms) > 0 {
		b.WriteString("struct VertexUniforms {\n")
		for _, u := range bufUniforms {
			fmt.Fprintf(&b, "    %s: %s,\n", u.name, wgslType(u.typ))
		}
		b.WriteString("};\n\n")
		b.WriteString("@group(0) @binding(0) var<uniform> uniforms: VertexUniforms;\n\n")
	}

	// Vertex function.
	b.WriteString("@vertex\nfn vs_main(in: VertexInput) -> VertexOutput {\n")
	b.WriteString("    var out: VertexOutput;\n")

	for _, line := range bodyLines {
		translated := translateWGSLVertexLine(line, attrs, uniforms, varyings)
		b.WriteString("    " + translated + "\n")
	}

	b.WriteString("    return out;\n")
	b.WriteString("}\n")

	return WGSLResult{Source: b.String(), Uniforms: uniformLayout}, nil
}

// GLSLToWGSLFragment translates a GLSL 330 fragment shader to WGSL.
func GLSLToWGSLFragment(glsl string) (WGSLResult, error) {
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

	var b strings.Builder

	// FragmentInput struct (matches VertexOutput).
	b.WriteString("struct FragmentInput {\n")
	b.WriteString("    @builtin(position) position: vec4<f32>,\n")
	for i, v := range varyings {
		fmt.Fprintf(&b, "    @location(%d) %s: %s,\n", i, v.name, wgslType(v.typ))
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

	uniformLayout := buildWGSLUniformLayout(bufUniforms)

	// Fragment uniform struct.
	if len(bufUniforms) > 0 {
		b.WriteString("struct FragmentUniforms {\n")
		for _, u := range bufUniforms {
			fmt.Fprintf(&b, "    %s: %s,\n", u.name, wgslType(u.typ))
		}
		b.WriteString("};\n\n")
		b.WriteString("@group(0) @binding(0) var<uniform> uniforms: FragmentUniforms;\n")
	}

	// Texture and sampler bindings (group 1 for textures).
	for i, s := range samplerUniforms {
		texBinding := i * 2
		sampBinding := i*2 + 1
		fmt.Fprintf(&b, "@group(1) @binding(%d) var %s: texture_2d<f32>;\n", texBinding, s.name)
		fmt.Fprintf(&b, "@group(1) @binding(%d) var %s_sampler: sampler;\n", sampBinding, s.name)
	}
	if len(samplerUniforms) > 0 || len(bufUniforms) > 0 {
		b.WriteString("\n")
	}

	// Emit Kage image helper functions. The Kage→GLSL compiler generates
	// helper functions (imageSrc0At, imageSrc0UnsafeAt, etc.) outside
	// void main(). Scan the GLSL source for calls to these functions and
	// emit WGSL equivalents that use textureSample.
	emitWGSLImageHelpers(&b, glsl, samplerUniforms)

	// Fragment function.
	b.WriteString("@fragment\nfn fs_main(in: FragmentInput) -> @location(0) vec4<f32> {\n")

	for _, line := range bodyLines {
		translated := translateWGSLFragmentLine(line, uniforms, varyings, samplerUniforms, fragOutName)
		b.WriteString("    " + translated + "\n")
	}

	b.WriteString("}\n")

	return WGSLResult{Source: b.String(), Uniforms: uniformLayout}, nil
}

// translateWGSLVertexLine translates a single line of vertex shader body to WGSL.
func translateWGSLVertexLine(line string, attrs []attribute, uniforms []uniform, varyings []varying) string {
	s := stripLineComment(line)

	// Local variable declarations (before type constructors).
	s = replaceWGSLLocalVarDecl(s)

	// Type constructors.
	s = replaceWGSLTypes(s)

	// GLSL built-in translations.
	s = replaceWGSLModCall(s)

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

// translateWGSLFragmentLine translates a single line of fragment shader body to WGSL.
func translateWGSLFragmentLine(line string, uniforms []uniform, varyings []varying, samplers []uniform, fragOutName string) string {
	s := stripLineComment(line)

	// Local variable declarations (before type constructors).
	s = replaceWGSLLocalVarDecl(s)

	// Type constructors.
	s = replaceWGSLTypes(s)

	// GLSL built-in translations.
	s = replaceWGSLModCall(s)

	// texture(sampler, uv) → textureSample(sampler, sampler_sampler, uv)
	for _, samp := range samplers {
		s = replaceWGSLTextureCall(s, samp.name)
	}

	// Varying references: vTexCoord → in.vTexCoord
	for _, v := range varyings {
		s = replaceIdentifier(s, v.name, "in."+v.name)
	}

	// Uniform references.
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

	// Bare "return;" is valid in GLSL void functions but invalid in WGSL
	// non-void functions. The fragment function returns vec4<f32>, so
	// convert bare returns to return a zero vector.
	if strings.TrimSpace(s) == "return;" {
		s = strings.Replace(s, "return;", "return vec4<f32>(0.0);", 1)
	}

	return s
}

// reLocalVar matches GLSL local variable declarations: type name = expr;
var reLocalVar = regexp.MustCompile(`^(\s*)(vec[234]|mat[34]|float|int|ivec[234]|bool)\s+(\w+)\s*=`)

// reLocalVarNoInit matches GLSL declarations without initializers: type name;
var reLocalVarNoInit = regexp.MustCompile(`^(\s*)(vec[234]|mat[34]|float|int|ivec[234]|bool)\s+(\w+)\s*;`)

// replaceWGSLLocalVarDecl converts "type name = expr" to "var name: type = expr"
// and "type name;" to "var name: type;" (uninitialized).
func replaceWGSLLocalVarDecl(s string) string {
	// Try initialized declaration first.
	m := reLocalVar.FindStringSubmatch(s)
	if m != nil {
		prefix := m[1]
		glslT := m[2]
		varName := m[3]
		wT := wgslType(glslT)
		old := m[0]
		return strings.Replace(s, old, prefix+"var "+varName+": "+wT+" =", 1)
	}

	// Try uninitialized declaration.
	m = reLocalVarNoInit.FindStringSubmatch(s)
	if m != nil {
		prefix := m[1]
		glslT := m[2]
		varName := m[3]
		wT := wgslType(glslT)
		old := m[0]
		return strings.Replace(s, old, prefix+"var "+varName+": "+wT+";", 1)
	}

	return s
}

// reModCall matches GLSL mod(a, b) calls.
var reModCall = regexp.MustCompile(`\bmod\s*\(\s*([^,]+?)\s*,\s*([^)]+?)\s*\)`)

// replaceWGSLModCall translates GLSL mod(a, b) to WGSL (a % b).
// WGSL does not have a mod() function; the % operator provides the same
// behavior for float operands.
func replaceWGSLModCall(s string) string {
	return reModCall.ReplaceAllString(s, "($1 % $2)")
}

// stripLineComment removes trailing // comments from a line.
func stripLineComment(s string) string {
	// Don't strip inside string literals (rare in shaders, but be safe).
	if idx := strings.Index(s, "//"); idx >= 0 {
		return strings.TrimRight(s[:idx], " \t")
	}
	return s
}

// replaceWGSLTypeConstructor replaces a GLSL type name at word boundaries,
// but not when already followed by '<' (already WGSL-ified).
func replaceWGSLTypeConstructor(s, from, to string) string {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(from) + `\b`)
	result := re.ReplaceAllStringFunc(s, func(m string) string {
		idx := strings.Index(s, m)
		endIdx := idx + len(m)
		if endIdx < len(s) && s[endIdx] == '<' {
			return m // Already WGSL type, don't replace.
		}
		return to
	})
	return result
}

// replaceWGSLTypes replaces GLSL type constructors with WGSL equivalents.
func replaceWGSLTypes(s string) string {
	s = replaceWGSLTypeConstructor(s, "vec2", "vec2<f32>")
	s = replaceWGSLTypeConstructor(s, "vec3", "vec3<f32>")
	s = replaceWGSLTypeConstructor(s, "vec4", "vec4<f32>")
	s = replaceWGSLTypeConstructor(s, "mat3", "mat3x3<f32>")
	s = replaceWGSLTypeConstructor(s, "mat4", "mat4x4<f32>")
	s = replaceWGSLTypeConstructor(s, "ivec2", "vec2<i32>")
	s = replaceWGSLTypeConstructor(s, "ivec3", "vec3<i32>")
	s = replaceWGSLTypeConstructor(s, "ivec4", "vec4<i32>")
	return s
}

// replaceWGSLTextureCall replaces texture(sampler, uv) with textureSample(sampler, sampler_sampler, uv).
func replaceWGSLTextureCall(s, samplerName string) string {
	re := reTextureCall(samplerName)
	return re.ReplaceAllString(s, "textureSample("+samplerName+", "+samplerName+"_sampler, ")
}

// reTextureCall builds a regex for texture(samplerName, ...).
func reTextureCall(samplerName string) *regexp.Regexp {
	return regexp.MustCompile(`texture\s*\(\s*` + regexp.QuoteMeta(samplerName) + `\s*,\s*`)
}

// buildWGSLUniformLayout computes byte offsets for uniform fields using WGSL/std140 rules.
func buildWGSLUniformLayout(uniforms []uniform) []UniformField {
	if len(uniforms) == 0 {
		return nil
	}
	fields := make([]UniformField, len(uniforms))
	offset := 0
	for i, u := range uniforms {
		size := uniformSize(u.typ)
		align := 4
		if size >= 16 {
			align = 16
		} else if size == 8 {
			align = 8
		}
		if offset%align != 0 {
			offset += align - (offset % align)
		}
		fields[i] = UniformField{
			Name:   u.name,
			Type:   wgslType(u.typ),
			Offset: offset,
			Size:   size,
		}
		offset += size
	}
	return fields
}

// reImageSrcFunc detects Kage-generated imageSrcNAt/UnsafeAt function calls
// in GLSL source to determine which image indices need WGSL helpers.
var reImageSrcFunc = regexp.MustCompile(`imageSrc(\d)(At|UnsafeAt|Origin|Size)\b`)

// reImageDstFunc detects Kage-generated imageDst helper calls.
var reImageDstFunc = regexp.MustCompile(`imageDst(Origin|Size)\b`)

// emitWGSLImageHelpers scans the GLSL source for Kage-generated image
// helper function calls and emits equivalent WGSL function definitions.
// The Kage→GLSL compiler (shaderir/kage.go) emits these as GLSL functions
// outside void main(), which the WGSL translator otherwise skips.
func emitWGSLImageHelpers(b *strings.Builder, glsl string, samplers []uniform) {
	// Find which image indices are used.
	usedImages := map[int]bool{}
	for _, m := range reImageSrcFunc.FindAllStringSubmatch(glsl, -1) {
		idx := int(m[1][0] - '0')
		usedImages[idx] = true
	}

	useDst := reImageDstFunc.MatchString(glsl)

	if len(usedImages) == 0 && !useDst {
		return
	}

	// Build a sampler name lookup by index (uTexture0, uTexture1, etc.)
	samplerByIndex := map[int]string{}
	for _, s := range samplers {
		// The Kage compiler names them uTexture0, uTexture1, etc.
		for i := 0; i < 4; i++ {
			name := fmt.Sprintf("uTexture%d", i)
			if s.name == name {
				samplerByIndex[i] = name
			}
		}
		// Also handle the default uTexture (index 0).
		if s.name == "uTexture" {
			samplerByIndex[0] = "uTexture"
		}
	}

	for i := 0; i < 4; i++ {
		if !usedImages[i] {
			continue
		}
		texName, ok := samplerByIndex[i]
		if !ok {
			continue // no matching texture uniform
		}

		// imageSrcNAt — bounds-checked texture sample.
		fmt.Fprintf(b, "fn imageSrc%dAt(pos: vec2<f32>) -> vec4<f32> {\n", i)
		fmt.Fprintf(b, "    let origin = uniforms.uImageSrc%dOrigin;\n", i)
		fmt.Fprintf(b, "    let size = uniforms.uImageSrc%dSize;\n", i)
		b.WriteString("    if (pos.x < origin.x || pos.y < origin.y || pos.x >= origin.x + size.x || pos.y >= origin.y + size.y) {\n")
		b.WriteString("        return vec4<f32>(0.0);\n")
		b.WriteString("    }\n")
		fmt.Fprintf(b, "    return textureSample(%s, %s_sampler, pos);\n", texName, texName)
		b.WriteString("}\n\n")

		// imageSrcNUnsafeAt — unchecked texture sample.
		fmt.Fprintf(b, "fn imageSrc%dUnsafeAt(pos: vec2<f32>) -> vec4<f32> {\n", i)
		fmt.Fprintf(b, "    return textureSample(%s, %s_sampler, pos);\n", texName, texName)
		b.WriteString("}\n\n")

		// imageSrcNOrigin — returns origin uniform.
		fmt.Fprintf(b, "fn imageSrc%dOrigin() -> vec2<f32> {\n", i)
		fmt.Fprintf(b, "    return uniforms.uImageSrc%dOrigin;\n", i)
		b.WriteString("}\n\n")

		// imageSrcNSize — returns size uniform.
		fmt.Fprintf(b, "fn imageSrc%dSize() -> vec2<f32> {\n", i)
		fmt.Fprintf(b, "    return uniforms.uImageSrc%dSize;\n", i)
		b.WriteString("}\n\n")
	}

	if useDst {
		b.WriteString("fn imageDstOrigin() -> vec2<f32> {\n")
		b.WriteString("    return uniforms.uImageDstOrigin;\n")
		b.WriteString("}\n\n")

		b.WriteString("fn imageDstSize() -> vec2<f32> {\n")
		b.WriteString("    return uniforms.uImageDstSize;\n")
		b.WriteString("}\n\n")
	}
}
