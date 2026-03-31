package futureutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDebugPrintNilImage(t *testing.T) {
	// Should not panic.
	DebugPrint(nil, "hello")
	DebugPrintAt(nil, "hello", 0, 0)
}

func TestDebugPrintEmptyMessage(t *testing.T) {
	img := debugFontAtlas() // use atlas as a target (non-nil image)
	// Should not panic.
	DebugPrintAt(img, "", 0, 0)
	DebugPrint(img, "")
}

func TestDebugFontAtlasCreated(t *testing.T) {
	atlas := debugFontAtlas()
	require.NotNil(t, atlas)

	// Atlas should have correct dimensions.
	rows := (debugCharCount + debugCols - 1) / debugCols
	expectedW := debugCols * debugCharW
	expectedH := rows * debugCharH
	w, h := atlas.Size()
	require.Equal(t, expectedW, w)
	require.Equal(t, expectedH, h)
}

func TestDebugFontAtlasSingleton(t *testing.T) {
	a1 := debugFontAtlas()
	a2 := debugFontAtlas()
	require.True(t, a1 == a2, "atlas should be a singleton")
}

func TestDebugGlyphReturnsNilForUnsupported(t *testing.T) {
	require.Nil(t, debugGlyph(0))   // control char
	require.Nil(t, debugGlyph(31))  // control char
	require.Nil(t, debugGlyph(127)) // DEL
	require.Nil(t, debugGlyph(200)) // extended
	require.Nil(t, debugGlyph(-1))  // negative
}

func TestDebugGlyphSpace(t *testing.T) {
	// Space (32) should be nil (blank glyph).
	require.Nil(t, debugGlyph(' '))
}

func TestDebugGlyphPrintableChars(t *testing.T) {
	// All printable non-space ASCII should have glyph data.
	for ch := rune(33); ch < 127; ch++ {
		g := debugGlyph(ch)
		require.NotNil(t, g, "missing glyph for %c (%d)", ch, ch)
		require.Greater(t, len(g), 0, "empty glyph for %c (%d)", ch, ch)
	}
}

func TestDebugPrintAtDrawsWithoutPanic(t *testing.T) {
	atlas := debugFontAtlas()
	require.NotNil(t, atlas)

	// Drawing various strings should not panic.
	DebugPrintAt(atlas, "Hello, World!", 0, 0)
	DebugPrintAt(atlas, "Line1\nLine2", 10, 10)
	DebugPrintAt(atlas, "~!@#$%^&*()", 0, 0)
}

func TestDebugPrintDrawsWithoutPanic(t *testing.T) {
	atlas := debugFontAtlas()
	require.NotNil(t, atlas)
	DebugPrint(atlas, "test")
}

func TestDebugPrintConstants(t *testing.T) {
	require.Equal(t, 6, debugCharW)
	require.Equal(t, 16, debugCharH)
	require.Equal(t, 32, debugCharStart)
	require.Equal(t, 127, debugCharEnd)
	require.Equal(t, 95, debugCharCount)
	require.Equal(t, 16, debugCols)
}

func TestDebugPrintAtNonASCII(t *testing.T) {
	atlas := debugFontAtlas()
	require.NotNil(t, atlas)
	// Non-ASCII characters should be silently skipped, not panic.
	DebugPrintAt(atlas, "\u00e9\u00f1\u00fc\U0001F600", 0, 0)
}
