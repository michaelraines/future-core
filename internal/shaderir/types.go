// Package shaderir provides a Kage-to-GLSL transpiler for Future Render.
//
// Kage is a Go-syntax-compatible shading language used by Ebitengine.
// This package parses Kage source and emits GLSL 330 core shaders.
package shaderir

import "fmt"

// Type represents a Kage/shader type.
type Type int

// Type constants for Kage/GLSL types.
const (
	TypeNone Type = iota
	TypeBool
	TypeInt
	TypeFloat
	TypeVec2
	TypeVec3
	TypeVec4
	TypeIVec2
	TypeIVec3
	TypeIVec4
	TypeMat2
	TypeMat3
	TypeMat4
	TypeTexture
)

// GLSLName returns the GLSL type name for this type.
func (t Type) GLSLName() string {
	switch t {
	case TypeBool:
		return "bool"
	case TypeInt:
		return "int"
	case TypeFloat:
		return "float"
	case TypeVec2:
		return "vec2"
	case TypeVec3:
		return "vec3"
	case TypeVec4:
		return "vec4"
	case TypeIVec2:
		return "ivec2"
	case TypeIVec3:
		return "ivec3"
	case TypeIVec4:
		return "ivec4"
	case TypeMat2:
		return "mat2"
	case TypeMat3:
		return "mat3"
	case TypeMat4:
		return "mat4"
	case TypeTexture:
		return "sampler2D"
	default:
		return "void"
	}
}

// String returns the Kage type name.
func (t Type) String() string {
	switch t {
	case TypeBool:
		return "bool"
	case TypeInt:
		return "int"
	case TypeFloat:
		return "float"
	case TypeVec2:
		return "vec2"
	case TypeVec3:
		return "vec3"
	case TypeVec4:
		return "vec4"
	case TypeIVec2:
		return "ivec2"
	case TypeIVec3:
		return "ivec3"
	case TypeIVec4:
		return "ivec4"
	case TypeMat2:
		return "mat2"
	case TypeMat3:
		return "mat3"
	case TypeMat4:
		return "mat4"
	case TypeTexture:
		return "texture"
	default:
		return "none"
	}
}

// ParseType maps a Kage type name to a Type constant.
func ParseType(name string) (Type, error) {
	switch name {
	case "bool":
		return TypeBool, nil
	case "int":
		return TypeInt, nil
	case "float":
		return TypeFloat, nil
	case "vec2":
		return TypeVec2, nil
	case "vec3":
		return TypeVec3, nil
	case "vec4":
		return TypeVec4, nil
	case "ivec2":
		return TypeIVec2, nil
	case "ivec3":
		return TypeIVec3, nil
	case "ivec4":
		return TypeIVec4, nil
	case "mat2":
		return TypeMat2, nil
	case "mat3":
		return TypeMat3, nil
	case "mat4":
		return TypeMat4, nil
	case "texture", "sampler2D":
		return TypeTexture, nil
	default:
		return TypeNone, fmt.Errorf("shaderir: unknown type %q", name)
	}
}

// Uniform represents a shader uniform variable.
type Uniform struct {
	Name string
	Type Type
}

// UniformSize returns the number of float32 values needed for this type.
func (t Type) UniformSize() int {
	switch t {
	case TypeBool, TypeInt, TypeFloat:
		return 1
	case TypeVec2, TypeIVec2:
		return 2
	case TypeVec3, TypeIVec3:
		return 3
	case TypeVec4, TypeIVec4:
		return 4
	case TypeMat2:
		return 4
	case TypeMat3:
		return 9
	case TypeMat4:
		return 16
	default:
		return 0
	}
}
