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

// --- No-engine behavior for newly-added edge-triggered helpers. These
// mirror the existing AppendPressedKeys coverage: with no engine wired
// up the queries all return nil/empty or zero without panicking.

func TestAppendJustPressedKeysNoEngine(t *testing.T) {
	require.Empty(t, AppendJustPressedKeys(nil))
}

func TestAppendJustReleasedKeysNoEngine(t *testing.T) {
	require.Empty(t, AppendJustReleasedKeys(nil))
}

func TestAppendJustPressedKeysAppendsToExisting(t *testing.T) {
	existing := []futurerender.Key{futurerender.KeyEscape}
	result := AppendJustPressedKeys(existing)
	require.Equal(t, existing, result)
}

func TestAppendJustReleasedKeysAppendsToExisting(t *testing.T) {
	existing := []futurerender.Key{futurerender.KeyEscape}
	result := AppendJustReleasedKeys(existing)
	require.Equal(t, existing, result)
}

func TestAppendJustPressedTouchIDsNoEngine(t *testing.T) {
	require.Empty(t, AppendJustPressedTouchIDs(nil))

	// Non-nil slice is returned unchanged.
	existing := []futurerender.TouchID{1, 2}
	require.Equal(t, existing, AppendJustPressedTouchIDs(existing))
}

func TestAppendJustReleasedTouchIDsNoEngine(t *testing.T) {
	require.Empty(t, AppendJustReleasedTouchIDs(nil))

	existing := []futurerender.TouchID{1, 2}
	require.Equal(t, existing, AppendJustReleasedTouchIDs(existing))
}

func TestKeyPressDurationNoEngine(t *testing.T) {
	require.Equal(t, 0, KeyPressDuration(futurerender.KeyA))
}

func TestMouseButtonPressDurationNoEngine(t *testing.T) {
	require.Equal(t, 0, MouseButtonPressDuration(futurerender.MouseButtonLeft))
}

func TestIsGamepadButtonJustPressedNoEngine(t *testing.T) {
	require.False(t, IsGamepadButtonJustPressed(0, 0))
}

func TestIsGamepadButtonJustReleasedNoEngine(t *testing.T) {
	require.False(t, IsGamepadButtonJustReleased(0, 0))
}

func TestGamepadButtonPressDurationNoEngine(t *testing.T) {
	require.Equal(t, 0, GamepadButtonPressDuration(0, 0))
}
