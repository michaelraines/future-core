// Command sprite demonstrates M2 image rendering: loading a PNG,
// drawing it with transforms (scale, rotation), and alpha blending.
//
// Build: go build -tags glfw ./cmd/sprite
// Run:   ./sprite
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

type spriteGame struct {
	sprite *futurerender.Image
	angle  float64
}

func (g *spriteGame) Update() error {
	if futurerender.IsKeyPressed(futurerender.KeyEscape) {
		return futurerender.ErrTermination
	}
	g.angle += 0.02
	return nil
}

func (g *spriteGame) Draw(screen *futurerender.Image) {
	// Fill the screen with a dark blue background.
	screen.Fill(futurerender.ColorFromRGBA(0.1, 0.1, 0.3, 1.0))

	if g.sprite == nil {
		return
	}

	// Draw the sprite at the center of the screen with rotation.
	w, h := g.sprite.Size()
	opts := &futurerender.DrawImageOptions{}

	// Center the sprite on its own origin before rotating.
	opts.GeoM.Translate(-float64(w)/2, -float64(h)/2)
	opts.GeoM.Rotate(g.angle)
	opts.GeoM.Scale(2, 2)
	opts.GeoM.Translate(screenW/2, screenH/2)

	screen.DrawImage(g.sprite, opts)

	// Draw a second copy with half alpha, offset to the side.
	opts2 := &futurerender.DrawImageOptions{}
	opts2.GeoM.Translate(-float64(w)/2, -float64(h)/2)
	opts2.GeoM.Rotate(-g.angle)
	opts2.GeoM.Translate(screenW/4, screenH/2)
	opts2.ColorScale = futurerender.ColorFromRGBA(1, 1, 1, 0.5)

	screen.DrawImage(g.sprite, opts2)
}

func (g *spriteGame) Layout(_, _ int) (int, int) {
	return screenW, screenH
}

// generateCheckerboard creates a programmatic test sprite since we may
// not have a PNG file on disk. This is a 64x64 red/white checkerboard.
func generateCheckerboard() *futurerender.Image {
	const size = 64
	const tileSize = 8
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			tx, ty := x/tileSize, y/tileSize
			if (tx+ty)%2 == 0 {
				img.Set(x, y, color.RGBA{R: 220, G: 50, B: 50, A: 255})
			} else {
				img.Set(x, y, color.White)
			}
		}
	}
	return futurerender.NewImageFromImage(img)
}

func main() {
	_ = math.Pi // ensure math is used for angle computation above

	futurerender.SetWindowSize(800, 600)
	futurerender.SetWindowTitle("Future Render — Sprite Example")

	game := &spriteGame{}

	// We create the sprite inside the game loop since NewImageFromImage
	// requires the GPU device to be initialized. Use a lazy-init pattern.
	wrapper := &lazyInitGame{game: game}

	if err := futurerender.RunGame(wrapper); err != nil {
		log.Fatal(err)
	}
}

// lazyInitGame wraps spriteGame to initialize the sprite on first Update,
// after the GPU device is ready.
type lazyInitGame struct {
	game   *spriteGame
	inited bool
}

func (g *lazyInitGame) Update() error {
	if !g.inited {
		g.game.sprite = generateCheckerboard()
		g.inited = true
	}
	return g.game.Update()
}

func (g *lazyInitGame) Draw(screen *futurerender.Image) {
	g.game.Draw(screen)
}

func (g *lazyInitGame) Layout(outsideWidth, outsideHeight int) (int, int) {
	return g.game.Layout(outsideWidth, outsideHeight)
}
