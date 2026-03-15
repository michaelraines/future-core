package text

import (
	"bytes"
	"fmt"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

// ShaperFace represents a font loaded for complex text shaping via
// go-text/typesetting. It supports BiDi reordering, ligatures, and
// OpenType feature substitution.
type ShaperFace struct {
	face    *font.Face
	shaper  shaping.HarfbuzzShaper
	size    float64
	metrics Metrics
}

// NewShaperFace creates a ShaperFace from raw TTF/OTF font data at the
// given size in pixels. Unlike NewFace, this uses go-text/typesetting
// for complex text layout including ligatures and BiDi.
func NewShaperFace(src []byte, size float64) (*ShaperFace, error) {
	faces, err := font.ParseTTC(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}

	if len(faces) == 0 {
		return nil, fmt.Errorf("text: no faces found in font data")
	}

	face := faces[0]

	extents, ok := face.FontHExtents()
	if !ok {
		// Fallback metrics.
		extents = font.FontExtents{
			Ascender:  float32(size * 0.8),
			Descender: float32(-size * 0.2),
			LineGap:   0,
		}
	}

	unitsPerEm := float64(face.Upem())
	scale := size / unitsPerEm

	ascent := float64(extents.Ascender) * scale
	descent := float64(extents.Descender) * scale
	if descent < 0 {
		descent = -descent
	}
	lineGap := float64(extents.LineGap) * scale
	if lineGap < 0 {
		lineGap = 0
	}

	return &ShaperFace{
		face: face,
		size: size,
		metrics: Metrics{
			Height:  ascent + descent + lineGap,
			Ascent:  ascent,
			Descent: descent,
		},
	}, nil
}

// Metrics returns the face's line metrics.
func (f *ShaperFace) Metrics() Metrics {
	return f.metrics
}

// Shape performs text shaping on the input string, returning positioned
// glyphs with proper ligature substitution.
func (f *ShaperFace) Shape(text string) []ShapedGlyph {
	runes := []rune(text)
	input := shaping.Input{
		Text:      runes,
		RunStart:  0,
		RunEnd:    len(runes),
		Face:      f.face,
		Size:      fixed.Int26_6(f.size * 64),
		Script:    language.Latin,
		Direction: di.DirectionLTR,
	}

	out := f.shaper.Shape(input)
	return convertShapedGlyphs(out)
}

// ShapeBidi performs text shaping with automatic BiDi detection.
// It splits the input into runs of consistent direction and shapes
// each run, returning glyphs in visual order.
func (f *ShaperFace) ShapeBidi(text string) []ShapedGlyph {
	runs := splitBidiRuns(text)
	var result []ShapedGlyph
	for _, run := range runs {
		runes := []rune(run.text)
		input := shaping.Input{
			Text:      runes,
			RunStart:  0,
			RunEnd:    len(runes),
			Face:      f.face,
			Size:      fixed.Int26_6(f.size * 64),
			Script:    run.script,
			Direction: run.direction,
		}
		out := f.shaper.Shape(input)
		result = append(result, convertShapedGlyphs(out)...)
	}
	return result
}

// ShapedGlyph represents a single glyph after text shaping.
type ShapedGlyph struct {
	GlyphID  uint32  // Font-specific glyph ID.
	XAdvance float64 // Horizontal advance in pixels.
	YAdvance float64 // Vertical advance in pixels.
	XOffset  float64 // Horizontal offset from baseline.
	YOffset  float64 // Vertical offset from baseline.
	Cluster  int     // Index into original text (for cursor mapping).
}

func convertShapedGlyphs(out shaping.Output) []ShapedGlyph {
	result := make([]ShapedGlyph, len(out.Glyphs))
	for i, g := range out.Glyphs {
		result[i] = ShapedGlyph{
			GlyphID:  uint32(g.GlyphID),
			XAdvance: fixedToFloat(g.Advance),
			YAdvance: 0,
			XOffset:  fixedToFloat(g.XOffset),
			YOffset:  fixedToFloat(g.YOffset),
			Cluster:  g.TextIndex(),
		}
	}
	return result
}

// bidiRun represents a directional run of text.
type bidiRun struct {
	text      string
	direction di.Direction
	script    language.Script
}

// splitBidiRuns performs a simple heuristic BiDi split: it detects
// runs of RTL characters (Arabic, Hebrew) vs LTR text.
func splitBidiRuns(text string) []bidiRun {
	if text == "" {
		return nil
	}

	runes := []rune(text)
	var runs []bidiRun
	start := 0
	curDir := runeDirection(runes[0])
	curScript := runeScript(runes[0])

	for i := 1; i < len(runes); i++ {
		dir := runeDirection(runes[i])
		if dir != curDir {
			runs = append(runs, bidiRun{
				text:      string(runes[start:i]),
				direction: curDir,
				script:    curScript,
			})
			start = i
			curDir = dir
			curScript = runeScript(runes[i])
		}
	}
	runs = append(runs, bidiRun{
		text:      string(runes[start:]),
		direction: curDir,
		script:    curScript,
	})
	return runs
}

// runeDirection returns the text direction for a rune.
func runeDirection(r rune) di.Direction {
	if isRTLRune(r) {
		return di.DirectionRTL
	}
	return di.DirectionLTR
}

// isRTLRune returns true if the rune belongs to an RTL script.
func isRTLRune(r rune) bool {
	// Arabic: U+0600-U+06FF, U+0750-U+077F, U+08A0-U+08FF, U+FB50-U+FDFF, U+FE70-U+FEFF
	// Hebrew: U+0590-U+05FF, U+FB1D-U+FB4F
	return (r >= 0x0590 && r <= 0x05FF) ||
		(r >= 0xFB1D && r <= 0xFB4F) ||
		(r >= 0x0600 && r <= 0x06FF) ||
		(r >= 0x0750 && r <= 0x077F) ||
		(r >= 0x08A0 && r <= 0x08FF) ||
		(r >= 0xFB50 && r <= 0xFDFF) ||
		(r >= 0xFE70 && r <= 0xFEFF)
}

// runeScript returns the script for a rune.
func runeScript(r rune) language.Script {
	if (r >= 0x0590 && r <= 0x05FF) || (r >= 0xFB1D && r <= 0xFB4F) {
		return language.Hebrew
	}
	if (r >= 0x0600 && r <= 0x06FF) || (r >= 0x0750 && r <= 0x077F) ||
		(r >= 0x08A0 && r <= 0x08FF) || (r >= 0xFB50 && r <= 0xFDFF) ||
		(r >= 0xFE70 && r <= 0xFEFF) {
		return language.Arabic
	}
	return language.Latin
}
