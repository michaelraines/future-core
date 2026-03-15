package math

import (
	gomath "math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQuatIdentity(t *testing.T) {
	q := QuatIdentity()
	v := q.RotateVec3(NewVec3(1, 2, 3))
	require.InDelta(t, 1.0, v.X, 1e-9)
	require.InDelta(t, 2.0, v.Y, 1e-9)
	require.InDelta(t, 3.0, v.Z, 1e-9)
}

func TestQuatFromAxisAngle(t *testing.T) {
	q := QuatFromAxisAngle(Vec3UnitZ(), gomath.Pi/2)
	v := q.RotateVec3(NewVec3(1, 0, 0))
	require.InDelta(t, 0.0, v.X, 1e-9)
	require.InDelta(t, 1.0, v.Y, 1e-9)
	require.InDelta(t, 0.0, v.Z, 1e-9)
}

func TestQuatMul(t *testing.T) {
	q1 := QuatFromAxisAngle(Vec3UnitZ(), gomath.Pi/2)
	q2 := QuatFromAxisAngle(Vec3UnitZ(), gomath.Pi/2)
	q := q1.Mul(q2) // 180 degrees around Z
	v := q.RotateVec3(NewVec3(1, 0, 0))
	require.InDelta(t, -1.0, v.X, 1e-9)
	require.InDelta(t, 0.0, v.Y, 1e-9)
	require.InDelta(t, 0.0, v.Z, 1e-9)
}

func TestQuatInverse(t *testing.T) {
	q := QuatFromAxisAngle(Vec3UnitY(), gomath.Pi/3)
	inv := q.Inverse()
	product := q.Mul(inv)
	require.True(t, product.ApproxEqual(QuatIdentity(), 1e-9))
}

func TestQuatSlerp(t *testing.T) {
	q1 := QuatIdentity()
	q2 := QuatFromAxisAngle(Vec3UnitZ(), gomath.Pi/2)
	mid := q1.Slerp(q2, 0.5)
	v := mid.RotateVec3(NewVec3(1, 0, 0))
	expected := NewVec2(1, 0).Rotate(gomath.Pi / 4)
	require.InDelta(t, expected.X, v.X, 1e-9)
	require.InDelta(t, expected.Y, v.Y, 1e-9)
}

func TestQuatToMat4(t *testing.T) {
	q := QuatFromAxisAngle(Vec3UnitZ(), gomath.Pi/2)
	m := q.ToMat4()
	v := m.MulVec3Point(NewVec3(1, 0, 0))
	require.InDelta(t, 0.0, v.X, 1e-9)
	require.InDelta(t, 1.0, v.Y, 1e-9)
	require.InDelta(t, 0.0, v.Z, 1e-9)
}

func TestQuatToAxisAngle(t *testing.T) {
	axis := Vec3UnitY()
	angle := gomath.Pi / 3
	q := QuatFromAxisAngle(axis, angle)
	gotAxis, gotAngle := q.ToAxisAngle()
	require.InDelta(t, axis.X, gotAxis.X, 1e-9)
	require.InDelta(t, axis.Y, gotAxis.Y, 1e-9)
	require.InDelta(t, axis.Z, gotAxis.Z, 1e-9)
	require.InDelta(t, angle, gotAngle, 1e-9)
}

func TestQuatFromEuler(t *testing.T) {
	// QuatFromEuler(pitch, yaw, roll) with ZYX rotation order
	// yaw=pi/2 rotates (1,0,0) to (0,1,0)
	q := QuatFromEuler(0, gomath.Pi/2, 0)
	v := q.RotateVec3(NewVec3(1, 0, 0))
	require.InDelta(t, 0.0, v.X, 1e-9)
	require.InDelta(t, 1.0, v.Y, 1e-9)
	require.InDelta(t, 0.0, v.Z, 1e-9)

	// Identity from zero angles
	q2 := QuatFromEuler(0, 0, 0)
	require.True(t, q2.ApproxEqual(QuatIdentity(), 1e-9))
}

func TestQuatConjugate(t *testing.T) {
	q := QuatFromAxisAngle(Vec3UnitY(), gomath.Pi/4)
	c := q.Conjugate()
	require.InDelta(t, -q.X, c.X, 1e-9)
	require.InDelta(t, -q.Y, c.Y, 1e-9)
	require.InDelta(t, -q.Z, c.Z, 1e-9)
	require.InDelta(t, q.W, c.W, 1e-9)
}

func TestQuatString(t *testing.T) {
	q := QuatIdentity()
	s := q.String()
	require.Equal(t, "Quat(0, 0, 0, 1)", s)
}

func TestQuatNormalize(t *testing.T) {
	q := Quat{X: 1, Y: 2, Z: 3, W: 4}
	n := q.Normalize()
	require.InDelta(t, 1.0, n.Len(), 1e-9)

	// Zero quaternion normalizes to identity
	z := Quat{}.Normalize()
	require.Equal(t, QuatIdentity(), z)
}

func TestQuatDot(t *testing.T) {
	q1 := QuatIdentity()
	q2 := QuatIdentity()
	require.InDelta(t, 1.0, q1.Dot(q2), 1e-9)
}

func TestQuatLenSq(t *testing.T) {
	q := QuatIdentity()
	require.InDelta(t, 1.0, q.LenSq(), 1e-9)
}

func TestQuatSlerpEndpoints(t *testing.T) {
	q1 := QuatFromAxisAngle(Vec3UnitX(), gomath.Pi/6)
	q2 := QuatFromAxisAngle(Vec3UnitX(), gomath.Pi/3)

	// t=0 should return q1
	s0 := q1.Slerp(q2, 0)
	require.True(t, s0.ApproxEqual(q1, 1e-9))

	// t=1 should return q2
	s1 := q1.Slerp(q2, 1)
	require.True(t, s1.ApproxEqual(q2, 1e-9))
}

func TestQuatInverseZero(t *testing.T) {
	q := Quat{}
	inv := q.Inverse()
	require.Equal(t, QuatIdentity(), inv)
}

func TestQuatApproxEqualDoubleCover(t *testing.T) {
	q := QuatFromAxisAngle(Vec3{X: 0, Y: 1, Z: 0}, Pi/4)
	neg := Quat{X: -q.X, Y: -q.Y, Z: -q.Z, W: -q.W}
	require.True(t, q.ApproxEqual(neg, 1e-9), "q and -q represent the same rotation")
}

func TestQuatSlerpNegativeDot(t *testing.T) {
	q1 := QuatFromAxisAngle(Vec3{X: 0, Y: 1, Z: 0}, 0.1)
	// Create a quaternion where dot product is negative
	q2 := QuatFromAxisAngle(Vec3{X: 0, Y: 1, Z: 0}, Pi+0.1)
	result := q1.Slerp(q2, 0.5)
	require.InDelta(t, 1.0, result.Len(), 1e-6, "slerp result should be unit quaternion")
}

func TestQuatSlerpNearIdentity(t *testing.T) {
	q1 := QuatIdentity()
	q2 := QuatFromAxisAngle(Vec3{X: 0, Y: 1, Z: 0}, 1e-6)
	result := q1.Slerp(q2, 0.5)
	require.InDelta(t, 1.0, result.Len(), 1e-6)
}

func TestQuatToAxisAngleIdentity(t *testing.T) {
	axis, angle := QuatIdentity().ToAxisAngle()
	require.InDelta(t, 0.0, angle, 1e-9)
	// When angle is near zero, axis defaults to (1,0,0)
	require.InDelta(t, 1.0, axis.X, 1e-9)
}

func BenchmarkQuatRotateVec3(b *testing.B) {
	q := QuatFromAxisAngle(Vec3UnitY(), 0.5)
	v := NewVec3(1, 2, 3)
	for b.Loop() {
		_ = q.RotateVec3(v)
	}
}

func BenchmarkQuatSlerp(b *testing.B) {
	q1 := QuatIdentity()
	q2 := QuatFromAxisAngle(Vec3UnitZ(), gomath.Pi/2)
	for b.Loop() {
		_ = q1.Slerp(q2, 0.5)
	}
}
