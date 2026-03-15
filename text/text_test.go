package text

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/image/font/gofont/goregular"

	futurerender "github.com/michaelraines/future-render"
	fmath "github.com/michaelraines/future-render/math"
)

func cleanupAtlases(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		globalAtlases = map[*Face]*fontAtlas{}
	})
}

// --- Face tests ---

func TestNewFace(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)
	require.NotNil(t, face)
	require.InDelta(t, 24.0, face.size, 1e-9)
}

func TestNewFaceInvalidData(t *testing.T) {
	_, err := NewFace([]byte("not a font"), 24)
	require.Error(t, err)
}

func TestFaceMetrics(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	m := face.Metrics()
	require.Greater(t, m.Height, 0.0)
	require.Greater(t, m.Ascent, 0.0)
	require.Greater(t, m.Descent, 0.0)
	require.Greater(t, m.Height, m.Ascent)
}

func TestFaceClose(t *testing.T) {
	cleanupAtlases(t)

	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	target := futurerender.NewImage(200, 200)

	// Draw to create atlas and cache entries.
	Draw(target, "Hello", face, 10, 20, nil)
	_, ok := globalAtlases[face]
	require.True(t, ok, "atlas should exist after Draw")
	require.NotEmpty(t, face.cache.entries)

	// Close should remove the atlas and clear the cache.
	face.Close()
	_, ok = globalAtlases[face]
	require.False(t, ok, "atlas should be removed after Close")
	require.Empty(t, face.cache.entries)
}

func TestFaceCloseWithoutAtlas(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	// Close on a face that was never used should not panic.
	face.Close()
}

func TestFaceMeasure(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	w := face.Measure("Hello")
	require.Greater(t, w, 0.0)

	// Longer text should be wider.
	w2 := face.Measure("Hello, World!")
	require.Greater(t, w2, w)
}

func TestFaceMeasureEmpty(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	require.InDelta(t, 0.0, face.Measure(""), 1e-9)
}

// --- Glyph cache tests ---

func TestGlyphCacheHitMiss(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	atlas := newFontAtlas()
	atlas.image = futurerender.NewImage(256, 256)

	// First call is a miss — rasterizes.
	g1 := face.cache.get('A', atlas)
	require.NotNil(t, g1)
	require.False(t, g1.empty)
	require.Greater(t, g1.advance, 0.0)
	require.Greater(t, g1.width, 0)
	require.Greater(t, g1.height, 0)

	// Second call is a hit — same pointer.
	g2 := face.cache.get('A', atlas)
	require.Equal(t, g1, g2)
}

func TestGlyphCacheSpace(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	atlas := newFontAtlas()
	atlas.image = futurerender.NewImage(256, 256)

	g := face.cache.get(' ', atlas)
	require.NotNil(t, g)
	require.True(t, g.empty)
	require.Greater(t, g.advance, 0.0)
}

func TestGlyphCacheMultipleRunes(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	atlas := newFontAtlas()
	atlas.image = futurerender.NewImage(512, 512)

	for _, r := range "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789" {
		g := face.cache.get(r, atlas)
		require.NotNil(t, g)
		require.Greater(t, g.advance, 0.0, "rune %c", r)
	}

	// All should now be cached.
	require.Len(t, face.cache.entries, 62)
}

func TestGlyphCacheNilAtlas(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	// Should not panic even with nil atlas — glyph just won't have atlas coords.
	g := face.cache.get('A', nil)
	require.NotNil(t, g)
	require.False(t, g.empty)
}

func TestGlyphCacheInvalidatedOnAtlasGrow(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	atlas := &fontAtlas{size: 16}
	atlas.image = futurerender.NewImage(16, 16)

	// Cache a glyph.
	g1 := face.cache.get('A', atlas)
	require.NotNil(t, g1)
	require.Len(t, face.cache.entries, 1)

	gen0 := atlas.generation

	// Force a grow — this increments generation and rebuilds the atlas image.
	grew := atlas.grow()
	require.True(t, grew)
	require.Greater(t, atlas.generation, gen0)

	// The cache still has the stale entry for 'A'. The next get() call
	// should detect the generation mismatch, clear the cache, and re-rasterize.
	g2 := face.cache.get('A', atlas)
	require.NotNil(t, g2)
	// After invalidation, only 'A' should be in the cache (freshly rasterized).
	require.Len(t, face.cache.entries, 1)
	// The pointer must differ — it's a new entry, not the stale one.
	require.True(t, g1 != g2, "entry pointer should differ after re-rasterize")
}

func TestGlyphCacheNotInvalidatedWithoutGrow(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	atlas := newFontAtlas()
	atlas.image = futurerender.NewImage(512, 512)

	g1 := face.cache.get('B', atlas)
	g2 := face.cache.get('B', atlas)
	require.Equal(t, g1, g2, "same atlas generation should return cached entry")
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
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	// Should not panic.
	Draw(nil, "Hello", face, 0, 0, nil)
}

func TestDrawNilFace(t *testing.T) {
	img := futurerender.NewImage(100, 100)
	// Should not panic.
	Draw(img, "Hello", nil, 0, 0, nil)
}

func TestDrawEmptyString(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)
	img := futurerender.NewImage(100, 100)

	// Should not panic and should be a no-op.
	Draw(img, "", face, 0, 0, nil)
}

func TestDrawCreatesAtlas(t *testing.T) {
	cleanupAtlases(t)

	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	target := futurerender.NewImage(200, 200)

	// Draw text — should create atlas for this face.
	Draw(target, "Hi", face, 10, 20, nil)

	atlas, ok := globalAtlases[face]
	require.True(t, ok)
	require.NotNil(t, atlas)
	require.NotNil(t, atlas.image)
}

func TestDrawWithColorScale(t *testing.T) {
	cleanupAtlases(t)

	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	target := futurerender.NewImage(200, 200)

	opts := &DrawOptions{
		ColorScale: fmath.Color{R: 1, G: 0, B: 0, A: 1},
	}
	// Should not panic.
	Draw(target, "Red", face, 10, 20, opts)
}

func TestDrawDefaultsToWhite(t *testing.T) {
	cleanupAtlases(t)

	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	target := futurerender.NewImage(200, 200)

	// Zero color should default to white.
	opts := &DrawOptions{}
	Draw(target, "A", face, 0, 0, opts)
}

func TestDrawSpacesOnly(t *testing.T) {
	cleanupAtlases(t)

	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	target := futurerender.NewImage(200, 200)

	// Should not panic — spaces are empty glyphs, no DrawImage calls.
	Draw(target, "   ", face, 10, 20, nil)
}

func TestAtlasForReusesFace(t *testing.T) {
	cleanupAtlases(t)

	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	a1 := atlasFor(face)
	a2 := atlasFor(face)
	require.Equal(t, a1, a2)
}

func TestAtlasForDifferentFaces(t *testing.T) {
	cleanupAtlases(t)

	face1, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)
	face2, err := NewFace(goregular.TTF, 48)
	require.NoError(t, err)

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

	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	target := futurerender.NewImage(400, 400)
	// Should not panic with multi-line text.
	Draw(target, "Hello\nWorld", face, 10, 20, nil)
}

func TestDrawMultilineWithAlignment(t *testing.T) {
	cleanupAtlases(t)

	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	target := futurerender.NewImage(400, 400)

	for _, align := range []Align{AlignLeft, AlignCenter, AlignRight} {
		opts := &DrawOptions{Align: align}
		Draw(target, "Short\nLonger line here", face, 10, 20, opts)
	}
}

// --- Word wrapping tests ---

func TestWrapLines(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	// Measure a known word to set a reasonable maxWidth.
	wordW := face.Measure("Hello")
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
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	lines := WrapLines("Hello\nWorld", face, 1000)
	require.Equal(t, []string{"Hello", "World"}, lines)
}

func TestWrapLinesEmptyParagraphs(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	lines := WrapLines("A\n\nB", face, 1000)
	require.Equal(t, []string{"A", "", "B"}, lines)
}

func TestWrapLinesLongWord(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

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
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	lines := WrapLines("Hello", face, 0)
	require.Equal(t, []string{"Hello"}, lines)
}

func TestDrawWrapped(t *testing.T) {
	cleanupAtlases(t)

	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	target := futurerender.NewImage(400, 400)
	// Should not panic.
	DrawWrapped(target, "The quick brown fox jumps over the lazy dog", face, 10, 20, 200, nil)
}

func TestDrawWrappedNilTarget(t *testing.T) {
	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)
	DrawWrapped(nil, "Hello", face, 0, 0, 100, nil)
}

func TestDrawWrappedWithAlignment(t *testing.T) {
	cleanupAtlases(t)

	face, err := NewFace(goregular.TTF, 24)
	require.NoError(t, err)

	target := futurerender.NewImage(400, 400)
	opts := &DrawOptions{Align: AlignCenter}
	DrawWrapped(target, "Short\nA much longer line", face, 10, 20, 300, opts)
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
	require.Equal(t, Align(0), AlignLeft)
	require.Equal(t, Align(1), AlignCenter)
	require.Equal(t, Align(2), AlignRight)
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
