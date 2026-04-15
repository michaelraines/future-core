// Command texttest reproduces the missing text bug.
//
// Two modes:
//   - Default: draws directly to the engine screen (works)
//   - OFFSCREEN=1: draws to an offscreen image, then composites to screen
//     (mimics the component system's Frame/OffscreenBuffer pattern)
package main

import (
	"bytes"
	_ "embed"
	"image/color"
	"log"
	"os"

	fc "github.com/michaelraines/future-core"
	"github.com/michaelraines/future-core/text"
	"github.com/michaelraines/future-core/vector"
)

//go:embed BitstreamVeraSansMono-xj20.ttf
var fontData []byte

var useOffscreen = os.Getenv("OFFSCREEN") == "1"

type game struct {
	source    *text.GoTextFaceSource
	offscreen *fc.Image
	frame     int
}

func (g *game) Update() error { return nil }

// warmupDraw does a full draw cycle to populate the AA buffer,
// simulating what happens on frame 1 before the screenshot on frame 3.
func (g *game) warmupDraw(target *fc.Image) {
	vector.DrawFilledRect(target, 20, 70, 200, 60,
		color.RGBA{R: 0, G: 255, B: 0, A: 255}, true)
	vector.DrawFilledCircle(target, 300, 100, 30,
		color.RGBA{R: 255, G: 255, B: 255, A: 255}, true)
}

func (g *game) Draw(screen *fc.Image) {
	g.frame++

	// Target is either the screen or an offscreen image.
	target := screen
	if useOffscreen {
		if g.offscreen == nil {
			w, h := screen.Size()
			g.offscreen = fc.NewImage(w, h)
		}
		// Mimic the component system: Clear before draw, then composite.
		g.offscreen.Clear()
		target = g.offscreen
	}

	// Mimic Frame's StyleConfig.Draw: fill background before content.
	if useOffscreen {
		target.Fill(color.RGBA{R: 20, G: 20, B: 30, A: 255})
	}

	// On frame 1, do a warmup draw (AA draws without text) to populate
	// the persistent AA buffer. On frame 2+, do the full draw.
	if g.frame == 1 && useOffscreen {
		g.warmupDraw(target)
	} else {
		drawContent(target, g.source)
	}

	// If offscreen, composite onto the actual screen (like Frame does).
	if useOffscreen && g.offscreen != nil {
		screen.DrawImage(g.offscreen, nil)
	}
}

func drawContent(screen *fc.Image, source *text.GoTextFaceSource) {
	face12 := &text.GoTextFace{Source: source, Size: 12}
	face14 := &text.GoTextFace{Source: source, Size: 14}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	green := color.RGBA{R: 0, G: 255, B: 0, A: 255}

	// --- Text BEFORE any AA draw ---
	opts := &text.DrawOptions{}
	opts.GeoM.Translate(20, 20)
	opts.ColorScale.Scale(1, 1, 1, 1)
	text.Draw(screen, "BEFORE AA (size 14)", face14, opts)

	opts2 := &text.DrawOptions{}
	opts2.GeoM.Translate(20, 40)
	opts2.ColorScale.Scale(1, 1, 1, 1)
	text.Draw(screen, "BEFORE AA (size 12)", face12, opts2)

	// --- AA vector draw ---
	vector.DrawFilledRect(screen, 20, 70, 200, 60, green, true)

	// --- Text AFTER AA draw ---
	opts3 := &text.DrawOptions{}
	opts3.GeoM.Translate(20, 150)
	opts3.ColorScale.Scale(1, 1, 1, 1)
	text.Draw(screen, "AFTER AA (size 14)", face14, opts3)

	opts4 := &text.DrawOptions{}
	opts4.GeoM.Translate(20, 170)
	opts4.ColorScale.Scale(1, 1, 1, 1)
	text.Draw(screen, "AFTER AA (size 12)", face12, opts4)

	// --- Another AA draw + text ---
	vector.DrawFilledCircle(screen, 300, 100, 30, white, true)

	opts5 := &text.DrawOptions{}
	opts5.GeoM.Translate(20, 200)
	opts5.ColorScale.Scale(1, 1, 1, 1)
	text.Draw(screen, "AFTER 2nd AA (size 12)", face12, opts5)
}

func (g *game) Layout(_, _ int) (int, int) { return 512, 256 }

func main() {
	source, err := text.NewGoTextFaceSource(bytes.NewReader(fontData))
	if err != nil {
		log.Fatal(err)
	}
	fc.SetWindowSize(512, 256)
	fc.SetWindowTitle("Text + AA Interaction Test")
	if err := fc.RunGame(&game{source: source}); err != nil {
		log.Fatal(err)
	}
}
