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
	s = replaceWGSLClampSaturate(s)

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
	s = replaceWGSLClampSaturate(s)

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

	// Bare "return;" in GLSL fragment shaders is always redundant after
	// the fragColor→return translation: either it follows a fragColor
	// assignment (already translated to "return value;") or it's an
	// early exit where fragColor was set earlier. Skip it to avoid
	// WGSL "unreachable code" warnings.
	if strings.TrimSpace(s) == "return;" {
		return ""
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

// replaceWGSLClampSaturate rewrites `clamp(X, 0, 1)` / `clamp(X, 0.0, 1.0)`
// as `saturate(X)`. Motivation: GLSL (and Kage) auto-broadcast scalar
// min/max args to match a vector first arg — `clamp(vec3, 0, 1)` is
// valid and means per-channel clamp to [0,1]. WGSL is strictly typed
// and rejects `clamp(vec3<f32>, abstract-int, abstract-int)` with
// "no matching call". The WGSL `saturate(e)` built-in is defined for
// scalars AND vectors of any float type and is semantically identical
// to `clamp(e, 0.0, 1.0)`, so it threads through without needing full
// type analysis of the first argument.
//
// Uses lexical scanning with paren-depth tracking rather than a single
// regex because the first argument can itself contain commas and nested
// parens (e.g. `clamp(mix(a, b, t) * scale, 0, 1)`).
func replaceWGSLClampSaturate(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		// Find the next `clamp(` that isn't part of a longer identifier.
		idx := strings.Index(s[i:], "clamp(")
		if idx < 0 {
			out.WriteString(s[i:])
			break
		}
		absIdx := i + idx
		if absIdx > 0 && isIdentRune(s[absIdx-1]) {
			// Something like `myClamp(` — skip past this match.
			out.WriteString(s[i : absIdx+len("clamp(")])
			i = absIdx + len("clamp(")
			continue
		}
		// Emit everything up to and including `clamp(`.
		out.WriteString(s[i:absIdx])
		argStart := absIdx + len("clamp(")
		// Walk forward to find the matching `)`, tracking paren depth.
		depth := 1
		end := argStart
		for end < len(s) && depth > 0 {
			switch s[end] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					goto foundEnd
				}
			}
			end++
		}
	foundEnd:
		if depth != 0 {
			// Malformed — emit the rest unchanged and stop.
			out.WriteString(s[absIdx:])
			return out.String()
		}
		args := splitTopLevelCommas(s[argStart:end])
		if len(args) == 3 &&
			isZeroLiteral(strings.TrimSpace(args[1])) &&
			isOneLiteral(strings.TrimSpace(args[2])) {
			out.WriteString("saturate(")
			out.WriteString(strings.TrimSpace(args[0]))
			out.WriteString(")")
		} else {
			// Not the saturate pattern — preserve the original call.
			out.WriteString("clamp(")
			out.WriteString(s[argStart:end])
			out.WriteString(")")
		}
		i = end + 1
	}
	return out.String()
}

// splitTopLevelCommas splits a string on commas that are not nested
// inside parentheses, matching how a function-argument list is parsed.
func splitTopLevelCommas(s string) []string {
	var parts []string
	depth := 0
	last := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[last:i])
				last = i + 1
			}
		}
	}
	parts = append(parts, s[last:])
	return parts
}

func isZeroLiteral(s string) bool { return s == "0" || s == "0.0" || s == "0." || s == ".0" }
func isOneLiteral(s string) bool  { return s == "1" || s == "1.0" || s == "1." }

func isIdentRune(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
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

// replaceWGSLTextureCall replaces texture(samplerName, uv) with
// textureSampleLevel(samplerName, samplerName_sampler, uv, 0.0).
// Uses textureSampleLevel instead of textureSample because
// textureSample requires uniform control flow, which fails in any
// fragment shader that has non-uniform if-branches (common in lighting).
//
// 2D ASSUMPTION: LOD is hardcoded to 0.0 because Phase 1 only uses
// non-mipmapped 2D textures. For 3D with mipmapped textures, this
// should either use textureSample (automatic mip from derivatives,
// but requires uniform control flow) or compute the LOD explicitly.
// See FUTURE_3D.md.
func replaceWGSLTextureCall(s, samplerName string) string {
	re := reTextureCall(samplerName)
	return re.ReplaceAllStringFunc(s, func(match string) string {
		// match is "texture(samplerName, uv)" — extract the UV argument.
		inner := match[strings.Index(match, ",")+1 : len(match)-1]
		inner = strings.TrimSpace(inner)
		return fmt.Sprintf("textureSampleLevel(%s, %s_sampler, %s, 0.0)", samplerName, samplerName, inner)
	})
}

// reTextureCall builds a regex for texture(samplerName, uv).
// Captures the full call including the closing paren.
func reTextureCall(samplerName string) *regexp.Regexp {
	return regexp.MustCompile(`texture\s*\(\s*` + regexp.QuoteMeta(samplerName) + `\s*,\s*[^)]+\)`)
}

// buildWGSLUniformLayout delegates std140 arithmetic to
// buildStd140Layout (layout.go) and then rewrites each field's Type
// from the GLSL name to its WGSL equivalent so the downstream WGSL
// struct emitter (`buildUniformStructWGSL`) gets valid syntax. The
// shared layout helper guarantees the offsets match every other
// backend that consumes ExtractUniformLayout.
func buildWGSLUniformLayout(uniforms []uniform) []UniformField {
	fields := buildStd140Layout(uniforms)
	for i := range fields {
		fields[i].Type = wgslType(fields[i].Type)
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
		// Uses textureSampleLevel (explicit LOD=0) instead of textureSample
		// because textureSample requires uniform control flow. Shaders that
		// have ANY non-uniform if-branch before the call site would fail
		// validation even if the call is outside the branch.
		//
		// Kage's `kage:unit pixels` directive (the common case for 2D effects
		// like lighting) passes pixel coordinates to imageSrcNAt. WGSL
		// textureSampleLevel expects normalized UVs, so we divide by the
		// texture dimensions before sampling.
		//
		// 2D ASSUMPTION: LOD 0.0 is correct for non-mipmapped 2D textures.
		// For 3D, mipmapped textures need automatic LOD (textureSample) or
		// explicit LOD computation. See FUTURE_3D.md.
		fmt.Fprintf(b, "fn imageSrc%dAt(pos: vec2<f32>) -> vec4<f32> {\n", i)
		fmt.Fprintf(b, "    let texDim = vec2<f32>(textureDimensions(%s));\n", texName)
		fmt.Fprintf(b, "    let uv = pos / texDim;\n")
		fmt.Fprintf(b, "    let sampled = textureSampleLevel(%s, %s_sampler, uv, 0.0);\n", texName, texName)
		// Bounds check uses the texture's actual dimensions rather than the
		// uImageSrcNOrigin/Size uniforms: those uniforms describe sub-image
		// regions within an atlas and aren't populated by the CPU-side
		// draw path today, so relying on them would make imageSrcNAt
		// always return vec4(0) (lit up the lighting demo bug where every
		// shader that sampled a source image wrote zero).
		b.WriteString("    let inBounds = pos.x >= 0.0 && pos.y >= 0.0 && pos.x < texDim.x && pos.y < texDim.y;\n")
		b.WriteString("    return select(vec4<f32>(0.0), sampled, inBounds);\n")
		b.WriteString("}\n\n")

		// imageSrcNUnsafeAt — unchecked texture sample.
		fmt.Fprintf(b, "fn imageSrc%dUnsafeAt(pos: vec2<f32>) -> vec4<f32> {\n", i)
		fmt.Fprintf(b, "    let texDim = vec2<f32>(textureDimensions(%s));\n", texName)
		fmt.Fprintf(b, "    return textureSampleLevel(%s, %s_sampler, pos / texDim, 0.0);\n", texName, texName)
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
