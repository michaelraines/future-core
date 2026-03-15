package shaderir

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// --- Type tests ---

func TestTypeGLSLName(t *testing.T) {
	tests := []struct {
		t    Type
		want string
	}{
		{TypeBool, "bool"},
		{TypeInt, "int"},
		{TypeFloat, "float"},
		{TypeVec2, "vec2"},
		{TypeVec3, "vec3"},
		{TypeVec4, "vec4"},
		{TypeIVec2, "ivec2"},
		{TypeIVec3, "ivec3"},
		{TypeIVec4, "ivec4"},
		{TypeMat2, "mat2"},
		{TypeMat3, "mat3"},
		{TypeMat4, "mat4"},
		{TypeTexture, "sampler2D"},
		{TypeNone, "void"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require.Equal(t, tt.want, tt.t.GLSLName())
		})
	}
}

func TestTypeString(t *testing.T) {
	require.Equal(t, "float", TypeFloat.String())
	require.Equal(t, "vec4", TypeVec4.String())
	require.Equal(t, "texture", TypeTexture.String())
	require.Equal(t, "none", TypeNone.String())
}

func TestParseType(t *testing.T) {
	tests := []struct {
		name    string
		want    Type
		wantErr bool
	}{
		{"float", TypeFloat, false},
		{"vec2", TypeVec2, false},
		{"vec3", TypeVec3, false},
		{"vec4", TypeVec4, false},
		{"mat4", TypeMat4, false},
		{"int", TypeInt, false},
		{"bool", TypeBool, false},
		{"mat2", TypeMat2, false},
		{"mat3", TypeMat3, false},
		{"ivec2", TypeIVec2, false},
		{"ivec3", TypeIVec3, false},
		{"ivec4", TypeIVec4, false},
		{"unknown", TypeNone, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseType(tt.name)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestUniformSize(t *testing.T) {
	require.Equal(t, 1, TypeFloat.UniformSize())
	require.Equal(t, 1, TypeInt.UniformSize())
	require.Equal(t, 1, TypeBool.UniformSize())
	require.Equal(t, 2, TypeVec2.UniformSize())
	require.Equal(t, 3, TypeVec3.UniformSize())
	require.Equal(t, 4, TypeVec4.UniformSize())
	require.Equal(t, 4, TypeMat2.UniformSize())
	require.Equal(t, 9, TypeMat3.UniformSize())
	require.Equal(t, 16, TypeMat4.UniformSize())
	require.Equal(t, 0, TypeNone.UniformSize())
	require.Equal(t, 2, TypeIVec2.UniformSize())
	require.Equal(t, 3, TypeIVec3.UniformSize())
	require.Equal(t, 4, TypeIVec4.UniformSize())
}

// --- Builtin tests ---

func TestIsBuiltinFunc(t *testing.T) {
	require.True(t, IsBuiltinFunc("sin"))
	require.True(t, IsBuiltinFunc("cos"))
	require.True(t, IsBuiltinFunc("mix"))
	require.True(t, IsBuiltinFunc("imageSrc0At"))
	require.True(t, IsBuiltinFunc("imageDstSize"))
	require.False(t, IsBuiltinFunc("myCustomFunc"))
}

func TestGLSLBuiltin(t *testing.T) {
	require.Equal(t, "sin", GLSLBuiltin("sin"))
	require.Equal(t, "atan", GLSLBuiltin("atan2"))
	require.Equal(t, "dFdx", GLSLBuiltin("dfdx"))
	require.Equal(t, "", GLSLBuiltin("imageSrc0At"))
	require.Equal(t, "", GLSLBuiltin("unknown"))
}

func TestIsImageBuiltin(t *testing.T) {
	require.True(t, IsImageBuiltin("imageSrc0At"))
	require.True(t, IsImageBuiltin("imageSrc3UnsafeAt"))
	require.True(t, IsImageBuiltin("imageDstOrigin"))
	require.True(t, IsImageBuiltin("imageDstSize"))
	require.False(t, IsImageBuiltin("sin"))
}

func TestIsConstructor(t *testing.T) {
	require.True(t, IsConstructor("vec2"))
	require.True(t, IsConstructor("vec4"))
	require.True(t, IsConstructor("mat4"))
	require.True(t, IsConstructor("float"))
	require.False(t, IsConstructor("myType"))
}

// --- Compile tests ---

func TestCompileSimpleFragment(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return imageSrc0At(srcPos)
}
`)
	result, err := Compile(src)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Vertex shader should have #version 330 core.
	require.Contains(t, result.VertexShader, "#version 330 core")
	require.Contains(t, result.VertexShader, "uProjection")
	require.Contains(t, result.VertexShader, "aPosition")

	// Fragment shader should have image helper functions.
	require.Contains(t, result.FragmentShader, "#version 330 core")
	require.Contains(t, result.FragmentShader, "imageSrc0At")
	require.Contains(t, result.FragmentShader, "fragColor")

	require.Empty(t, result.Uniforms)
}

func TestCompileWithUniforms(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

var Time float
var Color vec4

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return color
}
`)
	result, err := Compile(src)
	require.NoError(t, err)
	require.Len(t, result.Uniforms, 2)
	require.Equal(t, "Time", result.Uniforms[0].Name)
	require.Equal(t, TypeFloat, result.Uniforms[0].Type)
	require.Equal(t, "Color", result.Uniforms[1].Name)
	require.Equal(t, TypeVec4, result.Uniforms[1].Type)

	// User uniforms should appear in GLSL.
	require.Contains(t, result.FragmentShader, "uniform float Time")
	require.Contains(t, result.FragmentShader, "uniform vec4 Color")
}

func TestCompileWithPixelUnit(t *testing.T) {
	src := []byte(`
//go:build ignore

//kage:unit pixels

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return color
}
`)
	result, err := Compile(src)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestCompileWithIfStatement(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

var Threshold float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	c := imageSrc0At(srcPos)
	if c.a < Threshold {
		return vec4(0)
	}
	return c
}
`)
	result, err := Compile(src)
	require.NoError(t, err)
	require.Contains(t, result.FragmentShader, "if (")
}

func TestCompileWithForLoop(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	c := vec4(0)
	for i := 0; i < 4; i++ {
		c += imageSrc0At(srcPos)
	}
	return c
}
`)
	result, err := Compile(src)
	require.NoError(t, err)
	require.Contains(t, result.FragmentShader, "for (")
}

func TestCompileWithMathBuiltins(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	v := sin(srcPos.x) * cos(srcPos.y)
	return vec4(v, v, v, 1.0)
}
`)
	result, err := Compile(src)
	require.NoError(t, err)
	require.Contains(t, result.FragmentShader, "sin(")
	require.Contains(t, result.FragmentShader, "cos(")
}

func TestCompileMissingFragment(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

func NotFragment(x int) int {
	return x
}
`)
	_, err := Compile(src)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing Fragment")
}

func TestCompileInvalidSyntax(t *testing.T) {
	src := []byte(`this is not valid Go`)
	_, err := Compile(src)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse error")
}

func TestCompileInvalidUniformType(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

var Foo string

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return color
}
`)
	_, err := Compile(src)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown type")
}

func TestCompileFragmentWrongReturnType(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) float {
	return 1.0
}
`)
	_, err := Compile(src)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must return vec4")
}

func TestCompileImageHelpers(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	origin := imageSrc0Origin()
	size := imageSrc0Size()
	return imageSrc0At(srcPos + origin + size)
}
`)
	result, err := Compile(src)
	require.NoError(t, err)

	// Should generate helper functions for all 4 texture slots.
	for i := 0; i < 4; i++ {
		require.True(t, strings.Contains(result.FragmentShader, "imageSrc"+string(rune('0'+i))+"At"))
		require.True(t, strings.Contains(result.FragmentShader, "imageSrc"+string(rune('0'+i))+"UnsafeAt"))
		require.True(t, strings.Contains(result.FragmentShader, "imageSrc"+string(rune('0'+i))+"Origin"))
		require.True(t, strings.Contains(result.FragmentShader, "imageSrc"+string(rune('0'+i))+"Size"))
	}
	require.Contains(t, result.FragmentShader, "imageDstOrigin")
	require.Contains(t, result.FragmentShader, "imageDstSize")
}

func TestCompileImageDstTextureSize(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	size := imageDstTextureSize()
	return vec4(size, 0.0, 1.0)
}
`)
	result, err := Compile(src)
	require.NoError(t, err)
	require.Contains(t, result.FragmentShader, "imageDstTextureSize")
	// The helper should be defined as a function returning vec2.
	require.Contains(t, result.FragmentShader, "vec2 imageDstTextureSize()")
}

func TestCompileVaryings(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return color
}
`)
	result, err := Compile(src)
	require.NoError(t, err)

	// Fragment shader should declare varyings.
	require.Contains(t, result.FragmentShader, "in vec2 vTexCoord")
	require.Contains(t, result.FragmentShader, "in vec4 vColor")
	require.Contains(t, result.FragmentShader, "in vec4 vDstPos")
}

func TestCompileCustomParameterNames(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

func Fragment(pos vec4, uv vec2, col vec4) vec4 {
	return col
}
`)
	result, err := Compile(src)
	require.NoError(t, err)
	// Should use the custom parameter names in the main body.
	require.Contains(t, result.FragmentShader, "vec4 pos = vDstPos")
	require.Contains(t, result.FragmentShader, "vec2 uv = vTexCoord")
	require.Contains(t, result.FragmentShader, "vec4 col = vColor")
}

func TestCompileElseIfChain(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	if color.a > 0.5 {
		return color
	} else {
		return vec4(0)
	}
}
`)
	result, err := Compile(src)
	require.NoError(t, err)
	require.Contains(t, result.FragmentShader, "} else {")
}

func TestCompileIncDecStmt(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	var x int
	x++
	x--
	return color
}
`)
	result, err := Compile(src)
	require.NoError(t, err)
	require.Contains(t, result.FragmentShader, "x++")
	require.Contains(t, result.FragmentShader, "x--")
}

func TestCompileAssignOps(t *testing.T) {
	src := []byte(`
//go:build ignore

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	c := color
	c += vec4(0.1, 0.1, 0.1, 0)
	c *= vec4(0.5, 0.5, 0.5, 1)
	return c
}
`)
	result, err := Compile(src)
	require.NoError(t, err)
	require.Contains(t, result.FragmentShader, "+=")
	require.Contains(t, result.FragmentShader, "*=")
}
