package vector

import (
	"image/color"
	gomath "math"

	futurerender "github.com/michaelraines/future-core"
)

// DrawFilledRect draws a filled rectangle on dst.
func DrawFilledRect(dst *futurerender.Image, x, y, width, height float32, clr color.Color, antialias bool) {
	r, g, b, a := colorToFloat(clr)
	vertices := []futurerender.Vertex{
		{DstX: x, DstY: y, ColorR: r, ColorG: g, ColorB: b, ColorA: a},
		{DstX: x + width, DstY: y, ColorR: r, ColorG: g, ColorB: b, ColorA: a},
		{DstX: x + width, DstY: y + height, ColorR: r, ColorG: g, ColorB: b, ColorA: a},
		{DstX: x, DstY: y + height, ColorR: r, ColorG: g, ColorB: b, ColorA: a},
	}
	indices := []uint16{0, 1, 2, 0, 2, 3}
	dst.DrawTriangles(vertices, indices, nil, &futurerender.DrawTrianglesOptions{
		AntiAlias: antialias,
	})
}

// StrokeRect draws a stroked rectangle on dst.
func StrokeRect(dst *futurerender.Image, x, y, width, height, strokeWidth float32, clr color.Color, antialias bool) {
	var p Path
	p.MoveTo(x, y)
	p.LineTo(x+width, y)
	p.LineTo(x+width, y+height)
	p.LineTo(x, y+height)
	p.Close()

	vertices, indices := p.AppendVerticesAndIndicesForStroke(nil, nil, &StrokeOptions{Width: strokeWidth})
	r, g, b, a := colorToFloat(clr)
	for i := range vertices {
		vertices[i].ColorR = r
		vertices[i].ColorG = g
		vertices[i].ColorB = b
		vertices[i].ColorA = a
	}
	dst.DrawTriangles(vertices, indices, nil, &futurerender.DrawTrianglesOptions{
		AntiAlias: antialias,
	})
}

// DrawFilledCircle draws a filled circle on dst.
func DrawFilledCircle(dst *futurerender.Image, cx, cy, radius float32, clr color.Color, antialias bool) {
	segments := circleSegments(radius)
	r, g, b, a := colorToFloat(clr)

	vertices := make([]futurerender.Vertex, 0, segments+1)
	indices := make([]uint16, 0, segments*3)

	// Center vertex.
	vertices = append(vertices, futurerender.Vertex{
		DstX: cx, DstY: cy, ColorR: r, ColorG: g, ColorB: b, ColorA: a,
	})

	for i := 0; i <= segments; i++ {
		angle := float64(i) / float64(segments) * 2 * gomath.Pi
		px := cx + radius*float32(gomath.Cos(angle))
		py := cy + radius*float32(gomath.Sin(angle))
		vertices = append(vertices, futurerender.Vertex{
			DstX: px, DstY: py, ColorR: r, ColorG: g, ColorB: b, ColorA: a,
		})
		if i > 0 {
			indices = append(indices, 0, uint16(i), uint16(i+1))
		}
	}

	dst.DrawTriangles(vertices, indices, nil, &futurerender.DrawTrianglesOptions{
		AntiAlias: antialias,
	})
}

// StrokeCircle draws a stroked circle on dst.
func StrokeCircle(dst *futurerender.Image, cx, cy, radius, strokeWidth float32, clr color.Color, antialias bool) {
	segments := circleSegments(radius)
	var p Path
	for i := 0; i <= segments; i++ {
		angle := float64(i) / float64(segments) * 2 * gomath.Pi
		px := cx + radius*float32(gomath.Cos(angle))
		py := cy + radius*float32(gomath.Sin(angle))
		if i == 0 {
			p.MoveTo(px, py)
		} else {
			p.LineTo(px, py)
		}
	}
	p.Close()

	vertices, indices := p.AppendVerticesAndIndicesForStroke(nil, nil, &StrokeOptions{Width: strokeWidth})
	r, g, b, a := colorToFloat(clr)
	for i := range vertices {
		vertices[i].ColorR = r
		vertices[i].ColorG = g
		vertices[i].ColorB = b
		vertices[i].ColorA = a
	}
	dst.DrawTriangles(vertices, indices, nil, &futurerender.DrawTrianglesOptions{
		AntiAlias: antialias,
	})
}

// StrokeLine draws a line from (x0,y0) to (x1,y1) on dst.
func StrokeLine(dst *futurerender.Image, x0, y0, x1, y1, strokeWidth float32, clr color.Color, antialias bool) {
	var p Path
	p.MoveTo(x0, y0)
	p.LineTo(x1, y1)

	vertices, indices := p.AppendVerticesAndIndicesForStroke(nil, nil, &StrokeOptions{Width: strokeWidth})
	r, g, b, a := colorToFloat(clr)
	for i := range vertices {
		vertices[i].ColorR = r
		vertices[i].ColorG = g
		vertices[i].ColorB = b
		vertices[i].ColorA = a
	}
	dst.DrawTriangles(vertices, indices, nil, &futurerender.DrawTrianglesOptions{
		AntiAlias: antialias,
	})
}

// colorToFloat converts a color.Color to premultiplied float32 RGBA
// components in [0, 1]. color.Color.RGBA() is defined to return
// premultiplied values in [0, 0xFFFF]; we just normalize by 0xFFFF.
//
// Vertex colors must stay premultiplied because the default sprite
// shader does `texture(uTex, uv) * vColor` and the BlendSourceOver
// preset uses (One, OneMinusSrcAlpha) — the premultiplied-alpha blend
// equation. Dividing RGB by alpha here (to "straight" form) would make
// a half-opaque white appear as fully-opaque white: SourceOver would
// compute `1 + dst * 0.5` instead of the correct `0.5 + dst * 0.5`,
// producing blown-out glows and ghosted halos. Matches what
// libs/rendering/ebiten/vector.go's applyVertexColor does.
func colorToFloat(clr color.Color) (r, g, b, a float32) {
	cr, cg, cb, ca := clr.RGBA()
	return float32(cr) / 0xffff, float32(cg) / 0xffff,
		float32(cb) / 0xffff, float32(ca) / 0xffff
}

// circleSegments returns the number of segments for a circle approximation.
func circleSegments(radius float32) int {
	n := int(gomath.Ceil(gomath.Pi * float64(radius) / 2))
	if n < 16 {
		n = 16
	}
	if n > 256 {
		n = 256
	}
	return n
}
