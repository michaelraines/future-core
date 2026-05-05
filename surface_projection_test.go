package futurerender

import (
	"testing"

	"github.com/stretchr/testify/require"

	fmath "github.com/michaelraines/future-core/math"
)

// transform applies the projection to a logical-screen point and
// returns the resulting clip-space (x, y).
func transformPoint(m fmath.Mat4, x, y float64) (clipX, clipY float64) {
	v := m.MulVec4(fmath.Vec4{X: x, Y: y, Z: 0, W: 1})
	return v.X, v.Y
}

func TestBuildAndroidProjection(t *testing.T) {
	// Each case picks a distinct (W, H) so unparam doesn't flag the
	// dimension parameters as constant. The clip-space expectations are
	// invariant to the screen size — Mat4Ortho normalises (0,0)→(-1,+1)
	// and (W,H)→(+1,-1) regardless — so the rotation can be checked
	// against fixed clip-space targets.
	tests := []struct {
		name                       string
		w, h                       int
		rotationDeg                int
		topLeftX, topLeftY         float64
		bottomRightX, bottomRightY float64
	}{
		{
			name: "unrotated",
			w:    2600, h: 1200,
			rotationDeg: 0,
			topLeftX:    -1, topLeftY: +1,
			bottomRightX: +1, bottomRightY: -1,
		},
		{
			// VK_SURFACE_TRANSFORM_ROTATE_90 → engine pre-rotates by
			// -90° (CW in our Y-up clip space). Logical-(0,0) (the
			// unrotated clip top-left, -1,+1) maps to clip (+1,+1);
			// logical-(W,H) (clip +1,-1) maps to clip (-1,-1).
			name: "rotate90",
			w:    1280, h: 720,
			rotationDeg: 90,
			topLeftX:    +1, topLeftY: +1,
			bottomRightX: -1, bottomRightY: -1,
		},
		{
			// 180°: every clip-space coordinate negates regardless of
			// rotation direction.
			name: "rotate180",
			w:    800, h: 600,
			rotationDeg: 180,
			topLeftX:    +1, topLeftY: -1,
			bottomRightX: -1, bottomRightY: +1,
		},
		{
			// VK_SURFACE_TRANSFORM_ROTATE_270 → engine pre-rotates by
			// -270° = +90° CCW. Logical-(0,0) → clip (-1,-1); logical-
			// (W,H) → clip (+1,+1).
			name: "rotate270",
			w:    1024, h: 768,
			rotationDeg: 270,
			topLeftX:    -1, topLeftY: -1,
			bottomRightX: +1, bottomRightY: +1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proj := buildAndroidProjection(tt.w, tt.h, tt.rotationDeg)

			cx, cy := transformPoint(proj, 0, 0)
			require.InDelta(t, tt.topLeftX, cx, 1e-9, "logical (0,0) → clip X")
			require.InDelta(t, tt.topLeftY, cy, 1e-9, "logical (0,0) → clip Y")

			cx, cy = transformPoint(proj, float64(tt.w), float64(tt.h))
			require.InDelta(t, tt.bottomRightX, cx, 1e-9, "logical (W,H) → clip X")
			require.InDelta(t, tt.bottomRightY, cy, 1e-9, "logical (W,H) → clip Y")
		})
	}
}

func TestBuildAndroidProjectionUnknownRotationFallsThrough(t *testing.T) {
	// Anything other than 90/180/270 should be treated as 0 (no
	// rotation). 45° isn't a valid SurfaceTransform — silently
	// rotating by it would scramble the rendering on any future
	// backend that reports an unrecognized transform.
	rotated := buildAndroidProjection(640, 480, 45)
	plain := buildAndroidProjection(640, 480, 0)
	for i := range rotated {
		require.InDelta(t, plain[i], rotated[i], 1e-12)
	}
}
