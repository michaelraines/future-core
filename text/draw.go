package text

import (
	"strings"
	"sync"
	"unicode"

	futurerender "github.com/michaelraines/future-core"
)

// Align specifies text alignment for primary (horizontal) or secondary
// (vertical) axes.
type Align int

// Align constants matching Ebitengine's text/v2 alignment.
const (
	AlignStart  Align = iota // Default start alignment (left for LTR).
	AlignCenter              // Center alignment.
	AlignEnd                 // End alignment (right for LTR).
)

// LayoutOptions controls text layout independent of rendering.
type LayoutOptions struct {
	// LineSpacing is the distance in pixels between baselines of consecutive
	// lines. If zero, the face's natural line height is used.
	LineSpacing float64

	// PrimaryAlign specifies horizontal text alignment.
	PrimaryAlign Align

	// SecondaryAlign specifies vertical text alignment.
	SecondaryAlign Align
}

// DrawOptions controls how text is drawn. It embeds DrawImageOptions for
// transform and color control, plus LayoutOptions for text-specific layout.
type DrawOptions struct {
	// DrawImageOptions provides GeoM, ColorScale, Blend, and Filter.
	futurerender.DrawImageOptions

	// LayoutOptions provides LineSpacing, PrimaryAlign, SecondaryAlign.
	LayoutOptions
}

// globalAtlases maps Face values to their atlas. Each Face gets its own
// atlas so glyph sizes don't conflict.
var globalAtlases = map[Face]*fontAtlas{}

// globalAtlasesMu protects concurrent access to globalAtlases.
var globalAtlasesMu sync.Mutex

// atlasFor returns (or creates) the font atlas for the given face.
func atlasFor(f Face) *fontAtlas {
	globalAtlasesMu.Lock()
	defer globalAtlasesMu.Unlock()
	a, ok := globalAtlases[f]
	if !ok {
		a = newFontAtlas()
		globalAtlases[f] = a
	}
	return a
}

// Draw renders text on the target image. Position the text using
// opts.GeoM.Translate(). Newline characters produce multiple lines.
//
// This matches Ebitengine's text/v2.Draw signature.
func Draw(target *futurerender.Image, s string, face Face, opts *DrawOptions) {
	if target == nil || face == nil || s == "" {
		return
	}

	var lineSpacing float64
	primaryAlign := AlignStart
	secondaryAlign := AlignStart
	var cs futurerender.ColorScale
	var geoM futurerender.GeoM
	if opts != nil {
		lineSpacing = opts.LineSpacing
		primaryAlign = opts.PrimaryAlign
		secondaryAlign = opts.SecondaryAlign
		cs = opts.ColorScale
		geoM = opts.GeoM
	}

	metrics := face.Metrics()

	// Calculate advances for each line (matching Ebitengine's forEachLine).
	var advances []float64
	var lineCount int
	for t := s; ; {
		lineCount++
		line, rest, found := cutLine(t)
		a := face.advance(line)
		advances = append(advances, a)
		if !found {
			break
		}
		t = rest
	}

	// Boundary height for secondary alignment.
	// Matches Ebitengine: (lineCount-1)*lineSpacing + HAscent + HDescent
	boundaryHeight := float64(lineCount-1)*lineSpacing + metrics.Ascent + metrics.Descent

	// Secondary (vertical) offset — start with ascent so origin is at baseline.
	offsetY := metrics.Ascent
	switch secondaryAlign {
	case AlignStart:
		// No additional offset.
	case AlignCenter:
		offsetY -= boundaryHeight / 2
	case AlignEnd:
		offsetY -= boundaryHeight
	}

	var originY float64
	var indexOffset int
	for i, t := 0, s; ; i++ {
		line, rest, found := cutLine(t)

		// Primary (horizontal) alignment.
		var originX float64
		switch primaryAlign {
		case AlignStart:
			originX = 0
		case AlignCenter:
			originX = -advances[i] / 2
		case AlignEnd:
			originX = -advances[i]
		}

		if line != "" {
			face.drawGlyphs(target, line, originX+0, originY+offsetY, cs, geoM)
		}

		if !found {
			break
		}
		t = rest
		indexOffset += len(line) + 1
		originY += lineSpacing
	}
}

// cutLine splits at the first newline, returning (line, rest, found).
func cutLine(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

// DrawWrapped renders text with word wrapping at the given maximum width.
// Words that exceed maxWidth are placed on their own line. Lines are broken
// at whitespace boundaries. Explicit newlines in the input are preserved.
func DrawWrapped(target *futurerender.Image, s string, face Face, maxWidth float64, opts *DrawOptions) {
	if target == nil || face == nil || s == "" || maxWidth <= 0 {
		return
	}

	lines := WrapLines(s, face, maxWidth)

	primaryAlign := AlignStart
	if opts != nil {
		primaryAlign = opts.PrimaryAlign
	}

	refWidth := maxWidth
	if primaryAlign == AlignStart {
		refWidth = 0
	}

	metrics := face.Metrics()
	lineH := metrics.Height
	if opts != nil && opts.LineSpacing > 0 {
		lineH = opts.LineSpacing
	}

	var cs futurerender.ColorScale
	var geoM futurerender.GeoM
	if opts != nil {
		cs = opts.ColorScale
		geoM = opts.GeoM
	}

	for i, line := range lines {
		if line == "" {
			continue
		}
		ox := 0.0
		if primaryAlign != AlignStart {
			lineWidth := face.advance(line)
			switch primaryAlign {
			case AlignStart:
				// No offset needed.
			case AlignCenter:
				ox = -refWidth/2 + (refWidth-lineWidth)/2
			case AlignEnd:
				ox = -lineWidth
			}
		}
		oy := float64(i) * lineH
		face.drawGlyphs(target, line, ox, oy, cs, geoM)
	}
}

// WrapLines splits text into lines that fit within maxWidth pixels.
// Explicit newlines are preserved. Words are split at whitespace boundaries.
func WrapLines(s string, face Face, maxWidth float64) []string {
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
func wrapParagraph(s string, face Face, maxWidth float64) []string {
	words := splitWords(s)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	var line string
	lineWidth := 0.0
	spaceWidth := face.advance(" ")

	for _, word := range words {
		wordWidth := face.advance(word)

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

// splitLines splits text on newline characters.
func splitLines(s string) []string {
	return strings.Split(s, "\n")
}
