package shaderir

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// CompileResult holds the output of a Kage-to-GLSL compilation.
type CompileResult struct {
	VertexShader   string
	FragmentShader string
	Uniforms       []Uniform
}

// Compile parses Kage source and transpiles it to GLSL 330 core.
func Compile(src []byte) (*CompileResult, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "kage.go", src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("shaderir: parse error: %w", err)
	}

	c := &compiler{
		fset:          fset,
		uniforms:      nil,
		usesPixelUnit: false,
		localTypes:    make(map[string]string),
	}

	// Process directives from comments.
	for _, cg := range file.Comments {
		for _, comment := range cg.List {
			if strings.Contains(comment.Text, "kage:unit pixels") {
				c.usesPixelUnit = true
			}
		}
	}

	// Extract uniforms from package-level var declarations.
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}
		for _, spec := range genDecl.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			typeName := c.typeString(vs.Type)
			t, terr := ParseType(typeName)
			if terr != nil {
				return nil, fmt.Errorf("shaderir: uniform %s: %w", vs.Names[0].Name, terr)
			}
			for _, name := range vs.Names {
				c.uniforms = append(c.uniforms, Uniform{
					Name: name.Name,
					Type: t,
				})
			}
		}
	}

	// Find the Fragment function.
	var fragmentFunc *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name.Name == "Fragment" {
			fragmentFunc = fn
		}
	}

	if fragmentFunc == nil {
		return nil, fmt.Errorf("shaderir: missing Fragment function")
	}

	// Validate Fragment signature: func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4
	if err := c.validateFragmentSig(fragmentFunc); err != nil {
		return nil, err
	}

	// Generate GLSL.
	fragGLSL := c.emitFragmentShader(fragmentFunc)
	vertGLSL := c.emitVertexShader()

	return &CompileResult{
		VertexShader:   vertGLSL,
		FragmentShader: fragGLSL,
		Uniforms:       c.uniforms,
	}, nil
}

type compiler struct {
	fset          *token.FileSet
	uniforms      []Uniform
	usesPixelUnit bool
	// Fragment parameter names (may differ from default).
	fragDstPos string
	fragSrcPos string
	fragColor  string
	// localTypes tracks inferred types for local variables (name → GLSL type).
	// Populated during compilation for use by inferType.
	localTypes map[string]string
}

func (c *compiler) validateFragmentSig(fn *ast.FuncDecl) error {
	params := fn.Type.Params
	if params == nil || len(params.List) < 3 {
		return fmt.Errorf("shaderir: Fragment must have 3 parameters (dstPos vec4, srcPos vec2, color vec4)")
	}

	// Extract parameter names for use in transpilation.
	for i, p := range params.List[:3] {
		if len(p.Names) == 0 {
			return fmt.Errorf("shaderir: Fragment parameter %d must have a name", i+1)
		}
	}
	c.fragDstPos = params.List[0].Names[0].Name
	c.fragSrcPos = params.List[1].Names[0].Name
	c.fragColor = params.List[2].Names[0].Name

	// Validate return type.
	results := fn.Type.Results
	if results == nil || len(results.List) != 1 {
		return fmt.Errorf("shaderir: Fragment must return vec4")
	}
	retType := c.typeString(results.List[0].Type)
	if retType != "vec4" {
		return fmt.Errorf("shaderir: Fragment must return vec4, got %s", retType)
	}

	return nil
}

func (c *compiler) emitVertexShader() string {
	var b strings.Builder
	b.WriteString("#version 330 core\n\n")
	b.WriteString("layout(location = 0) in vec2 aPosition;\n")
	b.WriteString("layout(location = 1) in vec2 aTexCoord;\n")
	b.WriteString("layout(location = 2) in vec4 aColor;\n\n")
	b.WriteString("uniform mat4 uProjection;\n\n")
	b.WriteString("out vec2 vTexCoord;\n")
	b.WriteString("out vec4 vColor;\n")
	b.WriteString("out vec4 vDstPos;\n\n")
	b.WriteString("void main() {\n")
	b.WriteString("    vTexCoord = aTexCoord;\n")
	b.WriteString("    vColor = aColor;\n")
	b.WriteString("    gl_Position = uProjection * vec4(aPosition, 0.0, 1.0);\n")
	// Kage's `dstPos` fragment input is documented to be the destination
	// pixel coordinates (matching the `kage:unit pixels` directive). Pass
	// through the raw input position — NOT gl_Position, which is already
	// in clip space and would make light shaders' pixel-space distance
	// comparisons (e.g. `distance(dstPos.xy, uniforms.Center)` where
	// Center is in pixels) return huge values, triggering early returns.
	b.WriteString("    vDstPos = vec4(aPosition, 0.0, 1.0);\n")
	b.WriteString("}\n")
	return b.String()
}

func (c *compiler) emitFragmentShader(fn *ast.FuncDecl) string {
	var b strings.Builder
	b.WriteString("#version 330 core\n\n")
	b.WriteString("in vec2 vTexCoord;\n")
	b.WriteString("in vec4 vColor;\n")
	b.WriteString("in vec4 vDstPos;\n\n")

	// Emit texture uniforms.
	b.WriteString("uniform sampler2D uTexture0;\n")
	b.WriteString("uniform sampler2D uTexture1;\n")
	b.WriteString("uniform sampler2D uTexture2;\n")
	b.WriteString("uniform sampler2D uTexture3;\n")

	// Emit image metadata uniforms.
	b.WriteString("uniform vec2 uImageDstOrigin;\n")
	b.WriteString("uniform vec2 uImageDstSize;\n")
	b.WriteString("uniform vec2 uImageSrc0Origin;\n")
	b.WriteString("uniform vec2 uImageSrc0Size;\n")
	b.WriteString("uniform vec2 uImageSrc1Origin;\n")
	b.WriteString("uniform vec2 uImageSrc1Size;\n")
	b.WriteString("uniform vec2 uImageSrc2Origin;\n")
	b.WriteString("uniform vec2 uImageSrc2Size;\n")
	b.WriteString("uniform vec2 uImageSrc3Origin;\n")
	b.WriteString("uniform vec2 uImageSrc3Size;\n\n")

	// Emit user uniforms.
	for _, u := range c.uniforms {
		fmt.Fprintf(&b, "uniform %s %s;\n", u.Type.GLSLName(), u.Name)
	}
	if len(c.uniforms) > 0 {
		b.WriteString("\n")
	}

	b.WriteString("out vec4 fragColor;\n\n")

	// Emit image helper functions.
	c.emitImageHelpers(&b)

	// Emit Fragment function body as main().
	b.WriteString("void main() {\n")
	b.WriteString("    vec4 ")
	b.WriteString(c.fragDstPos)
	b.WriteString(" = vDstPos;\n")
	b.WriteString("    vec2 ")
	b.WriteString(c.fragSrcPos)
	b.WriteString(" = vTexCoord;\n")
	b.WriteString("    vec4 ")
	b.WriteString(c.fragColor)
	b.WriteString(" = vColor;\n")

	// Register fragment parameter types for type inference.
	c.localTypes[c.fragDstPos] = "vec4"
	c.localTypes[c.fragSrcPos] = "vec2"
	c.localTypes[c.fragColor] = "vec4"

	// Transpile the function body.
	c.emitBlock(&b, fn.Body, 1)

	b.WriteString("}\n")
	return b.String()
}

func (c *compiler) emitImageHelpers(b *strings.Builder) {
	for i := 0; i < 4; i++ {
		fmt.Fprintf(b, "vec4 imageSrc%dAt(vec2 pos) {\n", i)
		fmt.Fprintf(b, "    vec2 origin = uImageSrc%dOrigin;\n", i)
		fmt.Fprintf(b, "    vec2 size = uImageSrc%dSize;\n", i)
		fmt.Fprintf(b, "    if (pos.x < origin.x || pos.y < origin.y || pos.x >= origin.x + size.x || pos.y >= origin.y + size.y) {\n")
		b.WriteString("        return vec4(0.0);\n")
		b.WriteString("    }\n")
		fmt.Fprintf(b, "    return texture(uTexture%d, pos);\n", i)
		b.WriteString("}\n\n")

		fmt.Fprintf(b, "vec4 imageSrc%dUnsafeAt(vec2 pos) {\n", i)
		fmt.Fprintf(b, "    return texture(uTexture%d, pos);\n", i)
		b.WriteString("}\n\n")

		fmt.Fprintf(b, "vec2 imageSrc%dOrigin() {\n", i)
		fmt.Fprintf(b, "    return uImageSrc%dOrigin;\n", i)
		b.WriteString("}\n\n")

		fmt.Fprintf(b, "vec2 imageSrc%dSize() {\n", i)
		fmt.Fprintf(b, "    return uImageSrc%dSize;\n", i)
		b.WriteString("}\n\n")
	}

	b.WriteString("vec2 imageDstOrigin() {\n")
	b.WriteString("    return uImageDstOrigin;\n")
	b.WriteString("}\n\n")

	b.WriteString("vec2 imageDstSize() {\n")
	b.WriteString("    return uImageDstSize;\n")
	b.WriteString("}\n\n")

	b.WriteString("vec2 imageDstTextureSize() {\n")
	b.WriteString("    return uImageDstSize;\n")
	b.WriteString("}\n\n")
}

func (c *compiler) emitBlock(b *strings.Builder, block *ast.BlockStmt, indent int) {
	for _, stmt := range block.List {
		c.emitStmt(b, stmt, indent)
	}
}

func (c *compiler) emitStmt(b *strings.Builder, stmt ast.Stmt, indent int) {
	prefix := strings.Repeat("    ", indent)

	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		if len(s.Results) == 1 {
			fmt.Fprintf(b, "%sfragColor = %s;\n", prefix, c.exprString(s.Results[0]))
			fmt.Fprintf(b, "%sreturn;\n", prefix)
		}

	case *ast.AssignStmt:
		c.emitAssign(b, s, prefix)

	case *ast.DeclStmt:
		genDecl, ok := s.Decl.(*ast.GenDecl)
		if ok && genDecl.Tok == token.VAR {
			for _, spec := range genDecl.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				typeName := c.typeString(vs.Type)
				for i, name := range vs.Names {
					c.localTypes[name.Name] = typeName
					if i < len(vs.Values) {
						fmt.Fprintf(b, "%s%s %s = %s;\n", prefix, typeName, name.Name, c.exprString(vs.Values[i]))
					} else {
						fmt.Fprintf(b, "%s%s %s;\n", prefix, typeName, name.Name)
					}
				}
			}
		}

	case *ast.IfStmt:
		c.emitIf(b, s, indent)

	case *ast.ForStmt:
		c.emitFor(b, s, indent)

	case *ast.ExprStmt:
		fmt.Fprintf(b, "%s%s;\n", prefix, c.exprString(s.X))

	case *ast.BlockStmt:
		fmt.Fprintf(b, "%s{\n", prefix)
		c.emitBlock(b, s, indent+1)
		fmt.Fprintf(b, "%s}\n", prefix)

	case *ast.IncDecStmt:
		fmt.Fprintf(b, "%s%s%s;\n", prefix, c.exprString(s.X), s.Tok.String())

	case *ast.BranchStmt:
		fmt.Fprintf(b, "%s%s;\n", prefix, s.Tok.String())
	}
}

func (c *compiler) emitAssign(b *strings.Builder, s *ast.AssignStmt, prefix string) {
	if s.Tok == token.DEFINE {
		// Short variable declaration: x := expr
		for i, lhs := range s.Lhs {
			if i >= len(s.Rhs) {
				continue
			}
			rhsStr := c.exprString(s.Rhs[i])
			typeName := c.inferType(s.Rhs[i])
			varName := c.exprString(lhs)
			c.localTypes[varName] = typeName
			fmt.Fprintf(b, "%s%s %s = %s;\n", prefix, typeName, varName, rhsStr)
		}
	} else {
		// Regular assignment.
		for i, lhs := range s.Lhs {
			if i < len(s.Rhs) {
				op := s.Tok.String()
				fmt.Fprintf(b, "%s%s %s %s;\n", prefix, c.exprString(lhs), op, c.exprString(s.Rhs[i]))
			}
		}
	}
}

func (c *compiler) emitIf(b *strings.Builder, s *ast.IfStmt, indent int) {
	prefix := strings.Repeat("    ", indent)
	fmt.Fprintf(b, "%sif (%s) {\n", prefix, c.exprString(s.Cond))
	c.emitBlock(b, s.Body, indent+1)
	if s.Else != nil {
		if elseIf, ok := s.Else.(*ast.IfStmt); ok {
			fmt.Fprintf(b, "%s} else ", prefix)
			c.emitIf(b, elseIf, indent)
			return
		}
		fmt.Fprintf(b, "%s} else {\n", prefix)
		if elseBlock, ok := s.Else.(*ast.BlockStmt); ok {
			c.emitBlock(b, elseBlock, indent+1)
		}
	}
	fmt.Fprintf(b, "%s}\n", prefix)
}

func (c *compiler) emitFor(b *strings.Builder, s *ast.ForStmt, indent int) {
	prefix := strings.Repeat("    ", indent)

	initStr := ""
	if s.Init != nil {
		if assign, ok := s.Init.(*ast.AssignStmt); ok {
			if assign.Tok == token.DEFINE && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
				typeName := c.inferType(assign.Rhs[0])
				initStr = fmt.Sprintf("%s %s = %s", typeName, c.exprString(assign.Lhs[0]), c.exprString(assign.Rhs[0]))
			} else {
				initStr = fmt.Sprintf("%s = %s", c.exprString(assign.Lhs[0]), c.exprString(assign.Rhs[0]))
			}
		}
	}

	condStr := ""
	if s.Cond != nil {
		condStr = c.exprString(s.Cond)
	}

	postStr := ""
	if s.Post != nil {
		switch post := s.Post.(type) {
		case *ast.IncDecStmt:
			postStr = c.exprString(post.X) + post.Tok.String()
		case *ast.AssignStmt:
			postStr = fmt.Sprintf("%s %s %s", c.exprString(post.Lhs[0]), post.Tok.String(), c.exprString(post.Rhs[0]))
		}
	}

	fmt.Fprintf(b, "%sfor (%s; %s; %s) {\n", prefix, initStr, condStr, postStr)
	c.emitBlock(b, s.Body, indent+1)
	fmt.Fprintf(b, "%s}\n", prefix)
}

func (c *compiler) exprString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.BasicLit:
		return c.literalString(e)
	case *ast.BinaryExpr:
		return fmt.Sprintf("(%s %s %s)", c.exprString(e.X), e.Op.String(), c.exprString(e.Y))
	case *ast.UnaryExpr:
		return fmt.Sprintf("%s%s", e.Op.String(), c.exprString(e.X))
	case *ast.ParenExpr:
		return fmt.Sprintf("(%s)", c.exprString(e.X))
	case *ast.CallExpr:
		return c.callString(e)
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", c.exprString(e.X), e.Sel.Name)
	case *ast.IndexExpr:
		return fmt.Sprintf("%s[%s]", c.exprString(e.X), c.exprString(e.Index))
	case *ast.CompositeLit:
		return c.compositeLitString(e)
	default:
		return "/* unsupported expr */"
	}
}

func (c *compiler) literalString(lit *ast.BasicLit) string {
	s := lit.Value
	// GLSL requires float literals to have a decimal point.
	if lit.Kind == token.INT {
		return s
	}
	if lit.Kind == token.FLOAT {
		// Ensure it has a decimal point.
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		return s
	}
	return s
}

func (c *compiler) callString(call *ast.CallExpr) string {
	funcName := c.exprString(call.Fun)

	// Handle type constructors (vec2, vec3, vec4, mat4, etc.).
	if IsConstructor(funcName) {
		args := c.argsString(call.Args)
		return fmt.Sprintf("%s(%s)", funcName, args)
	}

	// Handle image built-ins.
	if IsImageBuiltin(funcName) {
		args := c.argsString(call.Args)
		return fmt.Sprintf("%s(%s)", funcName, args)
	}

	// Handle regular built-in functions.
	if glsl := GLSLBuiltin(funcName); glsl != "" {
		args := c.argsString(call.Args)
		return fmt.Sprintf("%s(%s)", glsl, args)
	}

	// User-defined function or unknown.
	args := c.argsString(call.Args)
	return fmt.Sprintf("%s(%s)", funcName, args)
}

func (c *compiler) compositeLitString(lit *ast.CompositeLit) string {
	typeName := c.typeString(lit.Type)
	args := make([]string, len(lit.Elts))
	for i, elt := range lit.Elts {
		args[i] = c.exprString(elt)
	}
	return fmt.Sprintf("%s(%s)", typeName, strings.Join(args, ", "))
}

func (c *compiler) argsString(args []ast.Expr) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = c.exprString(arg)
	}
	return strings.Join(parts, ", ")
}

func (c *compiler) typeString(expr ast.Expr) string {
	if expr == nil {
		return "float" // Default to float for untyped.
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.ArrayType:
		return c.typeString(e.Elt) + "[]"
	default:
		return "float"
	}
}

func (c *compiler) inferType(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.INT:
			return "int"
		case token.FLOAT:
			return "float"
		default:
			return "float"
		}
	case *ast.CallExpr:
		funcName := c.exprString(e.Fun)
		if IsConstructor(funcName) {
			return funcName
		}
		// Image builtins that sample textures return vec4.
		if IsImageBuiltin(funcName) && (len(funcName) >= 5 && funcName[len(funcName)-2:] == "At") {
			return "vec4"
		}
		// Image origin/size builtins return vec2.
		if IsImageBuiltin(funcName) {
			return "vec2"
		}
		// Built-in functions that always return scalar.
		switch funcName {
		case "dot", "length", "distance":
			return "float"
		}
		// Most GLSL built-in functions (normalize, clamp, mix, abs, min,
		// max, reflect, etc.) preserve their first argument's type.
		if len(e.Args) > 0 {
			return c.inferType(e.Args[0])
		}
		return "float"
	case *ast.ParenExpr:
		return c.inferType(e.X)
	case *ast.BinaryExpr:
		// Scalar-vector broadcasting: `float - vec3` yields vec3.
		// Matrix-vector / matrix-matrix multiplies follow GLSL rules.
		return widenBinaryType(c.inferType(e.X), c.inferType(e.Y), e.Op.String())
	case *ast.UnaryExpr:
		return c.inferType(e.X)
	case *ast.SelectorExpr:
		sel := e.Sel.Name
		// Swizzles use characters from xyzwrgba and are 1-4 chars.
		if len(sel) >= 1 && len(sel) <= 4 && isSwizzle(sel) {
			switch len(sel) {
			case 1:
				return "float"
			case 2:
				return "vec2"
			case 3:
				return "vec3"
			case 4:
				return "vec4"
			}
		}
		// Struct field access (e.g., uniforms.LightDir) — check if the
		// field is a known uniform and use its declared type.
		fieldName := sel
		for _, u := range c.uniforms {
			if u.Name == fieldName {
				return u.Type.GLSLName()
			}
		}
		// Fall back to inferring from the parent expression context.
		return "float"
	case *ast.Ident:
		// Check uniforms.
		for _, u := range c.uniforms {
			if u.Name == e.Name {
				return u.Type.GLSLName()
			}
		}
		// Check local variables.
		if t, ok := c.localTypes[e.Name]; ok {
			return t
		}
		return "float"

	default:
		return "float"
	}
}

// widenBinaryType returns the GLSL type of a binary operation on two operands.
// GLSL rules: scalar-vector broadcasts to the vector; matrix*vector yields
// the vector; matrix*matrix yields the matrix; matrix*scalar yields the
// matrix. Same-category same-size operands preserve their type.
func widenBinaryType(a, b, op string) string {
	isScalar := func(t string) bool { return t == "float" || t == "int" || t == "bool" }
	isVec := func(t string) bool { return strings.HasPrefix(t, "vec") }
	isMat := func(t string) bool { return strings.HasPrefix(t, "mat") }

	if op == "*" {
		switch {
		case isMat(a) && isVec(b):
			return b
		case isVec(a) && isMat(b):
			return a
		}
	}
	switch {
	case isScalar(a) && (isVec(b) || isMat(b)):
		return b
	case (isVec(a) || isMat(a)) && isScalar(b):
		return a
	}
	return a
}

// isSwizzle returns true if s consists only of swizzle characters (xyzwrgba).
func isSwizzle(s string) bool {
	for _, c := range s {
		switch c {
		case 'x', 'y', 'z', 'w', 'r', 'g', 'b', 'a':
			continue
		default:
			return false
		}
	}
	return s != ""
}
