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

const (
	screenW = 400
	screenH = 400
)

type demoGame struct {
	offscreen  *futurerender.Image
	greenPixel *futurerender.Image
	inited     bool
}

func greenImg() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{G: 200, A: 255})
	return img
}

func (g *demoGame) Update() error {
	if !g.inited {
		g.inited = true
		// Skip filling the offscreen - just create it empty.
		g.offscreen = futurerender.NewImage(100, 100)
	}
	return nil
}

func (g *demoGame) Draw(screen *futurerender.Image) {
	// Use DrawImage with a green 1x1 pixel instead of Fill
	if g.greenPixel == nil {
		g.greenPixel = futurerender.NewImageFromImage(greenImg())
	}
	opts := &futurerender.DrawImageOptions{}
	opts.GeoM.Scale(400, 400)
	screen.DrawImage(g.greenPixel, opts)

	if !g.inited || g.offscreen == nil {
		return
	}

	opts2 := &futurerender.DrawImageOptions{}
	opts2.GeoM.Translate(50, 50)
	screen.DrawImage(g.offscreen, opts2)
}

func (g *demoGame) Layout(_, _ int) (int, int) {
	return screenW, screenH
}

func main() {
	futurerender.SetWindowSize(screenW, screenH)
	if err := futurerender.RunGame(&demoGame{}); err != nil {
		log.Fatal(err)
	}
}
