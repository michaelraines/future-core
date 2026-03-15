package text

import (
	"strings"
	"sync"
	"unicode"

	futurerender "github.com/michaelraines/future-render"
	fmath "github.com/michaelraines/future-render/math"
)

// Align specifies horizontal text alignment.
type Align int

// Align constants.
const (
	AlignLeft   Align = iota // Default left alignment.
	AlignCenter              // Center alignment.
	AlignRight               // Right alignment.
)

// DrawOptions controls how text is drawn.
type DrawOptions struct {
	// GeoM applies a 2D transformation to the text.
	GeoM futurerender.GeoM

	// ColorScale tints the text. A zero-valued ColorScale is treated as
	// opaque white (1,1,1,1), matching Ebitengine's convention so that
	// default DrawOptions{} draws white text.
	ColorScale fmath.Color

	// Align specifies horizontal text alignment. Default is AlignLeft.
	// For multi-line text, each line is aligned relative to the widest line
	// or the MaxWidth if set.
	Align Align
}

// globalAtlases maps Face pointers to their atlas. Each Face gets its own
// atlas so glyph sizes don't conflict.
var globalAtlases = map[*Face]*fontAtlas{}

// globalAtlasesMu protects concurrent access to globalAtlases.
var globalAtlasesMu sync.Mutex

// atlasFor returns (or creates) the font atlas for the given face.
func atlasFor(f *Face) *fontAtlas {
	globalAtlasesMu.Lock()
	defer globalAtlasesMu.Unlock()
	a, ok := globalAtlases[f]
	if !ok {
		a = newFontAtlas()
		globalAtlases[f] = a
	}
	return a
}

// Draw renders a single line of text at position (x, y) on the target image.
// The position specifies the top-left corner of the text (y is adjusted
// by the face's ascent so glyphs sit on the baseline at y + ascent).
// Newline characters in the string cause subsequent text to wrap to the
// next line automatically.
func Draw(target *futurerender.Image, s string, face *Face, x, y float64, opts *DrawOptions) {
	if target == nil || face == nil || s == "" {
		return
	}

	lines := strings.Split(s, "\n")
	if len(lines) == 1 {
		drawLine(target, s, face, x, y, 0, opts)
		return
	}

	// Multi-line: compute alignment offset relative to widest line.
	align := AlignLeft
	if opts != nil {
		align = opts.Align
	}
	refWidth := 0.0
	if align != AlignLeft {
		for _, line := range lines {
			w := face.Measure(line)
			if w > refWidth {
				refWidth = w
			}
		}
	}

	lineH := face.metrics.Height
	for i, line := range lines {
		drawLine(target, line, face, x, y+float64(i)*lineH, refWidth, opts)
	}
}

// DrawWrapped renders text with word wrapping at the given maximum width.
// Words that exceed maxWidth are placed on their own line. Lines are broken
// at whitespace boundaries. Explicit newlines in the input are preserved.
func DrawWrapped(target *futurerender.Image, s string, face *Face, x, y, maxWidth float64, opts *DrawOptions) {
	if target == nil || face == nil || s == "" || maxWidth <= 0 {
		return
	}

	lines := WrapLines(s, face, maxWidth)

	align := AlignLeft
	if opts != nil {
		align = opts.Align
	}

	refWidth := maxWidth
	if align == AlignLeft {
		refWidth = 0
	}

	lineH := face.metrics.Height
	for i, line := range lines {
		drawLine(target, line, face, x, y+float64(i)*lineH, refWidth, opts)
	}
}

// WrapLines splits text into lines that fit within maxWidth pixels.
// Explicit newlines are preserved. Words are split at whitespace boundaries.
func WrapLines(s string, face *Face, maxWidth float64) []string {
	if face == nil || maxWidth <= 0 {
		return []string{s}
	}

	var result []string
	for _, paragraph := range strings.Split(s, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		wrapped := wrapParagraph(paragraph, face, maxWidth)
		result = append(result, wrapped...)
	}
	return result
}

// wrapParagraph word-wraps a single paragraph (no newlines) to maxWidth.
func wrapParagraph(s string, face *Face, maxWidth float64) []string {
	words := splitWords(s)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	var line string
	lineWidth := 0.0
	spaceWidth := face.Measure(" ")

	for _, word := range words {
		wordWidth := face.Measure(word)

		if line == "" {
			// First word on line always goes in, even if it exceeds maxWidth.
			line = word
			lineWidth = wordWidth
			continue
		}

		// Check if adding this word (with space) exceeds the max width.
		if lineWidth+spaceWidth+wordWidth > maxWidth {
			lines = append(lines, line)
			line = word
			lineWidth = wordWidth
		} else {
			line += " " + word
			lineWidth += spaceWidth + wordWidth
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// splitWords splits text into words at whitespace boundaries, discarding
// extra whitespace.
func splitWords(s string) []string {
	var words []string
	start := -1
	for i, r := range s {
		if unicode.IsSpace(r) {
			if start >= 0 {
				words = append(words, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		words = append(words, s[start:])
	}
	return words
}

// drawLine renders a single line of text with optional alignment offset.
// refWidth is the reference width for alignment (widest line or maxWidth).
func drawLine(target *futurerender.Image, s string, face *Face, x, y, refWidth float64, opts *DrawOptions) {
	if s == "" {
		return
	}

	atlas := atlasFor(face)

	var geoM futurerender.GeoM
	var colorScale fmath.Color
	align := AlignLeft
	if opts != nil {
		geoM = opts.GeoM
		colorScale = opts.ColorScale
		align = opts.Align
	}

	// Default to white if zero.
	if colorScale == (fmath.Color{}) {
		colorScale = fmath.Color{R: 1, G: 1, B: 1, A: 1}
	}

	// Apply alignment offset.
	offsetX := 0.0
	if align != AlignLeft && refWidth > 0 {
		lineWidth := face.Measure(s)
		switch align {
		case AlignLeft:
			// No offset needed.
		case AlignCenter:
			offsetX = (refWidth - lineWidth) / 2
		case AlignRight:
			offsetX = refWidth - lineWidth
		}
	}

	curX := x + offsetX
	prev := rune(-1)
	for _, r := range s {
		// Apply kerning.
		if prev >= 0 {
			kern := face.face.Kern(prev, r)
			curX += fixedToFloat(kern)
		}

		g := face.cache.get(r, atlas)
		if g == nil {
			prev = r
			continue
		}
		if g.empty {
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
