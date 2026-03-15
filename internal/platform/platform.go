// Package platform defines the interface for platform-specific windowing,
// input, and timing operations.
package platform

// Window represents a platform window.
type Window interface {
	// Create creates the window with the given configuration.
	Create(cfg WindowConfig) error

	// Destroy closes and releases the window.
	Destroy()

	// ShouldClose returns whether the window has been requested to close.
	ShouldClose() bool

	// PollEvents processes pending input events.
	PollEvents()

	// SwapBuffers swaps the front and back buffers (for OpenGL).
	SwapBuffers()

	// Size returns the window size in logical pixels.
	Size() (int, int)

	// FramebufferSize returns the framebuffer size in physical pixels.
	FramebufferSize() (int, int)

	// DevicePixelRatio returns the ratio of physical to logical pixels.
	DevicePixelRatio() float64

	// SetTitle sets the window title.
	SetTitle(title string)

	// SetSize sets the window size in logical pixels.
	SetSize(width, height int)

	// SetFullscreen sets fullscreen mode.
	SetFullscreen(fullscreen bool)

	// IsFullscreen returns whether the window is in fullscreen mode.
	IsFullscreen() bool

	// SetCursorVisible shows or hides the cursor.
	SetCursorVisible(visible bool)

	// SetCursorLocked locks or unlocks the cursor to the window.
	SetCursorLocked(locked bool)

	// NativeHandle returns a platform-specific window handle.
	NativeHandle() uintptr

	// SetInputHandler sets the handler for input events.
	SetInputHandler(handler InputHandler)

	// PollGamepads queries connected gamepads and dispatches events.
	// This must be called each frame since gamepad input is polling-based.
	PollGamepads()
}

// WindowConfig holds configuration for window creation.
type WindowConfig struct {
	Title         string
	Width, Height int
	Resizable     bool
	Fullscreen    bool
	VSync         bool
	Decorated     bool // window decorations (title bar, borders)
	Transparent   bool
	HighDPI       bool
}

// DefaultWindowConfig returns a reasonable default window configuration.
func DefaultWindowConfig() WindowConfig {
	return WindowConfig{
		Title:     "Future Render",
		Width:     800,
		Height:    600,
		Resizable: true,
		Decorated: true,
		HighDPI:   true,
		VSync:     true,
	}
}

// InputHandler receives input events from the window.
type InputHandler interface {
	OnKeyEvent(event KeyEvent)
	OnCharEvent(char rune)
	OnMouseButtonEvent(event MouseButtonEvent)
	OnMouseMoveEvent(event MouseMoveEvent)
	OnMouseScrollEvent(event MouseScrollEvent)
	OnTouchEvent(event TouchEvent)
	OnGamepadEvent(event GamepadEvent)
	OnResizeEvent(width, height int)
}

// KeyEvent represents a keyboard event.
type KeyEvent struct {
	Key    Key
	Action Action
	Mods   Modifier
}

// MouseButtonEvent represents a mouse button event.
type MouseButtonEvent struct {
	Button MouseButton
	Action Action
	X, Y   float64
	Mods   Modifier
}

// MouseMoveEvent represents a mouse movement event.
type MouseMoveEvent struct {
	X, Y   float64
	DX, DY float64 // delta movement
}

// MouseScrollEvent represents a mouse scroll event.
type MouseScrollEvent struct {
	DX, DY float64
}

// TouchEvent represents a touch event.
type TouchEvent struct {
	ID       int
	Action   Action
	X, Y     float64
	Pressure float64
}

// GamepadEvent represents a gamepad state update.
type GamepadEvent struct {
	ID           int
	Axes         [6]float64
	Buttons      [16]bool
	Disconnected bool // true if the gamepad was disconnected
}

// Action represents an input action.
type Action int

const (
	// ActionPress indicates a key or button was pressed.
	ActionPress Action = iota
	// ActionRelease indicates a key or button was released.
	ActionRelease
	// ActionRepeat indicates a key is being held (repeat event).
	ActionRepeat
)

// Modifier represents keyboard modifier flags.
type Modifier int

const (
	// ModShift indicates the Shift key modifier.
	ModShift Modifier = 1 << iota
	// ModControl indicates the Control key modifier.
	ModControl
	// ModAlt indicates the Alt key modifier.
	ModAlt
	// ModSuper indicates the Super/Command key modifier.
	ModSuper
	// ModCapsLock indicates Caps Lock is active.
	ModCapsLock
	// ModNumLock indicates Num Lock is active.
	ModNumLock
)

// MouseButton represents a mouse button.
type MouseButton int

const (
	// MouseButtonLeft is the left mouse button.
	MouseButtonLeft MouseButton = iota
	// MouseButtonRight is the right mouse button.
	MouseButtonRight
	// MouseButtonMiddle is the middle mouse button.
	MouseButtonMiddle
	// MouseButton4 is the 4th mouse button.
	MouseButton4
	// MouseButton5 is the 5th mouse button.
	MouseButton5
)
