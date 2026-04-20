// Package vector provides 2D vector path construction and tessellation,
// matching Ebitengine's ebiten/v2/vector sub-package.
//
// Paths are constructed with MoveTo, LineTo, QuadTo, CubicTo, and Close.
// The resulting path can be tessellated into vertices and indices for
// filling or stroking via AppendVerticesAndIndicesForFilling and
// AppendVerticesAndIndicesForStroke.
//
// Standalone helper functions DrawFilledRect, StrokeRect, DrawFilledCircle,
// StrokeCircle, and StrokeLine are provided for common shapes.
package vector

import (
	gomath "math"

	futurerender "github.com/michaelraines/future-core"
)

// Path represents a 2D vector path made of sub-paths.
type Path struct {
	subPaths []subPath
	current  subPath
}

// subPath is a series of points forming a closed or open curve.
type subPath struct {
	points []point
	closed bool
}

type point struct {
	x, y float32
}

// MoveTo starts a new sub-path at the given position.
func (p *Path) MoveTo(x, y float32) {
	if len(p.current.points) > 0 {
		p.subPaths = append(p.subPaths, p.current)
		p.current = subPath{}
	}
	p.current.points = append(p.current.points, point{x, y})
}

// LineTo adds a line segment from the current position to (x, y).
func (p *Path) LineTo(x, y float32) {
	if len(p.current.points) == 0 {
		p.current.points = append(p.current.points, point{0, 0})
	}
	p.current.points = append(p.current.points, point{x, y})
}

// QuadTo adds a quadratic Bezier curve from the current position
// through (cpx, cpy) to (x, y).
func (p *Path) QuadTo(cpx, cpy, x, y float32) {
	if len(p.current.points) == 0 {
		p.current.points = append(p.current.points, point{0, 0})
	}
	last := p.current.points[len(p.current.points)-1]
	steps := quadBezierSteps(last.x, last.y, cpx, cpy, x, y)
	for i := 1; i <= steps; i++ {
		t := float32(i) / float32(steps)
		px := quadBezier(last.x, cpx, x, t)
		py := quadBezier(last.y, cpy, y, t)
		p.current.points = append(p.current.points, point{px, py})
	}
}

// CubicTo adds a cubic Bezier curve from the current position through
// (cp1x, cp1y) and (cp2x, cp2y) to (x, y).
func (p *Path) CubicTo(cp1x, cp1y, cp2x, cp2y, x, y float32) {
	if len(p.current.points) == 0 {
		p.current.points = append(p.current.points, point{0, 0})
	}
	last := p.current.points[len(p.current.points)-1]
	steps := cubicBezierSteps(last.x, last.y, cp1x, cp1y, cp2x, cp2y, x, y)
	for i := 1; i <= steps; i++ {
		t := float32(i) / float32(steps)
		px := cubicBezier(last.x, cp1x, cp2x, x, t)
		py := cubicBezier(last.y, cp1y, cp2y, y, t)
		p.current.points = append(p.current.points, point{px, py})
	}
}

// Close closes the current sub-path.
func (p *Path) Close() {
	p.current.closed = true
	p.subPaths = append(p.subPaths, p.current)
	p.current = subPath{}
}

// allSubPaths returns all sub-paths including the current open one.
func (p *Path) allSubPaths() []subPath {
	if len(p.current.points) > 0 {
		return append(p.subPaths, p.current)
	}
	return p.subPaths
}

// AppendVerticesAndIndicesForFilling tessellates the path for filling,
// appending to the given vertex and index slices. Returns the updated slices.
// This uses a simple fan tessellation from the first vertex of each sub-path.
func (p *Path) AppendVerticesAndIndicesForFilling(
	vertices []futurerender.Vertex,
	indices []uint16,
) (vs []futurerender.Vertex, is []uint16) {
	for _, sp := range p.allSubPaths() {
		pts := sp.points
		if len(pts) < 3 {
			continue
		}
		base := uint16(len(vertices))
		for _, pt := range pts {
			vertices = append(vertices, futurerender.Vertex{
				DstX:   pt.x,
				DstY:   pt.y,
				SrcX:   0,
				SrcY:   0,
				ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1,
			})
		}
		// Fan triangulation from first vertex.
		for i := 1; i < len(pts)-1; i++ {
			indices = append(indices, base, base+uint16(i), base+uint16(i+1))
		}
	}
	return vertices, indices
}

// AppendVerticesAndIndicesForStroke tessellates the path for stroking,
// appending to the given vertex and index slices. Returns the updated slices.
//
// Each segment becomes its own independent quad (4 fresh vertices, 2
// triangles) and joins between segments are emitted as separate fan
// geometry — the same layout Ebitengine's vector package uses. The
// deliberate consequence is that the outer corner of every join is
// covered by BOTH the prior segment's quad and the next segment's quad
// (overlap at the shared point), which under SourceOver blending
// alpha-stacks to a visibly brighter pixel. A shared-vertex strip
// (which an earlier revision implemented) avoided the overlap and
// produced uniformly-opaque strokes, but the side-effect was every
// alpha-stroked path looking dimmer on futurecore than on ebiten —
// the cell-geometric concentric circles in the vector-showcase
// rendered at roughly half ebiten's pixel intensity. Matching
// ebiten's layout here is a parity choice, not a correctness choice.
func (p *Path) AppendVerticesAndIndicesForStroke(
	vertices []futurerender.Vertex,
	indices []uint16,
	op *StrokeOptions,
) (vs []futurerender.Vertex, is []uint16) {
	width := float32(1)
	lineCap := LineCapButt
	lineJoin := LineJoinMiter
	miterLimit := float32(4) // default miter limit matching SVG/CSS
	if op != nil {
		if op.Width > 0 {
			width = op.Width
		}
		lineCap = op.LineCap
		lineJoin = op.LineJoin
		if op.MiterLimit > 0 {
			miterLimit = op.MiterLimit
		}
	}
	half := width / 2

	for _, sp := range p.allSubPaths() {
		pts := sp.points
		if len(pts) < 2 {
			continue
		}
		n := len(pts)
		segCount := n - 1
		if sp.closed {
			segCount = n
		}

		// Build per-segment rects: 4 corner points per segment, ordered
		// [start+ext, end+ext, start-ext, end-ext] so rect[0..3] match
		// ebiten's convention (used by the join + line-cap geometry
		// below). Degenerate (zero-length) segments are skipped.
		type rectQuad [4]point
		rects := make([]rectQuad, 0, segCount)
		for i := range segCount {
			pt := pts[i]
			nextIdx := i + 1
			if sp.closed {
				nextIdx %= n
			}
			nextPt := pts[nextIdx]
			dx := nextPt.x - pt.x
			dy := nextPt.y - pt.y
			dist := float32(gomath.Sqrt(float64(dx*dx + dy*dy)))
			if dist == 0 {
				continue
			}
			extX := dy * half / dist
			extY := -dx * half / dist
			rects = append(rects, rectQuad{
				{pt.x + extX, pt.y + extY},
				{nextPt.x + extX, nextPt.y + extY},
				{pt.x - extX, pt.y - extY},
				{nextPt.x - extX, nextPt.y - extY},
			})
		}
		if len(rects) == 0 {
			continue
		}

		for i, rect := range rects {
			// Per-segment quad: 4 fresh vertices, 2 triangles. Winding
			// (base, base+1, base+2) + (base+1, base+3, base+2) matches
			// ebiten so NonZero fills (when this path is routed via
			// stencil) are compatible.
			base := uint16(len(vertices)) //nolint:gosec // bounded by max vertex count
			for _, p := range rect {
				vertices = append(vertices, futurerender.Vertex{
					DstX: p.x, DstY: p.y,
					ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1,
				})
			}
			indices = append(indices, base, base+1, base+2, base+1, base+3, base+2)

			// Join between this segment and the next.
			var nextRect rectQuad
			switch {
			case i < len(rects)-1:
				nextRect = rects[i+1]
			case sp.closed:
				nextRect = rects[0]
			default:
				continue
			}

			// c is the center of the "end" edge of this rect, which
			// coincides with the center of the "start" edge of nextRect
			// — i.e. the path vertex being joined.
			c := point{
				x: (rect[1].x + rect[3].x) / 2,
				y: (rect[1].y + rect[3].y) / 2,
			}

			a0 := float32(gomath.Atan2(float64(rect[1].y-c.y), float64(rect[1].x-c.x)))
			a1 := float32(gomath.Atan2(float64(nextRect[0].y-c.y), float64(nextRect[0].x-c.x)))
			da := a1 - a0
			for da < 0 {
				da += 2 * gomath.Pi
			}
			if da == 0 {
				continue
			}

			appendStrokeJoin(&vertices, &indices, c, rect[:], nextRect[:], a0, a1, da, half, lineJoin, miterLimit)
		}

		// Line caps for open paths.
		if !sp.closed && lineCap != LineCapButt {
			startR, endR := rects[0], rects[len(rects)-1]
			appendStrokeCapStart(&vertices, &indices, startR[:], half, lineCap)
			appendStrokeCapEnd(&vertices, &indices, endR[:], half, lineCap)
		}
	}
	return vertices, indices
}

// appendStrokeJoin emits join geometry between two adjacent segment
// rects. For miter joins, it emits a triangle fan from the join center
// covering the outer corner (falling back to a bevel when the miter
// limit is exceeded). For bevel joins, a single triangle. For round
// joins, a fan of triangles approximating the outer arc.
//
//nolint:gocritic // rect/nextRect deliberately passed as slices for easier indexing
func appendStrokeJoin(vertices *[]futurerender.Vertex, indices *[]uint16, c point, rect, nextRect []point, a0, a1, da, half float32, join LineJoin, miterLimit float32) {
	if len(rect) < 4 || len(nextRect) < 4 {
		return
	}

	addVert := func(pt point) uint16 {
		idx := uint16(len(*vertices)) //nolint:gosec // bounded by max vertex count
		*vertices = append(*vertices, futurerender.Vertex{
			DstX: pt.x, DstY: pt.y,
			ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1,
		})
		return idx
	}

	// fanFill connects a previously-added center vertex to a run of
	// perimeter vertices [first..last] with triangles (center, i, i+1).
	fanFill := func(center, first, last uint16) {
		for k := first; k+1 <= last; k++ {
			*indices = append(*indices, center, k, k+1)
		}
	}

	switch join {
	case LineJoinMiter:
		delta := float32(gomath.Pi) - da
		exceed := float32(gomath.Abs(1/gomath.Sin(float64(delta/2)))) > miterLimit

		centerIdx := addVert(c)
		first := centerIdx + 1
		if da < gomath.Pi {
			// Outer corner on the +ext side.
			addVert(rect[1])
			if !exceed {
				addVert(crossingPointForTwoLines(rect[0], rect[1], nextRect[0], nextRect[1]))
			}
			addVert(nextRect[0])
		} else {
			// Outer corner on the -ext side.
			addVert(rect[3])
			if !exceed {
				addVert(crossingPointForTwoLines(rect[2], rect[3], nextRect[2], nextRect[3]))
			}
			addVert(nextRect[2])
		}
		last := uint16(len(*vertices) - 1) //nolint:gosec // bounded by max vertex count
		fanFill(centerIdx, first, last)

	case LineJoinBevel:
		centerIdx := addVert(c)
		if da < gomath.Pi {
			addVert(rect[1])
			addVert(nextRect[0])
		} else {
			addVert(rect[3])
			addVert(nextRect[2])
		}
		*indices = append(*indices, centerIdx, centerIdx+1, centerIdx+2)

	case LineJoinRound:
		// Fan approximating the outer arc between the two shoulder
		// vertices. Use a0 and a1 directly for the +ext side; add π
		// for the -ext side.
		startA, endA := a0, a1
		if da >= gomath.Pi {
			startA, endA = a0+float32(gomath.Pi), a1+float32(gomath.Pi)
		}
		sweep := endA - startA
		for sweep < 0 {
			sweep += 2 * float32(gomath.Pi)
		}
		if sweep > float32(gomath.Pi) {
			// Keep the shorter arc.
			sweep -= 2 * float32(gomath.Pi)
		}
		steps := int(gomath.Ceil(gomath.Abs(float64(sweep)) / (gomath.Pi / 8)))
		if steps < 2 {
			steps = 2
		}

		centerIdx := addVert(c)
		first := centerIdx + 1
		for i := 0; i <= steps; i++ {
			t := float32(i) / float32(steps)
			angle := startA + t*sweep
			addVert(point{
				x: c.x + half*float32(gomath.Cos(float64(angle))),
				y: c.y + half*float32(gomath.Sin(float64(angle))),
			})
		}
		last := uint16(len(*vertices) - 1) //nolint:gosec // bounded by max vertex count
		fanFill(centerIdx, first, last)
	}
}

// appendStrokeCapStart emits cap geometry at the first segment's
// starting edge (rect[0]..rect[2]). The cap opens AWAY from the path,
// i.e. on the side opposite rect[1]/rect[3].
//
//nolint:gocritic // rect passed as slice for easier indexing
func appendStrokeCapStart(vertices *[]futurerender.Vertex, indices *[]uint16, rect []point, half float32, lc LineCap) {
	if len(rect) < 4 {
		return
	}
	switch lc {
	case LineCapButt:
		// Nothing — butt cap is the default stroke endpoint.
	case LineCapSquare:
		// Extend rect[0] and rect[2] backward along the segment
		// direction by half-width.
		a := gomath.Atan2(float64(rect[0].y-rect[1].y), float64(rect[0].x-rect[1].x))
		s, cosA := gomath.Sincos(a)
		dx := float32(cosA) * half
		dy := float32(s) * half

		base := uint16(len(*vertices)) //nolint:gosec // bounded by max vertex count
		*vertices = append(*vertices,
			futurerender.Vertex{DstX: rect[0].x, DstY: rect[0].y, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			futurerender.Vertex{DstX: rect[0].x + dx, DstY: rect[0].y + dy, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			futurerender.Vertex{DstX: rect[2].x + dx, DstY: rect[2].y + dy, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			futurerender.Vertex{DstX: rect[2].x, DstY: rect[2].y, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		)
		*indices = append(*indices, base, base+1, base+2, base, base+2, base+3)

	case LineCapRound:
		c := point{
			x: (rect[0].x + rect[2].x) / 2,
			y: (rect[0].y + rect[2].y) / 2,
		}
		a := float32(gomath.Atan2(float64(rect[0].y-rect[2].y), float64(rect[0].x-rect[2].x)))
		appendRoundCap(vertices, indices, c, a, a+float32(gomath.Pi), -1, half)
	}
}

// appendStrokeCapEnd emits cap geometry at the last segment's ending
// edge (rect[1]..rect[3]). The cap opens AWAY from the path.
//
//nolint:gocritic // rect passed as slice for easier indexing
func appendStrokeCapEnd(vertices *[]futurerender.Vertex, indices *[]uint16, rect []point, half float32, lc LineCap) {
	if len(rect) < 4 {
		return
	}
	switch lc {
	case LineCapButt:
		// Nothing — butt cap is the default stroke endpoint.
	case LineCapSquare:
		a := gomath.Atan2(float64(rect[1].y-rect[0].y), float64(rect[1].x-rect[0].x))
		s, cosA := gomath.Sincos(a)
		dx := float32(cosA) * half
		dy := float32(s) * half

		base := uint16(len(*vertices)) //nolint:gosec // bounded by max vertex count
		*vertices = append(*vertices,
			futurerender.Vertex{DstX: rect[1].x, DstY: rect[1].y, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			futurerender.Vertex{DstX: rect[1].x + dx, DstY: rect[1].y + dy, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			futurerender.Vertex{DstX: rect[3].x + dx, DstY: rect[3].y + dy, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			futurerender.Vertex{DstX: rect[3].x, DstY: rect[3].y, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		)
		*indices = append(*indices, base, base+1, base+2, base, base+2, base+3)

	case LineCapRound:
		c := point{
			x: (rect[1].x + rect[3].x) / 2,
			y: (rect[1].y + rect[3].y) / 2,
		}
		a := float32(gomath.Atan2(float64(rect[1].y-rect[3].y), float64(rect[1].x-rect[3].x)))
		appendRoundCap(vertices, indices, c, a, a+float32(gomath.Pi), 1, half)
	}
}

// appendRoundCap emits a semicircle fan between angles a and b around
// center c, radius = half. direction (-1 or +1) picks which sweep keeps
// the cap on the outward side of the stroke.
func appendRoundCap(vertices *[]futurerender.Vertex, indices *[]uint16, c point, a, b float32, direction int, half float32) {
	sweep := b - a
	if direction < 0 {
		for sweep > 0 {
			sweep -= 2 * float32(gomath.Pi)
		}
	} else {
		for sweep < 0 {
			sweep += 2 * float32(gomath.Pi)
		}
	}
	steps := int(gomath.Ceil(gomath.Abs(float64(sweep)) / (gomath.Pi / 8)))
	if steps < 4 {
		steps = 4
	}

	center := uint16(len(*vertices)) //nolint:gosec // bounded by max vertex count
	*vertices = append(*vertices, futurerender.Vertex{
		DstX: c.x, DstY: c.y,
		ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1,
	})
	for i := 0; i <= steps; i++ {
		t := float32(i) / float32(steps)
		angle := a + t*sweep
		*vertices = append(*vertices, futurerender.Vertex{
			DstX:   c.x + half*float32(gomath.Cos(float64(angle))),
			DstY:   c.y + half*float32(gomath.Sin(float64(angle))),
			ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1,
		})
		if i > 0 {
			*indices = append(*indices, center, center+uint16(i), center+uint16(i+1)) //nolint:gosec // bounded by max vertex count
		}
	}
}

// lineForTwoPoints returns line coefficients ax + by + c = 0 through
// two points. Used by crossingPointForTwoLines for miter-corner
// calculation.
func lineForTwoPoints(p0, p1 point) (a, b, c float32) {
	a = p1.y - p0.y
	b = -(p1.x - p0.x)
	c = (p1.x-p0.x)*p0.y - (p1.y-p0.y)*p0.x
	return
}

// crossingPointForTwoLines returns the intersection of the infinite
// lines through (p00, p01) and (p10, p11). Used to compute the outer
// miter corner for LineJoinMiter: the lines of the two adjacent
// segments' outer edges intersect at exactly the miter point.
func crossingPointForTwoLines(p00, p01, p10, p11 point) point {
	a0, b0, c0 := lineForTwoPoints(p00, p01)
	a1, b1, c1 := lineForTwoPoints(p10, p11)
	det := a0*b1 - a1*b0
	if det == 0 {
		return p01 // parallel lines — degenerate; fall back to the edge end
	}
	return point{
		x: (b0*c1 - b1*c0) / det,
		y: (a1*c0 - a0*c1) / det,
	}
}

// segmentNormal returns the unit perpendicular offset vector for a line
// segment. Callers scale by the desired half-width.
func segmentNormal(a, b point, _ float32) (nx, ny float32) {
	dx := b.x - a.x
	dy := b.y - a.y
	l := float32(gomath.Sqrt(float64(dx*dx + dy*dy)))
	if l == 0 {
		return 0, 0
	}
	return -dy / l, dx / l
}

// joinOffset computes the join offset at point curr, where prev→curr and
// curr→next are adjacent segments. For miter and bevel joins, it returns
// the offset vector to add/subtract from curr for the two stroke vertices.
// For round joins, it emits additional fan triangles into vertices/indices.
func joinOffset(prev, curr, next point, half float32, join LineJoin, miterLimit float32, vertices *[]futurerender.Vertex, indices *[]uint16) (nx, ny float32) {
	// Unit normals for each segment.
	n1x, n1y := segmentNormal(prev, curr, 1)
	n2x, n2y := segmentNormal(curr, next, 1)

	// Miter direction: average of the two unit normals, normalized.
	mx := n1x + n2x
	my := n1y + n2y
	ml := float32(gomath.Sqrt(float64(mx*mx + my*my)))
	if ml < 1e-6 {
		// Segments are anti-parallel (180° turn). Use first segment normal.
		return n1x * half, n1y * half
	}
	mx /= ml
	my /= ml

	// Miter length = half / cos(θ/2), where cos(θ/2) = dot(miter, normal).
	dot := mx*n1x + my*n1y
	if dot < 1e-6 {
		dot = 1e-6
	}
	miterLen := half / dot

	// Check if miter exceeds the limit. miterLimit is the ratio of
	// miter length to half-width (matching SVG/CSS definition).
	miterExceeded := miterLen/half > miterLimit

	switch join {
	case LineJoinBevel:
		// Bevel: use the first segment's normal. The bevel triangle is
		// implicitly formed by the adjacent quad vertices (no extra geometry).
		return n1x * half, n1y * half

	case LineJoinRound:
		// Round: emit a fan of triangles between the two segment normals.
		appendRoundJoin(vertices, indices, curr, n1x, n1y, n2x, n2y, half)
		// Use the first segment's normal for the main stroke vertices.
		return n1x * half, n1y * half

	default: // LineJoinMiter
		if miterExceeded {
			// Fall back to bevel when miter limit is exceeded.
			return n1x * half, n1y * half
		}
		return mx * miterLen, my * miterLen
	}
}

// miterOffset is a convenience wrapper for the default miter join.
// Used by tests and any code that needs the simple miter computation.
func miterOffset(prev, curr, next point, half float32) (nx, ny float32) {
	return joinOffset(prev, curr, next, half, LineJoinMiter, 4, nil, nil)
}

// appendRoundJoin emits a triangle fan between two segment normals at a
// join point, producing a smooth arc.
func appendRoundJoin(vertices *[]futurerender.Vertex, indices *[]uint16, curr point, n1x, n1y, n2x, n2y, half float32) {
	if vertices == nil || indices == nil {
		return
	}

	angle1 := float32(gomath.Atan2(float64(n1y), float64(n1x)))
	angle2 := float32(gomath.Atan2(float64(n2y), float64(n2x)))

	// Ensure we sweep the shorter arc.
	diff := angle2 - angle1
	if diff > gomath.Pi {
		diff -= 2 * gomath.Pi
	} else if diff < -gomath.Pi {
		diff += 2 * gomath.Pi
	}

	steps := int(gomath.Abs(float64(diff)) / (gomath.Pi / 8))
	if steps < 2 {
		steps = 2
	}

	center := uint16(len(*vertices))
	*vertices = append(*vertices,
		futurerender.Vertex{DstX: curr.x, DstY: curr.y, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	)

	for i := 0; i <= steps; i++ {
		t := float32(i) / float32(steps)
		angle := angle1 + t*diff
		px := curr.x + half*float32(gomath.Cos(float64(angle)))
		py := curr.y + half*float32(gomath.Sin(float64(angle)))
		*vertices = append(*vertices,
			futurerender.Vertex{DstX: px, DstY: py, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		)
		if i > 0 {
			*indices = append(*indices, center, center+uint16(i), center+uint16(i+1))
		}
	}
}

// LineCap defines how the ends of a stroked path are rendered.
type LineCap int

const (
	// LineCapButt ends the stroke flush at the endpoint (default).
	LineCapButt LineCap = iota
	// LineCapRound adds a semicircle at each endpoint.
	LineCapRound
	// LineCapSquare extends the stroke by half the width beyond each endpoint.
	LineCapSquare
)

// LineJoin defines how two stroke segments are joined at a shared vertex.
type LineJoin int

const (
	// LineJoinMiter extends edges until they meet at a sharp point (default).
	LineJoinMiter LineJoin = iota
	// LineJoinBevel connects the outer corners with a straight line.
	LineJoinBevel
	// LineJoinRound connects the outer corners with an arc.
	LineJoinRound
)

// StrokeOptions specifies options for path stroking.
type StrokeOptions struct {
	// Width is the stroke width in pixels.
	Width float32

	// LineCap controls how the ends of the stroke are rendered.
	// Line caps are not rendered when the subpath is closed.
	// The default (zero) value is LineCapButt.
	LineCap LineCap

	// LineJoin controls how two stroke segments are joined.
	// The default (zero) value is LineJoinMiter.
	LineJoin LineJoin

	// MiterLimit is the miter limit for LineJoinMiter.
	// The default (zero) value is 0.
	MiterLimit float32
}

// --- Bezier helpers ---

func quadBezier(p0, p1, p2, t float32) float32 {
	mt := 1 - t
	return mt*mt*p0 + 2*mt*t*p1 + t*t*p2
}

func cubicBezier(p0, p1, p2, p3, t float32) float32 {
	mt := 1 - t
	return mt*mt*mt*p0 + 3*mt*mt*t*p1 + 3*mt*t*t*p2 + t*t*t*p3
}

func quadBezierSteps(x0, y0, cpx, cpy, x1, y1 float32) int {
	// Use control point deviation for better adaptive subdivision.
	d1x := float64(cpx - (x0+x1)/2)
	d1y := float64(cpy - (y0+y1)/2)
	dx := float64(x1 - x0)
	dy := float64(y1 - y0)
	dist := gomath.Sqrt(dx*dx+dy*dy) + gomath.Sqrt(d1x*d1x+d1y*d1y)
	n := int(dist / 4)
	if n < 4 {
		n = 4
	}
	if n > 256 {
		n = 256
	}
	return n
}

func cubicBezierSteps(x0, y0, cp1x, cp1y, cp2x, cp2y, x1, y1 float32) int {
	// Use control point deviations for better adaptive subdivision.
	d1x := float64(cp1x - x0)
	d1y := float64(cp1y - y0)
	d2x := float64(cp2x - x1)
	d2y := float64(cp2y - y1)
	dx := float64(x1 - x0)
	dy := float64(y1 - y0)
	dist := gomath.Sqrt(dx*dx+dy*dy) + gomath.Sqrt(d1x*d1x+d1y*d1y) + gomath.Sqrt(d2x*d2x+d2y*d2y)
	n := int(dist / 3)
	if n < 4 {
		n = 4
	}
	if n > 256 {
		n = 256
	}
	return n
}
