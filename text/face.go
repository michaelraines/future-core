// Package text provides font loading and text rendering for Future Render.
//
// Text is rendered by rasterizing glyph bitmaps into a shared atlas texture,
// then drawing each glyph via Image.DrawImage. The batcher automatically
// merges all glyphs from the same atlas into minimal GPU draw calls.
//
// Basic usage:
//
//	source, err := text.NewGoTextFaceSource(bytes.NewReader(ttfData))
//	face := &text.GoTextFace{Source: source, Size: 24}
//	// in Draw():
//	text.Draw(screen, "Hello!", face, nil)
package text

import (
	"fmt"
	"io"
	"sync"

	futurerender "github.com/michaelraines/future-core"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Face is the interface for font faces used with Draw and Measure.
// Face is a sealed interface — it cannot be implemented outside this package.
type Face interface {
	// Metrics returns the face's line metrics.
	Metrics() Metrics

	// unexported methods seal the interface and provide internal functionality.
	advance(text string) float64
	drawGlyphs(target *futurerender.Image, s string, ox, oy float64, cs futurerender.ColorScale, geoM futurerender.GeoM)
	close()
	private()
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

// GoTextFaceSource holds parsed font data that can create GoTextFace instances.
type GoTextFaceSource struct {
	otFont *opentype.Font
}

// NewGoTextFaceSource creates a GoTextFaceSource from font data read from
// an io.Reader.
func NewGoTextFaceSource(r io.Reader) (*GoTextFaceSource, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("text: read font data: %w", err)
	}
	otFont, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("text: parse font: %w", err)
	}
	return &GoTextFaceSource{otFont: otFont}, nil
}

// GoTextFace is a Face implementation backed by a GoTextFaceSource at a
// specific size. Create one by setting the exported fields:
//
//	face := &text.GoTextFace{Source: source, Size: 24}
type GoTextFace struct {
	// Source is the parsed font data.
	Source *GoTextFaceSource
	// Size is the font size in pixels.
	Size float64

	// internal fields initialized lazily on first use.
	initOnce sync.Once
	face     font.Face
	met      Metrics
	cache    *glyphCache
}

// ensureInit lazily initializes the underlying font face, metrics, and cache.
func (f *GoTextFace) ensureInit() {
	f.initOnce.Do(func() {
		if f.Source == nil || f.Source.otFont == nil {
			return
		}
		face, err := opentype.NewFace(f.Source.otFont, &opentype.FaceOptions{
			Size:    f.Size,
			DPI:     72, // 72 DPI so that points == pixels.
			Hinting: font.HintingFull,
		})
		if err != nil {
			return
		}
		f.face = face

		fm := face.Metrics()
		f.met = Metrics{
			Height:  fixedToFloat(fm.Height),
			Ascent:  fixedToFloat(fm.Ascent),
			Descent: fixedToFloat(fm.Descent),
		}

		f.cache = newGlyphCache(face)
	})
}

// Metrics returns the face's line metrics.
func (f *GoTextFace) Metrics() Metrics {
	f.ensureInit()
	return f.met
}

// advance returns the advance width of a string in pixels.
func (f *GoTextFace) advance(text string) float64 {
	f.ensureInit()
	if f.face == nil {
		return 0
	}
	return measureAdvance(f.face, text)
}

// drawGlyphs renders a single line of text at the given offset, applying
// the color scale and geometry transform.
func (f *GoTextFace) drawGlyphs(target *futurerender.Image, s string, ox, oy float64, cs futurerender.ColorScale, geoM futurerender.GeoM) {
	f.ensureInit()
	if f.face == nil || s == "" {
		return
	}

	atlas := atlasFor(f)

	curX := ox
	prev := rune(-1)
	for _, r := range s {
		// Apply kerning.
		if prev >= 0 {
			kern := f.face.Kern(prev, r)
			curX += fixedToFloat(kern)
		}

		g := f.cache.get(r, atlas)
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

		drawOpts := &futurerender.DrawImageOptions{}
		drawOpts.ColorScale = cs
		drawOpts.GeoM.Translate(curX+g.bearingX, oy+g.bearingY+f.met.Ascent)
		drawOpts.GeoM.Concat(geoM)
		target.DrawImage(glyphImg, drawOpts)

		curX += g.advance
		prev = r
	}
}

// close releases resources associated with this GoTextFace.
func (f *GoTextFace) close() {
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
	if f.cache != nil {
		clear(f.cache.entries)
	}
	// Close the underlying font face if it supports io.Closer.
	if f.face != nil {
		if closer, ok := f.face.(io.Closer); ok {
			_ = closer.Close()
		}
	}
}

// Close releases resources associated with this GoTextFace, including its
// glyph cache and atlas texture. After calling Close, the Face must not be used.
func (f *GoTextFace) Close() {
	f.close()
}

// private seals the Face interface.
func (f *GoTextFace) private() {}

// NewFace creates a Face from raw TTF/OTF font data at the given size in
// pixels. This is a convenience function equivalent to creating a
// GoTextFaceSource and GoTextFace manually.
func NewFace(src []byte, size float64) (Face, error) {
	otFont, err := opentype.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("text: parse font: %w", err)
	}

	source := &GoTextFaceSource{otFont: otFont}
	face := &GoTextFace{Source: source, Size: size}
	face.ensureInit()
	if face.face == nil {
		return nil, fmt.Errorf("text: create face: failed to initialize font")
	}
	return face, nil
}

// Measure returns the width and height of the text when rendered with the
// given face and line spacing. For single-line text, height is the face's
// line height. For multi-line text, lineSpacing controls the distance between
// lines (0 uses the face's default line height).
func Measure(text string, face Face, lineSpacing float64) (width, height float64) {
	if face == nil || text == "" {
		return 0, 0
	}

	lines := splitLines(text)
	metrics := face.Metrics()
	lineH := metrics.Height
	if lineSpacing > 0 {
		lineH = lineSpacing
	}

	maxW := 0.0
	for _, line := range lines {
		w := face.advance(line)
		if w > maxW {
			maxW = w
		}
	}

	n := len(lines)
	h := metrics.Height // first line uses natural height
	if n > 1 {
		h += float64(n-1) * lineH
	}

	return maxW, h
}

// Advance returns the advance width of the text in pixels.
func Advance(text string, face Face) float64 {
	if face == nil {
		return 0
	}
	return face.advance(text)
}

// measureAdvance computes the advance width of a string using a font.Face.
func measureAdvance(f font.Face, text string) float64 {
	var adv fixed.Int26_6
	prev := rune(-1)
	for _, r := range text {
		if prev >= 0 {
			adv += f.Kern(prev, r)
		}
		a, ok := f.GlyphAdvance(r)
		if ok {
			adv += a
		}
		prev = r
	}
	return fixedToFloat(adv)
}

// fixedToFloat converts a fixed.Int26_6 to float64.
func fixedToFloat(v fixed.Int26_6) float64 {
	return float64(v) / 64.0
}
