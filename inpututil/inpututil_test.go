package inpututil

import (
	"testing"

	"github.com/stretchr/testify/require"

	futurerender "github.com/michaelraines/future-core"
)

func TestAppendPressedKeysNoEngine(t *testing.T) {
	// Without an engine running, no keys should be pressed.
	keys := AppendPressedKeys(nil)
	require.Empty(t, keys)
}

func TestPressedKeysNoEngine(t *testing.T) {
	keys := PressedKeys()
	require.Empty(t, keys)
}

func TestIsMouseButtonJustPressedNoEngine(t *testing.T) {
	// Without an engine running, should return false.
	require.False(t, IsMouseButtonJustPressed(futurerender.MouseButtonLeft))
}

func TestAppendPressedKeysAppendsToExisting(t *testing.T) {
	// Even with no engine, the function should work with a non-nil slice.
	existing := []futurerender.Key{futurerender.KeyA}
	result := AppendPressedKeys(existing)
	// No keys pressed, so result should be unchanged.
	require.Equal(t, []futurerender.Key{futurerender.KeyA}, result)
}
