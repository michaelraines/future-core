package text

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/image/font/gofont/goregular"

	futurerender "github.com/michaelraines/future-core"
)

func cleanupAtlases(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		globalAtlases = map[Face]*fontAtlas{}
	})
}

// newTestFace is a helper that creates a *GoTextFace for testing.
func newTestFace(t *testing.T, size float64) *GoTextFace {
	t.Helper()
	source, err := NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	require.NoError(t, err)
	face := &GoTextFace{Source: source, Size: size}
	return face
}

// --- GoTextFaceSource tests ---

func TestNewGoTextFaceSource(t *testing.T) {
	source, err := NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	require.NoError(t, err)
	require.NotNil(t, source)
	require.NotNil(t, source.f)
}

func TestNewGoTextFaceSourceInvalidData(t *testing.T) {
	_, err := NewGoTextFaceSource(bytes.NewReader([]byte("not a font")))
	require.Error(t, err)
}

// --- GoTextFace tests ---

func TestGoTextFaceMetrics(t *testing.T) {
	face := newTestFace(t, 24)
	m := face.Metrics()
	require.Greater(t, m.Height, 0.0)
	require.Greater(t, m.Ascent, 0.0)
	require.Greater(t, m.Descent, 0.0)
}

func TestGoTextFaceNilSource(t *testing.T) {
	face := &GoTextFace{Source: nil, Size: 24}
	m := face.Metrics()
	require.InDelta(t, 0.0, m.Height, 1e-9)
}

// --- NewFace convenience tests ---

func TestNewFace(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)
	require.NotNil(t, face)

	gtf, ok := face.(*GoTextFace)
	require.True(t, ok)
	require.InDelta(t, 24.0, gtf.Size, 1e-9)
}

func TestNewFaceInvalidData(t *testing.T) {
	_, err := NewFace([]byte("not a font"), 24)
	require.Error(t, err)
}

func TestFaceMetrics(t *testing.T) {
	face := newTestFace(t, 24)

	m := face.Metrics()
	require.Greater(t, m.Height, 0.0)
	require.Greater(t, m.Ascent, 0.0)
	require.Greater(t, m.Descent, 0.0)
	require.Greater(t, m.Height, m.Ascent)
}

func TestFaceClose(t *testing.T) {
	face := newTestFace(t, 24)
	// Close should not panic.
	face.Close()
}

// --- Measure tests ---

func TestMeasure(t *testing.T) {
	face := newTestFace(t, 24)

	w, h := Measure("Hello", face, 0)
	require.Greater(t, w, 0.0)
	require.Greater(t, h, 0.0)

	// Longer text should be wider.
	w2, h2 := Measure("Hello, World!", face, 0)
	require.Greater(t, w2, w)
	require.InDelta(t, h, h2, 1e-9, "single-line height should match")
}

func TestMeasureEmpty(t *testing.T) {
	face := newTestFace(t, 24)

	w, h := Measure("", face, 0)
	require.InDelta(t, 0.0, w, 1e-9)
	require.InDelta(t, 0.0, h, 1e-9)
}

func TestMeasureNilFace(t *testing.T) {
	w, h := Measure("Hello", nil, 0)
	require.InDelta(t, 0.0, w, 1e-9)
	require.InDelta(t, 0.0, h, 1e-9)
}

func TestMeasureMultiline(t *testing.T) {
	face := newTestFace(t, 24)

	w1, h1 := Measure("Hello", face, 0)
	w2, h2 := Measure("Hello\nWorld", face, 0)

	// Width should be at least as wide as the widest single line.
	require.GreaterOrEqual(t, w2, w1, "multiline width should be >= single line width")
	// Height should increase for multi-line.
	require.Greater(t, h2, h1)
}

func TestMeasureWithLineSpacing(t *testing.T) {
	face := newTestFace(t, 24)

	_, h1 := Measure("A\nB", face, 0)
	_, h2 := Measure("A\nB", face, 50)

	// Custom line spacing should produce different height.
	require.NotEqual(t, h1, h2, "different line spacing should produce different height")
}

func TestAdvance(t *testing.T) {
	face := newTestFace(t, 24)

	w := Advance("Hello", face)
	require.Greater(t, w, 0.0)

	require.InDelta(t, 0.0, Advance("Hello", nil), 1e-9)
}

// --- Atlas tests ---

func TestAtlasAllocate(t *testing.T) {
	a := &fontAtlas{size: 256}
	a.image = futurerender.NewImage(256, 256)

	x, y, ok := a.allocate(20, 30)
	require.True(t, ok)
	require.Equal(t, 0, x)
	require.Equal(t, 0, y)

	// Second allocation in the same row.
	x2, y2, ok2 := a.allocate(15, 25)
	require.True(t, ok2)
	require.Equal(t, 21, x2) // 20 + 1px padding
	require.Equal(t, 0, y2)
}

func TestAtlasNewRow(t *testing.T) {
	a := &fontAtlas{size: 50}
	a.image = futurerender.NewImage(50, 50)

	// Fill the first row.
	_, _, ok := a.allocate(45, 10)
	require.True(t, ok)

	// Next allocation won't fit in the first row, starts a new row.
	x, y, ok := a.allocate(10, 8)
	require.True(t, ok)
	require.Equal(t, 0, x)
	require.Equal(t, 11, y) // 10 + 1px padding
}

func TestAtlasAllocateZero(t *testing.T) {
	a := &fontAtlas{size: 256}
	a.image = futurerender.NewImage(256, 256)

	_, _, ok := a.allocate(0, 10)
	require.False(t, ok)

	_, _, ok = a.allocate(10, 0)
	require.False(t, ok)
}

func TestAtlasGrowth(t *testing.T) {
	a := &fontAtlas{size: 16}
	a.image = futurerender.NewImage(16, 16)

	// Fill the entire 16x16 atlas.
	_, _, ok := a.allocate(15, 15)
	require.True(t, ok)

	// Next allocation triggers growth.
	_, _, ok = a.allocate(10, 10)
	require.True(t, ok)
	require.Equal(t, 32, a.size)
}

func TestAtlasGrowthIncrementsGeneration(t *testing.T) {
	a := &fontAtlas{size: 16}
	a.image = futurerender.NewImage(16, 16)

	require.Equal(t, 0, a.generation)

	require.True(t, a.grow())
	require.Equal(t, 1, a.generation)
	require.Equal(t, 32, a.size)

	require.True(t, a.grow())
	require.Equal(t, 2, a.generation)
	require.Equal(t, 64, a.size)
}

func TestAtlasGrowthLimit(t *testing.T) {
	a := &fontAtlas{size: maxAtlasSize}
	a.image = futurerender.NewImage(maxAtlasSize, maxAtlasSize)

	// Fill it.
	_, _, ok := a.allocate(maxAtlasSize-1, maxAtlasSize-1)
	require.True(t, ok)

	// Can't grow beyond max — allocation fails.
	_, _, ok = a.allocate(10, 10)
	require.False(t, ok)
}

func TestAtlasSubImage(t *testing.T) {
	a := &fontAtlas{size: 256}
	a.image = futurerender.NewImage(256, 256)

	sub := a.subImage(10, 20, 30, 40)
	require.NotNil(t, sub)
	w, h := sub.Size()
	require.Equal(t, 30, w)
	require.Equal(t, 40, h)
}

func TestAtlasSubImageNilImage(t *testing.T) {
	a := &fontAtlas{size: 256}
	require.Nil(t, a.subImage(0, 0, 10, 10))
}

func TestAtlasUploadNilImage(t *testing.T) {
	a := &fontAtlas{size: 256}
	// Should not panic.
	a.upload(make([]byte, 40), 0, 0, 1, 1)
}

func TestAtlasEnsureImageLazy(t *testing.T) {
	a := newFontAtlas()
	require.Nil(t, a.image)

	a.ensureImage()
	require.NotNil(t, a.image)

	// Calling again should not create a new image.
	img := a.image
	a.ensureImage()
	require.Equal(t, img, a.image)
}

// --- Draw tests ---

func TestDrawNilTarget(t *testing.T) {
	face := newTestFace(t, 24)
	// Should not panic.
	Draw(nil, "Hello", face, nil)
}

func TestDrawNilFace(t *testing.T) {
	img := futurerender.NewImage(100, 100)
	// Should not panic.
	Draw(img, "Hello", nil, nil)
}

func TestDrawEmptyString(t *testing.T) {
	face := newTestFace(t, 24)
	img := futurerender.NewImage(100, 100)

	// Should not panic and should be a no-op.
	Draw(img, "", face, nil)
}

func TestDrawBasic(t *testing.T) {
	cleanupAtlases(t)

	face := newTestFace(t, 24)
	target := futurerender.NewImage(200, 200)

	opts := &DrawOptions{}
	opts.GeoM.Translate(10, 20)
	// Should not panic.
	Draw(target, "Hi", face, opts)
}

func TestDrawWithColorScale(t *testing.T) {
	cleanupAtlases(t)

	face := newTestFace(t, 24)
	target := futurerender.NewImage(200, 200)

	opts := &DrawOptions{}
	opts.ColorScale.Scale(1, 0, 0, 1)
	opts.GeoM.Translate(10, 20)
	// Should not panic.
	Draw(target, "Red", face, opts)
}

func TestDrawDefaultsToWhite(t *testing.T) {
	cleanupAtlases(t)

	face := newTestFace(t, 24)
	target := futurerender.NewImage(200, 200)

	// Zero color should default to white.
	opts := &DrawOptions{}
	Draw(target, "A", face, opts)
}

func TestDrawSpacesOnly(t *testing.T) {
	cleanupAtlases(t)

	face := newTestFace(t, 24)
	target := futurerender.NewImage(200, 200)

	// Should not panic — spaces are empty glyphs, no DrawImage calls.
	Draw(target, "   ", face, nil)
}

func TestAtlasForReusesFace(t *testing.T) {
	cleanupAtlases(t)

	face := newTestFace(t, 24)

	a1 := atlasFor(face)
	a2 := atlasFor(face)
	require.Equal(t, a1, a2)
}

func TestAtlasForDifferentFaces(t *testing.T) {
	cleanupAtlases(t)

	face1 := newTestFace(t, 24)
	face2 := newTestFace(t, 48)

	a1 := atlasFor(face1)
	a2 := atlasFor(face2)
	require.False(t, a1 == a2, "different faces should have different atlases")
}

// --- Fixed-point conversion tests ---

func TestFixedToFloat(t *testing.T) {
	require.InDelta(t, 1.0, fixedToFloat(64), 1e-9)
	require.InDelta(t, 0.5, fixedToFloat(32), 1e-9)
	require.InDelta(t, 0.0, fixedToFloat(0), 1e-9)
}

func TestFixedFloorCeil(t *testing.T) {
	require.Equal(t, 1, fixedFloor(64))
	require.Equal(t, 1, fixedFloor(127))
	require.Equal(t, 2, fixedCeil(65))
	require.Equal(t, 1, fixedCeil(64))
	require.Equal(t, 0, fixedCeil(0))
}

// --- Multi-line text tests ---

func TestDrawMultiline(t *testing.T) {
	cleanupAtlases(t)

	face := newTestFace(t, 24)
	target := futurerender.NewImage(400, 400)

	opts := &DrawOptions{}
	opts.GeoM.Translate(10, 20)
	// Should not panic with multi-line text.
	Draw(target, "Hello\nWorld", face, opts)
}

func TestDrawMultilineWithAlignment(t *testing.T) {
	cleanupAtlases(t)

	face := newTestFace(t, 24)
	target := futurerender.NewImage(400, 400)

	for _, align := range []Align{AlignStart, AlignCenter, AlignEnd} {
		opts := &DrawOptions{}
		opts.PrimaryAlign = align
		opts.GeoM.Translate(10, 20)
		Draw(target, "Short\nLonger line here", face, opts)
	}
}

func TestDrawWithSecondaryAlign(t *testing.T) {
	cleanupAtlases(t)

	face := newTestFace(t, 24)
	target := futurerender.NewImage(400, 400)

	for _, align := range []Align{AlignStart, AlignCenter, AlignEnd} {
		opts := &DrawOptions{}
		opts.SecondaryAlign = align
		opts.GeoM.Translate(200, 200)
		Draw(target, "Hello\nWorld", face, opts)
	}
}

func TestDrawWithLineSpacing(t *testing.T) {
	cleanupAtlases(t)

	face := newTestFace(t, 24)
	target := futurerender.NewImage(400, 400)

	opts := &DrawOptions{}
	opts.LineSpacing = 40
	opts.GeoM.Translate(10, 20)
	Draw(target, "Line1\nLine2", face, opts)
}

// --- Word wrapping tests ---

func TestWrapLines(t *testing.T) {
	face := newTestFace(t, 24)

	// Measure a known word to set a reasonable maxWidth.
	wordW := Advance("Hello", face)
	require.Greater(t, wordW, 0.0)

	// Two words that fit on one line.
	lines := WrapLines("Hello World", face, wordW*3)
	require.Equal(t, []string{"Hello World"}, lines)

	// Two words that don't fit on one line.
	lines = WrapLines("Hello World", face, wordW*1.5)
	require.Len(t, lines, 2)
	require.Equal(t, "Hello", lines[0])
	require.Equal(t, "World", lines[1])
}

func TestWrapLinesPreservesNewlines(t *testing.T) {
	face := newTestFace(t, 24)

	lines := WrapLines("Hello\nWorld", face, 1000)
	require.Equal(t, []string{"Hello", "World"}, lines)
}

func TestWrapLinesEmptyParagraphs(t *testing.T) {
	face := newTestFace(t, 24)

	lines := WrapLines("A\n\nB", face, 1000)
	require.Equal(t, []string{"A", "", "B"}, lines)
}

func TestWrapLinesLongWord(t *testing.T) {
	face := newTestFace(t, 24)

	// A single long word always stays on its own line.
	lines := WrapLines("Supercalifragilistic", face, 10)
	require.Len(t, lines, 1)
	require.Equal(t, "Supercalifragilistic", lines[0])
}

func TestWrapLinesNilFace(t *testing.T) {
	lines := WrapLines("Hello", nil, 100)
	require.Equal(t, []string{"Hello"}, lines)
}

func TestWrapLinesZeroWidth(t *testing.T) {
	face := newTestFace(t, 24)

	lines := WrapLines("Hello", face, 0)
	require.Equal(t, []string{"Hello"}, lines)
}

func TestDrawWrapped(t *testing.T) {
	cleanupAtlases(t)

	face := newTestFace(t, 24)
	target := futurerender.NewImage(400, 400)

	opts := &DrawOptions{}
	opts.GeoM.Translate(10, 20)
	// Should not panic.
	DrawWrapped(target, "The quick brown fox jumps over the lazy dog", face, 200, opts)
}

func TestDrawWrappedNilTarget(t *testing.T) {
	face := newTestFace(t, 24)
	DrawWrapped(nil, "Hello", face, 100, nil)
}

func TestDrawWrappedWithAlignment(t *testing.T) {
	cleanupAtlases(t)

	face := newTestFace(t, 24)
	target := futurerender.NewImage(400, 400)

	opts := &DrawOptions{}
	opts.PrimaryAlign = AlignCenter
	opts.GeoM.Translate(10, 20)
	DrawWrapped(target, "Short\nA much longer line", face, 300, opts)
}

// --- splitWords tests ---

func TestSplitWords(t *testing.T) {
	require.Equal(t, []string{"Hello", "World"}, splitWords("Hello World"))
	require.Equal(t, []string{"One"}, splitWords("  One  "))
	require.Empty(t, splitWords("   "))
	require.Empty(t, splitWords(""))
	require.Equal(t, []string{"A", "B", "C"}, splitWords("A  B\tC"))
}

// --- Alignment constants test ---

func TestAlignConstants(t *testing.T) {
	require.Equal(t, Align(0), AlignStart)
	require.Equal(t, Align(1), AlignCenter)
	require.Equal(t, Align(2), AlignEnd)
}

// --- Shaping tests ---

func TestNewShaperFace(t *testing.T) {
	face, err := NewShaperFace(goregular.TTF, 24)
	require.NoError(t, err)
	require.NotNil(t, face)

	m := face.Metrics()
	require.Greater(t, m.Height, 0.0)
	require.Greater(t, m.Ascent, 0.0)
	require.Greater(t, m.Descent, 0.0)
}

func TestNewShaperFaceInvalidData(t *testing.T) {
	_, err := NewShaperFace([]byte("not a font"), 24)
	require.Error(t, err)
}

func TestShaperFaceShape(t *testing.T) {
	face, err := NewShaperFace(goregular.TTF, 24)
	require.NoError(t, err)

	glyphs := face.Shape("Hello")
	require.NotEmpty(t, glyphs)
	// Each glyph should have positive advance for Latin text.
	for _, g := range glyphs {
		require.Greater(t, g.XAdvance, 0.0, "glyph %d", g.GlyphID)
	}
}

func TestShaperFaceShapeEmpty(t *testing.T) {
	face, err := NewShaperFace(goregular.TTF, 24)
	require.NoError(t, err)

	glyphs := face.Shape("")
	require.Empty(t, glyphs)
}

func TestShaperFaceShapeBidi(t *testing.T) {
	face, err := NewShaperFace(goregular.TTF, 24)
	require.NoError(t, err)

	// Mixed LTR + RTL text (Hebrew characters).
	glyphs := face.ShapeBidi("Hello \u05E9\u05DC\u05D5\u05DD World")
	require.NotEmpty(t, glyphs)
}

// --- BiDi run splitting tests ---

func TestSplitBidiRunsLTR(t *testing.T) {
	runs := splitBidiRuns("Hello World")
	require.Len(t, runs, 1)
	require.Equal(t, "Hello World", runs[0].text)
}

func TestSplitBidiRunsRTL(t *testing.T) {
	runs := splitBidiRuns("\u05E9\u05DC\u05D5\u05DD")
	require.Len(t, runs, 1)
}

func TestSplitBidiRunsMixed(t *testing.T) {
	runs := splitBidiRuns("Hello \u05E9\u05DC\u05D5\u05DD World")
	require.Greater(t, len(runs), 1)
}

func TestSplitBidiRunsEmpty(t *testing.T) {
	runs := splitBidiRuns("")
	require.Nil(t, runs)
}

func TestRuneDirection(t *testing.T) {
	require.False(t, isRTLRune('A'))
	require.False(t, isRTLRune('z'))
	require.True(t, isRTLRune('\u05E9')) // Hebrew Shin
	require.True(t, isRTLRune('\u0627')) // Arabic Alef
}

func TestRuneScript(t *testing.T) {
	require.NotZero(t, runeScript('A'))
	require.NotZero(t, runeScript('\u05E9'))
	require.NotZero(t, runeScript('\u0627'))
}

// --- Face interface compliance test ---

func TestFaceInterfaceCompliance(t *testing.T) {
	face := newTestFace(t, 24)
	// GoTextFace must satisfy the Face interface.
	var _ Face = face
}

// --- splitLines test ---

func TestSplitLines(t *testing.T) {
	require.Equal(t, []string{"a", "b", "c"}, splitLines("a\nb\nc"))
	require.Equal(t, []string{"single"}, splitLines("single"))
	require.Equal(t, []string{""}, splitLines(""))
}

// --- LayoutOptions test ---

func TestDrawOptionsHasLayoutFields(t *testing.T) {
	opts := &DrawOptions{}
	opts.LineSpacing = 30
	opts.PrimaryAlign = AlignCenter
	opts.SecondaryAlign = AlignEnd
	opts.GeoM.Translate(10, 20)
	opts.ColorScale.Scale(1, 0.5, 0, 1)

	require.InDelta(t, 30.0, opts.LineSpacing, 1e-9)
	require.Equal(t, AlignCenter, opts.PrimaryAlign)
	require.Equal(t, AlignEnd, opts.SecondaryAlign)
}
