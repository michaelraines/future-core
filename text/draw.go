package text

import (
	futurerender "github.com/michaelraines/future-render"
	fmath "github.com/michaelraines/future-render/math"
)

// DrawOptions controls how text is drawn.
type DrawOptions struct {
	// GeoM applies a 2D transformation to the text.
	GeoM futurerender.GeoM

	// ColorScale tints the text. Zero value draws white text.
	ColorScale fmath.Color
}

// globalAtlases maps Face pointers to their atlas. Each Face gets its own
// atlas so glyph sizes don't conflict.
var globalAtlases = map[*Face]*fontAtlas{}

// atlasFor returns (or creates) the font atlas for the given face.
func atlasFor(f *Face) *fontAtlas {
	a, ok := globalAtlases[f]
	if !ok {
		a = newFontAtlas()
		globalAtlases[f] = a
	}
	return a
}

// Draw renders text at position (x, y) on the target image.
// The position specifies the top-left corner of the text (y is adjusted
// by the face's ascent so glyphs sit on the baseline at y + ascent).
func Draw(target *futurerender.Image, s string, face *Face, x, y float64, opts *DrawOptions) {
	if target == nil || face == nil || s == "" {
		return
	}

	atlas := atlasFor(face)

	var geoM futurerender.GeoM
	var colorScale fmath.Color
	if opts != nil {
		geoM = opts.GeoM
		colorScale = opts.ColorScale
	}

	// Default to white if zero.
	if colorScale == (fmath.Color{}) {
		colorScale = fmath.Color{R: 1, G: 1, B: 1, A: 1}
	}

	curX := x
	prev := rune(-1)
	for _, r := range s {
		// Apply kerning.
		if prev >= 0 {
			kern := face.face.Kern(prev, r)
			curX += fixedToFloat(kern)
		}

		g := face.cache.get(r, atlas)
		if g == nil || g.empty {
			curX += g.advance
			prev = r
			continue
		}

		glyphImg := atlas.subImage(g.atlasX, g.atlasY, g.width, g.height)
		if glyphImg == nil {
			curX += g.advance
			prev = r
			continue
		}

		drawOpts := &futurerender.DrawImageOptions{
			ColorScale: colorScale,
		}
		drawOpts.GeoM.Translate(curX+g.bearingX, y+g.bearingY+face.metrics.Ascent)
		drawOpts.GeoM.Concat(geoM)
		target.DrawImage(glyphImg, drawOpts)

		curX += g.advance
		prev = r
	}
}
