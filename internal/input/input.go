// Package input aggregates input state from the platform layer and provides
// a clean query API to game code.
package input

import (
	"github.com/michaelraines/future-render/internal/platform"
)

// State tracks the current and previous frame's input state.
type State struct {
	// Keyboard
	keys     [platform.KeyCount]bool
	prevKeys [platform.KeyCount]bool
	chars    []rune

	// Mouse
	mouseButtons       [5]bool
	prevMouseButtons   [5]bool
	mouseX, mouseY     float64
	mouseDX, mouseDY   float64
	scrollDX, scrollDY float64

	// Touch
	touches map[int]Touch

	// Gamepads
	gamepads map[int]Gamepad
}

// Touch represents an active touch point.
type Touch struct {
	X, Y     float64
	Pressure float64
}

// Gamepad represents the state of a connected gamepad.
type Gamepad struct {
	Axes    [6]float64
	Buttons [16]bool
}

// New creates a new input state manager.
func New() *State {
	return &State{
		touches:  make(map[int]Touch),
		gamepads: make(map[int]Gamepad),
	}
}

// Update advances the input state to the next frame. Call at the beginning
// of each update tick.
func (s *State) Update() {
	s.prevKeys = s.keys
	s.prevMouseButtons = s.mouseButtons
	s.mouseDX = 0
	s.mouseDY = 0
	s.scrollDX = 0
	s.scrollDY = 0
	s.chars = s.chars[:0]
}

// --- InputHandler interface implementation ---

// OnKeyEvent handles a key event from the platform.
func (s *State) OnKeyEvent(event platform.KeyEvent) {
	if event.Key < 0 || int(event.Key) >= len(s.keys) {
		return
	}
	switch event.Action {
	case platform.ActionPress, platform.ActionRepeat:
		s.keys[event.Key] = true
	case platform.ActionRelease:
		s.keys[event.Key] = false
	}
}

// OnCharEvent handles a character input event.
func (s *State) OnCharEvent(char rune) {
	s.chars = append(s.chars, char)
}

// OnMouseButtonEvent handles a mouse button event.
func (s *State) OnMouseButtonEvent(event platform.MouseButtonEvent) {
	if event.Button < 0 || int(event.Button) >= len(s.mouseButtons) {
		return
	}
	switch event.Action {
	case platform.ActionPress:
		s.mouseButtons[event.Button] = true
	case platform.ActionRelease:
		s.mouseButtons[event.Button] = false
	case platform.ActionRepeat:
		// Mouse buttons don't typically repeat; no-op.
	}
	s.mouseX = event.X
	s.mouseY = event.Y
}

// OnMouseMoveEvent handles a mouse movement event.
func (s *State) OnMouseMoveEvent(event platform.MouseMoveEvent) {
	s.mouseX = event.X
	s.mouseY = event.Y
	s.mouseDX += event.DX
	s.mouseDY += event.DY
}

// OnMouseScrollEvent handles a mouse scroll event.
func (s *State) OnMouseScrollEvent(event platform.MouseScrollEvent) {
	s.scrollDX += event.DX
	s.scrollDY += event.DY
}

// OnTouchEvent handles a touch event.
func (s *State) OnTouchEvent(event platform.TouchEvent) {
	switch event.Action {
	case platform.ActionPress:
		s.touches[event.ID] = Touch{X: event.X, Y: event.Y, Pressure: event.Pressure}
	case platform.ActionRelease:
		delete(s.touches, event.ID)
	default:
		s.touches[event.ID] = Touch{X: event.X, Y: event.Y, Pressure: event.Pressure}
	}
}

// OnGamepadEvent handles a gamepad state update.
func (s *State) OnGamepadEvent(event platform.GamepadEvent) {
	if event.Disconnected {
		delete(s.gamepads, event.ID)
		return
	}
	s.gamepads[event.ID] = Gamepad{Axes: event.Axes, Buttons: event.Buttons}
}

// RemoveGamepad removes a gamepad from the state (disconnected).
func (s *State) RemoveGamepad(id int) {
	delete(s.gamepads, id)
}

// OnResizeEvent handles a window resize. No-op for input state.
func (s *State) OnResizeEvent(_, _ int) {}

// --- Query API ---

// IsKeyPressed returns whether the key is currently pressed.
func (s *State) IsKeyPressed(key platform.Key) bool {
	if key < 0 || int(key) >= len(s.keys) {
		return false
	}
	return s.keys[key]
}

// IsKeyJustPressed returns whether the key was pressed this frame.
func (s *State) IsKeyJustPressed(key platform.Key) bool {
	if key < 0 || int(key) >= len(s.keys) {
		return false
	}
	return s.keys[key] && !s.prevKeys[key]
}

// IsKeyJustReleased returns whether the key was released this frame.
func (s *State) IsKeyJustReleased(key platform.Key) bool {
	if key < 0 || int(key) >= len(s.keys) {
		return false
	}
	return !s.keys[key] && s.prevKeys[key]
}

// InputChars returns the runes input since the last frame.
func (s *State) InputChars() []rune {
	if len(s.chars) == 0 {
		return nil
	}
	result := make([]rune, len(s.chars))
	copy(result, s.chars)
	return result
}

// IsMouseButtonPressed returns whether the mouse button is currently pressed.
func (s *State) IsMouseButtonPressed(button platform.MouseButton) bool {
	if button < 0 || int(button) >= len(s.mouseButtons) {
		return false
	}
	return s.mouseButtons[button]
}

// IsMouseButtonJustPressed returns whether the mouse button was pressed this frame.
func (s *State) IsMouseButtonJustPressed(button platform.MouseButton) bool {
	if button < 0 || int(button) >= len(s.mouseButtons) {
		return false
	}
	return s.mouseButtons[button] && !s.prevMouseButtons[button]
}

// IsMouseButtonJustReleased returns whether the mouse button was released this frame.
func (s *State) IsMouseButtonJustReleased(button platform.MouseButton) bool {
	if button < 0 || int(button) >= len(s.mouseButtons) {
		return false
	}
	return !s.mouseButtons[button] && s.prevMouseButtons[button]
}

// MousePosition returns the current mouse position.
func (s *State) MousePosition() (x, y float64) {
	return s.mouseX, s.mouseY
}

// MouseDelta returns the mouse movement delta since last frame.
func (s *State) MouseDelta() (dx, dy float64) {
	return s.mouseDX, s.mouseDY
}

// ScrollDelta returns the scroll wheel delta since last frame.
func (s *State) ScrollDelta() (dx, dy float64) {
	return s.scrollDX, s.scrollDY
}

// TouchIDs returns the IDs of all active touch points.
func (s *State) TouchIDs() []int {
	ids := make([]int, 0, len(s.touches))
	for id := range s.touches {
		ids = append(ids, id)
	}
	return ids
}

// TouchPosition returns the position of a touch point.
func (s *State) TouchPosition(id int) (x, y float64, ok bool) {
	t, ok := s.touches[id]
	if !ok {
		return 0, 0, false
	}
	return t.X, t.Y, true
}

// GamepadIDs returns the IDs of all connected gamepads.
func (s *State) GamepadIDs() []int {
	ids := make([]int, 0, len(s.gamepads))
	for id := range s.gamepads {
		ids = append(ids, id)
	}
	return ids
}

// GamepadAxis returns the value of a gamepad axis.
func (s *State) GamepadAxis(id, axis int) float64 {
	gp, ok := s.gamepads[id]
	if !ok || axis < 0 || axis >= len(gp.Axes) {
		return 0
	}
	return gp.Axes[axis]
}

// GamepadButton returns whether a gamepad button is pressed.
func (s *State) GamepadButton(id, button int) bool {
	gp, ok := s.gamepads[id]
	if !ok || button < 0 || button >= len(gp.Buttons) {
		return false
	}
	return gp.Buttons[button]
}
