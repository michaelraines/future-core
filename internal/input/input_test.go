package input

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/platform"
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
