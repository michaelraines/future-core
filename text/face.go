// Package text provides font loading and text rendering for Future Render.
//
// Text is rendered by rasterizing glyph bitmaps into a shared atlas texture,
// then drawing each glyph via Image.DrawImage. The batcher automatically
// merges all glyphs from the same atlas into minimal GPU draw calls.
//
// Basic usage:
//
//	face, err := text.NewFace(ttfData, 24)
//	// in Draw():
//	text.Draw(screen, "Hello!", face, 10, 40, nil)
package text

import (
	"fmt"
	"io"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Face represents a font at a specific size, ready for rendering.
type Face struct {
	otFont *opentype.Font
	face   font.Face
	size   float64

	metrics Metrics
	cache   *glyphCache
}

// Metrics holds line metrics for a Face.
type Metrics struct {
	// Height is the recommended line height (ascent + descent + line gap).
	Height float64
	// Ascent is the distance from the baseline to the top of a line.
	Ascent float64
	// Descent is the distance from the baseline to the bottom of a line
	// (positive value).
	Descent float64
}

// NewFace creates a Face from raw TTF/OTF font data at the given size in pixels.
func NewFace(src []byte, size float64) (*Face, error) {
	otFont, err := opentype.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("text: parse font: %w", err)
	}

	// 72 DPI so that points == pixels.
	face, err := opentype.NewFace(otFont, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("text: create face: %w", err)
	}

	fm := face.Metrics()
	m := Metrics{
		Height:  fixedToFloat(fm.Height),
		Ascent:  fixedToFloat(fm.Ascent),
		Descent: fixedToFloat(fm.Descent),
	}

	f := &Face{
		otFont:  otFont,
		face:    face,
		size:    size,
		metrics: m,
	}
	f.cache = newGlyphCache(f)
	return f, nil
}

// Metrics returns the face's line metrics.
func (f *Face) Metrics() Metrics {
	return f.metrics
}

// Close releases resources associated with this Face, including its glyph
// cache and atlas texture. After calling Close, the Face must not be used.
func (f *Face) Close() {
	// Remove and dispose the atlas for this face.
	globalAtlasesMu.Lock()
	if a, ok := globalAtlases[f]; ok {
		if a.image != nil {
			a.image.Dispose()
		}
		delete(globalAtlases, f)
	}
	globalAtlasesMu.Unlock()
	// Clear the glyph cache.
	clear(f.cache.entries)
	// Close the underlying font face if it supports io.Closer.
	if closer, ok := f.face.(io.Closer); ok {
		_ = closer.Close()
	}
}

// Measure returns the advance width of a string in pixels.
func (f *Face) Measure(text string) float64 {
	var advance fixed.Int26_6
	prev := rune(-1)
	for _, r := range text {
		if prev >= 0 {
			advance += f.face.Kern(prev, r)
		}
		adv, ok := f.face.GlyphAdvance(r)
		if ok {
			advance += adv
		}
		prev = r
	}
	return fixedToFloat(advance)
}

// fixedToFloat converts a fixed.Int26_6 to float64.
func fixedToFloat(v fixed.Int26_6) float64 {
	return float64(v) / 64.0
}
