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
