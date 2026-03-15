package math

import (
	"fmt"
	gomath "math"
)

// Vec2 represents a 2D vector or point.
type Vec2 struct {
	X, Y float64
}

// NewVec2 creates a Vec2 from x and y components.
func NewVec2(x, y float64) Vec2 {
	return Vec2{X: x, Y: y}
}

// Vec2Zero returns the zero vector.
func Vec2Zero() Vec2 { return Vec2{} }

// Vec2One returns a vector with all components set to 1.
func Vec2One() Vec2 { return Vec2{X: 1, Y: 1} }

// Vec2Up returns the up direction (0, -1) in screen coordinates.
func Vec2Up() Vec2 { return Vec2{X: 0, Y: -1} }

// Vec2Down returns the down direction (0, 1) in screen coordinates.
func Vec2Down() Vec2 { return Vec2{X: 0, Y: 1} }

// Vec2Left returns the left direction (-1, 0).
func Vec2Left() Vec2 { return Vec2{X: -1, Y: 0} }

// Vec2Right returns the right direction (1, 0).
func Vec2Right() Vec2 { return Vec2{X: 1, Y: 0} }

// Add returns the sum of v and other.
func (v Vec2) Add(other Vec2) Vec2 {
	return Vec2{X: v.X + other.X, Y: v.Y + other.Y}
}

// Sub returns the difference of v and other.
func (v Vec2) Sub(other Vec2) Vec2 {
	return Vec2{X: v.X - other.X, Y: v.Y - other.Y}
}

// Mul returns v scaled by s.
func (v Vec2) Mul(s float64) Vec2 {
	return Vec2{X: v.X * s, Y: v.Y * s}
}

// Div returns v divided by s. If s is zero, the result contains +Inf or NaN.
func (v Vec2) Div(s float64) Vec2 {
	return Vec2{X: v.X / s, Y: v.Y / s}
}

// MulVec returns the component-wise product of v and other.
func (v Vec2) MulVec(other Vec2) Vec2 {
	return Vec2{X: v.X * other.X, Y: v.Y * other.Y}
}

// DivVec returns the component-wise quotient of v and other.
func (v Vec2) DivVec(other Vec2) Vec2 {
	return Vec2{X: v.X / other.X, Y: v.Y / other.Y}
}

// Dot returns the dot product of v and other.
func (v Vec2) Dot(other Vec2) float64 {
	return v.X*other.X + v.Y*other.Y
}

// Cross returns the 2D cross product (the z-component of the 3D cross product
// with z=0). This is useful for determining winding order and signed area.
func (v Vec2) Cross(other Vec2) float64 {
	return v.X*other.Y - v.Y*other.X
}

// Len returns the length (magnitude) of v.
func (v Vec2) Len() float64 {
	return gomath.Hypot(v.X, v.Y)
}

// LenSq returns the squared length of v. Faster than Len when only
// comparison is needed.
func (v Vec2) LenSq() float64 {
	return v.X*v.X + v.Y*v.Y
}

// Normalize returns a unit vector in the direction of v.
// Returns the zero vector if v has zero length.
func (v Vec2) Normalize() Vec2 {
	l := v.Len()
	if l == 0 {
		return Vec2{}
	}
	return Vec2{X: v.X / l, Y: v.Y / l}
}

// Distance returns the distance between v and other.
func (v Vec2) Distance(other Vec2) float64 {
	return v.Sub(other).Len()
}

// DistanceSq returns the squared distance between v and other.
func (v Vec2) DistanceSq(other Vec2) float64 {
	return v.Sub(other).LenSq()
}

// Lerp returns the linear interpolation between v and other by t.
// t=0 returns v, t=1 returns other.
func (v Vec2) Lerp(other Vec2, t float64) Vec2 {
	return Vec2{
		X: v.X + (other.X-v.X)*t,
		Y: v.Y + (other.Y-v.Y)*t,
	}
}

// Rotate returns v rotated by angle radians around the origin.
func (v Vec2) Rotate(angle float64) Vec2 {
	sin, cos := gomath.Sincos(angle)
	return Vec2{
		X: v.X*cos - v.Y*sin,
		Y: v.X*sin + v.Y*cos,
	}
}

// Angle returns the angle of v from the positive X axis in radians [-π, π].
func (v Vec2) Angle() float64 {
	return gomath.Atan2(v.Y, v.X)
}

// AngleTo returns the signed angle from v to other in radians [-π, π].
func (v Vec2) AngleTo(other Vec2) float64 {
	return gomath.Atan2(v.Cross(other), v.Dot(other))
}

// Negate returns the negation of v.
func (v Vec2) Negate() Vec2 {
	return Vec2{X: -v.X, Y: -v.Y}
}

// Perpendicular returns a vector perpendicular to v (rotated 90° counter-clockwise).
func (v Vec2) Perpendicular() Vec2 {
	return Vec2{X: -v.Y, Y: v.X}
}

// Reflect returns v reflected across the given normal.
// The normal must be normalized.
func (v Vec2) Reflect(normal Vec2) Vec2 {
	d := 2.0 * v.Dot(normal)
	return Vec2{X: v.X - d*normal.X, Y: v.Y - d*normal.Y}
}

// Clamp returns v with each component clamped to [lo, hi].
func (v Vec2) Clamp(lo, hi Vec2) Vec2 {
	return Vec2{
		X: clamp(v.X, lo.X, hi.X),
		Y: clamp(v.Y, lo.Y, hi.Y),
	}
}

// Floor returns v with each component floored.
func (v Vec2) Floor() Vec2 {
	return Vec2{X: gomath.Floor(v.X), Y: gomath.Floor(v.Y)}
}

// Ceil returns v with each component ceiled.
func (v Vec2) Ceil() Vec2 {
	return Vec2{X: gomath.Ceil(v.X), Y: gomath.Ceil(v.Y)}
}

// Round returns v with each component rounded to the nearest integer.
func (v Vec2) Round() Vec2 {
	return Vec2{X: gomath.Round(v.X), Y: gomath.Round(v.Y)}
}

// Abs returns v with each component as its absolute value.
func (v Vec2) Abs() Vec2 {
	return Vec2{X: gomath.Abs(v.X), Y: gomath.Abs(v.Y)}
}

// Min returns the component-wise minimum of v and other.
func (v Vec2) Min(other Vec2) Vec2 {
	return Vec2{X: gomath.Min(v.X, other.X), Y: gomath.Min(v.Y, other.Y)}
}

// Max returns the component-wise maximum of v and other.
func (v Vec2) Max(other Vec2) Vec2 {
	return Vec2{X: gomath.Max(v.X, other.X), Y: gomath.Max(v.Y, other.Y)}
}

// ApproxEqual returns whether v and other are approximately equal within epsilon.
func (v Vec2) ApproxEqual(other Vec2, epsilon float64) bool {
	return gomath.Abs(v.X-other.X) <= epsilon && gomath.Abs(v.Y-other.Y) <= epsilon
}

// String returns a human-readable representation of v.
func (v Vec2) String() string {
	return fmt.Sprintf("Vec2(%g, %g)", v.X, v.Y)
}
