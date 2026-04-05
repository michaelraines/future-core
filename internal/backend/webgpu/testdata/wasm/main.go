//go:build js

package main

import (
	"image"
	"image/color"
	"log"
	"os"

	futurerender "github.com/michaelraines/future-core"
)

func init() {
	if err := os.Setenv("FUTURE_CORE_BACKEND", "webgpu"); err != nil {
		_ = err
	}
}

type demoGame struct{}

func (g *demoGame) Update() error { return nil }

func (g *demoGame) Draw(screen *futurerender.Image) {
	// Yellow fill + blue DrawImage.
	screen.Fill(color.NRGBA{R: 255, G: 200, A: 255})

	if blueImg == nil {
		blueImg = futurerender.NewImageFromImage(makeBlue())
	}
	opts := &futurerender.DrawImageOptions{}
	opts.GeoM.Scale(50, 50)
	opts.GeoM.Translate(175, 175)
	screen.DrawImage(blueImg, opts)
}

func (g *demoGame) Layout(_, _ int) (int, int) { return 400, 400 }

var blueImg *futurerender.Image

func makeBlue() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{B: 200, A: 255})
	return img
}

func main() {
	futurerender.SetWindowSize(400, 400)
	if err := futurerender.RunGame(&demoGame{}); err != nil {
		log.Fatal(err)
	}
}
