package futurerender

// This file provides the public input API, wrapping the internal input system
// with user-friendly functions matching Ebitengine's API.

import (
	"github.com/michaelraines/future-render/internal/platform"
)

// IsKeyPressed returns whether the given key is currently pressed.
func IsKeyPressed(key Key) bool {
	e := getEngine()
	if e == nil || e.inputState == nil {
		return false
	}
	return e.inputState.IsKeyPressed(keyToInternal(key))
}

// IsKeyJustPressed returns whether the key was pressed this frame (edge detection).
func IsKeyJustPressed(key Key) bool {
	e := getEngine()
	if e == nil || e.inputState == nil {
		return false
	}
	return e.inputState.IsKeyJustPressed(keyToInternal(key))
}

// IsKeyJustReleased returns whether the key was released this frame (edge detection).
func IsKeyJustReleased(key Key) bool {
	e := getEngine()
	if e == nil || e.inputState == nil {
		return false
	}
	return e.inputState.IsKeyJustReleased(keyToInternal(key))
}

// InputChars returns the Unicode characters input since the last frame.
func InputChars() []rune {
	e := getEngine()
	if e == nil || e.inputState == nil {
		return nil
	}
	return e.inputState.InputChars()
}

// IsMouseButtonPressed returns whether the given mouse button is pressed.
func IsMouseButtonPressed(button MouseButton) bool {
	e := getEngine()
	if e == nil || e.inputState == nil {
		return false
	}
	return e.inputState.IsMouseButtonPressed(platform.MouseButton(button))
}

// CursorPosition returns the current cursor position in logical pixels.
func CursorPosition() (x, y int) {
	e := getEngine()
	if e == nil || e.inputState == nil {
		return 0, 0
	}
	fx, fy := e.inputState.MousePosition()
	return int(fx), int(fy)
}

// Wheel returns the mouse wheel delta since the last frame.
func Wheel() (xoff, yoff float64) {
	e := getEngine()
	if e == nil || e.inputState == nil {
		return 0, 0
	}
	return e.inputState.ScrollDelta()
}

// TouchIDs returns the IDs of all active touch points.
func TouchIDs() []TouchID {
	e := getEngine()
	if e == nil || e.inputState == nil {
		return nil
	}
	ids := e.inputState.TouchIDs()
	if len(ids) == 0 {
		return nil
	}
	result := make([]TouchID, len(ids))
	for i, id := range ids {
		result[i] = TouchID(id)
	}
	return result
}

// TouchPosition returns the position of a touch point.
func TouchPosition(id TouchID) (x, y int) {
	e := getEngine()
	if e == nil || e.inputState == nil {
		return 0, 0
	}
	fx, fy, ok := e.inputState.TouchPosition(int(id))
	if !ok {
		return 0, 0
	}
	return int(fx), int(fy)
}

// GamepadIDs returns the IDs of connected gamepads.
func GamepadIDs() []GamepadID {
	e := getEngine()
	if e == nil || e.inputState == nil {
		return nil
	}
	ids := e.inputState.GamepadIDs()
	if len(ids) == 0 {
		return nil
	}
	result := make([]GamepadID, len(ids))
	for i, id := range ids {
		result[i] = GamepadID(id)
	}
	return result
}

// GamepadAxisValue returns the value of a gamepad axis.
func GamepadAxisValue(id GamepadID, axis int) float64 {
	e := getEngine()
	if e == nil || e.inputState == nil {
		return 0
	}
	return e.inputState.GamepadAxis(int(id), axis)
}

// IsGamepadButtonPressed returns whether a gamepad button is pressed.
func IsGamepadButtonPressed(id GamepadID, button GamepadButton) bool {
	e := getEngine()
	if e == nil || e.inputState == nil {
		return false
	}
	return e.inputState.GamepadButton(int(id), int(button))
}

// Key represents a keyboard key.
type Key int

// Key constants matching Ebitengine's key names.
const (
	KeyA Key = iota
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ
	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9
	KeySpace
	KeyApostrophe
	KeyComma
	KeyMinus
	KeyPeriod
	KeySlash
	KeySemicolon
	KeyEqual
	KeyLeftBracket
	KeyBackslash
	KeyRightBracket
	KeyGraveAccent
	KeyEnter
	KeyEscape
	KeyTab
	KeyBackspace
	KeyInsert
	KeyDelete
	KeyRight
	KeyLeft
	KeyDown
	KeyUp
	KeyPageUp
	KeyPageDown
	KeyHome
	KeyEnd
	KeyCapsLock
	KeyScrollLock
	KeyNumLock
	KeyPrintScreen
	KeyPause
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
	KeyKP0
	KeyKP1
	KeyKP2
	KeyKP3
	KeyKP4
	KeyKP5
	KeyKP6
	KeyKP7
	KeyKP8
	KeyKP9
	KeyKPDecimal
	KeyKPDivide
	KeyKPMultiply
	KeyKPSubtract
	KeyKPAdd
	KeyKPEnter
	KeyKPEqual
	KeyLeftShift
	KeyLeftControl
	KeyLeftAlt
	KeyLeftSuper
	KeyRightShift
	KeyRightControl
	KeyRightAlt
	KeyRightSuper
	KeyMenu
	keyCount // sentinel — not exported
)

// keyMap maps public Key values to platform.Key values.
var keyMap [keyCount]platform.Key

func init() {
	// Default all to KeyUnknown.
	for i := range keyMap {
		keyMap[i] = platform.KeyUnknown
	}

	keyMap[KeyA] = platform.KeyA
	keyMap[KeyB] = platform.KeyB
	keyMap[KeyC] = platform.KeyC
	keyMap[KeyD] = platform.KeyD
	keyMap[KeyE] = platform.KeyE
	keyMap[KeyF] = platform.KeyF
	keyMap[KeyG] = platform.KeyG
	keyMap[KeyH] = platform.KeyH
	keyMap[KeyI] = platform.KeyI
	keyMap[KeyJ] = platform.KeyJ
	keyMap[KeyK] = platform.KeyK
	keyMap[KeyL] = platform.KeyL
	keyMap[KeyM] = platform.KeyM
	keyMap[KeyN] = platform.KeyN
	keyMap[KeyO] = platform.KeyO
	keyMap[KeyP] = platform.KeyP
	keyMap[KeyQ] = platform.KeyQ
	keyMap[KeyR] = platform.KeyR
	keyMap[KeyS] = platform.KeyS
	keyMap[KeyT] = platform.KeyT
	keyMap[KeyU] = platform.KeyU
	keyMap[KeyV] = platform.KeyV
	keyMap[KeyW] = platform.KeyW
	keyMap[KeyX] = platform.KeyX
	keyMap[KeyY] = platform.KeyY
	keyMap[KeyZ] = platform.KeyZ
	keyMap[Key0] = platform.Key0
	keyMap[Key1] = platform.Key1
	keyMap[Key2] = platform.Key2
	keyMap[Key3] = platform.Key3
	keyMap[Key4] = platform.Key4
	keyMap[Key5] = platform.Key5
	keyMap[Key6] = platform.Key6
	keyMap[Key7] = platform.Key7
	keyMap[Key8] = platform.Key8
	keyMap[Key9] = platform.Key9
	keyMap[KeySpace] = platform.KeySpace
	keyMap[KeyApostrophe] = platform.KeyApostrophe
	keyMap[KeyComma] = platform.KeyComma
	keyMap[KeyMinus] = platform.KeyMinus
	keyMap[KeyPeriod] = platform.KeyPeriod
	keyMap[KeySlash] = platform.KeySlash
	keyMap[KeySemicolon] = platform.KeySemicolon
	keyMap[KeyEqual] = platform.KeyEqual
	keyMap[KeyLeftBracket] = platform.KeyLeftBracket
	keyMap[KeyBackslash] = platform.KeyBackslash
	keyMap[KeyRightBracket] = platform.KeyRightBracket
	keyMap[KeyGraveAccent] = platform.KeyGraveAccent
	keyMap[KeyEnter] = platform.KeyEnter
	keyMap[KeyEscape] = platform.KeyEscape
	keyMap[KeyTab] = platform.KeyTab
	keyMap[KeyBackspace] = platform.KeyBackspace
	keyMap[KeyInsert] = platform.KeyInsert
	keyMap[KeyDelete] = platform.KeyDelete
	keyMap[KeyRight] = platform.KeyRight
	keyMap[KeyLeft] = platform.KeyLeft
	keyMap[KeyDown] = platform.KeyDown
	keyMap[KeyUp] = platform.KeyUp
	keyMap[KeyPageUp] = platform.KeyPageUp
	keyMap[KeyPageDown] = platform.KeyPageDown
	keyMap[KeyHome] = platform.KeyHome
	keyMap[KeyEnd] = platform.KeyEnd
	keyMap[KeyCapsLock] = platform.KeyCapsLock
	keyMap[KeyScrollLock] = platform.KeyScrollLock
	keyMap[KeyNumLock] = platform.KeyNumLock
	keyMap[KeyPrintScreen] = platform.KeyPrintScreen
	keyMap[KeyPause] = platform.KeyPause
	keyMap[KeyF1] = platform.KeyF1
	keyMap[KeyF2] = platform.KeyF2
	keyMap[KeyF3] = platform.KeyF3
	keyMap[KeyF4] = platform.KeyF4
	keyMap[KeyF5] = platform.KeyF5
	keyMap[KeyF6] = platform.KeyF6
	keyMap[KeyF7] = platform.KeyF7
	keyMap[KeyF8] = platform.KeyF8
	keyMap[KeyF9] = platform.KeyF9
	keyMap[KeyF10] = platform.KeyF10
	keyMap[KeyF11] = platform.KeyF11
	keyMap[KeyF12] = platform.KeyF12
	keyMap[KeyKP0] = platform.KeyKP0
	keyMap[KeyKP1] = platform.KeyKP1
	keyMap[KeyKP2] = platform.KeyKP2
	keyMap[KeyKP3] = platform.KeyKP3
	keyMap[KeyKP4] = platform.KeyKP4
	keyMap[KeyKP5] = platform.KeyKP5
	keyMap[KeyKP6] = platform.KeyKP6
	keyMap[KeyKP7] = platform.KeyKP7
	keyMap[KeyKP8] = platform.KeyKP8
	keyMap[KeyKP9] = platform.KeyKP9
	keyMap[KeyKPDecimal] = platform.KeyKPDecimal
	keyMap[KeyKPDivide] = platform.KeyKPDivide
	keyMap[KeyKPMultiply] = platform.KeyKPMultiply
	keyMap[KeyKPSubtract] = platform.KeyKPSubtract
	keyMap[KeyKPAdd] = platform.KeyKPAdd
	keyMap[KeyKPEnter] = platform.KeyKPEnter
	keyMap[KeyKPEqual] = platform.KeyKPEqual
	keyMap[KeyLeftShift] = platform.KeyLeftShift
	keyMap[KeyLeftControl] = platform.KeyLeftControl
	keyMap[KeyLeftAlt] = platform.KeyLeftAlt
	keyMap[KeyLeftSuper] = platform.KeyLeftSuper
	keyMap[KeyRightShift] = platform.KeyRightShift
	keyMap[KeyRightControl] = platform.KeyRightControl
	keyMap[KeyRightAlt] = platform.KeyRightAlt
	keyMap[KeyRightSuper] = platform.KeyRightSuper
	keyMap[KeyMenu] = platform.KeyMenu
}

// keyToInternal converts a public Key to the internal platform.Key.
func keyToInternal(k Key) platform.Key {
	if k < 0 || int(k) >= len(keyMap) {
		return platform.KeyUnknown
	}
	return keyMap[k]
}

// MouseButton represents a mouse button.
type MouseButton int

// MouseButton constants.
const (
	MouseButtonLeft MouseButton = iota
	MouseButtonRight
	MouseButtonMiddle
)

// TouchID represents a touch point identifier.
type TouchID int

// GamepadID represents a gamepad identifier.
type GamepadID int

// GamepadButton represents a gamepad button.
type GamepadButton int
