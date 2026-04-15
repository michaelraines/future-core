// Package inpututil provides utility functions for input handling,
// complementing the main futurerender input API.
// This matches Ebitengine's inpututil sub-package.
package inpututil

import (
	futurerender "github.com/michaelraines/future-core"
)

// AppendPressedKeys appends currently pressed keys to the given slice
// and returns the result. If keys is nil, a new slice is allocated.
func AppendPressedKeys(keys []futurerender.Key) []futurerender.Key {
	for k := futurerender.Key(0); k < futurerender.KeyMax; k++ {
		if futurerender.IsKeyPressed(k) {
			keys = append(keys, k)
		}
	}
	return keys
}

// PressedKeys returns a slice of all currently pressed keys.
func PressedKeys() []futurerender.Key {
	return AppendPressedKeys(nil)
}

// IsMouseButtonJustPressed returns whether the given mouse button was
// pressed this frame (edge detection).
func IsMouseButtonJustPressed(button futurerender.MouseButton) bool {
	return futurerender.IsMouseButtonJustPressed(button)
}

// AppendJustPressedKeys appends every key that transitioned from
// released to pressed this tick. Order is ascending by key code.
func AppendJustPressedKeys(keys []futurerender.Key) []futurerender.Key {
	for k := futurerender.Key(0); k < futurerender.KeyMax; k++ {
		if futurerender.IsKeyJustPressed(k) {
			keys = append(keys, k)
		}
	}
	return keys
}

// AppendJustReleasedKeys appends every key that transitioned from
// pressed to released this tick.
func AppendJustReleasedKeys(keys []futurerender.Key) []futurerender.Key {
	for k := futurerender.Key(0); k < futurerender.KeyMax; k++ {
		if futurerender.IsKeyJustReleased(k) {
			keys = append(keys, k)
		}
	}
	return keys
}

// AppendJustPressedTouchIDs appends IDs of touches that started this
// tick (present now but absent last tick).
func AppendJustPressedTouchIDs(ids []futurerender.TouchID) []futurerender.TouchID {
	return futurerender.AppendJustPressedTouchIDs(ids)
}

// AppendJustReleasedTouchIDs appends IDs of touches that ended this
// tick (present last tick but absent now).
func AppendJustReleasedTouchIDs(ids []futurerender.TouchID) []futurerender.TouchID {
	return futurerender.AppendJustReleasedTouchIDs(ids)
}

// KeyPressDuration returns the number of ticks the key has been held
// continuously. Returns 0 if the key is not held.
func KeyPressDuration(key futurerender.Key) int {
	return futurerender.KeyPressDuration(key)
}

// MouseButtonPressDuration returns the number of ticks the mouse button
// has been held continuously. Returns 0 if not held.
func MouseButtonPressDuration(button futurerender.MouseButton) int {
	return futurerender.MouseButtonPressDuration(button)
}

// IsGamepadButtonJustPressed returns whether the given gamepad button
// transitioned from released to pressed this tick.
func IsGamepadButtonJustPressed(id futurerender.GamepadID, button futurerender.GamepadButton) bool {
	return futurerender.IsGamepadButtonJustPressed(id, button)
}

// IsGamepadButtonJustReleased returns whether the given gamepad button
// transitioned from pressed to released this tick.
func IsGamepadButtonJustReleased(id futurerender.GamepadID, button futurerender.GamepadButton) bool {
	return futurerender.IsGamepadButtonJustReleased(id, button)
}

// GamepadButtonPressDuration returns the number of ticks the given
// gamepad button has been held continuously.
func GamepadButtonPressDuration(id futurerender.GamepadID, button futurerender.GamepadButton) int {
	return futurerender.GamepadButtonPressDuration(id, button)
}
