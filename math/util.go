package math

import gomath "math"

// Common mathematical constants.
const (
	Pi      = gomath.Pi
	Deg2Rad = Pi / 180.0
	Rad2Deg = 180.0 / Pi
	Epsilon = 1e-10
)

// clamp restricts v to the range [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Clamp restricts v to the range [lo, hi].
func Clamp(v, lo, hi float64) float64 {
	return clamp(v, lo, hi)
}

// Lerp returns the linear interpolation between a and b by t.
func Lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

// InverseLerp returns the interpolation factor t such that Lerp(a, b, t) == v.
func InverseLerp(a, b, v float64) float64 {
	if a == b {
		return 0
	}
	return (v - a) / (b - a)
}

// Remap maps v from the range [inMin, inMax] to [outMin, outMax].
func Remap(v, inMin, inMax, outMin, outMax float64) float64 {
	t := InverseLerp(inMin, inMax, v)
	return Lerp(outMin, outMax, t)
}

// ApproxEqual returns whether a and b are approximately equal within epsilon.
func ApproxEqual(a, b, epsilon float64) bool {
	return gomath.Abs(a-b) <= epsilon
}

// SmoothStep returns a smooth Hermite interpolation between 0 and 1 when
// edge0 < v < edge1. If edge0 == edge1, returns 0.
func SmoothStep(edge0, edge1, v float64) float64 {
	if edge0 == edge1 {
		return 0
	}
	t := clamp((v-edge0)/(edge1-edge0), 0, 1)
	return t * t * (3 - 2*t)
}

// NextPowerOf2 returns the smallest power of 2 >= n.
// For n > 2^31, the result wraps to 0 due to uint32 overflow.
// If n is already a power of 2, it is returned unchanged.
func NextPowerOf2(n uint32) uint32 {
	if n == 0 {
		return 1
	}
	if n > (1 << 31) {
		return 0
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	return n + 1
}

// IsPowerOf2 returns whether n is a power of 2.
func IsPowerOf2(n uint32) bool {
	return n > 0 && (n&(n-1)) == 0
}
