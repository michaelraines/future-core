package math

import (
	gomath "math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMat4Identity(t *testing.T) {
	m := Mat4Identity()
	v := m.MulVec4(NewVec4(1, 2, 3, 1))
	require.InDelta(t, 1.0, v.X, 1e-9)
	require.InDelta(t, 2.0, v.Y, 1e-9)
	require.InDelta(t, 3.0, v.Z, 1e-9)
	require.InDelta(t, 1.0, v.W, 1e-9)
}

func TestMat4Translate(t *testing.T) {
	m := Mat4Translate(10, 20, 30)
	v := m.MulVec3Point(NewVec3(1, 2, 3))
	require.InDelta(t, 11.0, v.X, 1e-9)
	require.InDelta(t, 22.0, v.Y, 1e-9)
	require.InDelta(t, 33.0, v.Z, 1e-9)
}

func TestMat4Scale(t *testing.T) {
	m := Mat4Scale(2, 3, 4)
	v := m.MulVec3Point(NewVec3(1, 1, 1))
	require.InDelta(t, 2.0, v.X, 1e-9)
	require.InDelta(t, 3.0, v.Y, 1e-9)
	require.InDelta(t, 4.0, v.Z, 1e-9)
}

func TestMat4RotateZ(t *testing.T) {
	m := Mat4RotateZ(gomath.Pi / 2)
	v := m.MulVec3Point(NewVec3(1, 0, 0))
	require.InDelta(t, 0.0, v.X, 1e-9)
	require.InDelta(t, 1.0, v.Y, 1e-9)
	require.InDelta(t, 0.0, v.Z, 1e-9)
}

func TestMat4MulIdentity(t *testing.T) {
	a := Mat4Translate(1, 2, 3)
	b := Mat4Identity()
	c := a.Mul(b)
	require.True(t, c.ApproxEqual(a, 1e-9))
}

func TestMat4Inverse(t *testing.T) {
	m := Mat4Translate(5, 10, 15).Mul(Mat4Scale(2, 3, 4))
	inv, ok := m.Inverse()
	require.True(t, ok, "expected invertible matrix")
	product := m.Mul(inv)
	require.True(t, product.ApproxEqual(Mat4Identity(), 1e-9))
}

func TestMat4InverseSingular(t *testing.T) {
	m := Mat4{} // zero matrix
	_, ok := m.Inverse()
	require.False(t, ok, "zero matrix should not be invertible")
}

func TestMat4Determinant(t *testing.T) {
	d := Mat4Identity().Determinant()
	require.InDelta(t, 1.0, d, 1e-9)

	d = Mat4Scale(2, 3, 4).Determinant()
	require.InDelta(t, 24.0, d, 1e-9)
}

func TestMat4Ortho(t *testing.T) {
	m := Mat4Ortho(0, 800, 600, 0, -1, 1)
	center := m.MulVec3Point(NewVec3(400, 300, 0))
	require.InDelta(t, 0.0, center.X, 1e-9)
	require.InDelta(t, 0.0, center.Y, 1e-9)
	require.InDelta(t, 0.0, center.Z, 1e-9)
}

func TestMat4Perspective(t *testing.T) {
	m := Mat4Perspective(gomath.Pi/4, 16.0/9.0, 0.1, 100)
	p := m.MulVec4(NewVec4(0, 0, -0.1, 1))
	ndc := p.PerspectiveDivide()
	require.InDelta(t, -1.0, ndc.Z, 1e-6)
}

func TestMat4LookAt(t *testing.T) {
	m := Mat4LookAt(NewVec3(0, 0, 5), Vec3Zero(), Vec3UnitY())
	p := m.MulVec3Point(NewVec3(0, 0, 5))
	require.InDelta(t, 0.0, p.X, 1e-9)
	require.InDelta(t, 0.0, p.Y, 1e-9)
	require.InDelta(t, 0.0, p.Z, 1e-9)
}

func TestMat4Translation(t *testing.T) {
	m := Mat4Translate(1, 2, 3)
	tr := m.Translation()
	require.InDelta(t, 1.0, tr.X, 1e-9)
	require.InDelta(t, 2.0, tr.Y, 1e-9)
	require.InDelta(t, 3.0, tr.Z, 1e-9)
}

func TestMat4Float32(t *testing.T) {
	m := Mat4Identity()
	f := m.Float32()
	require.Equal(t, float32(1), f[0])
	require.Equal(t, float32(1), f[5])
	require.Equal(t, float32(1), f[10])
	require.Equal(t, float32(1), f[15])
	require.Equal(t, float32(0), f[1])
	require.Equal(t, float32(0), f[4])

	ortho := Mat4Ortho(0, 800, 600, 0, -1, 1)
	of := ortho.Float32()
	require.NotEqual(t, float32(0), of[0])
	require.NotEqual(t, float32(0), of[5])
}

func TestMat4RotateX(t *testing.T) {
	m := Mat4RotateX(gomath.Pi / 2)
	v := m.MulVec3Point(NewVec3(0, 1, 0))
	require.InDelta(t, 0.0, v.X, 1e-9)
	require.InDelta(t, 0.0, v.Y, 1e-9)
	require.InDelta(t, 1.0, v.Z, 1e-9)
}

func TestMat4RotateY(t *testing.T) {
	m := Mat4RotateY(gomath.Pi / 2)
	v := m.MulVec3Point(NewVec3(1, 0, 0))
	require.InDelta(t, 0.0, v.X, 1e-9)
	require.InDelta(t, 0.0, v.Y, 1e-9)
	require.InDelta(t, -1.0, v.Z, 1e-9)
}

func TestMat4RotateAxis(t *testing.T) {
	// Rotating around Z axis should give same result as Mat4RotateZ
	m := Mat4RotateAxis(Vec3UnitZ(), gomath.Pi/2)
	v := m.MulVec3Point(NewVec3(1, 0, 0))
	require.InDelta(t, 0.0, v.X, 1e-9)
	require.InDelta(t, 1.0, v.Y, 1e-9)
	require.InDelta(t, 0.0, v.Z, 1e-9)
}

func TestMat4At(t *testing.T) {
	m := Mat4Identity()
	require.InDelta(t, 1.0, m.At(0, 0), 1e-9)
	require.InDelta(t, 0.0, m.At(0, 1), 1e-9)
	require.InDelta(t, 1.0, m.At(3, 3), 1e-9)
}

func TestMat4Set(t *testing.T) {
	m := Mat4Identity()
	m = m.Set(0, 1, 5.0)
	require.InDelta(t, 5.0, m.At(0, 1), 1e-9)
	require.InDelta(t, 1.0, m.At(0, 0), 1e-9)
}

func TestMat4Col(t *testing.T) {
	m := Mat4Translate(1, 2, 3)
	col3 := m.Col(3)
	require.InDelta(t, 1.0, col3.X, 1e-9)
	require.InDelta(t, 2.0, col3.Y, 1e-9)
	require.InDelta(t, 3.0, col3.Z, 1e-9)
	require.InDelta(t, 1.0, col3.W, 1e-9)
}

func TestMat4Row(t *testing.T) {
	m := Mat4Translate(1, 2, 3)
	row0 := m.Row(0)
	require.InDelta(t, 1.0, row0.X, 1e-9)
	require.InDelta(t, 0.0, row0.Y, 1e-9)
	require.InDelta(t, 0.0, row0.Z, 1e-9)
	require.InDelta(t, 1.0, row0.W, 1e-9)
}

func TestMat4MulVec3Dir(t *testing.T) {
	m := Mat4Translate(10, 20, 30)
	// Direction should not be affected by translation
	v := m.MulVec3Dir(NewVec3(1, 0, 0))
	require.InDelta(t, 1.0, v.X, 1e-9)
	require.InDelta(t, 0.0, v.Y, 1e-9)
	require.InDelta(t, 0.0, v.Z, 1e-9)

	// Direction should be affected by scale
	m2 := Mat4Scale(2, 3, 4)
	v2 := m2.MulVec3Dir(NewVec3(1, 1, 1))
	require.InDelta(t, 2.0, v2.X, 1e-9)
	require.InDelta(t, 3.0, v2.Y, 1e-9)
	require.InDelta(t, 4.0, v2.Z, 1e-9)
}

func TestMat4Transpose(t *testing.T) {
	m := Mat4Translate(1, 2, 3)
	mt := m.Transpose()
	// Transpose of transpose should be the original
	require.True(t, m.ApproxEqual(mt.Transpose(), 1e-9))
	// Check a specific value: m[12] = 1 (translation X) should be at mt[3]
	require.InDelta(t, m.At(0, 3), mt.At(3, 0), 1e-9)
}

func TestMat4Mat3(t *testing.T) {
	m := Mat4Scale(2, 3, 4)
	m3 := m.Mat3()
	require.InDelta(t, 2.0, m3.At(0, 0), 1e-9)
	require.InDelta(t, 3.0, m3.At(1, 1), 1e-9)
	require.InDelta(t, 4.0, m3.At(2, 2), 1e-9)
	require.InDelta(t, 0.0, m3.At(0, 1), 1e-9)
}

func TestMat4String(t *testing.T) {
	s := Mat4Identity().String()
	require.Contains(t, s, "Mat4[")
}

func TestMat4MulVec3PointWZero(t *testing.T) {
	// Create a matrix where the w component will be zero
	m := Mat4{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 0, // w row is zero
	}
	result := m.MulVec3Point(Vec3{X: 1, Y: 2, Z: 3})
	require.InDelta(t, 0.0, result.X, 1e-9)
	require.InDelta(t, 0.0, result.Y, 1e-9)
	require.InDelta(t, 0.0, result.Z, 1e-9)
}

func BenchmarkMat4Mul(b *testing.B) {
	a := Mat4Translate(1, 2, 3)
	c := Mat4RotateZ(0.5)
	for b.Loop() {
		_ = a.Mul(c)
	}
}

func BenchmarkMat4Inverse(b *testing.B) {
	m := Mat4Translate(1, 2, 3).Mul(Mat4Scale(2, 3, 4))
	for b.Loop() {
		_, _ = m.Inverse()
	}
}

func BenchmarkMat4MulVec4(b *testing.B) {
	m := Mat4Translate(1, 2, 3)
	v := NewVec4(1, 2, 3, 1)
	for b.Loop() {
		_ = m.MulVec4(v)
	}
}
