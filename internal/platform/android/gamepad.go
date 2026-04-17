//go:build android

package android

import (
	mkey "golang.org/x/mobile/event/key"

	"github.com/michaelraines/future-core/internal/platform"
)

// gamepadState tracks button state derived from Android key events.
// On Android, gamepad D-pad and face buttons generate key events.
// Analog stick input requires JNI access to MotionEvent (future work).
type gamepadState struct {
	buttons [16]bool
	axes    [6]float64
	active  bool
}

// gamepadButtonMap maps x/mobile key codes to gamepad button indices.
// Button layout follows the standard mapping used by cocoa/win32:
//
//	[0]=A, [1]=B, [2]=X, [3]=Y,
//	[4]=LB, [5]=RB, [6]=Options, [7]=Menu,
//	[8]=LThumb, [9]=RThumb,
//	[10]=DUp, [11]=DDown, [12]=DLeft, [13]=DRight
var gamepadButtonMap = map[mkey.Code]int{
	mkey.CodeKeypadEnter: 0,  // A / confirm (common mapping)
	mkey.CodeEscape:      1,  // B / back
	mkey.CodeUpArrow:     10, // D-pad up
	mkey.CodeDownArrow:   11, // D-pad down
	mkey.CodeLeftArrow:   12, // D-pad left
	mkey.CodeRightArrow:  13, // D-pad right
}

var currentGamepad gamepadState

// handleGamepadKeyEvent processes a key event that may be from a gamepad.
// Returns true if the event was consumed as a gamepad input.
func handleGamepadKeyEvent(handler platform.InputHandler, code mkey.Code, pressed bool) bool {
	btnIdx, ok := gamepadButtonMap[code]
	if !ok {
		return false
	}

	currentGamepad.buttons[btnIdx] = pressed
	currentGamepad.active = true

	handler.OnGamepadEvent(platform.GamepadEvent{
		ID:      0,
		Buttons: currentGamepad.buttons,
		// Axes remain zero — analog sticks require MotionEvent via JNI.
	})
	return true
}

// handleRawGamepadKey processes a JNI-sourced button press keyed by the
// Android KEYCODE_BUTTON_* integer. Returns true if the code mapped to a
// known gamepad button.
func handleRawGamepadKey(handler platform.InputHandler, _ int, keyCode int, pressed bool) bool {
	btnIdx, ok := androidGamepadButtonMap[keyCode]
	if !ok {
		return false
	}
	currentGamepad.buttons[btnIdx] = pressed
	currentGamepad.active = true
	handler.OnGamepadEvent(platform.GamepadEvent{
		ID:      0,
		Axes:    currentGamepad.axes,
		Buttons: currentGamepad.buttons,
	})
	return true
}

// handleRawGamepadAxis updates the cached axis state and dispatches a
// GamepadEvent. axis is an Android MotionEvent.AXIS_* integer.
func handleRawGamepadAxis(handler platform.InputHandler, _ int, axis int, value float32) {
	idx, ok := mapAndroidAxis(axis)
	if !ok {
		return
	}
	currentGamepad.axes[idx] = float64(value)
	currentGamepad.active = true
	handler.OnGamepadEvent(platform.GamepadEvent{
		ID:      0,
		Axes:    currentGamepad.axes,
		Buttons: currentGamepad.buttons,
	})
}

// mapAndroidAxis maps an Android MotionEvent axis constant to the 6-slot
// axis array used by platform.GamepadEvent. Layout:
//
//	[0]=LeftX  [1]=LeftY  [2]=RightX  [3]=RightY  [4]=LTrigger  [5]=RTrigger
func mapAndroidAxis(axis int) (int, bool) {
	switch axis {
	case AxisX:
		return 0, true
	case AxisY:
		return 1, true
	case AxisZ:
		return 2, true
	case AxisRZ:
		return 3, true
	case AxisLTrigger:
		return 4, true
	case AxisRTrigger:
		return 5, true
	}
	return 0, false
}
