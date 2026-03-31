//go:build js

// Command wasm_demo runs the full Future Render engine in a browser
// via WebGPU + WASM. It draws animated colored rectangles.
package main

import (
	"log"
	"math"
	"os"

	"image"

	futurerender "github.com/michaelraines/future-core"
)

func init() {
	// Force soft backend in headless browser (SwiftShader WebGPU canvas
	// compositing doesn't work in headless Chromium). The soft rasterizer's
	// ReadScreen is a simple memcpy that works everywhere.
	if err := os.Setenv("FUTURE_CORE_BACKEND", "soft"); err != nil {
		// Fallback: os.Setenv may not work in all WASM runtimes.
		_ = err
	}
}

const (
	screenW = 400
	screenH = 400
)

type demoGame struct {
	frame int
}

func (g *demoGame) Update() error {
	g.frame++
	return nil
}

func (g *demoGame) Draw(screen *futurerender.Image) {
	// Dark blue-gray background.
	screen.Fill(futurerender.ColorFromRGBA(0.12, 0.12, 0.2, 1.0))

	t := float64(g.frame) * 0.03

	// Red bar — oscillates horizontally.
	rx := int(screenW * (0.3 + 0.15*math.Sin(t)))
	fillRect(screen, rx, 40, 160, 60, 0.9, 0.2, 0.2, 1.0)

	// Green bar — oscillates vertically.
	gy := int(screenH * (0.3 + 0.15*math.Cos(t*1.3)))
	fillRect(screen, 40, gy, 60, 160, 0.2, 0.9, 0.2, 1.0)

	// Blue square — oscillates diagonally.
	d := int(40 * math.Sin(t*0.7))
	fillRect(screen, 200+d, 200+d, 140, 140, 0.2, 0.2, 0.9, 1.0)

	// Yellow center square — pulses size.
	s := int(40 + 20*math.Sin(t*2))
	fillRect(screen, 200-s/2, 200-s/2, s, s, 0.95, 0.9, 0.2, 1.0)

	// White border rectangle to prove rendering works.
	fillRect(screen, 5, 5, screenW-10, 3, 1, 1, 1, 1)         // top
	fillRect(screen, 5, screenH-8, screenW-10, 3, 1, 1, 1, 1) // bottom
	fillRect(screen, 5, 5, 3, screenH-10, 1, 1, 1, 1)         // left
	fillRect(screen, screenW-8, 5, 3, screenH-10, 1, 1, 1, 1) // right
}

func (g *demoGame) Layout(_, _ int) (int, int) {
	return screenW, screenH
}

// fillRect draws a filled rectangle using SubImage + Fill.
func fillRect(screen *futurerender.Image, x, y, w, h int, r, g, b, a float64) {
	sub := screen.SubImage(image.Rect(x, y, x+w, y+h))
	if sub != nil {
		sub.Fill(futurerender.ColorFromRGBA(r, g, b, a))
	}
}

func main() {
	futurerender.SetWindowSize(screenW, screenH)
	futurerender.SetWindowTitle("Future Render — WebGPU WASM Demo")
	if err := futurerender.RunGame(&demoGame{}); err != nil {
		log.Fatal(err)
	}
}
