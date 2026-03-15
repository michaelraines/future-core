package math

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClamp(t *testing.T) {
	tests := []struct {
		name           string
		v, lo, hi, exp float64
	}{
		{"in range", 5, 0, 10, 5},
		{"below lo", -1, 0, 10, 0},
		{"above hi", 15, 0, 10, 10},
		{"at lo", 0, 0, 10, 0},
		{"at hi", 10, 0, 10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.InDelta(t, tt.exp, Clamp(tt.v, tt.lo, tt.hi), 1e-9)
		})
	}
}

func TestLerp(t *testing.T) {
	tests := []struct {
		name       string
		a, b, tVal float64
		expected   float64
	}{
		{"t=0", 0, 10, 0, 0},
		{"t=1", 0, 10, 1, 10},
		{"t=0.5", 0, 10, 0.5, 5},
		{"negative", -10, 10, 0.5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.InDelta(t, tt.expected, Lerp(tt.a, tt.b, tt.tVal), 1e-9)
		})
	}
}

func TestInverseLerp(t *testing.T) {
	tests := []struct {
		name     string
		a, b, v  float64
		expected float64
	}{
		{"middle", 0, 10, 5, 0.5},
		{"start", 0, 10, 0, 0},
		{"end", 0, 10, 10, 1},
		{"equal a b", 5, 5, 5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.InDelta(t, tt.expected, InverseLerp(tt.a, tt.b, tt.v), 1e-9)
		})
	}
}

func TestRemap(t *testing.T) {
	// Map 5 from [0, 10] to [0, 100]
	require.InDelta(t, 50.0, Remap(5, 0, 10, 0, 100), 1e-9)
	// Map 0 from [0, 10] to [100, 200]
	require.InDelta(t, 100.0, Remap(0, 0, 10, 100, 200), 1e-9)
}

func TestSmoothStep(t *testing.T) {
	tests := []struct {
		name     string
		e0, e1   float64
		v        float64
		expected float64
	}{
		{"below edge0", 0, 1, -1, 0},
		{"above edge1", 0, 1, 2, 1},
		{"middle", 0, 1, 0.5, 0.5},
		{"at edge0", 0, 1, 0, 0},
		{"at edge1", 0, 1, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.InDelta(t, tt.expected, SmoothStep(tt.e0, tt.e1, tt.v), 1e-9)
		})
	}
}

func TestNextPowerOf2(t *testing.T) {
	tests := []struct {
		name     string
		n        uint32
		expected uint32
	}{
		{"zero", 0, 1},
		{"one", 1, 1},
		{"two", 2, 2},
		{"three", 3, 4},
		{"five", 5, 8},
		{"sixteen", 16, 16},
		{"seventeen", 17, 32},
		{"255", 255, 256},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, NextPowerOf2(tt.n))
		})
	}
}

func TestIsPowerOf2(t *testing.T) {
	tests := []struct {
		name     string
		n        uint32
		expected bool
	}{
		{"zero", 0, false},
		{"one", 1, true},
		{"two", 2, true},
		{"three", 3, false},
		{"four", 4, true},
		{"256", 256, true},
		{"100", 100, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expected {
				require.True(t, IsPowerOf2(tt.n))
			} else {
				require.False(t, IsPowerOf2(tt.n))
			}
		})
	}
}

func TestSmoothStepEqualEdges(t *testing.T) {
	require.InDelta(t, 0.0, SmoothStep(5, 5, 10), 1e-9)
	require.InDelta(t, 0.0, SmoothStep(5, 5, 0), 1e-9)
}

func TestNextPowerOf2Overflow(t *testing.T) {
	require.Equal(t, uint32(0), NextPowerOf2(1<<31+1))
}

func TestApproxEqual(t *testing.T) {
	require.True(t, ApproxEqual(1.0, 1.0000000001, 1e-9))
	require.False(t, ApproxEqual(1.0, 2.0, 1e-9))
}
