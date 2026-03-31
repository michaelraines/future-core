// Command text demonstrates text rendering: large titles, word-wrapped
// paragraphs, and left/center/right alignment modes.
//
// Build: go build -tags glfw ./cmd/text
// Run:   ./text
package main

import (
	"log"

	futurerender "github.com/michaelraines/future-core"
	"github.com/michaelraines/future-core/text"
	"golang.org/x/image/font/gofont/goregular"
)

const (
	screenW = 640
	screenH = 480
)

type textGame struct {
	titleFace text.Face
	bodyFace  text.Face
}

func (g *textGame) Update() error {
	if futurerender.IsKeyPressed(futurerender.KeyEscape) {
		return futurerender.ErrTermination
	}
	return nil
}

func drawText(screen *futurerender.Image, s string, face text.Face, x, y float64, cs futurerender.ColorScale) {
	opts := &text.DrawOptions{}
	opts.ColorScale = cs
	opts.GeoM.Translate(x, y)
	text.Draw(screen, s, face, opts)
}

func drawTextAligned(screen *futurerender.Image, s string, face text.Face, x, y, maxW float64, cs futurerender.ColorScale, align text.Align) {
	opts := &text.DrawOptions{}
	opts.ColorScale = cs
	opts.GeoM.Translate(x, y)
	opts.PrimaryAlign = align
	text.DrawWrapped(screen, s, face, maxW, opts)
}

func colorScale(r, g, b, a float32) futurerender.ColorScale {
	var cs futurerender.ColorScale
	cs.Scale(r, g, b, a)
	return cs
}

func (g *textGame) Draw(screen *futurerender.Image) {
	screen.Fill(futurerender.ColorFromRGBA(0.05, 0.05, 0.1, 1.0))

	if g.titleFace == nil || g.bodyFace == nil {
		return
	}

	// Title — large, centered text.
	titleOpts := &text.DrawOptions{}
	titleOpts.ColorScale = colorScale(0.9, 0.8, 0.3, 1)
	titleOpts.GeoM.Translate(0, 20)
	titleOpts.PrimaryAlign = text.AlignCenter
	text.Draw(screen, "Future Render Text Demo", g.titleFace, titleOpts)

	// Word-wrapped paragraph.
	paragraph := "This example demonstrates the text rendering capabilities of Future Render. " +
		"Text is rasterized into a glyph atlas and drawn using the GPU batcher for " +
		"efficient rendering. Word wrapping automatically breaks long lines at whitespace " +
		"boundaries to fit within the specified maximum width."

	drawTextAligned(screen, paragraph, g.bodyFace, 40, 80, screenW-80, colorScale(0.9, 0.9, 0.9, 1), text.AlignStart)

	// Alignment examples — each line uses a different alignment mode.
	lineH := g.bodyFace.Metrics().Height
	baseY := 240.0

	drawText(screen, "Left-aligned text", g.bodyFace, 40, baseY, colorScale(0.6, 0.9, 0.6, 1))

	drawTextAligned(screen, "Center-aligned text", g.bodyFace, 40, baseY+lineH*2, screenW-80, colorScale(0.6, 0.6, 0.9, 1), text.AlignCenter)

	drawTextAligned(screen, "Right-aligned text", g.bodyFace, 40, baseY+lineH*4, screenW-80, colorScale(0.9, 0.6, 0.6, 1), text.AlignEnd)

	// Footer.
	drawTextAligned(screen, "Press Escape to exit", g.bodyFace, 40, screenH-60, screenW-80, colorScale(0.5, 0.5, 0.5, 1), text.AlignCenter)
}

func (g *textGame) Layout(_, _ int) (int, int) {
	return screenW, screenH
}

func main() {
	futurerender.SetWindowSize(screenW, screenH)
	futurerender.SetWindowTitle("Future Render \u2014 Text Example")

	game := &textGame{}
	wrapper := &lazyInitGame{game: game}

	if err := futurerender.RunGame(wrapper); err != nil {
		log.Fatal(err)
	}
}

// lazyInitGame wraps textGame to initialize font faces on first Update,
// after the GPU device is ready.
type lazyInitGame struct {
	game   *textGame
	inited bool
}

func (g *lazyInitGame) Update() error {
	if !g.inited {
		var err error
		g.game.titleFace, err = text.NewFace(goregular.TTF, 36)
		if err != nil {
			return err
		}
		g.game.bodyFace, err = text.NewFace(goregular.TTF, 18)
		if err != nil {
			return err
		}
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
