package math

import (
	"fmt"
	gomath "math"
)

// Quat represents a quaternion for 3D rotation.
// The quaternion is stored as (X, Y, Z, W) where W is the scalar component.
type Quat struct {
	X, Y, Z, W float64
}

// QuatIdentity returns the identity quaternion (no rotation).
func QuatIdentity() Quat {
	return Quat{W: 1}
}

// QuatFromAxisAngle creates a quaternion from an axis and angle (radians).
// The axis must be normalized.
func QuatFromAxisAngle(axis Vec3, angle float64) Quat {
	halfAngle := angle / 2.0
	sin, cos := gomath.Sincos(halfAngle)
	return Quat{
		X: axis.X * sin,
		Y: axis.Y * sin,
		Z: axis.Z * sin,
		W: cos,
	}
}

// QuatFromEuler creates a quaternion from Euler angles (radians).
// Rotation order is ZYX (yaw, pitch, roll).
func QuatFromEuler(pitch, yaw, roll float64) Quat {
	sp, cp := gomath.Sincos(pitch / 2.0)
	sy, cy := gomath.Sincos(yaw / 2.0)
	sr, cr := gomath.Sincos(roll / 2.0)

	return Quat{
		X: sr*cp*cy - cr*sp*sy,
		Y: cr*sp*cy + sr*cp*sy,
		Z: cr*cp*sy - sr*sp*cy,
		W: cr*cp*cy + sr*sp*sy,
	}
}

// Mul returns the quaternion product of q and other.
// This represents the composition of rotations: first other, then q.
func (q Quat) Mul(other Quat) Quat {
	return Quat{
		X: q.W*other.X + q.X*other.W + q.Y*other.Z - q.Z*other.Y,
		Y: q.W*other.Y - q.X*other.Z + q.Y*other.W + q.Z*other.X,
		Z: q.W*other.Z + q.X*other.Y - q.Y*other.X + q.Z*other.W,
		W: q.W*other.W - q.X*other.X - q.Y*other.Y - q.Z*other.Z,
	}
}

// RotateVec3 rotates a Vec3 by this quaternion.
func (q Quat) RotateVec3(v Vec3) Vec3 {
	// Optimized quaternion-vector rotation: v' = q * v * q^-1
	// Using the formula: v' = v + 2w(u × v) + 2(u × (u × v))
	// where q = (u, w), u = (x, y, z)
	u := Vec3{X: q.X, Y: q.Y, Z: q.Z}
	uv := u.Cross(v)
	uuv := u.Cross(uv)
	return v.Add(uv.Mul(2 * q.W)).Add(uuv.Mul(2))
}

// Dot returns the dot product of q and other.
func (q Quat) Dot(other Quat) float64 {
	return q.X*other.X + q.Y*other.Y + q.Z*other.Z + q.W*other.W
}

// Len returns the length of q.
func (q Quat) Len() float64 {
	return gomath.Sqrt(q.X*q.X + q.Y*q.Y + q.Z*q.Z + q.W*q.W)
}

// LenSq returns the squared length of q.
func (q Quat) LenSq() float64 {
	return q.X*q.X + q.Y*q.Y + q.Z*q.Z + q.W*q.W
}

// Normalize returns a unit quaternion.
func (q Quat) Normalize() Quat {
	l := q.Len()
	if l == 0 {
		return QuatIdentity()
	}
	return Quat{X: q.X / l, Y: q.Y / l, Z: q.Z / l, W: q.W / l}
}

// Conjugate returns the conjugate of q.
func (q Quat) Conjugate() Quat {
	return Quat{X: -q.X, Y: -q.Y, Z: -q.Z, W: q.W}
}

// Inverse returns the inverse of q.
func (q Quat) Inverse() Quat {
	lenSq := q.LenSq()
	if lenSq == 0 {
		return QuatIdentity()
	}
	invLen := 1.0 / lenSq
	return Quat{X: -q.X * invLen, Y: -q.Y * invLen, Z: -q.Z * invLen, W: q.W * invLen}
}

// Slerp returns the spherical linear interpolation between q and other by t.
// t=0 returns q, t=1 returns other.
func (q Quat) Slerp(other Quat, t float64) Quat {
	dot := q.Dot(other)

	// If the dot product is negative, negate one quaternion to take the
	// shorter arc.
	if dot < 0 {
		other = Quat{X: -other.X, Y: -other.Y, Z: -other.Z, W: -other.W}
		dot = -dot
	}

	// If quaternions are very close, use linear interpolation to avoid
	// division by zero in sin.
	if dot > 0.9995 {
		return Quat{
			X: q.X + (other.X-q.X)*t,
			Y: q.Y + (other.Y-q.Y)*t,
			Z: q.Z + (other.Z-q.Z)*t,
			W: q.W + (other.W-q.W)*t,
		}.Normalize()
	}

	theta := gomath.Acos(clamp(dot, -1, 1))
	sinTheta := gomath.Sin(theta)
	s0 := gomath.Sin((1-t)*theta) / sinTheta
	s1 := gomath.Sin(t*theta) / sinTheta

	return Quat{
		X: q.X*s0 + other.X*s1,
		Y: q.Y*s0 + other.Y*s1,
		Z: q.Z*s0 + other.Z*s1,
		W: q.W*s0 + other.W*s1,
	}
}

// ToMat4 converts the quaternion to a 4x4 rotation matrix.
func (q Quat) ToMat4() Mat4 {
	x2 := q.X * q.X
	y2 := q.Y * q.Y
	z2 := q.Z * q.Z
	xy := q.X * q.Y
	xz := q.X * q.Z
	yz := q.Y * q.Z
	wx := q.W * q.X
	wy := q.W * q.Y
	wz := q.W * q.Z

	return Mat4{
		1 - 2*(y2+z2), 2 * (xy + wz), 2 * (xz - wy), 0,
		2 * (xy - wz), 1 - 2*(x2+z2), 2 * (yz + wx), 0,
		2 * (xz + wy), 2 * (yz - wx), 1 - 2*(x2+y2), 0,
		0, 0, 0, 1,
	}
}

// ToAxisAngle converts the quaternion to axis-angle representation.
// Returns the axis and angle in radians.
func (q Quat) ToAxisAngle() (axis Vec3, angle float64) {
	q = q.Normalize()
	angle = 2.0 * gomath.Acos(clamp(q.W, -1, 1))
	sin := gomath.Sqrt(1 - q.W*q.W)
	if sin < 1e-10 {
		return Vec3{X: 1}, angle
	}
	return Vec3{X: q.X / sin, Y: q.Y / sin, Z: q.Z / sin}, angle
}

// ApproxEqual returns whether q and other are approximately equal within epsilon.
func (q Quat) ApproxEqual(other Quat, epsilon float64) bool {
	// Quaternions q and -q represent the same rotation (double cover).
	// Compare against both and accept the closer one.
	if q.Dot(other) < 0 {
		other = Quat{X: -other.X, Y: -other.Y, Z: -other.Z, W: -other.W}
	}
	return gomath.Abs(q.X-other.X) <= epsilon &&
		gomath.Abs(q.Y-other.Y) <= epsilon &&
		gomath.Abs(q.Z-other.Z) <= epsilon &&
		gomath.Abs(q.W-other.W) <= epsilon
}

// String returns a human-readable representation of q.
func (q Quat) String() string {
	return fmt.Sprintf("Quat(%g, %g, %g, %g)", q.X, q.Y, q.Z, q.W)
}
