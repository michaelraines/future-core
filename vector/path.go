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
// Adjacent segments share vertices at corners via miter joins, eliminating
// gaps that would otherwise appear at outer corners of closed paths.
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

		// Track the actual vertex index of each point's first main
		// vertex (the +nx vertex). joinOffset can append fan vertices
		// for round joins BEFORE the main vertices for that point, so
		// the main pair isn't at a fixed `base + i*2` offset — the
		// quad-index loop must look up where each point's vertices
		// actually landed. The previous code assumed 2 vertices per
		// point and broke for any path with LineJoinRound, producing
		// dashed/discontinuous strokes (each quad pointed at fan
		// vertices instead of the main ones).
		pointVertex := make([]uint16, n)

		for i := 0; i < n; i++ {
			var nx, ny float32

			switch {
			case sp.closed:
				prev := (i - 1 + n) % n
				next := (i + 1) % n
				nx, ny = joinOffset(pts[prev], pts[i], pts[next], half, lineJoin, miterLimit, &vertices, &indices)
			case i == 0:
				nx, ny = segmentNormal(pts[0], pts[1], half)
			case i == n-1:
				nx, ny = segmentNormal(pts[n-2], pts[n-1], half)
			default:
				nx, ny = joinOffset(pts[i-1], pts[i], pts[i+1], half, lineJoin, miterLimit, &vertices, &indices)
			}

			pointVertex[i] = uint16(len(vertices)) //nolint:gosec // bounded by max vertex count
			vertices = append(vertices,
				futurerender.Vertex{DstX: pts[i].x + nx, DstY: pts[i].y + ny, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
				futurerender.Vertex{DstX: pts[i].x - nx, DstY: pts[i].y - ny, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			)
		}

		// Generate quad indices for each segment, using the recorded
		// vertex offsets rather than assuming a regular i*2 stride.
		segCount := n - 1
		if sp.closed {
			segCount = n
		}
		for i := 0; i < segCount; i++ {
			j := i + 1
			if sp.closed {
				j %= n
			}
			v0 := pointVertex[i]
			v1 := pointVertex[i] + 1
			v2 := pointVertex[j]
			v3 := pointVertex[j] + 1
			indices = append(indices, v0, v1, v2, v1, v3, v2)
		}

		// Line caps for open paths.
		if !sp.closed && lineCap != LineCapButt {
			appendLineCap(&vertices, &indices, pts[0], pts[1], half, lineCap)
			appendLineCap(&vertices, &indices, pts[n-1], pts[n-2], half, lineCap)
		}
	}
	return vertices, indices
}

// segmentNormal returns the perpendicular offset vector for a line segment,
// scaled by half (the half-width of the stroke).
func segmentNormal(a, b point, half float32) (nx, ny float32) {
	dx := b.x - a.x
	dy := b.y - a.y
	l := float32(gomath.Sqrt(float64(dx*dx + dy*dy)))
	if l == 0 {
		return 0, 0
	}
	return -dy / l * half, dx / l * half
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

// appendLineCap adds cap geometry at the endpoint of an open path.
// endpoint is the path endpoint, interior is the adjacent point.
// isStart is true for the first point, false for the last.
func appendLineCap(vertices *[]futurerender.Vertex, indices *[]uint16, endpoint, interior point, half float32, lineCap LineCap) {
	nx, ny := segmentNormal(interior, endpoint, half)
	// Direction along the segment, pointing outward from the path.
	dx := endpoint.x - interior.x
	dy := endpoint.y - interior.y
	dl := float32(gomath.Sqrt(float64(dx*dx + dy*dy)))
	if dl == 0 {
		return
	}
	dx /= dl
	dy /= dl

	switch lineCap {
	case LineCapButt:
		// Nothing to append — butt cap is the default stroke endpoint.
	case LineCapSquare:
		// Extend the endpoint by half-width away from the path.
		// dx already points from interior toward endpoint (away from path).
		ext := half
		base := uint16(len(*vertices))
		*vertices = append(*vertices,
			futurerender.Vertex{DstX: endpoint.x + nx + dx*ext, DstY: endpoint.y + ny + dy*ext, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			futurerender.Vertex{DstX: endpoint.x - nx + dx*ext, DstY: endpoint.y - ny + dy*ext, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			futurerender.Vertex{DstX: endpoint.x + nx, DstY: endpoint.y + ny, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			futurerender.Vertex{DstX: endpoint.x - nx, DstY: endpoint.y - ny, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		)
		*indices = append(*indices, base, base+1, base+2, base+1, base+3, base+2)

	case LineCapRound:
		// Semicircle fan at the endpoint, opening away from the path.
		// The outward direction is (dx, dy). The semicircle sweeps from
		// outwardAngle - π/2 to outwardAngle + π/2.
		cx, cy := endpoint.x, endpoint.y
		outwardAngle := float32(gomath.Atan2(float64(dy), float64(dx)))
		steps := circleCapSteps(half)

		center := uint16(len(*vertices))
		*vertices = append(*vertices,
			futurerender.Vertex{DstX: cx, DstY: cy, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		)

		for i := 0; i <= steps; i++ {
			t := float32(i) / float32(steps)
			angle := (outwardAngle - gomath.Pi/2) + t*gomath.Pi
			px := cx + half*float32(gomath.Cos(float64(angle)))
			py := cy + half*float32(gomath.Sin(float64(angle)))
			*vertices = append(*vertices,
				futurerender.Vertex{DstX: px, DstY: py, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			)
			if i > 0 {
				*indices = append(*indices, center, center+uint16(i), center+uint16(i+1))
			}
		}
	}
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

// circleCapSteps returns the number of segments for a round cap semicircle.
func circleCapSteps(radius float32) int {
	n := int(gomath.Ceil(float64(radius) * gomath.Pi / 4))
	if n < 4 {
		return 4
	}
	if n > 32 {
		return 32
	}
	return n
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
