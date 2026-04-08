package vector

import (
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"

	futurerender "github.com/michaelraines/future-core"
)

func TestPathMoveTo(t *testing.T) {
	var p Path
	p.MoveTo(10, 20)
	require.Len(t, p.current.points, 1)
	require.InDelta(t, float32(10), p.current.points[0].x, 1e-6)
	require.InDelta(t, float32(20), p.current.points[0].y, 1e-6)
}

func TestPathLineTo(t *testing.T) {
	var p Path
	p.MoveTo(0, 0)
	p.LineTo(10, 20)
	require.Len(t, p.current.points, 2)
}

func TestPathLineToWithoutMoveTo(t *testing.T) {
	var p Path
	p.LineTo(10, 20)
	// Should auto-add origin.
	require.Len(t, p.current.points, 2)
	require.InDelta(t, float32(0), p.current.points[0].x, 1e-6)
}

func TestPathClose(t *testing.T) {
	var p Path
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	p.LineTo(10, 10)
	p.Close()
	require.Len(t, p.subPaths, 1)
	require.True(t, p.subPaths[0].closed)
	require.Len(t, p.current.points, 0)
}

func TestPathMultipleSubPaths(t *testing.T) {
	var p Path
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	p.Close()
	p.MoveTo(20, 20)
	p.LineTo(30, 20)
	p.Close()
	require.Len(t, p.subPaths, 2)
}

func TestPathQuadTo(t *testing.T) {
	var p Path
	p.MoveTo(0, 0)
	p.QuadTo(50, 50, 100, 0)
	// Should have start point plus bezier-interpolated points.
	require.Greater(t, len(p.current.points), 2)
	// Last point should be approximately (100, 0).
	last := p.current.points[len(p.current.points)-1]
	require.InDelta(t, float32(100), last.x, 1e-3)
	require.InDelta(t, float32(0), last.y, 1e-3)
}

func TestPathCubicTo(t *testing.T) {
	var p Path
	p.MoveTo(0, 0)
	p.CubicTo(33, 50, 66, 50, 100, 0)
	require.Greater(t, len(p.current.points), 2)
	last := p.current.points[len(p.current.points)-1]
	require.InDelta(t, float32(100), last.x, 1e-3)
	require.InDelta(t, float32(0), last.y, 1e-3)
}

func TestAppendVerticesAndIndicesForFilling(t *testing.T) {
	var p Path
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(100, 100)
	p.LineTo(0, 100)
	p.Close()

	var verts []futurerender.Vertex
	var idxs []uint16
	verts, idxs = p.AppendVerticesAndIndicesForFilling(verts, idxs)
	require.Equal(t, 4, len(verts))
	require.Equal(t, 6, len(idxs)) // 2 triangles for a quad
}

func TestAppendVerticesAndIndicesForFillingTooFewPoints(t *testing.T) {
	var p Path
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	// Only 2 points - not enough for a triangle.

	var verts []futurerender.Vertex
	var idxs []uint16
	verts, idxs = p.AppendVerticesAndIndicesForFilling(verts, idxs)
	require.Empty(t, verts)
	require.Empty(t, idxs)
}

func TestAppendVerticesAndIndicesForStroke(t *testing.T) {
	var p Path
	p.MoveTo(0, 0)
	p.LineTo(100, 0)

	var verts []futurerender.Vertex
	var idxs []uint16
	verts, idxs = p.AppendVerticesAndIndicesForStroke(verts, idxs, &StrokeOptions{Width: 2})
	require.Equal(t, 4, len(verts))
	require.Equal(t, 6, len(idxs))
}

func TestAppendVerticesAndIndicesForStrokeClosed(t *testing.T) {
	var p Path
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(100, 100)
	p.Close()

	var verts []futurerender.Vertex
	var idxs []uint16
	verts, idxs = p.AppendVerticesAndIndicesForStroke(verts, idxs, &StrokeOptions{Width: 2})
	// Miter joins share vertices at corners: 3 points × 2 = 6 vertices.
	// 3 edges × 6 indices = 18 indices.
	require.Equal(t, 6, len(verts))
	require.Equal(t, 18, len(idxs))
}

func TestStrokeDefaultWidth(t *testing.T) {
	var p Path
	p.MoveTo(0, 0)
	p.LineTo(100, 0)

	var verts []futurerender.Vertex
	var idxs []uint16
	// nil options should use default width of 1.
	verts, idxs = p.AppendVerticesAndIndicesForStroke(verts, idxs, nil)
	require.Equal(t, 4, len(verts))
	require.Equal(t, 6, len(idxs))
}

func TestDrawFilledRectNoRenderer(t *testing.T) {
	img := futurerender.NewImage(100, 100)
	// Should not panic without a renderer.
	DrawFilledRect(img, 10, 10, 50, 50, color.White, false)
}

func TestStrokeRectNoRenderer(t *testing.T) {
	img := futurerender.NewImage(100, 100)
	StrokeRect(img, 10, 10, 50, 50, 2, color.White, false)
}

func TestDrawFilledCircleNoRenderer(t *testing.T) {
	img := futurerender.NewImage(100, 100)
	DrawFilledCircle(img, 50, 50, 25, color.White, false)
}

func TestStrokeCircleNoRenderer(t *testing.T) {
	img := futurerender.NewImage(100, 100)
	StrokeCircle(img, 50, 50, 25, 2, color.White, false)
}

func TestStrokeLineNoRenderer(t *testing.T) {
	img := futurerender.NewImage(100, 100)
	StrokeLine(img, 0, 0, 100, 100, 2, color.White, false)
}

func TestColorToFloat(t *testing.T) {
	r, g, b, a := colorToFloat(color.NRGBA{R: 255, G: 128, B: 0, A: 255})
	require.InDelta(t, float32(1.0), r, 0.01)
	require.InDelta(t, float32(0.5), g, 0.01)
	require.InDelta(t, float32(0.0), b, 0.01)
	require.InDelta(t, float32(1.0), a, 0.01)
}

func TestColorToFloatTransparent(t *testing.T) {
	r, g, b, a := colorToFloat(color.NRGBA{})
	require.InDelta(t, float32(0), r, 1e-6)
	require.InDelta(t, float32(0), g, 1e-6)
	require.InDelta(t, float32(0), b, 1e-6)
	require.InDelta(t, float32(0), a, 1e-6)
}

func TestCircleSegments(t *testing.T) {
	// Small radius.
	n := circleSegments(5)
	require.GreaterOrEqual(t, n, 16)

	// Large radius.
	n = circleSegments(200)
	require.LessOrEqual(t, n, 256)
}

func TestQuadBezier(t *testing.T) {
	// t=0 should be p0, t=1 should be p2.
	require.InDelta(t, float32(0), quadBezier(0, 50, 100, 0), 1e-6)
	require.InDelta(t, float32(100), quadBezier(0, 50, 100, 1), 1e-6)
}

func TestCubicBezier(t *testing.T) {
	require.InDelta(t, float32(0), cubicBezier(0, 33, 66, 100, 0), 1e-6)
	require.InDelta(t, float32(100), cubicBezier(0, 33, 66, 100, 1), 1e-6)
}

func TestAllSubPathsIncludesOpen(t *testing.T) {
	var p Path
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	// Don't close.
	all := p.allSubPaths()
	require.Len(t, all, 1)
}

func TestAllSubPathsEmpty(t *testing.T) {
	var p Path
	all := p.allSubPaths()
	require.Empty(t, all)
}

func TestStrokeMiterJoinRect(t *testing.T) {
	// A closed rectangle should produce 8 vertices (4 corners × 2) with
	// miter joins, ensuring all corners are covered without gaps.
	var p Path
	p.MoveTo(10, 10)
	p.LineTo(110, 10)
	p.LineTo(110, 110)
	p.LineTo(10, 110)
	p.Close()

	var verts []futurerender.Vertex
	var idxs []uint16
	verts, idxs = p.AppendVerticesAndIndicesForStroke(verts, idxs, &StrokeOptions{Width: 2})

	// 4 points × 2 vertices = 8 vertices, 4 edges × 6 indices = 24 indices.
	require.Equal(t, 8, len(verts))
	require.Equal(t, 24, len(idxs))

	// At the top-left corner (10,10), miter offset should be (1,1) for width=2.
	// Inner vertex: (11, 11), Outer vertex: (9, 9).
	require.InDelta(t, float32(11), verts[0].DstX, 0.01)
	require.InDelta(t, float32(11), verts[0].DstY, 0.01)
	require.InDelta(t, float32(9), verts[1].DstX, 0.01)
	require.InDelta(t, float32(9), verts[1].DstY, 0.01)

	// At the top-right corner (110,10), miter offset should be (-1,1).
	// Inner vertex: (109, 11), Outer vertex: (111, 9).
	require.InDelta(t, float32(109), verts[2].DstX, 0.01)
	require.InDelta(t, float32(11), verts[2].DstY, 0.01)
	require.InDelta(t, float32(111), verts[3].DstX, 0.01)
	require.InDelta(t, float32(9), verts[3].DstY, 0.01)
}

func TestStrokeMiterJoinOpenPolyline(t *testing.T) {
	// Open polyline: endpoints use segment normals, middle uses miter.
	var p Path
	p.MoveTo(0, 0)
	p.LineTo(50, 0)
	p.LineTo(50, 50)

	var verts []futurerender.Vertex
	var idxs []uint16
	verts, idxs = p.AppendVerticesAndIndicesForStroke(verts, idxs, &StrokeOptions{Width: 2})

	// 3 points × 2 vertices = 6, 2 segments × 6 indices = 12.
	require.Equal(t, 6, len(verts))
	require.Equal(t, 12, len(idxs))

	// First point (0,0): butt cap with segment normal (0,1).
	require.InDelta(t, float32(0), verts[0].DstX, 0.01)
	require.InDelta(t, float32(1), verts[0].DstY, 0.01)
	require.InDelta(t, float32(0), verts[1].DstX, 0.01)
	require.InDelta(t, float32(-1), verts[1].DstY, 0.01)

	// Middle point (50,0): miter of horizontal and vertical segments.
	// Should offset by (-1, 1) direction scaled by half*sqrt(2).
	require.InDelta(t, float32(49), verts[2].DstX, 0.01)
	require.InDelta(t, float32(1), verts[2].DstY, 0.01)
	require.InDelta(t, float32(51), verts[3].DstX, 0.01)
	require.InDelta(t, float32(-1), verts[3].DstY, 0.01)
}

func TestSegmentNormal(t *testing.T) {
	// Horizontal segment: normal points up/down.
	nx, ny := segmentNormal(point{0, 0}, point{10, 0}, 1)
	require.InDelta(t, float32(0), nx, 1e-6)
	require.InDelta(t, float32(1), ny, 1e-6)

	// Vertical segment: normal points left/right.
	nx, ny = segmentNormal(point{0, 0}, point{0, 10}, 1)
	require.InDelta(t, float32(-1), nx, 1e-6)
	require.InDelta(t, float32(0), ny, 1e-6)

	// Zero-length segment: zero normal.
	nx, ny = segmentNormal(point{5, 5}, point{5, 5}, 1)
	require.InDelta(t, float32(0), nx, 1e-6)
	require.InDelta(t, float32(0), ny, 1e-6)
}

func TestMiterOffset(t *testing.T) {
	// 90° corner (rectangle corner): miter at 45°.
	half := float32(1)
	nx, ny := miterOffset(point{0, 0}, point{10, 0}, point{10, 10}, half)
	// Miter direction is (-1,1)/√2, length = 1/cos(45°) = √2.
	// Offset = (-1, 1).
	require.InDelta(t, float32(-1), nx, 0.01)
	require.InDelta(t, float32(1), ny, 0.01)

	// Anti-parallel segments (180° turn): falls back to segment normal.
	nx, ny = miterOffset(point{0, 0}, point{10, 0}, point{0, 0}, half)
	// Normal of first segment (0,0)→(10,0) is (0, 1).
	require.InDelta(t, float32(0), nx, 0.01)
	require.InDelta(t, float32(1), ny, 0.01)
}
