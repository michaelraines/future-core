// Package input aggregates input state from the platform layer and provides
// a clean query API to game code.
package input

import (
	"maps"

	"github.com/michaelraines/future-core/internal/platform"
)

// State tracks the current and previous frame's input state.
type State struct {
	// Keyboard. keyDuration[k] is the number of ticks key k has been held
	// continuously; reset to 0 the tick it is released.
	keys        [platform.KeyCount]bool
	prevKeys    [platform.KeyCount]bool
	keyDuration [platform.KeyCount]int
	chars       []rune

	// Mouse. mouseButtonDuration mirrors keyDuration for mouse buttons.
	mouseButtons        [5]bool
	prevMouseButtons    [5]bool
	mouseButtonDuration [5]int
	mouseX, mouseY      float64
	mouseDX, mouseDY    float64
	scrollDX, scrollDY  float64

	// Touch. prevTouches is a snapshot of touches at the start of the
	// current tick (captured in Update) so justPressed / justReleased
	// can be computed from set difference against touches.
	touches     map[int]Touch
	prevTouches map[int]Touch

	// Gamepads. prevGamepads mirrors prevTouches for edge-triggered
	// button queries. gamepadButtonDuration[id][b] tracks hold time.
	gamepads              map[int]Gamepad
	prevGamepads          map[int]Gamepad
	gamepadButtonDuration map[int]*[16]int
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
		touches:               make(map[int]Touch),
		prevTouches:           make(map[int]Touch),
		gamepads:              make(map[int]Gamepad),
		prevGamepads:          make(map[int]Gamepad),
		gamepadButtonDuration: make(map[int]*[16]int),
	}
}

// BeginTick runs at the START of each game tick, AFTER any queued
// platform events for this tick have been applied to the state and
// BEFORE the game reads it. It increments per-key / per-button
// duration counters for anything currently held (and resets to 0 for
// anything released), so the game sees a 1-based duration on the
// first tick it observes a press.
//
// BeginTick does NOT clear per-tick accumulators (scroll delta,
// character buffer, mouse movement delta) — those must survive into
// the game's read. EndTick does the clearing.
func (s *State) BeginTick() {
	for i := range s.keys {
		if s.keys[i] {
			s.keyDuration[i]++
		} else {
			s.keyDuration[i] = 0
		}
	}
	for i := range s.mouseButtons {
		if s.mouseButtons[i] {
			s.mouseButtonDuration[i]++
		} else {
			s.mouseButtonDuration[i] = 0
		}
	}
	for id, gp := range s.gamepads {
		dur := s.gamepadButtonDuration[id]
		if dur == nil {
			dur = &[16]int{}
			s.gamepadButtonDuration[id] = dur
		}
		for b, pressed := range gp.Buttons {
			if pressed {
				dur[b]++
			} else {
				dur[b] = 0
			}
		}
	}
	// Drop duration counters for gamepads that went away.
	for id := range s.gamepadButtonDuration {
		if _, ok := s.gamepads[id]; !ok {
			delete(s.gamepadButtonDuration, id)
		}
	}
}

// EndTick runs at the END of each game tick (after game.Update has
// finished reading state). It snapshots the current held state into
// prev* so the NEXT tick's edge-triggered queries (IsKeyJustPressed,
// IsGamepadButtonJustPressed, AppendJustPressedTouchIDs) can compare
// against it, and clears per-tick accumulators so the next tick
// starts with a fresh scroll/mouse/character buffer.
func (s *State) EndTick() {
	s.prevKeys = s.keys
	s.prevMouseButtons = s.mouseButtons

	s.prevTouches = make(map[int]Touch, len(s.touches))
	maps.Copy(s.prevTouches, s.touches)

	s.prevGamepads = make(map[int]Gamepad, len(s.gamepads))
	maps.Copy(s.prevGamepads, s.gamepads)

	s.mouseDX = 0
	s.mouseDY = 0
	s.scrollDX = 0
	s.scrollDY = 0
	s.chars = s.chars[:0]
}

// Update is a convenience that runs BeginTick + EndTick back-to-back.
// This matches the original pre-split behavior and is what tests use
// to model "advance one whole tick". Engines should call BeginTick
// and EndTick around game.Update instead — see engine_js.go /
// engine_desktop.go for the reason why.
func (s *State) Update() {
	s.BeginTick()
	s.EndTick()
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

// TouchPressure returns the pressure of a touch point (0.0 to 1.0).
// Returns 0 if the touch point is not active.
func (s *State) TouchPressure(id int) float64 {
	t, ok := s.touches[id]
	if !ok {
		return 0
	}
	return t.Pressure
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

// KeyPressDuration returns the number of ticks the key has been held
// continuously. Returns 0 if the key is not held, or if the key is out
// of range.
func (s *State) KeyPressDuration(key platform.Key) int {
	if key < 0 || int(key) >= len(s.keys) {
		return 0
	}
	return s.keyDuration[key]
}

// MouseButtonPressDuration returns the number of ticks the mouse button
// has been held continuously. Returns 0 if not held or out of range.
func (s *State) MouseButtonPressDuration(button platform.MouseButton) int {
	if button < 0 || int(button) >= len(s.mouseButtons) {
		return 0
	}
	return s.mouseButtonDuration[button]
}

// AppendPressedKeys appends every currently-held key to the slice and
// returns the result. The slice is appended in ascending key-code order
// so callers get a deterministic iteration.
func (s *State) AppendPressedKeys(keys []platform.Key) []platform.Key {
	for k, pressed := range s.keys {
		if pressed {
			keys = append(keys, platform.Key(k))
		}
	}
	return keys
}

// AppendJustPressedKeys appends every key that transitioned from released
// to pressed this tick. Order is ascending by key code.
func (s *State) AppendJustPressedKeys(keys []platform.Key) []platform.Key {
	for k := range s.keys {
		if s.keys[k] && !s.prevKeys[k] {
			keys = append(keys, platform.Key(k))
		}
	}
	return keys
}

// AppendJustReleasedKeys appends every key that transitioned from pressed
// to released this tick. Order is ascending by key code.
func (s *State) AppendJustReleasedKeys(keys []platform.Key) []platform.Key {
	for k := range s.keys {
		if !s.keys[k] && s.prevKeys[k] {
			keys = append(keys, platform.Key(k))
		}
	}
	return keys
}

// AppendJustPressedTouchIDs appends IDs of touches that started this tick
// (present now but absent last tick). Order is not specified — touch IDs
// are typically unordered anyway.
func (s *State) AppendJustPressedTouchIDs(ids []int) []int {
	for id := range s.touches {
		if _, had := s.prevTouches[id]; !had {
			ids = append(ids, id)
		}
	}
	return ids
}

// AppendJustReleasedTouchIDs appends IDs of touches that ended this tick
// (present last tick but absent now).
func (s *State) AppendJustReleasedTouchIDs(ids []int) []int {
	for id := range s.prevTouches {
		if _, still := s.touches[id]; !still {
			ids = append(ids, id)
		}
	}
	return ids
}

// IsGamepadButtonJustPressed returns whether the given button on the
// given gamepad transitioned from released to pressed this tick.
func (s *State) IsGamepadButtonJustPressed(id, button int) bool {
	return s.gamepadButtonEdge(id, button, false, true)
}

// IsGamepadButtonJustReleased returns whether the given button on the
// given gamepad transitioned from pressed to released this tick.
func (s *State) IsGamepadButtonJustReleased(id, button int) bool {
	return s.gamepadButtonEdge(id, button, true, false)
}

func (s *State) gamepadButtonEdge(id, button int, wantPrev, wantNow bool) bool {
	if button < 0 || button >= 16 {
		return false
	}
	gp, nowConnected := s.gamepads[id]
	prev, prevConnected := s.prevGamepads[id]
	nowHeld := nowConnected && gp.Buttons[button]
	prevHeld := prevConnected && prev.Buttons[button]
	return prevHeld == wantPrev && nowHeld == wantNow
}

// GamepadButtonPressDuration returns the number of ticks the given
// gamepad button has been held continuously. Returns 0 if the gamepad
// is disconnected, the button is out of range, or not held.
func (s *State) GamepadButtonPressDuration(id, button int) int {
	if button < 0 || button >= 16 {
		return 0
	}
	dur, ok := s.gamepadButtonDuration[id]
	if !ok {
		return 0
	}
	return dur[button]
}

// GamepadButtonCount returns the number of buttons on the given gamepad.
// Returns 0 if the gamepad is not connected. Currently always reports 16
// (the fixed internal button capacity); when the platform layer starts
// reporting true per-pad button counts this will switch to that.
func (s *State) GamepadButtonCount(id int) int {
	if _, ok := s.gamepads[id]; !ok {
		return 0
	}
	return 16
}

// GamepadAxisCount returns the number of axes on the given gamepad.
// Returns 0 if the gamepad is not connected.
func (s *State) GamepadAxisCount(id int) int {
	if _, ok := s.gamepads[id]; !ok {
		return 0
	}
	return 6
}
