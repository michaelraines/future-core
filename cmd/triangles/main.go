//go:build glfw

// Command triangles demonstrates M3 custom geometry: drawing overlapping
// triangles with DrawTriangles using both NonZero and EvenOdd fill rules.
//
// The left half shows NonZero (default) — overlapping regions are drawn.
// The right half shows EvenOdd — overlapping regions are transparent (XOR).
//
// Build: go build -tags glfw ./cmd/triangles
// Run:   ./triangles
package main

import (
	"image"
	"image/color"
	"log"
	"math"

	futurerender "github.com/michaelraines/future-render"
)

const (
	screenW = 640
	screenH = 480
)

type triangleGame struct {
	white *futurerender.Image
	angle float64
}

func (g *triangleGame) Update() error {
	if futurerender.IsKeyPressed(futurerender.KeyEscape) {
		return futurerender.ErrTermination
	}
	g.angle += 0.01
	return nil
}

func (g *triangleGame) Draw(screen *futurerender.Image) {
	screen.Fill(futurerender.ColorFromRGBA(0.1, 0.1, 0.15, 1))

	// Generate two overlapping triangles that rotate slowly.
	cx1, cy1 := float32(160), float32(240)
	cx2, cy2 := float32(480), float32(240)
	r := float32(100)

	// Triangle vertices: two overlapping triangles per side.
	a := float32(g.angle)
	verts1 := makeStarVertices(cx1, cy1, r, a)
	verts2 := makeStarVertices(cx2, cy2, r, a)

	indices := []uint16{
		0, 1, 2, // first triangle
		3, 4, 5, // second (overlapping) triangle
	}

	// Left side: NonZero fill rule (default) — overlapping regions visible.
	screen.DrawTriangles(verts1, indices, g.white, &futurerender.DrawTrianglesOptions{
		FillRule: futurerender.FillRuleNonZero,
	})

	// Right side: EvenOdd fill rule — overlapping regions transparent.
	screen.DrawTriangles(verts2, indices, g.white, &futurerender.DrawTrianglesOptions{
		FillRule: futurerender.FillRuleEvenOdd,
	})
}

func (g *triangleGame) Layout(_, _ int) (int, int) {
	return screenW, screenH
}

// makeStarVertices generates two overlapping triangles (a Star of David pattern).
func makeStarVertices(cx, cy, r, angle float32) []futurerender.Vertex {
	// Triangle 1: pointing up
	v := make([]futurerender.Vertex, 6)
	for i := 0; i < 3; i++ {
		a := angle + float32(i)*2*math.Pi/3 - math.Pi/2
		v[i] = futurerender.Vertex{
			DstX:   cx + r*float32(math.Cos(float64(a))),
			DstY:   cy + r*float32(math.Sin(float64(a))),
			SrcX:   0.5,
			SrcY:   0.5,
			ColorR: 0.2, ColorG: 0.6, ColorB: 1.0, ColorA: 0.8,
		}
	}
	// Triangle 2: pointing down (rotated 60°)
	for i := 0; i < 3; i++ {
		a := angle + float32(i)*2*math.Pi/3 + math.Pi/6
		v[3+i] = futurerender.Vertex{
			DstX:   cx + r*0.8*float32(math.Cos(float64(a))),
			DstY:   cy + r*0.8*float32(math.Sin(float64(a))),
			SrcX:   0.5,
			SrcY:   0.5,
			ColorR: 1.0, ColorG: 0.4, ColorB: 0.2, ColorA: 0.8,
		}
	}
	return v
}

func main() {
	g := &triangleGame{}

	// Create a 1×1 white texture for solid-color geometry.
	whiteImg := image.NewRGBA(image.Rect(0, 0, 1, 1))
	whiteImg.Set(0, 0, color.White)
	g.white = futurerender.NewImageFromImage(whiteImg)

	if err := futurerender.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
