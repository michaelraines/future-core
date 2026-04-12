// Package text provides font loading and text rendering for Future Render.
//
// Text is rendered by shaping glyphs with go-text/typesetting (HarfBuzz),
// rasterizing outlines via golang.org/x/image/vector.Rasterizer, and drawing
// each glyph via Image.DrawImage. This matches Ebitengine's text/v2 pipeline
// for pixel-identical output.
//
// Basic usage:
//
//	source, err := text.NewGoTextFaceSource(bytes.NewReader(ttfData))
//	face := &text.GoTextFace{Source: source, Size: 24}
//	// in Draw():
//	text.Draw(screen, "Hello!", face, nil)
package text

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"io"
	"math"
	"sync"

	futurerender "github.com/michaelraines/future-core"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	glanguage "github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
	gvector "golang.org/x/image/vector"
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
// This matches Ebitengine's GoTextFaceSource.
type GoTextFaceSource struct {
	f      *font.Face
	shaper shaping.HarfbuzzShaper

	glyphImageCache map[glyphImageKey]*futurerender.Image
	mu              sync.Mutex
}

type glyphImageKey struct {
	gid     opentype.GID
	xoffset fixed.Int26_6
	yoffset fixed.Int26_6
	size    float64
}

// NewGoTextFaceSource creates a GoTextFaceSource from font data read from
// an io.Reader.
func NewGoTextFaceSource(r io.Reader) (*GoTextFaceSource, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("text: read font data: %w", err)
	}

	faces, err := font.ParseTTC(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("text: parse font: %w", err)
	}
	if len(faces) == 0 {
		return nil, fmt.Errorf("text: no faces found in font data")
	}

	return &GoTextFaceSource{
		f:               faces[0],
		glyphImageCache: make(map[glyphImageKey]*futurerender.Image),
	}, nil
}

func (g *GoTextFaceSource) scale(size float64) float64 {
	return size / float64(g.f.Upem())
}

// shapedGlyph holds a shaped glyph plus its scaled outline segments and bounds.
type shapedGlyph struct {
	sg             shaping.Glyph
	scaledSegments []opentype.Segment
	bounds         fixed.Rectangle26_6
}

// singleFontmap implements shaping.Fontmap for a single face.
type singleFontmap struct {
	face *font.Face
}

func (s *singleFontmap) ResolveFace(_ rune) *font.Face {
	return s.face
}

// shape runs HarfBuzz shaping on the text for the given face.
// Matches Ebitengine's shape() method in gotextfacesource.go.
func (g *GoTextFaceSource) shape(text string, size float64) ([]shaping.Output, []shapedGlyph) {
	g.mu.Lock()
	defer g.mu.Unlock()

	runes := []rune(text)
	input := shaping.Input{
		Text:      runes,
		RunStart:  0,
		RunEnd:    len(runes),
		Direction: di.DirectionLTR,
		Face:      g.f,
		Size:      float64ToFixed26_6(size),
		Script:    glanguage.Latin,
	}

	// Split and shape exactly like Ebitengine.
	var seg shaping.Segmenter
	inputs := seg.Split(input, &singleFontmap{face: g.f})

	outputs := make([]shaping.Output, len(inputs))
	var gs []shapedGlyph
	scale := float32(g.scale(size))

	for i, inp := range inputs {
		out := g.shaper.Shape(inp)
		outputs[i] = out
		(shaping.Line{out}).AdjustBaselines()

		for _, gl := range out.Glyphs {
			var segs []opentype.Segment
			switch data := g.f.GlyphData(gl.GlyphID).(type) {
			case font.GlyphOutline:
				segs = data.Segments
			case font.GlyphSVG:
				segs = data.Outline.Segments
			case font.GlyphBitmap:
				if data.Outline != nil {
					segs = data.Outline.Segments
				}
			}

			scaledSegs := make([]opentype.Segment, len(segs))
			for idx, s := range segs {
				scaledSegs[idx] = s
				for j := range s.Args {
					scaledSegs[idx].Args[j].X *= scale
					scaledSegs[idx].Args[j].Y *= -scale // Y-flip: font coords → screen coords
				}
			}

			gs = append(gs, shapedGlyph{
				sg:             gl,
				scaledSegments: scaledSegs,
				bounds:         segmentsToBounds(scaledSegs),
			})
		}
	}

	return outputs, gs
}

// getOrCreateGlyphImage returns a cached glyph image or creates one.
func (g *GoTextFaceSource) getOrCreateGlyphImage(key glyphImageKey, segs []opentype.Segment, subpixelOffset fixed.Point26_6, bounds fixed.Rectangle26_6) *futurerender.Image {
	g.mu.Lock()
	defer g.mu.Unlock()

	if img, ok := g.glyphImageCache[key]; ok {
		return img
	}

	img := segmentsToImage(segs, subpixelOffset, bounds)
	if img != nil {
		g.glyphImageCache[key] = img
	}
	return img
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
}

// Metrics returns the face's line metrics matching Ebitengine's calculation.
func (f *GoTextFace) Metrics() Metrics {
	if f.Source == nil {
		return Metrics{}
	}
	scale := f.Source.scale(f.Size)
	var m Metrics
	if h, ok := f.Source.f.FontHExtents(); ok {
		m.Ascent = float64(h.Ascender) * scale
		m.Descent = float64(-h.Descender) * scale
		lineGap := float64(h.LineGap) * scale
		m.Height = m.Ascent + m.Descent + lineGap
	}
	return m
}

// advance returns the advance width of a string in pixels.
func (f *GoTextFace) advance(text string) float64 {
	if f.Source == nil {
		return 0
	}
	outputs, _ := f.Source.shape(text, f.Size)
	var a fixed.Int26_6
	for _, output := range outputs {
		a += output.Advance
	}
	return fixed26_6ToFloat64(a)
}

// drawGlyphs renders a single line of text at the given offset, applying
// the color scale and geometry transform. Matches Ebitengine's glyph
// positioning exactly.
func (f *GoTextFace) drawGlyphs(target *futurerender.Image, s string, ox, oy float64, cs futurerender.ColorScale, geoM futurerender.GeoM) {
	if s == "" || f.Source == nil {
		return
	}

	_, gs := f.Source.shape(s, f.Size)

	origin := fixed.Point26_6{
		X: float64ToFixed26_6(ox),
		Y: float64ToFixed26_6(oy),
	}

	for _, g := range gs {
		o := origin.Add(fixed.Point26_6{
			X: g.sg.XOffset,
			Y: -g.sg.YOffset,
		})

		img, imgX, imgY := f.glyphImage(g, o)
		if img != nil {
			drawOpts := &futurerender.DrawImageOptions{}
			drawOpts.ColorScale = cs
			drawOpts.GeoM.Translate(float64(imgX), float64(imgY))
			drawOpts.GeoM.Concat(geoM)
			target.DrawImage(img, drawOpts)
		}

		// Advance the pen by the glyph's advance. The text package only
		// supports horizontal layout (Y advance is always zero); see
		// `Metrics.Height` and `face.advance` which both assume LTR
		// horizontal text. go-text/typesetting@v0.3+ unified XAdvance
		// and YAdvance into a single Advance field per glyph.
		origin = origin.Add(fixed.Point26_6{
			X: g.sg.Advance,
			Y: 0,
		})
	}
}

// glyphImage returns the rasterized glyph image and the integer pixel
// position at which it should be drawn on the target. img is the cached
// glyph bitmap; imgX and imgY are the top-left screen-space coordinates
// after subpixel adjustment. Matches Ebitengine's subpixel positioning.
func (f *GoTextFace) glyphImage(g shapedGlyph, origin fixed.Point26_6) (img *futurerender.Image, imgX, imgY int) {
	// For horizontal text: vary X subpixel, floor Y.
	origin.X = adjustGranularity(origin.X, f.Metrics())
	origin.Y &^= ((1 << 6) - 1)

	b := g.bounds

	subpixelOffset := fixed.Point26_6{
		X: (origin.X + b.Min.X) & ((1 << 6) - 1),
		Y: (origin.Y + b.Min.Y) & ((1 << 6) - 1),
	}

	key := glyphImageKey{
		gid:     g.sg.GlyphID,
		xoffset: subpixelOffset.X,
		yoffset: subpixelOffset.Y,
		size:    f.Size,
	}

	img = f.Source.getOrCreateGlyphImage(key, g.scaledSegments, subpixelOffset, b)

	imgX = (origin.X + b.Min.X).Floor()
	imgY = (origin.Y + b.Min.Y).Floor()
	return img, imgX, imgY
}

// Close releases resources associated with this GoTextFace.
func (f *GoTextFace) Close() {
	f.close()
}

// close releases resources.
func (f *GoTextFace) close() {}

// private seals the Face interface.
func (f *GoTextFace) private() {}

// NewFace creates a Face from raw TTF/OTF font data at the given size in
// pixels. This is a convenience function.
func NewFace(src []byte, size float64) (Face, error) {
	source, err := NewGoTextFaceSource(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	return &GoTextFace{Source: source, Size: size}, nil
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
	// Match Ebitengine: height = (n-1)*lineSpacing + HAscent + HDescent
	h := metrics.Ascent + metrics.Descent
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

// --- Rasterization helpers (matching Ebitengine's gotextseg.go) ---

// segmentsToBounds computes the bounding box of scaled outline segments.
func segmentsToBounds(segs []opentype.Segment) fixed.Rectangle26_6 {
	if len(segs) == 0 {
		return fixed.Rectangle26_6{}
	}

	minX := float32(math.Inf(1))
	minY := float32(math.Inf(1))
	maxX := float32(math.Inf(-1))
	maxY := float32(math.Inf(-1))

	for _, seg := range segs {
		n := 1
		switch seg.Op {
		case opentype.SegmentOpMoveTo, opentype.SegmentOpLineTo:
			n = 1
		case opentype.SegmentOpQuadTo:
			n = 2
		case opentype.SegmentOpCubeTo:
			n = 3
		}
		for i := 0; i < n; i++ {
			x := seg.Args[i].X
			y := seg.Args[i].Y
			if minX > x {
				minX = x
			}
			if minY > y {
				minY = y
			}
			if maxX < x {
				maxX = x
			}
			if maxY < y {
				maxY = y
			}
		}
	}

	return fixed.Rectangle26_6{
		Min: fixed.Point26_6{
			X: float32ToFixed26_6(minX),
			Y: float32ToFixed26_6(minY),
		},
		Max: fixed.Point26_6{
			X: float32ToFixed26_6(maxX),
			Y: float32ToFixed26_6(maxY),
		},
	}
}

// segmentsToImage rasterizes outline segments into an image using the same
// golang.org/x/image/vector.Rasterizer as Ebitengine.
func segmentsToImage(segs []opentype.Segment, subpixelOffset fixed.Point26_6, glyphBounds fixed.Rectangle26_6) *futurerender.Image {
	if len(segs) == 0 {
		return nil
	}

	w := (glyphBounds.Max.X - glyphBounds.Min.X).Ceil()
	h := (glyphBounds.Max.Y - glyphBounds.Min.Y).Ceil()
	if w == 0 || h == 0 {
		return nil
	}

	// Match Ebitengine: always add 1 to the size.
	w++
	h++

	biasX := fixed26_6ToFloat32(-glyphBounds.Min.X + subpixelOffset.X)
	biasY := fixed26_6ToFloat32(-glyphBounds.Min.Y + subpixelOffset.Y)

	rast := gvector.NewRasterizer(w, h)
	rast.DrawOp = draw.Src
	for _, seg := range segs {
		switch seg.Op {
		case opentype.SegmentOpMoveTo:
			rast.MoveTo(seg.Args[0].X+biasX, seg.Args[0].Y+biasY)
		case opentype.SegmentOpLineTo:
			rast.LineTo(seg.Args[0].X+biasX, seg.Args[0].Y+biasY)
		case opentype.SegmentOpQuadTo:
			rast.QuadTo(
				seg.Args[0].X+biasX, seg.Args[0].Y+biasY,
				seg.Args[1].X+biasX, seg.Args[1].Y+biasY,
			)
		case opentype.SegmentOpCubeTo:
			rast.CubeTo(
				seg.Args[0].X+biasX, seg.Args[0].Y+biasY,
				seg.Args[1].X+biasX, seg.Args[1].Y+biasY,
				seg.Args[2].X+biasX, seg.Args[2].Y+biasY,
			)
		}
	}
	rast.ClosePath()

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	rast.Draw(dst, dst.Bounds(), image.Opaque, image.Point{})
	return futurerender.NewImageFromImage(dst)
}

// --- Fixed-point conversion helpers ---

func float64ToFixed26_6(v float64) fixed.Int26_6 {
	return fixed.Int26_6(v * (1 << 6))
}

func fixed26_6ToFloat64(v fixed.Int26_6) float64 {
	return float64(v) / (1 << 6)
}

func float32ToFixed26_6(v float32) fixed.Int26_6 {
	return fixed.Int26_6(v * (1 << 6))
}

func fixed26_6ToFloat32(v fixed.Int26_6) float32 {
	return float32(v) / (1 << 6)
}

// fixedToFloat converts a fixed.Int26_6 to float64.
// Kept for backward compatibility with shaping.go.
func fixedToFloat(v fixed.Int26_6) float64 {
	return float64(v) / 64.0
}

// fixedFloor returns the integer floor of a fixed.Int26_6 value.
// 64 fixed-point units = 1 pixel; values < 64 floor to 0, etc.
func fixedFloor(v fixed.Int26_6) int {
	return int(v) >> 6
}

// fixedCeil returns the integer ceiling of a fixed.Int26_6 value.
// Mirrors `fixed.Int26_6.Ceil()` semantics for the unit tests.
func fixedCeil(v fixed.Int26_6) int {
	return int(v+0x3f) >> 6
}

// glyphVariationCount determines subpixel rendering granularity based on font
// metrics, matching Ebitengine's implementation exactly.
func glyphVariationCount(m Metrics) int {
	s := m.Ascent + m.Descent
	if s < 20 {
		return 8
	}
	if s < 40 {
		return 4
	}
	if s < 80 {
		return 2
	}
	return 1
}

// adjustGranularity quantizes the subpixel position to match Ebitengine's
// glyph variation count.
func adjustGranularity(x fixed.Int26_6, m Metrics) fixed.Int26_6 {
	c := glyphVariationCount(m)
	factor := (1 << 6) / fixed.Int26_6(c)
	return x / factor * factor
}
