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

		// Generate strip of quads along the polyline.
		for i := 0; i < len(pts)-1; i++ {
			a := pts[i]
			b := pts[i+1]
			dx := b.x - a.x
			dy := b.y - a.y
			l := float32(gomath.Sqrt(float64(dx*dx + dy*dy)))
			if l == 0 {
				continue
			}
			// Normal perpendicular to segment.
			nx := -dy / l * half
			ny := dx / l * half

			base := uint16(len(vertices))
			vertices = append(vertices,
				futurerender.Vertex{DstX: a.x + nx, DstY: a.y + ny, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
				futurerender.Vertex{DstX: a.x - nx, DstY: a.y - ny, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
				futurerender.Vertex{DstX: b.x + nx, DstY: b.y + ny, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
				futurerender.Vertex{DstX: b.x - nx, DstY: b.y - ny, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			)
			indices = append(indices, base, base+1, base+2, base+1, base+3, base+2)
		}

		// Close the stroke if the sub-path is closed.
		if sp.closed && len(pts) >= 3 {
			a := pts[len(pts)-1]
			b := pts[0]
			dx := b.x - a.x
			dy := b.y - a.y
			l := float32(gomath.Sqrt(float64(dx*dx + dy*dy)))
			if l > 0 {
				nx := -dy / l * half
				ny := dx / l * half
				base := uint16(len(vertices))
				vertices = append(vertices,
					futurerender.Vertex{DstX: a.x + nx, DstY: a.y + ny, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
					futurerender.Vertex{DstX: a.x - nx, DstY: a.y - ny, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
					futurerender.Vertex{DstX: b.x + nx, DstY: b.y + ny, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
					futurerender.Vertex{DstX: b.x - nx, DstY: b.y - ny, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
				)
				indices = append(indices, base, base+1, base+2, base+1, base+3, base+2)
			}
		}
	}
	return vertices, indices
}

// StrokeOptions specifies options for path stroking.
type StrokeOptions struct {
	// Width is the stroke width in pixels.
	Width float32
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
