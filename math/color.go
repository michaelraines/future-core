package math

import (
	"fmt"
	gomath "math"
)

// Color represents an RGBA color with components in the range [0, 1].
// Colors are stored in linear space unless otherwise noted.
type Color struct {
	R, G, B, A float64
}

// NewColor creates a Color from RGBA components in [0, 1].
func NewColor(r, g, b, a float64) Color {
	return Color{R: r, G: g, B: b, A: a}
}

// ColorFromRGBA creates a Color from 8-bit RGBA values [0, 255].
func ColorFromRGBA(r, g, b, a uint8) Color {
	return Color{
		R: float64(r) / 255.0,
		G: float64(g) / 255.0,
		B: float64(b) / 255.0,
		A: float64(a) / 255.0,
	}
}

// ColorFromHex creates a Color from a 32-bit RGBA hex value (0xRRGGBBAA).
func ColorFromHex(hex uint32) Color {
	return Color{
		R: float64((hex>>24)&0xFF) / 255.0,
		G: float64((hex>>16)&0xFF) / 255.0,
		B: float64((hex>>8)&0xFF) / 255.0,
		A: float64(hex&0xFF) / 255.0,
	}
}

// Predefined colors.
var (
	ColorWhite       = Color{R: 1, G: 1, B: 1, A: 1}
	ColorBlack       = Color{R: 0, G: 0, B: 0, A: 1}
	ColorTransparent = Color{R: 0, G: 0, B: 0, A: 0}
	ColorRed         = Color{R: 1, G: 0, B: 0, A: 1}
	ColorGreen       = Color{R: 0, G: 1, B: 0, A: 1}
	ColorBlue        = Color{R: 0, G: 0, B: 1, A: 1}
	ColorYellow      = Color{R: 1, G: 1, B: 0, A: 1}
	ColorCyan        = Color{R: 0, G: 1, B: 1, A: 1}
	ColorMagenta     = Color{R: 1, G: 0, B: 1, A: 1}
)

// RGBA returns the color as 8-bit RGBA values.
func (c Color) RGBA() (r, g, b, a uint8) {
	return uint8(gomath.Round(clamp(c.R, 0, 1) * 255)),
		uint8(gomath.Round(clamp(c.G, 0, 1) * 255)),
		uint8(gomath.Round(clamp(c.B, 0, 1) * 255)),
		uint8(gomath.Round(clamp(c.A, 0, 1) * 255))
}

// Vec4 returns the color as a Vec4 (R, G, B, A).
func (c Color) Vec4() Vec4 {
	return Vec4{X: c.R, Y: c.G, Z: c.B, W: c.A}
}

// Mul returns the component-wise product of two colors.
func (c Color) Mul(other Color) Color {
	return Color{
		R: c.R * other.R,
		G: c.G * other.G,
		B: c.B * other.B,
		A: c.A * other.A,
	}
}

// Scale returns the color with RGB components scaled by s. Alpha is unchanged.
func (c Color) Scale(s float64) Color {
	return Color{R: c.R * s, G: c.G * s, B: c.B * s, A: c.A}
}

// WithAlpha returns the color with alpha set to a.
func (c Color) WithAlpha(a float64) Color {
	return Color{R: c.R, G: c.G, B: c.B, A: a}
}

// Lerp returns the linear interpolation between c and other by t.
func (c Color) Lerp(other Color, t float64) Color {
	return Color{
		R: c.R + (other.R-c.R)*t,
		G: c.G + (other.G-c.G)*t,
		B: c.B + (other.B-c.B)*t,
		A: c.A + (other.A-c.A)*t,
	}
}

// Clamp returns the color with all components clamped to [0, 1].
func (c Color) Clamp() Color {
	return Color{
		R: clamp(c.R, 0, 1),
		G: clamp(c.G, 0, 1),
		B: clamp(c.B, 0, 1),
		A: clamp(c.A, 0, 1),
	}
}

// Premultiply returns the color with RGB pre-multiplied by alpha.
func (c Color) Premultiply() Color {
	return Color{R: c.R * c.A, G: c.G * c.A, B: c.B * c.A, A: c.A}
}

// ToSRGB converts a linear-space color to sRGB space.
func (c Color) ToSRGB() Color {
	return Color{
		R: linearToSRGB(c.R),
		G: linearToSRGB(c.G),
		B: linearToSRGB(c.B),
		A: c.A,
	}
}

// ToLinear converts an sRGB-space color to linear space.
func (c Color) ToLinear() Color {
	return Color{
		R: sRGBToLinear(c.R),
		G: sRGBToLinear(c.G),
		B: sRGBToLinear(c.B),
		A: c.A,
	}
}

// ApproxEqual returns whether c and other are approximately equal within epsilon.
func (c Color) ApproxEqual(other Color, epsilon float64) bool {
	return gomath.Abs(c.R-other.R) <= epsilon &&
		gomath.Abs(c.G-other.G) <= epsilon &&
		gomath.Abs(c.B-other.B) <= epsilon &&
		gomath.Abs(c.A-other.A) <= epsilon
}

// String returns a human-readable representation of c.
func (c Color) String() string {
	return fmt.Sprintf("Color(%g, %g, %g, %g)", c.R, c.G, c.B, c.A)
}

// linearToSRGB converts a single linear component to sRGB.
func linearToSRGB(c float64) float64 {
	if c <= 0.0031308 {
		return c * 12.92
	}
	return 1.055*gomath.Pow(c, 1.0/2.4) - 0.055
}

// sRGBToLinear converts a single sRGB component to linear.
func sRGBToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return gomath.Pow((c+0.055)/1.055, 2.4)
}

// ColorMatrix represents a 5x4 color transformation matrix.
// This is used for per-draw-call color manipulation, matching Ebitengine's
// ColorM functionality.
//
// The matrix transforms colors as:
//
//	| R' |   | M[0]  M[1]  M[2]  M[3]  M[4]  |   | R |
//	| G' | = | M[5]  M[6]  M[7]  M[8]  M[9]  | × | G |
//	| B' |   | M[10] M[11] M[12] M[13] M[14] |   | B |
//	| A' |   | M[15] M[16] M[17] M[18] M[19] |   | A |
//	                                               | 1 |
type ColorMatrix [20]float64

// ColorMatrixIdentity returns the identity color matrix (no transformation).
func ColorMatrixIdentity() ColorMatrix {
	return ColorMatrix{
		1, 0, 0, 0, 0,
		0, 1, 0, 0, 0,
		0, 0, 1, 0, 0,
		0, 0, 0, 1, 0,
	}
}

// ColorMatrixScale returns a color matrix that scales RGBA by the given factors.
func ColorMatrixScale(r, g, b, a float64) ColorMatrix {
	return ColorMatrix{
		r, 0, 0, 0, 0,
		0, g, 0, 0, 0,
		0, 0, b, 0, 0,
		0, 0, 0, a, 0,
	}
}

// ColorMatrixTranslate returns a color matrix that offsets RGBA by the given amounts.
func ColorMatrixTranslate(r, g, b, a float64) ColorMatrix {
	return ColorMatrix{
		1, 0, 0, 0, r,
		0, 1, 0, 0, g,
		0, 0, 1, 0, b,
		0, 0, 0, 1, a,
	}
}

// Apply transforms a color by this matrix.
func (cm ColorMatrix) Apply(c Color) Color {
	return Color{
		R: cm[0]*c.R + cm[1]*c.G + cm[2]*c.B + cm[3]*c.A + cm[4],
		G: cm[5]*c.R + cm[6]*c.G + cm[7]*c.B + cm[8]*c.A + cm[9],
		B: cm[10]*c.R + cm[11]*c.G + cm[12]*c.B + cm[13]*c.A + cm[14],
		A: cm[15]*c.R + cm[16]*c.G + cm[17]*c.B + cm[18]*c.A + cm[19],
	}
}

// Concat returns the composition of cm and other (cm applied after other).
func (cm ColorMatrix) Concat(other ColorMatrix) ColorMatrix {
	var result ColorMatrix
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			result[r*5+c] = cm[r*5]*other[c] +
				cm[r*5+1]*other[5+c] +
				cm[r*5+2]*other[10+c] +
				cm[r*5+3]*other[15+c]
		}
		// Translation column
		result[r*5+4] = cm[r*5]*other[4] +
			cm[r*5+1]*other[9] +
			cm[r*5+2]*other[14] +
			cm[r*5+3]*other[19] +
			cm[r*5+4]
	}
	return result
}

// IsIdentity returns whether cm is the identity matrix.
func (cm ColorMatrix) IsIdentity() bool {
	id := ColorMatrixIdentity()
	return cm == id
}
