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
	if op != nil && op.Width > 0 {
		width = op.Width
	}
	half := width / 2

	for _, sp := range p.allSubPaths() {
		pts := sp.points
		if len(pts) < 2 {
			continue
		}

		n := len(pts)
		base := uint16(len(vertices))

		// Generate 2 vertices per point with miter-adjusted offsets at corners.
		for i := 0; i < n; i++ {
			var nx, ny float32

			switch {
			case sp.closed:
				prev := (i - 1 + n) % n
				next := (i + 1) % n
				nx, ny = miterOffset(pts[prev], pts[i], pts[next], half)
			case i == 0:
				nx, ny = segmentNormal(pts[0], pts[1], half)
			case i == n-1:
				nx, ny = segmentNormal(pts[n-2], pts[n-1], half)
			default:
				nx, ny = miterOffset(pts[i-1], pts[i], pts[i+1], half)
			}

			vertices = append(vertices,
				futurerender.Vertex{DstX: pts[i].x + nx, DstY: pts[i].y + ny, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
				futurerender.Vertex{DstX: pts[i].x - nx, DstY: pts[i].y - ny, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			)
		}

		// Generate quad indices for each segment.
		segCount := n - 1
		if sp.closed {
			segCount = n
		}
		for i := 0; i < segCount; i++ {
			j := i + 1
			if sp.closed {
				j %= n
			}
			v0 := base + uint16(i*2)
			v1 := base + uint16(i*2+1)
			v2 := base + uint16(j*2)
			v3 := base + uint16(j*2+1)
			indices = append(indices, v0, v1, v2, v1, v3, v2)
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

// miterOffset computes the miter join offset at point curr, where prev→curr
// and curr→next are adjacent segments. Returns the offset vector to
// add/subtract from curr to get the two stroke vertices.
func miterOffset(prev, curr, next point, half float32) (nx, ny float32) {
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
	if dot < 0.25 {
		// Very acute angle: limit miter to prevent long spikes.
		dot = 0.25
	}
	miterLen := half / dot

	return mx * miterLen, my * miterLen
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
