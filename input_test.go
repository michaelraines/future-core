package futurerender

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/input"
	"github.com/michaelraines/future-core/internal/platform"
)

// withNilEngine is defined in engine_test.go.

// withInputEngine sets up a globalEngine with a real input.State for testing.
func withInputEngine(t *testing.T) *input.State {
	t.Helper()
	old := getEngine()
	s := input.New()
	setEngine(&engine{inputState: s})
	t.Cleanup(func() { setEngine(old) })
	return s
}

// --- Nil engine tests (stubs return defaults) ---

func TestIsKeyPressedNilEngine(t *testing.T) {
	withNilEngine(t)
	require.False(t, IsKeyPressed(KeyA))
	require.False(t, IsKeyPressed(KeySpace))
	require.False(t, IsKeyPressed(KeyEnter))
}

func TestIsKeyJustPressedNilEngine(t *testing.T) {
	withNilEngine(t)
	require.False(t, IsKeyJustPressed(KeyA))
}

func TestIsKeyJustReleasedNilEngine(t *testing.T) {
	withNilEngine(t)
	require.False(t, IsKeyJustReleased(KeyA))
}

func TestInputCharsNilEngine(t *testing.T) {
	withNilEngine(t)
	require.Nil(t, InputChars())
}

func TestIsMouseButtonPressedNilEngine(t *testing.T) {
	withNilEngine(t)
	require.False(t, IsMouseButtonPressed(MouseButtonLeft))
	require.False(t, IsMouseButtonPressed(MouseButtonRight))
	require.False(t, IsMouseButtonPressed(MouseButtonMiddle))
}

func TestCursorPositionNilEngine(t *testing.T) {
	withNilEngine(t)
	x, y := CursorPosition()
	require.Equal(t, 0, x)
	require.Equal(t, 0, y)
}

func TestWheelNilEngine(t *testing.T) {
	withNilEngine(t)
	xoff, yoff := Wheel()
	require.InDelta(t, 0.0, xoff, 1e-6)
	require.InDelta(t, 0.0, yoff, 1e-6)
}

func TestTouchIDsNilEngine(t *testing.T) {
	withNilEngine(t)
	require.Nil(t, TouchIDs())
}

func TestTouchPositionNilEngine(t *testing.T) {
	withNilEngine(t)
	x, y := TouchPosition(TouchID(0))
	require.Equal(t, 0, x)
	require.Equal(t, 0, y)
}

func TestTouchPressureNilEngine(t *testing.T) {
	withNilEngine(t)
	require.InDelta(t, 0.0, TouchPressure(TouchID(0)), 1e-6)
}

func TestGamepadIDsNilEngine(t *testing.T) {
	withNilEngine(t)
	require.Nil(t, GamepadIDs())
}

func TestGamepadAxisValueNilEngine(t *testing.T) {
	withNilEngine(t)
	val := GamepadAxisValue(GamepadID(0), 0)
	require.InDelta(t, 0.0, val, 1e-6)
}

func TestIsGamepadButtonPressedNilEngine(t *testing.T) {
	withNilEngine(t)
	require.False(t, IsGamepadButtonPressed(GamepadID(0), GamepadButton(0)))
}

// --- Key and mouse button constants ---

func TestKeyConstants(t *testing.T) {
	require.NotEqual(t, KeyA, KeyB)
	require.NotEqual(t, KeySpace, KeyEnter)
	require.NotEqual(t, KeyLeft, KeyRight)
}

func TestMouseButtonConstants(t *testing.T) {
	require.Equal(t, MouseButton(0), MouseButtonLeft)
	require.Equal(t, MouseButton(1), MouseButtonRight)
	require.Equal(t, MouseButton(2), MouseButtonMiddle)
}

// --- Wired input tests (engine + input.State present) ---

func TestIsKeyPressedWired(t *testing.T) {
	s := withInputEngine(t)

	require.False(t, IsKeyPressed(KeyA))

	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyA, Action: platform.ActionPress})
	require.True(t, IsKeyPressed(KeyA))

	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyA, Action: platform.ActionRelease})
	require.False(t, IsKeyPressed(KeyA))
}

func TestIsKeyJustPressedWired(t *testing.T) {
	s := withInputEngine(t)

	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeySpace, Action: platform.ActionPress})
	require.True(t, IsKeyJustPressed(KeySpace))

	s.Update()
	require.False(t, IsKeyJustPressed(KeySpace))
	require.True(t, IsKeyPressed(KeySpace))
}

func TestIsKeyJustReleasedWired(t *testing.T) {
	s := withInputEngine(t)

	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyEscape, Action: platform.ActionPress})
	s.Update()
	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyEscape, Action: platform.ActionRelease})
	require.True(t, IsKeyJustReleased(KeyEscape))

	s.Update()
	require.False(t, IsKeyJustReleased(KeyEscape))
}

func TestIsMouseButtonPressedWired(t *testing.T) {
	s := withInputEngine(t)

	require.False(t, IsMouseButtonPressed(MouseButtonLeft))

	s.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: platform.MouseButtonLeft,
		Action: platform.ActionPress,
	})
	require.True(t, IsMouseButtonPressed(MouseButtonLeft))
}

func TestCursorPositionWired(t *testing.T) {
	s := withInputEngine(t)

	s.OnMouseMoveEvent(platform.MouseMoveEvent{X: 150.7, Y: 200.3})
	x, y := CursorPosition()
	require.Equal(t, 150, x)
	require.Equal(t, 200, y)
}

func TestWheelWired(t *testing.T) {
	s := withInputEngine(t)

	s.OnMouseScrollEvent(platform.MouseScrollEvent{DX: 0.5, DY: -1.5})
	xoff, yoff := Wheel()
	require.InDelta(t, 0.5, xoff, 1e-6)
	require.InDelta(t, -1.5, yoff, 1e-6)
}

func TestTouchIDsWired(t *testing.T) {
	s := withInputEngine(t)

	s.OnTouchEvent(platform.TouchEvent{ID: 1, Action: platform.ActionPress, X: 10, Y: 20})
	ids := TouchIDs()
	require.Len(t, ids, 1)
	require.Equal(t, TouchID(1), ids[0])
}

func TestTouchPositionWired(t *testing.T) {
	s := withInputEngine(t)

	s.OnTouchEvent(platform.TouchEvent{ID: 5, Action: platform.ActionPress, X: 42.8, Y: 99.1})
	x, y := TouchPosition(TouchID(5))
	require.Equal(t, 42, x)
	require.Equal(t, 99, y)

	// Unknown touch.
	x, y = TouchPosition(TouchID(99))
	require.Equal(t, 0, x)
	require.Equal(t, 0, y)
}

func TestTouchPressureWired(t *testing.T) {
	s := withInputEngine(t)

	// No touch — returns 0.
	require.InDelta(t, 0.0, TouchPressure(TouchID(99)), 1e-6)

	// Press with pressure.
	s.OnTouchEvent(platform.TouchEvent{ID: 1, Action: platform.ActionPress, X: 10, Y: 20, Pressure: 0.7})
	require.InDelta(t, 0.7, TouchPressure(TouchID(1)), 1e-6)

	// Release — returns 0.
	s.OnTouchEvent(platform.TouchEvent{ID: 1, Action: platform.ActionRelease})
	require.InDelta(t, 0.0, TouchPressure(TouchID(1)), 1e-6)
}

func TestGamepadIDsWired(t *testing.T) {
	s := withInputEngine(t)

	s.OnGamepadEvent(platform.GamepadEvent{ID: 0})
	ids := GamepadIDs()
	require.Len(t, ids, 1)
	require.Equal(t, GamepadID(0), ids[0])
}

func TestGamepadAxisValueWired(t *testing.T) {
	s := withInputEngine(t)

	axes := [6]float64{0.75, -0.5}
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Axes: axes})
	require.InDelta(t, 0.75, GamepadAxisValue(GamepadID(0), 0), 1e-6)
	require.InDelta(t, -0.5, GamepadAxisValue(GamepadID(0), 1), 1e-6)
}

func TestIsGamepadButtonPressedWired(t *testing.T) {
	s := withInputEngine(t)

	buttons := [16]bool{true, false, true}
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	require.True(t, IsGamepadButtonPressed(GamepadID(0), GamepadButton(0)))
	require.False(t, IsGamepadButtonPressed(GamepadID(0), GamepadButton(1)))
	require.True(t, IsGamepadButtonPressed(GamepadID(0), GamepadButton(2)))
}

// --- InputChars wired ---

func TestInputCharsWired(t *testing.T) {
	s := withInputEngine(t)

	require.Nil(t, InputChars())

	s.OnCharEvent('H')
	s.OnCharEvent('i')
	chars := InputChars()
	require.Equal(t, []rune{'H', 'i'}, chars)

	s.Update()
	require.Nil(t, InputChars())
}

// --- Key mapping correctness ---

func TestKeyMapping(t *testing.T) {
	tests := []struct {
		pub  Key
		plat platform.Key
	}{
		{KeyA, platform.KeyA},
		{KeyZ, platform.KeyZ},
		{Key0, platform.Key0},
		{Key9, platform.Key9},
		{KeySpace, platform.KeySpace},
		{KeyEnter, platform.KeyEnter},
		{KeyEscape, platform.KeyEscape},
		{KeyTab, platform.KeyTab},
		{KeyBackspace, platform.KeyBackspace},
		{KeyUp, platform.KeyUp},
		{KeyDown, platform.KeyDown},
		{KeyLeft, platform.KeyLeft},
		{KeyRight, platform.KeyRight},
		{KeyF1, platform.KeyF1},
		{KeyF12, platform.KeyF12},
		{KeyLeftShift, platform.KeyLeftShift},
		{KeyLeftControl, platform.KeyLeftControl},
		{KeyLeftAlt, platform.KeyLeftAlt},
		{KeyRightShift, platform.KeyRightShift},
		{KeyRightControl, platform.KeyRightControl},
		{KeyRightAlt, platform.KeyRightAlt},
		{KeyInsert, platform.KeyInsert},
		{KeyDelete, platform.KeyDelete},
		{KeyHome, platform.KeyHome},
		{KeyEnd, platform.KeyEnd},
		{KeyPageUp, platform.KeyPageUp},
		{KeyPageDown, platform.KeyPageDown},
	}
	for _, tt := range tests {
		require.Equal(t, tt.plat, keyToInternal(tt.pub), "key %d", tt.pub)
	}
}

func TestKeyF13ThroughF24(t *testing.T) {
	tests := []struct {
		pub  Key
		plat platform.Key
	}{
		{KeyF13, platform.KeyF13},
		{KeyF14, platform.KeyF14},
		{KeyF15, platform.KeyF15},
		{KeyF16, platform.KeyF16},
		{KeyF17, platform.KeyF17},
		{KeyF18, platform.KeyF18},
		{KeyF19, platform.KeyF19},
		{KeyF20, platform.KeyF20},
		{KeyF21, platform.KeyF21},
		{KeyF22, platform.KeyF22},
		{KeyF23, platform.KeyF23},
		{KeyF24, platform.KeyF24},
	}
	for _, tt := range tests {
		require.Equal(t, tt.plat, keyToInternal(tt.pub), "key %d", tt.pub)
	}
}

func TestEbitenKeyAliases(t *testing.T) {
	// Arrow keys
	require.Equal(t, KeyDown, KeyArrowDown)
	require.Equal(t, KeyLeft, KeyArrowLeft)
	require.Equal(t, KeyRight, KeyArrowRight)
	require.Equal(t, KeyUp, KeyArrowUp)

	// Punctuation aliases
	require.Equal(t, KeyGraveAccent, KeyBackquote)
	require.Equal(t, KeyLeftBracket, KeyBracketLeft)
	require.Equal(t, KeyRightBracket, KeyBracketRight)
	require.Equal(t, KeyApostrophe, KeyQuote)

	// Modifier aliases
	require.Equal(t, KeyLeftAlt, KeyAltLeft)
	require.Equal(t, KeyRightAlt, KeyAltRight)
	require.Equal(t, KeyLeftControl, KeyControlLeft)
	require.Equal(t, KeyRightControl, KeyControlRight)
	require.Equal(t, KeyLeftShift, KeyShiftLeft)
	require.Equal(t, KeyRightShift, KeyShiftRight)
	require.Equal(t, KeyLeftSuper, KeyMetaLeft)
	require.Equal(t, KeyRightSuper, KeyMetaRight)
	require.Equal(t, KeyMenu, KeyContextMenu)

	// Digit aliases
	require.Equal(t, Key0, KeyDigit0)
	require.Equal(t, Key9, KeyDigit9)

	// Numpad aliases
	require.Equal(t, KeyKP0, KeyNumpad0)
	require.Equal(t, KeyKP9, KeyNumpad9)
	require.Equal(t, KeyKPAdd, KeyNumpadAdd)
	require.Equal(t, KeyKPDecimal, KeyNumpadDecimal)
	require.Equal(t, KeyKPDivide, KeyNumpadDivide)
	require.Equal(t, KeyKPEnter, KeyNumpadEnter)
	require.Equal(t, KeyKPEqual, KeyNumpadEqual)
	require.Equal(t, KeyKPMultiply, KeyNumpadMultiply)
	require.Equal(t, KeyKPSubtract, KeyNumpadSubtract)
}

func TestKeyString(t *testing.T) {
	tests := []struct {
		key  Key
		want string
	}{
		{KeyA, "A"},
		{KeyZ, "Z"},
		{Key0, "0"},
		{Key9, "9"},
		{KeySpace, "Space"},
		{KeyEnter, "Enter"},
		{KeyEscape, "Escape"},
		{KeyF1, "F1"},
		{KeyF12, "F12"},
		{KeyLeftShift, "LeftShift"},
		{KeyMenu, "Menu"},
		{KeyKP0, "KP0"},
		{KeyKPAdd, "KPAdd"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require.Equal(t, tt.want, tt.key.String())
		})
	}
}

func TestKeyStringUnknown(t *testing.T) {
	require.Equal(t, "Unknown(-1)", Key(-1).String())
	require.Equal(t, "Unknown(9999)", Key(9999).String())
}

func TestKeyToInternalOutOfBounds(t *testing.T) {
	require.Equal(t, platform.KeyUnknown, keyToInternal(Key(-1)))
	require.Equal(t, platform.KeyUnknown, keyToInternal(Key(9999)))
}

// --- Empty collections return nil ---

// --- New public API surface: durations + edge-triggered gamepad +
// edge-triggered touch. Each has a nil-engine case and a wired case.

func TestKeyPressDurationNilEngine(t *testing.T) {
	withNilEngine(t)
	require.Equal(t, 0, KeyPressDuration(KeyA))
}

func TestKeyPressDurationWired(t *testing.T) {
	s := withInputEngine(t)

	s.OnKeyEvent(platform.KeyEvent{Key: platform.KeyA, Action: platform.ActionPress})
	s.Update()
	require.Equal(t, 1, KeyPressDuration(KeyA))

	s.Update()
	require.Equal(t, 2, KeyPressDuration(KeyA))
}

func TestMouseButtonPressDurationNilEngine(t *testing.T) {
	withNilEngine(t)
	require.Equal(t, 0, MouseButtonPressDuration(MouseButtonLeft))
}

func TestMouseButtonPressDurationWired(t *testing.T) {
	s := withInputEngine(t)

	s.OnMouseButtonEvent(platform.MouseButtonEvent{Button: platform.MouseButtonLeft, Action: platform.ActionPress})
	s.Update()
	require.Equal(t, 1, MouseButtonPressDuration(MouseButtonLeft))
}

func TestIsGamepadButtonJustPressedNilEngine(t *testing.T) {
	withNilEngine(t)
	require.False(t, IsGamepadButtonJustPressed(0, 0))
}

func TestIsGamepadButtonJustPressedWired(t *testing.T) {
	s := withInputEngine(t)

	buttons := [16]bool{}
	buttons[0] = true
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	require.True(t, IsGamepadButtonJustPressed(GamepadID(0), GamepadButton(0)))

	s.Update()
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	require.False(t, IsGamepadButtonJustPressed(GamepadID(0), GamepadButton(0)))
}

func TestIsGamepadButtonJustReleasedNilEngine(t *testing.T) {
	withNilEngine(t)
	require.False(t, IsGamepadButtonJustReleased(0, 0))
}

func TestIsGamepadButtonJustReleasedWired(t *testing.T) {
	s := withInputEngine(t)

	buttons := [16]bool{}
	buttons[0] = true
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	s.Update()
	buttons[0] = false
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	require.True(t, IsGamepadButtonJustReleased(GamepadID(0), GamepadButton(0)))
}

func TestGamepadButtonPressDurationNilEngine(t *testing.T) {
	withNilEngine(t)
	require.Equal(t, 0, GamepadButtonPressDuration(0, 0))
}

func TestGamepadButtonPressDurationWired(t *testing.T) {
	s := withInputEngine(t)

	buttons := [16]bool{}
	buttons[2] = true
	s.OnGamepadEvent(platform.GamepadEvent{ID: 0, Buttons: buttons})
	s.Update()
	require.Equal(t, 1, GamepadButtonPressDuration(GamepadID(0), GamepadButton(2)))
}

func TestGamepadButtonCountNilEngine(t *testing.T) {
	withNilEngine(t)
	require.Equal(t, 0, GamepadButtonCount(0))
}

func TestGamepadButtonCountWired(t *testing.T) {
	s := withInputEngine(t)

	// Unknown: 0.
	require.Equal(t, 0, GamepadButtonCount(GamepadID(0)))

	s.OnGamepadEvent(platform.GamepadEvent{ID: 0})
	require.Equal(t, 16, GamepadButtonCount(GamepadID(0)))
}

func TestGamepadAxisCountNilEngine(t *testing.T) {
	withNilEngine(t)
	require.Equal(t, 0, GamepadAxisCount(0))
}

func TestGamepadAxisCountWired(t *testing.T) {
	s := withInputEngine(t)

	require.Equal(t, 0, GamepadAxisCount(GamepadID(0)))

	s.OnGamepadEvent(platform.GamepadEvent{ID: 0})
	require.Equal(t, 6, GamepadAxisCount(GamepadID(0)))
}

func TestAppendJustPressedTouchIDsNilEngine(t *testing.T) {
	withNilEngine(t)
	require.Empty(t, AppendJustPressedTouchIDs(nil))
	// Non-nil slice is preserved even when no engine is attached.
	existing := []TouchID{99}
	require.Equal(t, existing, AppendJustPressedTouchIDs(existing))
}

func TestAppendJustPressedTouchIDsWired(t *testing.T) {
	s := withInputEngine(t)

	s.OnTouchEvent(platform.TouchEvent{ID: 1, X: 10, Y: 20, Action: platform.ActionPress})
	s.OnTouchEvent(platform.TouchEvent{ID: 2, X: 30, Y: 40, Action: platform.ActionPress})
	ids := AppendJustPressedTouchIDs(nil)
	require.ElementsMatch(t, []TouchID{1, 2}, ids)

	// After Update, nothing is just-pressed even though both touches are still held.
	s.Update()
	require.Empty(t, AppendJustPressedTouchIDs(nil))
}

func TestAppendJustReleasedTouchIDsNilEngine(t *testing.T) {
	withNilEngine(t)
	require.Empty(t, AppendJustReleasedTouchIDs(nil))

	existing := []TouchID{77}
	require.Equal(t, existing, AppendJustReleasedTouchIDs(existing))
}

func TestAppendJustReleasedTouchIDsWired(t *testing.T) {
	s := withInputEngine(t)

	s.OnTouchEvent(platform.TouchEvent{ID: 1, X: 10, Y: 20, Action: platform.ActionPress})
	s.Update()

	require.Empty(t, AppendJustReleasedTouchIDs(nil))

	s.OnTouchEvent(platform.TouchEvent{ID: 1, Action: platform.ActionRelease})
	require.ElementsMatch(t, []TouchID{1}, AppendJustReleasedTouchIDs(nil))

	s.Update()
	require.Empty(t, AppendJustReleasedTouchIDs(nil))
}

func TestTouchIDsEmptyReturnsNil(t *testing.T) {
	_ = withInputEngine(t)
	require.Nil(t, TouchIDs())
}

func TestGamepadIDsEmptyReturnsNil(t *testing.T) {
	_ = withInputEngine(t)
	require.Nil(t, GamepadIDs())
}
