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
