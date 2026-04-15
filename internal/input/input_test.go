package input

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/platform"
)

func TestNew(t *testing.T) {
	s := New()
	require.NotNil(t, s)
	require.NotNil(t, s.touches)
	require.NotNil(t, s.gamepads)
}

// --- Keyboard ---

func TestKeyPressRelease(t *testing.T) {
	s := New()

	require.False(t, s.IsKeyPressed(platform.KeyA))

	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyA, Action: platform.ActionPress})
	require.True(t, s.IsKeyPressed(platform.KeyA))

	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyA, Action: platform.ActionRelease})
	require.False(t, s.IsKeyPressed(platform.KeyA))
}

func TestKeyRepeatKeepsPressed(t *testing.T) {
	s := New()
	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeySpace, Action: platform.ActionPress})
	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeySpace, Action: platform.ActionRepeat})
	require.True(t, s.IsKeyPressed(platform.KeySpace))
}

func TestIsKeyJustPressed(t *testing.T) {
	s := New()

	// Press in first frame.
	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyA, Action: platform.ActionPress})
	require.True(t, s.IsKeyJustPressed(platform.KeyA))
	require.False(t, s.IsKeyJustReleased(platform.KeyA))

	// Advance frame — no longer "just pressed".
	s.Update()
	require.True(t, s.IsKeyPressed(platform.KeyA))
	require.False(t, s.IsKeyJustPressed(platform.KeyA))
}

func TestIsKeyJustReleased(t *testing.T) {
	s := New()

	// Press.
	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyB, Action: platform.ActionPress})
	s.Update()

	// Release.
	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyB, Action: platform.ActionRelease})
	require.True(t, s.IsKeyJustReleased(platform.KeyB))
	require.False(t, s.IsKeyJustPressed(platform.KeyB))

	// Advance — no longer "just released".
	s.Update()
	require.False(t, s.IsKeyJustReleased(platform.KeyB))
}

func TestKeyOutOfBounds(t *testing.T) {
	s := New()

	// Negative key.
	s.OnKeyEvent(platform.KeyEvent{Key: -1, Action: platform.ActionPress})
	require.False(t, s.IsKeyPressed(-1))
	require.False(t, s.IsKeyJustPressed(-1))
	require.False(t, s.IsKeyJustReleased(-1))

	// Key beyond range.
	bigKey := platform.KeyCount + 10
	s.OnKeyEvent(platform.KeyEvent{Key: bigKey, Action: platform.ActionPress})
	require.False(t, s.IsKeyPressed(bigKey))
}

// --- Mouse buttons ---

func TestMouseButtonPressRelease(t *testing.T) {
	s := New()

	require.False(t, s.IsMouseButtonPressed(platform.MouseButtonLeft))

	s.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: platform.MouseButtonLeft,
		Action: platform.ActionPress,
		X:      10, Y: 20,
	})
	require.True(t, s.IsMouseButtonPressed(platform.MouseButtonLeft))

	s.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: platform.MouseButtonLeft,
		Action: platform.ActionRelease,
		X:      10, Y: 20,
	})
	require.False(t, s.IsMouseButtonPressed(platform.MouseButtonLeft))
}

func TestIsMouseButtonJustPressed(t *testing.T) {
	s := New()

	s.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: platform.MouseButtonRight,
		Action: platform.ActionPress,
	})
	require.True(t, s.IsMouseButtonJustPressed(platform.MouseButtonRight))

	s.Update()
	require.True(t, s.IsMouseButtonPressed(platform.MouseButtonRight))
	require.False(t, s.IsMouseButtonJustPressed(platform.MouseButtonRight))
}

func TestMouseButtonJustReleased(t *testing.T) {
	s := New()

	// Press the button.
	s.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: platform.MouseButtonLeft,
		Action: platform.ActionPress,
	})
	require.True(t, s.IsMouseButtonPressed(platform.MouseButtonLeft))
	require.False(t, s.IsMouseButtonJustReleased(platform.MouseButtonLeft))

	// Advance frame so the press is no longer "just pressed".
	s.Update()
	require.True(t, s.IsMouseButtonPressed(platform.MouseButtonLeft))
	require.False(t, s.IsMouseButtonJustReleased(platform.MouseButtonLeft))

	// Release the button.
	s.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: platform.MouseButtonLeft,
		Action: platform.ActionRelease,
	})
	require.False(t, s.IsMouseButtonPressed(platform.MouseButtonLeft))
	require.True(t, s.IsMouseButtonJustReleased(platform.MouseButtonLeft))

	// Advance frame — no longer "just released".
	s.Update()
	require.False(t, s.IsMouseButtonJustReleased(platform.MouseButtonLeft))
}

func TestMouseButtonJustReleasedOutOfBounds(t *testing.T) {
	s := New()
	require.False(t, s.IsMouseButtonJustReleased(-1))
	require.False(t, s.IsMouseButtonJustReleased(100))
}

func TestMouseButtonRepeatNoOp(t *testing.T) {
	s := New()
	s.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: platform.MouseButtonMiddle,
		Action: platform.ActionRepeat,
	})
	require.False(t, s.IsMouseButtonPressed(platform.MouseButtonMiddle))
}

func TestMouseButtonOutOfBounds(t *testing.T) {
	s := New()
	s.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: -1,
		Action: platform.ActionPress,
	})
	require.False(t, s.IsMouseButtonPressed(-1))

	s.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: 100,
		Action: platform.ActionPress,
	})
	require.False(t, s.IsMouseButtonPressed(100))
}

func TestMouseButtonJustPressedOutOfBounds(t *testing.T) {
	s := New()
	require.False(t, s.IsMouseButtonJustPressed(-1))
	require.False(t, s.IsMouseButtonJustPressed(100))
}

// --- Mouse position and delta ---

func TestMousePosition(t *testing.T) {
	s := New()

	x, y := s.MousePosition()
	require.InDelta(t, 0.0, x, 1e-9)
	require.InDelta(t, 0.0, y, 1e-9)

	s.OnMouseMoveEvent(platform.MouseMoveEvent{X: 100, Y: 200, DX: 5, DY: 10})
	x, y = s.MousePosition()
	require.InDelta(t, 100.0, x, 1e-9)
	require.InDelta(t, 200.0, y, 1e-9)
}

func TestMouseDelta(t *testing.T) {
	s := New()

	s.OnMouseMoveEvent(platform.MouseMoveEvent{X: 10, Y: 20, DX: 3, DY: 4})
	s.OnMouseMoveEvent(platform.MouseMoveEvent{X: 15, Y: 25, DX: 5, DY: 5})

	// Deltas accumulate within a frame.
	dx, dy := s.MouseDelta()
	require.InDelta(t, 8.0, dx, 1e-9)
	require.InDelta(t, 9.0, dy, 1e-9)

	// Update resets deltas.
	s.Update()
	dx, dy = s.MouseDelta()
	require.InDelta(t, 0.0, dx, 1e-9)
	require.InDelta(t, 0.0, dy, 1e-9)
}

func TestMouseButtonEventUpdatesPosition(t *testing.T) {
	s := New()
	s.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: platform.MouseButtonLeft,
		Action: platform.ActionPress,
		X:      42, Y: 84,
	})
	x, y := s.MousePosition()
	require.InDelta(t, 42.0, x, 1e-9)
	require.InDelta(t, 84.0, y, 1e-9)
}

// --- Scroll ---

func TestScrollDelta(t *testing.T) {
	s := New()

	s.OnMouseScrollEvent(platform.MouseScrollEvent{DX: 1, DY: 2})
	s.OnMouseScrollEvent(platform.MouseScrollEvent{DX: 0.5, DY: -1})

	dx, dy := s.ScrollDelta()
	require.InDelta(t, 1.5, dx, 1e-9)
	require.InDelta(t, 1.0, dy, 1e-9)

	s.Update()
	dx, dy = s.ScrollDelta()
	require.InDelta(t, 0.0, dx, 1e-9)
	require.InDelta(t, 0.0, dy, 1e-9)
}

// --- Touch ---

func TestTouchPressAndRelease(t *testing.T) {
	s := New()

	require.Empty(t, s.TouchIDs())

	s.OnTouchEvent(platform.TouchEvent{ID: 1, Action: platform.ActionPress, X: 10, Y: 20, Pressure: 0.5})
	s.OnTouchEvent(platform.TouchEvent{ID: 2, Action: platform.ActionPress, X: 30, Y: 40, Pressure: 1.0})

	ids := s.TouchIDs()
	require.Len(t, ids, 2)

	x, y, ok := s.TouchPosition(1)
	require.True(t, ok)
	require.InDelta(t, 10.0, x, 1e-9)
	require.InDelta(t, 20.0, y, 1e-9)

	// Release touch 1.
	s.OnTouchEvent(platform.TouchEvent{ID: 1, Action: platform.ActionRelease})
	ids = s.TouchIDs()
	require.Len(t, ids, 1)

	_, _, ok = s.TouchPosition(1)
	require.False(t, ok)
}

func TestTouchMove(t *testing.T) {
	s := New()

	s.OnTouchEvent(platform.TouchEvent{ID: 1, Action: platform.ActionPress, X: 10, Y: 20, Pressure: 0.5})
	// Move (default action, not press or release).
	s.OnTouchEvent(platform.TouchEvent{ID: 1, Action: platform.ActionRepeat, X: 50, Y: 60, Pressure: 0.8})

	x, y, ok := s.TouchPosition(1)
	require.True(t, ok)
	require.InDelta(t, 50.0, x, 1e-9)
	require.InDelta(t, 60.0, y, 1e-9)
}

func TestTouchPositionNotFound(t *testing.T) {
	s := New()
	x, y, ok := s.TouchPosition(99)
	require.False(t, ok)
	require.InDelta(t, 0.0, x, 1e-9)
	require.InDelta(t, 0.0, y, 1e-9)
}

func TestTouchPressure(t *testing.T) {
	s := New()

	// No touch — returns 0.
	require.InDelta(t, 0.0, s.TouchPressure(1), 1e-9)

	// Press with pressure 0.5.
	s.OnTouchEvent(platform.TouchEvent{ID: 1, Action: platform.ActionPress, X: 10, Y: 20, Pressure: 0.5})
	require.InDelta(t, 0.5, s.TouchPressure(1), 1e-9)

	// Move with updated pressure.
	s.OnTouchEvent(platform.TouchEvent{ID: 1, Action: platform.ActionRepeat, X: 10, Y: 20, Pressure: 0.8})
	require.InDelta(t, 0.8, s.TouchPressure(1), 1e-9)

	// Release — returns 0.
	s.OnTouchEvent(platform.TouchEvent{ID: 1, Action: platform.ActionRelease})
	require.InDelta(t, 0.0, s.TouchPressure(1), 1e-9)
}

// --- Gamepad ---

func TestGamepadEvent(t *testing.T) {
	s := New()

	require.Empty(t, s.GamepadIDs())

	axes := [6]float64{0.5, -0.3, 0, 0, 0, 0}
	buttons := [16]bool{true, false, true}
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Axes: axes, Buttons: buttons})

	ids := s.GamepadIDs()
	require.Len(t, ids, 1)
	require.Equal(t, 0, ids[0])

	require.InDelta(t, 0.5, s.GamepadAxis(0, 0), 1e-9)
	require.InDelta(t, -0.3, s.GamepadAxis(0, 1), 1e-9)
	require.True(t, s.GamepadButton(0, 0))
	require.False(t, s.GamepadButton(0, 1))
	require.True(t, s.GamepadButton(0, 2))
}

func TestGamepadAxisOutOfBounds(t *testing.T) {
	s := New()
	require.InDelta(t, 0.0, s.GamepadAxis(99, 0), 1e-9)

	s.OnGamepadEvent(platform.GamepadEvent{ID: 0})
	require.InDelta(t, 0.0, s.GamepadAxis(0, -1), 1e-9)
	require.InDelta(t, 0.0, s.GamepadAxis(0, 100), 1e-9)
}

func TestGamepadButtonOutOfBounds(t *testing.T) {
	s := New()
	require.False(t, s.GamepadButton(99, 0))

	s.OnGamepadEvent(platform.GamepadEvent{ID: 0})
	require.False(t, s.GamepadButton(0, -1))
	require.False(t, s.GamepadButton(0, 100))
}

func TestGamepadDisconnect(t *testing.T) {
	s := New()

	// Connect gamepad.
	axes := [6]float64{0.5, -0.3, 0, 0, 0, 0}
	buttons := [16]bool{true}
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Axes: axes, Buttons: buttons})
	require.Len(t, s.GamepadIDs(), 1)

	// Disconnect via event.
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Disconnected: true})
	require.Empty(t, s.GamepadIDs())
	require.InDelta(t, 0.0, s.GamepadAxis(0, 0), 1e-9)
	require.False(t, s.GamepadButton(0, 0))
}

func TestRemoveGamepad(t *testing.T) {
	s := New()

	s.OnGamepadEvent(platform.GamepadEvent{ID: 1, Axes: [6]float64{1.0}})
	require.Len(t, s.GamepadIDs(), 1)

	s.RemoveGamepad(1)
	require.Empty(t, s.GamepadIDs())

	// Removing a non-existent gamepad is safe.
	s.RemoveGamepad(99)
}

// --- Character input ---

func TestOnCharEvent(t *testing.T) {
	s := New()

	require.Nil(t, s.InputChars())

	s.OnCharEvent('A')
	s.OnCharEvent('B')
	s.OnCharEvent('é')

	chars := s.InputChars()
	require.Equal(t, []rune{'A', 'B', 'é'}, chars)

	// InputChars returns a copy; calling again returns same data.
	chars2 := s.InputChars()
	require.Equal(t, chars, chars2)
}

func TestCharBufferClearedOnUpdate(t *testing.T) {
	s := New()

	s.OnCharEvent('X')
	require.Len(t, s.InputChars(), 1)

	s.Update()
	require.Nil(t, s.InputChars())
}

// --- OnResizeEvent ---

func TestOnResizeEventNoOp(t *testing.T) {
	s := New()
	// Should not panic.
	s.OnResizeEvent(800, 600)
}

// --- Press durations ---

func TestKeyPressDurationCountsTicksHeld(t *testing.T) {
	s := New()

	// Not held: 0.
	require.Equal(t, 0, s.KeyPressDuration(platform.KeyA))

	// Press, then advance: the first tick after a press registers
	// duration 1 (one full tick of being held).
	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyA, Action: platform.ActionPress})
	s.Update()
	require.Equal(t, 1, s.KeyPressDuration(platform.KeyA))

	// Two more ticks while still held.
	s.Update()
	s.Update()
	require.Equal(t, 3, s.KeyPressDuration(platform.KeyA))

	// Release: the next Update resets the counter. The release event
	// itself already cleared s.keys[A], so Update() sees it as not-held.
	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyA, Action: platform.ActionRelease})
	s.Update()
	require.Equal(t, 0, s.KeyPressDuration(platform.KeyA))
}

func TestKeyPressDurationOutOfBoundsReturnsZero(t *testing.T) {
	s := New()
	require.Equal(t, 0, s.KeyPressDuration(platform.Key(-1)))
	require.Equal(t, 0, s.KeyPressDuration(platform.Key(platform.KeyCount+10)))
}

func TestMouseButtonPressDurationCountsTicksHeld(t *testing.T) {
	s := New()

	require.Equal(t, 0, s.MouseButtonPressDuration(platform.MouseButtonLeft))

	s.OnMouseButtonEvent(platform.MouseButtonEvent{Button: platform.MouseButtonLeft, Action: platform.ActionPress})
	s.Update()
	require.Equal(t, 1, s.MouseButtonPressDuration(platform.MouseButtonLeft))

	s.Update()
	require.Equal(t, 2, s.MouseButtonPressDuration(platform.MouseButtonLeft))

	s.OnMouseButtonEvent(platform.MouseButtonEvent{Button: platform.MouseButtonLeft, Action: platform.ActionRelease})
	s.Update()
	require.Equal(t, 0, s.MouseButtonPressDuration(platform.MouseButtonLeft))
}

func TestMouseButtonPressDurationOutOfBoundsReturnsZero(t *testing.T) {
	s := New()
	require.Equal(t, 0, s.MouseButtonPressDuration(platform.MouseButton(-1)))
	require.Equal(t, 0, s.MouseButtonPressDuration(platform.MouseButton(99)))
}

// --- Key append helpers ---

func TestAppendPressedKeys(t *testing.T) {
	s := New()
	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyA, Action: platform.ActionPress})
	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyZ, Action: platform.ActionPress})

	keys := s.AppendPressedKeys(nil)
	require.ElementsMatch(t, []platform.Key{platform.KeyA, platform.KeyZ}, keys)

	// Existing slice is preserved and appended to.
	keys = s.AppendPressedKeys([]platform.Key{platform.KeySpace})
	require.Contains(t, keys, platform.KeySpace)
	require.Contains(t, keys, platform.KeyA)
	require.Contains(t, keys, platform.KeyZ)
}

func TestAppendJustPressedKeysOnlyFiresOnEdge(t *testing.T) {
	s := New()

	// Press two keys and verify JustPressed fires.
	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyA, Action: platform.ActionPress})
	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyB, Action: platform.ActionPress})
	require.ElementsMatch(t,
		[]platform.Key{platform.KeyA, platform.KeyB},
		s.AppendJustPressedKeys(nil))

	// After Update, holding the same keys does NOT re-fire JustPressed.
	s.Update()
	require.Empty(t, s.AppendJustPressedKeys(nil))

	// Pressing a new key fires JustPressed only for the new one.
	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyC, Action: platform.ActionPress})
	require.ElementsMatch(t,
		[]platform.Key{platform.KeyC},
		s.AppendJustPressedKeys(nil))
}

func TestAppendJustReleasedKeysOnlyFiresOnEdge(t *testing.T) {
	s := New()

	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyA, Action: platform.ActionPress})
	s.Update()

	// Nothing released yet.
	require.Empty(t, s.AppendJustReleasedKeys(nil))

	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyA, Action: platform.ActionRelease})
	require.ElementsMatch(t,
		[]platform.Key{platform.KeyA},
		s.AppendJustReleasedKeys(nil))

	// After Update the release edge is gone.
	s.Update()
	require.Empty(t, s.AppendJustReleasedKeys(nil))
}

// --- Touch edge triggers ---

func TestAppendJustPressedTouchIDsOnlyFiresOnBegan(t *testing.T) {
	s := New()

	s.OnTouchEvent(platform.TouchEvent{ID: 1, X: 10, Y: 20, Action: platform.ActionPress})
	s.OnTouchEvent(platform.TouchEvent{ID: 2, X: 30, Y: 40, Action: platform.ActionPress})
	require.ElementsMatch(t, []int{1, 2}, s.AppendJustPressedTouchIDs(nil))

	// After Update, still-held touches no longer count as just-pressed.
	s.Update()
	require.Empty(t, s.AppendJustPressedTouchIDs(nil))

	// A new touch fires; the still-held ones do not.
	s.OnTouchEvent(platform.TouchEvent{ID: 3, X: 50, Y: 60, Action: platform.ActionPress})
	require.ElementsMatch(t, []int{3}, s.AppendJustPressedTouchIDs(nil))
}

func TestAppendJustReleasedTouchIDsOnlyFiresOnEnded(t *testing.T) {
	s := New()

	s.OnTouchEvent(platform.TouchEvent{ID: 1, X: 10, Y: 20, Action: platform.ActionPress})
	s.Update()

	require.Empty(t, s.AppendJustReleasedTouchIDs(nil))

	s.OnTouchEvent(platform.TouchEvent{ID: 1, Action: platform.ActionRelease})
	require.ElementsMatch(t, []int{1}, s.AppendJustReleasedTouchIDs(nil))

	s.Update()
	require.Empty(t, s.AppendJustReleasedTouchIDs(nil))
}

func TestTouchEdgeIgnoresMoveOnlyUpdates(t *testing.T) {
	s := New()

	s.OnTouchEvent(platform.TouchEvent{ID: 1, X: 10, Y: 20, Action: platform.ActionPress})
	s.Update()
	// A move-only event (no press/release) must not look like a new press.
	s.OnTouchEvent(platform.TouchEvent{ID: 1, X: 15, Y: 25})
	require.Empty(t, s.AppendJustPressedTouchIDs(nil))
	require.Empty(t, s.AppendJustReleasedTouchIDs(nil))
}

// --- Gamepad edge triggers + duration + capability ---

func TestIsGamepadButtonJustPressedOnlyFiresOnEdge(t *testing.T) {
	s := New()

	buttons := [16]bool{}
	buttons[0] = true
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	require.True(t, s.IsGamepadButtonJustPressed(0, 0))

	// After Update, holding the same button no longer counts as just-pressed.
	s.Update()
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	require.False(t, s.IsGamepadButtonJustPressed(0, 0))

	// Releasing then pressing again re-fires.
	buttons[0] = false
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	s.Update()
	buttons[0] = true
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	require.True(t, s.IsGamepadButtonJustPressed(0, 0))
}

func TestIsGamepadButtonJustReleasedOnlyFiresOnEdge(t *testing.T) {
	s := New()

	buttons := [16]bool{}
	buttons[1] = true
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	s.Update()

	require.False(t, s.IsGamepadButtonJustReleased(0, 1))

	buttons[1] = false
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	require.True(t, s.IsGamepadButtonJustReleased(0, 1))

	s.Update()
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	require.False(t, s.IsGamepadButtonJustReleased(0, 1))
}

func TestGamepadButtonEdgeOutOfRangeReturnsFalse(t *testing.T) {
	s := New()
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0})
	require.False(t, s.IsGamepadButtonJustPressed(0, -1))
	require.False(t, s.IsGamepadButtonJustPressed(0, 99))
	require.False(t, s.IsGamepadButtonJustReleased(0, -1))
	require.False(t, s.IsGamepadButtonJustReleased(0, 99))
}

func TestGamepadButtonEdgeOnUnknownGamepad(t *testing.T) {
	s := New()
	// Neither prev nor current has this gamepad — no edges possible.
	require.False(t, s.IsGamepadButtonJustPressed(42, 0))
	require.False(t, s.IsGamepadButtonJustReleased(42, 0))
}

func TestGamepadButtonPressDurationCountsTicksHeld(t *testing.T) {
	s := New()

	require.Equal(t, 0, s.GamepadButtonPressDuration(0, 0))

	buttons := [16]bool{}
	buttons[0] = true
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	s.Update()
	require.Equal(t, 1, s.GamepadButtonPressDuration(0, 0))

	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	s.Update()
	require.Equal(t, 2, s.GamepadButtonPressDuration(0, 0))

	buttons[0] = false
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	s.Update()
	require.Equal(t, 0, s.GamepadButtonPressDuration(0, 0))
}

func TestGamepadButtonPressDurationOutOfRange(t *testing.T) {
	s := New()
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0})
	require.Equal(t, 0, s.GamepadButtonPressDuration(0, -1))
	require.Equal(t, 0, s.GamepadButtonPressDuration(0, 99))
	require.Equal(t, 0, s.GamepadButtonPressDuration(999, 0))
}

func TestGamepadButtonDurationCleanedUpOnDisconnect(t *testing.T) {
	s := New()

	buttons := [16]bool{}
	buttons[0] = true
	s.OnGamepadEvent(platform.GamepadEvent{ID: 7, Buttons: buttons})
	s.Update()
	require.Equal(t, 1, s.GamepadButtonPressDuration(7, 0))

	// Disconnect event.
	s.OnGamepadEvent(platform.GamepadEvent{ID: 7, Disconnected: true})
	s.Update()
	require.Equal(t, 0, s.GamepadButtonPressDuration(7, 0))
}

func TestGamepadButtonCountAndAxisCount(t *testing.T) {
	s := New()

	// Unknown gamepad → 0.
	require.Equal(t, 0, s.GamepadButtonCount(0))
	require.Equal(t, 0, s.GamepadAxisCount(0))

	s.OnGamepadEvent(platform.GamepadEvent{ID: 0})
	require.Equal(t, 16, s.GamepadButtonCount(0))
	require.Equal(t, 6, s.GamepadAxisCount(0))
}

// --- Update frame advance ---

func TestUpdateResetsDeltasAndCopiesState(t *testing.T) {
	s := New()

	// Press a key.
	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyA, Action: platform.ActionPress})
	require.True(t, s.IsKeyJustPressed(platform.KeyA))

	// Add mouse delta and scroll.
	s.OnMouseMoveEvent(platform.MouseMoveEvent{DX: 5, DY: 10})
	s.OnMouseScrollEvent(platform.MouseScrollEvent{DX: 1, DY: 2})

	// Advance frame.
	s.Update()

	// Key still pressed but not "just pressed".
	require.True(t, s.IsKeyPressed(platform.KeyA))
	require.False(t, s.IsKeyJustPressed(platform.KeyA))

	// Deltas reset.
	dx, dy := s.MouseDelta()
	require.InDelta(t, 0.0, dx, 1e-9)
	require.InDelta(t, 0.0, dy, 1e-9)

	sx, sy := s.ScrollDelta()
	require.InDelta(t, 0.0, sx, 1e-9)
	require.InDelta(t, 0.0, sy, 1e-9)
}
