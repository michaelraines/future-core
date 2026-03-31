package math

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestColorFromRGBA(t *testing.T) {
	c := ColorFromRGBA(255, 128, 0, 255)
	require.InDelta(t, 1.0, c.R, 0.01)
	require.InDelta(t, 1.0, c.A, 1e-9)
	require.InDelta(t, 128.0/255.0, c.G, 0.01)
}

func TestColorFromHex(t *testing.T) {
	c := ColorFromHex(0xFF0000FF)
	require.InDelta(t, ColorRed.R, c.R, 1e-9)
	require.InDelta(t, ColorRed.G, c.G, 1e-9)
	require.InDelta(t, ColorRed.B, c.B, 1e-9)
	require.InDelta(t, ColorRed.A, c.A, 1e-9)
}

func TestColorRGBA(t *testing.T) {
	r, g, b, a := ColorWhite.RGBA()
	require.Equal(t, uint32(0xffff), r)
	require.Equal(t, uint32(0xffff), g)
	require.Equal(t, uint32(0xffff), b)
	require.Equal(t, uint32(0xffff), a)
}

func TestColorRGBA8(t *testing.T) {
	r, g, b, a := ColorWhite.RGBA8()
	require.Equal(t, uint8(255), r)
	require.Equal(t, uint8(255), g)
	require.Equal(t, uint8(255), b)
	require.Equal(t, uint8(255), a)
}

func TestColorLerp(t *testing.T) {
	c := ColorBlack.Lerp(ColorWhite, 0.5)
	require.InDelta(t, 0.5, c.R, 1e-9)
	require.InDelta(t, 0.5, c.G, 1e-9)
}

func TestColorPremultiply(t *testing.T) {
	c := NewColor(1, 0.5, 0.25, 0.5).Premultiply()
	require.InDelta(t, 0.5, c.R, 1e-9)
	require.InDelta(t, 0.25, c.G, 1e-9)
	require.InDelta(t, 0.5, c.A, 1e-9)
}

func TestColorSRGBRoundtrip(t *testing.T) {
	c := NewColor(0.5, 0.2, 0.8, 1.0)
	roundtrip := c.ToSRGB().ToLinear()
	require.InDelta(t, c.R, roundtrip.R, 1e-6)
	require.InDelta(t, c.G, roundtrip.G, 1e-6)
	require.InDelta(t, c.B, roundtrip.B, 1e-6)
	require.InDelta(t, c.A, roundtrip.A, 1e-6)
}

func TestColorMatrixIdentity(t *testing.T) {
	cm := ColorMatrixIdentity()
	c := NewColor(0.3, 0.6, 0.9, 1.0)
	result := cm.Apply(c)
	require.InDelta(t, c.R, result.R, 1e-9)
	require.InDelta(t, c.G, result.G, 1e-9)
	require.InDelta(t, c.B, result.B, 1e-9)
	require.InDelta(t, c.A, result.A, 1e-9)
}

func TestColorMatrixScale(t *testing.T) {
	cm := ColorMatrixScale(0.5, 2, 0, 1)
	c := NewColor(1, 0.5, 1, 1)
	result := cm.Apply(c)
	require.InDelta(t, 0.5, result.R, 1e-9)
	require.InDelta(t, 1.0, result.G, 1e-9)
	require.InDelta(t, 0.0, result.B, 1e-9)
	require.InDelta(t, 1.0, result.A, 1e-9)
}

func TestColorMatrixConcat(t *testing.T) {
	a := ColorMatrixScale(2, 1, 1, 1)
	b := ColorMatrixTranslate(0.1, 0, 0, 0)
	combined := a.Concat(b)
	c := NewColor(0.5, 0.5, 0.5, 1)
	step := b.Apply(c)
	step = a.Apply(step)
	direct := combined.Apply(c)
	require.InDelta(t, step.R, direct.R, 1e-9)
	require.InDelta(t, step.G, direct.G, 1e-9)
	require.InDelta(t, step.B, direct.B, 1e-9)
	require.InDelta(t, step.A, direct.A, 1e-9)
}

func TestColorVec4(t *testing.T) {
	c := NewColor(0.1, 0.2, 0.3, 0.4)
	v := c.Vec4()
	require.InDelta(t, 0.1, v.X, 1e-9)
	require.InDelta(t, 0.2, v.Y, 1e-9)
	require.InDelta(t, 0.3, v.Z, 1e-9)
	require.InDelta(t, 0.4, v.W, 1e-9)
}

func TestColorMul(t *testing.T) {
	c := NewColor(0.5, 0.4, 0.3, 1.0).Mul(NewColor(2, 0.5, 1, 0.5))
	require.InDelta(t, 1.0, c.R, 1e-9)
	require.InDelta(t, 0.2, c.G, 1e-9)
	require.InDelta(t, 0.3, c.B, 1e-9)
	require.InDelta(t, 0.5, c.A, 1e-9)
}

func TestColorScale(t *testing.T) {
	c := NewColor(0.5, 0.4, 0.3, 1.0).Scale(2)
	require.InDelta(t, 1.0, c.R, 1e-9)
	require.InDelta(t, 0.8, c.G, 1e-9)
	require.InDelta(t, 0.6, c.B, 1e-9)
	require.InDelta(t, 1.0, c.A, 1e-9) // alpha unchanged
}

func TestColorWithAlpha(t *testing.T) {
	c := ColorRed.WithAlpha(0.5)
	require.InDelta(t, 1.0, c.R, 1e-9)
	require.InDelta(t, 0.0, c.G, 1e-9)
	require.InDelta(t, 0.0, c.B, 1e-9)
	require.InDelta(t, 0.5, c.A, 1e-9)
}

func TestColorClamp(t *testing.T) {
	c := NewColor(1.5, -0.5, 0.5, 2.0).Clamp()
	require.InDelta(t, 1.0, c.R, 1e-9)
	require.InDelta(t, 0.0, c.G, 1e-9)
	require.InDelta(t, 0.5, c.B, 1e-9)
	require.InDelta(t, 1.0, c.A, 1e-9)
}

func TestColorString(t *testing.T) {
	s := NewColor(1, 0, 0, 1).String()
	require.Equal(t, "Color(1, 0, 0, 1)", s)
}

func TestColorMatrixIsIdentity(t *testing.T) {
	require.True(t, ColorMatrixIdentity().IsIdentity())
	require.False(t, ColorMatrixScale(2, 1, 1, 1).IsIdentity())
}

func TestColorApproxEqual(t *testing.T) {
	a := NewColor(1, 0, 0, 1)
	b := NewColor(1.0000000001, 0, 0, 1)
	require.True(t, a.ApproxEqual(b, 1e-9))
	require.False(t, a.ApproxEqual(NewColor(0.5, 0, 0, 1), 1e-9))
}

func TestColorSRGBLowValues(t *testing.T) {
	// Test the linear branch of sRGB conversion (c <= 0.0031308)
	c := NewColor(0.001, 0.001, 0.001, 1.0)
	srgb := c.ToSRGB()
	require.InDelta(t, 0.001*12.92, srgb.R, 1e-9)

	// And back
	back := srgb.ToLinear()
	require.InDelta(t, c.R, back.R, 1e-9)
}
