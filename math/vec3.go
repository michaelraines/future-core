package math

import (
	"fmt"
	gomath "math"
)

// Vec3 represents a 3D vector or point.
type Vec3 struct {
	X, Y, Z float64
}

// NewVec3 creates a Vec3 from x, y, z components.
func NewVec3(x, y, z float64) Vec3 {
	return Vec3{X: x, Y: y, Z: z}
}

// Vec3Zero returns the zero vector.
func Vec3Zero() Vec3 { return Vec3{} }

// Vec3One returns a vector with all components set to 1.
func Vec3One() Vec3 { return Vec3{X: 1, Y: 1, Z: 1} }

// Vec3UnitX returns the unit vector along the X axis.
func Vec3UnitX() Vec3 { return Vec3{X: 1} }

// Vec3UnitY returns the unit vector along the Y axis.
func Vec3UnitY() Vec3 { return Vec3{Y: 1} }

// Vec3UnitZ returns the unit vector along the Z axis.
func Vec3UnitZ() Vec3 { return Vec3{Z: 1} }

// Vec2 returns the XY components as a Vec2.
func (v Vec3) Vec2() Vec2 {
	return Vec2{X: v.X, Y: v.Y}
}

// Add returns the sum of v and other.
func (v Vec3) Add(other Vec3) Vec3 {
	return Vec3{X: v.X + other.X, Y: v.Y + other.Y, Z: v.Z + other.Z}
}

// Sub returns the difference of v and other.
func (v Vec3) Sub(other Vec3) Vec3 {
	return Vec3{X: v.X - other.X, Y: v.Y - other.Y, Z: v.Z - other.Z}
}

// Mul returns v scaled by s.
func (v Vec3) Mul(s float64) Vec3 {
	return Vec3{X: v.X * s, Y: v.Y * s, Z: v.Z * s}
}

// Div returns v divided by s.
func (v Vec3) Div(s float64) Vec3 {
	return Vec3{X: v.X / s, Y: v.Y / s, Z: v.Z / s}
}

// MulVec returns the component-wise product of v and other.
func (v Vec3) MulVec(other Vec3) Vec3 {
	return Vec3{X: v.X * other.X, Y: v.Y * other.Y, Z: v.Z * other.Z}
}

// Dot returns the dot product of v and other.
func (v Vec3) Dot(other Vec3) float64 {
	return v.X*other.X + v.Y*other.Y + v.Z*other.Z
}

// Cross returns the cross product of v and other.
func (v Vec3) Cross(other Vec3) Vec3 {
	return Vec3{
		X: v.Y*other.Z - v.Z*other.Y,
		Y: v.Z*other.X - v.X*other.Z,
		Z: v.X*other.Y - v.Y*other.X,
	}
}

// Len returns the length of v.
func (v Vec3) Len() float64 {
	return gomath.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z)
}

// LenSq returns the squared length of v.
func (v Vec3) LenSq() float64 {
	return v.X*v.X + v.Y*v.Y + v.Z*v.Z
}

// Normalize returns a unit vector in the direction of v.
// Returns the zero vector if v has zero length.
func (v Vec3) Normalize() Vec3 {
	l := v.Len()
	if l == 0 {
		return Vec3{}
	}
	return Vec3{X: v.X / l, Y: v.Y / l, Z: v.Z / l}
}

// Distance returns the distance between v and other.
func (v Vec3) Distance(other Vec3) float64 {
	return v.Sub(other).Len()
}

// DistanceSq returns the squared distance between v and other.
func (v Vec3) DistanceSq(other Vec3) float64 {
	return v.Sub(other).LenSq()
}

// Lerp returns the linear interpolation between v and other by t.
func (v Vec3) Lerp(other Vec3, t float64) Vec3 {
	return Vec3{
		X: v.X + (other.X-v.X)*t,
		Y: v.Y + (other.Y-v.Y)*t,
		Z: v.Z + (other.Z-v.Z)*t,
	}
}

// Negate returns the negation of v.
func (v Vec3) Negate() Vec3 {
	return Vec3{X: -v.X, Y: -v.Y, Z: -v.Z}
}

// Reflect returns v reflected across the given normal.
// The normal must be normalized.
func (v Vec3) Reflect(normal Vec3) Vec3 {
	d := 2.0 * v.Dot(normal)
	return Vec3{X: v.X - d*normal.X, Y: v.Y - d*normal.Y, Z: v.Z - d*normal.Z}
}

// Clamp returns v with each component clamped to [lo, hi].
func (v Vec3) Clamp(lo, hi Vec3) Vec3 {
	return Vec3{
		X: clamp(v.X, lo.X, hi.X),
		Y: clamp(v.Y, lo.Y, hi.Y),
		Z: clamp(v.Z, lo.Z, hi.Z),
	}
}

// Abs returns v with each component as its absolute value.
func (v Vec3) Abs() Vec3 {
	return Vec3{X: gomath.Abs(v.X), Y: gomath.Abs(v.Y), Z: gomath.Abs(v.Z)}
}

// Min returns the component-wise minimum of v and other.
func (v Vec3) Min(other Vec3) Vec3 {
	return Vec3{
		X: gomath.Min(v.X, other.X),
		Y: gomath.Min(v.Y, other.Y),
		Z: gomath.Min(v.Z, other.Z),
	}
}

// Max returns the component-wise maximum of v and other.
func (v Vec3) Max(other Vec3) Vec3 {
	return Vec3{
		X: gomath.Max(v.X, other.X),
		Y: gomath.Max(v.Y, other.Y),
		Z: gomath.Max(v.Z, other.Z),
	}
}

// ApproxEqual returns whether v and other are approximately equal within epsilon.
func (v Vec3) ApproxEqual(other Vec3, epsilon float64) bool {
	return gomath.Abs(v.X-other.X) <= epsilon &&
		gomath.Abs(v.Y-other.Y) <= epsilon &&
		gomath.Abs(v.Z-other.Z) <= epsilon
}

// String returns a human-readable representation of v.
func (v Vec3) String() string {
	return fmt.Sprintf("Vec3(%g, %g, %g)", v.X, v.Y, v.Z)
}
