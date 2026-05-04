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
	"os"
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
	// oversample is the rasterisation oversampling factor (DPR). At
	// HiDPI the glyph image is rasterised at size×oversample atlas
	// pixels and drawn with a 1/oversample scale, so display pixel
	// density matches the physical framebuffer instead of the GPU
	// linear-upscaling a logical-resolution atlas. Stored in the key
	// so DPR changes (e.g. window dragged between displays) build a
	// fresh atlas rather than reusing the previous resolution.
	oversample float64
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
// oversample > 1 rasterises at higher resolution for HiDPI displays;
// the caller is responsible for applying the matching 1/oversample
// scale on the draw transform so the glyph appears at logical size.
func (g *GoTextFaceSource) getOrCreateGlyphImage(key glyphImageKey, segs []opentype.Segment, subpixelOffset fixed.Point26_6, bounds fixed.Rectangle26_6, oversample float64) *futurerender.Image {
	g.mu.Lock()
	defer g.mu.Unlock()

	if img, ok := g.glyphImageCache[key]; ok {
		return img
	}

	img := segmentsToImage(segs, subpixelOffset, bounds, oversample)
	if traceText && img != nil {
		w, h := img.Size()
		fmt.Fprintf(os.Stderr, "[text]   new glyph gid=%d size=%.0f img=%dx%d\n",
			key.gid, key.size, w, h)
	}
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
// traceText controls diagnostic logging for text rendering.
// Enabled by FUTURE_CORE_TRACE_TEXT=1. Logs glyph count, image creation,
// and draw positions to help diagnose invisible text in specific backends.
var traceText = os.Getenv("FUTURE_CORE_TRACE_TEXT") != ""

func (f *GoTextFace) drawGlyphs(target *futurerender.Image, s string, ox, oy float64, cs futurerender.ColorScale, geoM futurerender.GeoM) {
	if s == "" || f.Source == nil {
		return
	}

	_, gs := f.Source.shape(s, f.Size)

	// Oversample the glyph atlas for HiDPI displays. At DPR=2 the
	// engine's logical→physical viewport scaling stretches a logical-
	// resolution atlas 2x with linear filtering, producing blurry
	// chunky text. Rasterising at DPR× resolution and downscaling on
	// draw lets the GPU sample a high-resolution atlas at 1:1 with
	// the physical framebuffer, yielding crisp glyphs. Capped at 4×
	// to avoid pathological atlases on freak DPR values.
	oversample := futurerender.DeviceScaleFactor()
	if oversample < 1 {
		oversample = 1
	}
	if oversample > 4 {
		oversample = 4
	}
	invOversample := 1.0 / oversample

	if traceText {
		tx, ty := geoM.Apply(0, 0)
		fmt.Fprintf(os.Stderr, "[text] drawGlyphs size=%.0f glyphs=%d geo=(%.0f,%.0f) oversample=%.1f text=%q\n", f.Size, len(gs), tx, ty, oversample, truncate(s, 30))
	}

	origin := fixed.Point26_6{
		X: float64ToFixed26_6(ox),
		Y: float64ToFixed26_6(oy),
	}

	var drawn, skipped int
	for _, g := range gs {
		o := origin.Add(fixed.Point26_6{
			X: g.sg.XOffset,
			Y: -g.sg.YOffset,
		})

		img, imgX, imgY := f.glyphImage(g, o, oversample)
		if img != nil {
			drawOpts := &futurerender.DrawImageOptions{}
			drawOpts.ColorScale = cs
			// Downscale the oversampled atlas to logical pixel size
			// BEFORE translating, so the translate(imgX, imgY) lands
			// the image's logical-size top-left at the integer pixel
			// position we computed. Order: scale (around 0,0), then
			// translate.
			if oversample > 1 {
				drawOpts.GeoM.Scale(invOversample, invOversample)
			}
			drawOpts.GeoM.Translate(float64(imgX), float64(imgY))
			drawOpts.GeoM.Concat(geoM)
			if traceText && drawn == 0 {
				// Log position of first glyph only (to avoid flood).
				tw, th := target.Size()
				gw, gh := img.Size()
				fmt.Fprintf(os.Stderr, "[text]   pos=(%d,%d) glyph=%dx%d target=%dx%d cs=(%.2f,%.2f,%.2f,%.2f)\n",
					imgX, imgY, gw, gh, tw, th,
					cs.R(), cs.G(), cs.B(), cs.A())
			}
			target.DrawImage(img, drawOpts)
			drawn++
		} else {
			skipped++
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

	if traceText {
		fmt.Fprintf(os.Stderr, "[text]   drawn=%d skipped=%d\n", drawn, skipped)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// glyphImage returns the rasterized glyph image and the integer pixel
// position at which it should be drawn on the target. img is the cached
// glyph bitmap; imgX and imgY are the top-left screen-space coordinates
// after subpixel adjustment. Matches Ebitengine's subpixel positioning.
//
// oversample > 1 produces an atlas at oversample× resolution; the
// caller (drawGlyphs) must apply a 1/oversample scale on the draw
// transform so the glyph displays at logical size.
func (f *GoTextFace) glyphImage(g shapedGlyph, origin fixed.Point26_6, oversample float64) (img *futurerender.Image, imgX, imgY int) {
	// For horizontal text: vary X subpixel, floor Y.
	origin.X = adjustGranularity(origin.X, f.Metrics())
	origin.Y &^= ((1 << 6) - 1)

	b := g.bounds

	subpixelOffset := fixed.Point26_6{
		X: (origin.X + b.Min.X) & ((1 << 6) - 1),
		Y: (origin.Y + b.Min.Y) & ((1 << 6) - 1),
	}

	key := glyphImageKey{
		gid:        g.sg.GlyphID,
		xoffset:    subpixelOffset.X,
		yoffset:    subpixelOffset.Y,
		size:       f.Size,
		oversample: oversample,
	}

	img = f.Source.getOrCreateGlyphImage(key, g.scaledSegments, subpixelOffset, b, oversample)

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
//
// oversample > 1 produces an atlas at oversample× the logical glyph
// size — the rasteriser dimensions and segment coordinates are all
// multiplied by oversample, so the resulting image is a higher-
// resolution rasterisation of the same outline. The caller is
// responsible for applying a 1/oversample scale on the draw transform
// so the glyph appears at logical size on screen, with the GPU
// linear-filtering down from the oversampled atlas instead of up
// from a logical-resolution one. oversample <= 1 falls back to the
// original 1:1 logical rasterisation.
func segmentsToImage(segs []opentype.Segment, subpixelOffset fixed.Point26_6, glyphBounds fixed.Rectangle26_6, oversample float64) *futurerender.Image {
	if len(segs) == 0 {
		return nil
	}
	if oversample < 1 {
		oversample = 1
	}
	scale := float32(oversample)

	w := (glyphBounds.Max.X - glyphBounds.Min.X).Ceil()
	h := (glyphBounds.Max.Y - glyphBounds.Min.Y).Ceil()
	if w == 0 || h == 0 {
		if traceText {
			fmt.Fprintf(os.Stderr, "[text]   segmentsToImage: zero size w=%d h=%d, returning nil\n", w, h)
		}
		return nil
	}

	// Match Ebitengine: always add 1 to the size.
	w++
	h++
	// Oversample: scale the atlas dimensions up so the rasteriser
	// produces a high-resolution glyph. The +1 padding stays at
	// oversample× scale too — the rasteriser uses subpixel coverage
	// from the bias values, so a wider image keeps the antialiased
	// edge inside the canvas.
	if oversample > 1 {
		w = int(float64(w) * oversample)
		h = int(float64(h) * oversample)
	}

	biasX := fixed26_6ToFloat32(-glyphBounds.Min.X+subpixelOffset.X) * scale
	biasY := fixed26_6ToFloat32(-glyphBounds.Min.Y+subpixelOffset.Y) * scale

	rast := gvector.NewRasterizer(w, h)
	rast.DrawOp = draw.Src
	for _, seg := range segs {
		switch seg.Op {
		case opentype.SegmentOpMoveTo:
			rast.MoveTo(seg.Args[0].X*scale+biasX, seg.Args[0].Y*scale+biasY)
		case opentype.SegmentOpLineTo:
			rast.LineTo(seg.Args[0].X*scale+biasX, seg.Args[0].Y*scale+biasY)
		case opentype.SegmentOpQuadTo:
			rast.QuadTo(
				seg.Args[0].X*scale+biasX, seg.Args[0].Y*scale+biasY,
				seg.Args[1].X*scale+biasX, seg.Args[1].Y*scale+biasY,
			)
		case opentype.SegmentOpCubeTo:
			rast.CubeTo(
				seg.Args[0].X*scale+biasX, seg.Args[0].Y*scale+biasY,
				seg.Args[1].X*scale+biasX, seg.Args[1].Y*scale+biasY,
				seg.Args[2].X*scale+biasX, seg.Args[2].Y*scale+biasY,
			)
		}
	}
	rast.ClosePath()

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	rast.Draw(dst, dst.Bounds(), image.Opaque, image.Point{})
	if traceText {
		// Count non-transparent pixels in the rasterized glyph.
		nonZero := 0
		for i := 3; i < len(dst.Pix); i += 4 {
			if dst.Pix[i] > 0 {
				nonZero++
			}
		}
		fmt.Fprintf(os.Stderr, "[text]   rasterized %dx%d glyph: %d/%d opaque pixels\n", w, h, nonZero, w*h)
	}
	img := futurerender.NewImageFromImage(dst)
	if traceText && img == nil {
		fmt.Fprintf(os.Stderr, "[text]   segmentsToImage: NewImageFromImage returned nil for %dx%d glyph\n", w, h)
	}
	return img
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
