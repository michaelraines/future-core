package futureutil

import (
	"testing"

	futurerender "github.com/michaelraines/future-core"

	"github.com/stretchr/testify/require"
)

func TestDebugPrintNilImage(t *testing.T) {
	// Should not panic.
	DebugPrint(nil, "hello")
	DebugPrintAt(nil, "hello", 0, 0)
}

func TestDebugPrintEmptyMessage(t *testing.T) {
	img := futurerender.NewImage(100, 100)
	// Should not panic.
	DebugPrintAt(img, "", 0, 0)
	DebugPrint(img, "")
}

func TestDebugPrintAtlasInitialized(t *testing.T) {
	initDebugPrintAtlas()
	require.NotNil(t, debugPrintAtlas)

	w, h := debugPrintAtlas.Size()
	require.Greater(t, w, 0)
	require.Greater(t, h, 0)
}

func TestDebugPrintAtlasIsSingleton(t *testing.T) {
	initDebugPrintAtlas()
	a1 := debugPrintAtlas
	initDebugPrintAtlas()
	a2 := debugPrintAtlas
	require.True(t, a1 == a2, "atlas should be a singleton")
}

func TestDebugPrintAtDrawsWithoutPanic(t *testing.T) {
	img := futurerender.NewImage(200, 200)

	// Drawing various strings should not panic.
	DebugPrintAt(img, "Hello, World!", 0, 0)
	DebugPrintAt(img, "Line1\nLine2", 10, 10)
	DebugPrintAt(img, "~!@#$%^&*()", 0, 0)
}

func TestDebugPrintDrawsWithoutPanic(t *testing.T) {
	img := futurerender.NewImage(200, 200)
	DebugPrint(img, "test")
}

func TestDebugPrintAtNonASCII(t *testing.T) {
	img := futurerender.NewImage(200, 200)
	// Non-ASCII or unsupported characters should not panic.
	DebugPrintAt(img, "\u00e9\u00f1\u00fc\U0001F600", 0, 0)
}
