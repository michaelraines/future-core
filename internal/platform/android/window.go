//go:build android

// Package android implements the platform.Window interface for Android
// using golang.org/x/mobile/app for lifecycle, input, and display management.
package android

import (
	"sync"

	"golang.org/x/mobile/app"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
	"golang.org/x/mobile/event/touch"

	"github.com/michaelraines/future-core/internal/platform"
)

// Window implements platform.Window for Android via golang.org/x/mobile.
type Window struct {
	mu      sync.Mutex
	app     app.App
	handler platform.InputHandler

	// Current screen dimensions from the most recent size event.
	width, height int
	pixelsPerPt   float32
	widthPx, hPx  int // physical pixel dimensions
	shouldClose   bool
	focused       bool
	nativeWindow  uintptr

	// Event queue for processing in PollEvents.
	pendingEvents []interface{}
}

// New creates a new Android window.
func New() *Window {
	return &Window{
		width:       800,
		height:      600,
		pixelsPerPt: 2.0,
	}
}

// Create initializes the Android window. The actual window is provided by
// the Android runtime; this method prepares internal state.
func (w *Window) Create(_ platform.WindowConfig) error {
	// On Android, the window is created by the OS and provided via
	// app.Main(). This method initializes internal state.
	w.focused = true
	return nil
}

// Destroy releases Android window resources.
func (w *Window) Destroy() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.shouldClose = true
	w.app = nil
}

// ShouldClose returns true when the activity is being destroyed.
func (w *Window) ShouldClose() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.shouldClose
}

// PollEvents processes pending Android input and lifecycle events,
// dispatching them to the registered InputHandler.
func (w *Window) PollEvents() {
	w.mu.Lock()
	events := w.pendingEvents
	w.pendingEvents = nil
	handler := w.handler
	w.mu.Unlock()

	if handler == nil {
		return
	}

	for _, e := range events {
		switch ev := e.(type) {
		case touch.Event:
			w.dispatchTouch(handler, ev)
		case key.Event:
			w.dispatchKey(handler, ev)
		case size.Event:
			handler.OnResizeEvent(int(ev.WidthPx), int(ev.HeightPx))
		}
	}
}

// dispatchTouch converts a mobile touch event to platform.TouchEvent.
func (w *Window) dispatchTouch(handler platform.InputHandler, ev touch.Event) {
	var action platform.Action
	switch ev.Type {
	case touch.TypeBegin:
		action = platform.ActionPress
	case touch.TypeEnd:
		action = platform.ActionRelease
	case touch.TypeMove:
		// Use ActionRepeat for move events to distinguish from press/release.
		// The input layer treats this as continued contact.
		action = platform.ActionRepeat
	default:
		return
	}

	handler.OnTouchEvent(platform.TouchEvent{
		ID:       int(ev.Sequence),
		Action:   action,
		X:        float64(ev.X),
		Y:        float64(ev.Y),
		Pressure: 1.0, // x/mobile doesn't expose pressure
	})
}

// dispatchKey converts a mobile key event to platform.KeyEvent.
func (w *Window) dispatchKey(handler platform.InputHandler, ev key.Event) {
	// Check if this key event is from a gamepad (D-pad, face buttons).
	pressed := ev.Direction == key.DirPress || ev.Direction == key.DirNone
	if handleGamepadKeyEvent(handler, ev.Code, pressed) {
		return // consumed as gamepad input
	}

	platformKey := mapKey(ev.Code)
	if platformKey == platform.KeyUnknown {
		return
	}

	var action platform.Action
	switch ev.Direction {
	case key.DirPress:
		action = platform.ActionPress
	case key.DirRelease:
		action = platform.ActionRelease
	default:
		// key.DirNone means press+release in one event. Fire press.
		action = platform.ActionPress
	}

	var mods platform.Modifier
	if ev.Modifiers&key.ModShift != 0 {
		mods |= platform.ModShift
	}
	if ev.Modifiers&key.ModControl != 0 {
		mods |= platform.ModControl
	}
	if ev.Modifiers&key.ModAlt != 0 {
		mods |= platform.ModAlt
	}
	if ev.Modifiers&key.ModMeta != 0 {
		mods |= platform.ModSuper
	}

	handler.OnKeyEvent(platform.KeyEvent{
		Key:    platformKey,
		Action: action,
		Mods:   mods,
	})

	// For printable characters, also fire a char event.
	if ev.Rune > 0 && action == platform.ActionPress {
		handler.OnCharEvent(ev.Rune)
	}
}

// SwapBuffers is a no-op on Android when using Vulkan (presentation is
// handled by the swapchain). For soft rasterizer, publish via the app.
func (w *Window) SwapBuffers() {
	// Vulkan manages its own presentation via the swapchain.
	// Soft rasterizer path would need app.Publish() here.
}

// Size returns the window size in logical pixels (points).
func (w *Window) Size() (int, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.width, w.height
}

// FramebufferSize returns the framebuffer size in physical pixels.
func (w *Window) FramebufferSize() (int, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.widthPx, w.hPx
}

// DevicePixelRatio returns the ratio of physical to logical pixels.
func (w *Window) DevicePixelRatio() float64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pixelsPerPt <= 0 {
		return 1.0
	}
	// x/mobile reports pixels per point (1pt = 1/72 inch).
	// Android density is relative to 160 DPI baseline.
	// pixelsPerPt * 72 gives DPI; divide by 160 for density ratio.
	return float64(w.pixelsPerPt) * 72.0 / 160.0
}

// SetTitle is a no-op on Android (apps don't have title bars).
func (w *Window) SetTitle(_ string) {}

// SetSize is a no-op on Android (the OS controls window size).
func (w *Window) SetSize(_, _ int) {}

// SetFullscreen is a no-op on Android (apps are always fullscreen).
func (w *Window) SetFullscreen(_ bool) {}

// IsFullscreen returns true — Android apps are always fullscreen.
func (w *Window) IsFullscreen() bool { return true }

// SetCursorVisible is a no-op on Android (touch devices).
func (w *Window) SetCursorVisible(_ bool) {}

// SetCursorLocked is a no-op on Android (touch devices).
func (w *Window) SetCursorLocked(_ bool) {}

// NativeHandle returns the ANativeWindow pointer for Vulkan surface creation.
func (w *Window) NativeHandle() uintptr {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.nativeWindow
}

// SetInputHandler registers the handler for input events.
func (w *Window) SetInputHandler(handler platform.InputHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handler = handler
}

// PollGamepads is a stub — gamepad support requires JNI integration.
func (w *Window) PollGamepads() {}

// SetApp sets the x/mobile App reference for event processing.
// Called by the engine during initialization from app.Main.
func (w *Window) SetApp(a app.App) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.app = a
}

// HandleLifecycleEvent processes Android lifecycle transitions.
func (w *Window) HandleLifecycleEvent(e lifecycle.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()

	switch e.Crosses(lifecycle.StageFocused) {
	case lifecycle.CrossOn:
		w.focused = true
	case lifecycle.CrossOff:
		w.focused = false
	}

	if e.Crosses(lifecycle.StageDead) == lifecycle.CrossOn {
		w.shouldClose = true
	}
}

// HandleSizeEvent processes screen dimension changes.
func (w *Window) HandleSizeEvent(e size.Event) {
	w.mu.Lock()
	w.widthPx = e.WidthPx
	w.hPx = e.HeightPx
	w.pixelsPerPt = e.PixelsPerPt
	// Compute logical size using Android density-independent pixels (dp).
	// x/mobile's PixelsPerPt is pixels per typographic point (1pt = 1/72 inch).
	// Android density = (PixelsPerPt * 72) / 160. Logical dp = physical / density.
	// This gives dimensions in Android dp, consistent with DevicePixelRatio().
	if e.PixelsPerPt > 0 {
		density := float32(e.PixelsPerPt) * 72.0 / 160.0
		w.width = int(float32(e.WidthPx) / density)
		w.height = int(float32(e.HeightPx) / density)
	} else {
		w.width = e.WidthPx
		w.height = e.HeightPx
	}
	w.mu.Unlock()

	// Queue resize event for PollEvents dispatch.
	w.queueEvent(e)
}

// HandleTouchEvent queues a touch event for processing in PollEvents.
func (w *Window) HandleTouchEvent(e touch.Event) {
	w.queueEvent(e)
}

// HandleKeyEvent queues a key event for processing in PollEvents.
func (w *Window) HandleKeyEvent(e key.Event) {
	w.queueEvent(e)
}

// HandlePaintEvent is called when a frame should be rendered.
// Returns true if the window is focused and ready for rendering.
func (w *Window) HandlePaintEvent(_ paint.Event) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.focused && !w.shouldClose
}

// SetNativeWindow sets the ANativeWindow pointer from the Android surface.
func (w *Window) SetNativeWindow(handle uintptr) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.nativeWindow = handle
}

// IsFocused returns whether the activity is in the foreground.
func (w *Window) IsFocused() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.focused
}

func (w *Window) queueEvent(e interface{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pendingEvents = append(w.pendingEvents, e)
}

// --- Raw-input path (embedded mode / JNI) --------------------------------
//
// The NativeActivity path receives events via x/mobile/event/* types and
// routes them through HandleTouchEvent / HandleKeyEvent above. The embedded
// (gomobile-bind) path has no access to those types because they depend on
// the NDK looper; instead the Java layer calls into Go with primitive
// parameters, which we translate to platform.* types here.

// HandleRawTouch routes a JNI-sourced touch sample to the registered
// InputHandler. action is one of the MotionAction* constants (Down, Up,
// Move, Cancel, PointerDown, PointerUp); id is the pointer ID from
// MotionEvent.getPointerId(i); x/y are in physical pixels.
func (w *Window) HandleRawTouch(action, id int, x, y float32) {
	w.mu.Lock()
	handler := w.handler
	w.mu.Unlock()
	if handler == nil {
		return
	}

	var pa platform.Action
	switch action {
	case MotionActionDown, MotionActionPointerDown:
		pa = platform.ActionPress
	case MotionActionUp, MotionActionPointerUp, MotionActionCancel:
		pa = platform.ActionRelease
	case MotionActionMove:
		pa = platform.ActionRepeat
	default:
		return
	}

	handler.OnTouchEvent(platform.TouchEvent{
		ID:       id,
		Action:   pa,
		X:        float64(x),
		Y:        float64(y),
		Pressure: 1.0,
	})
}

// HandleRawKey routes a JNI-sourced key event to the registered
// InputHandler. keyCode is an Android KeyEvent.KEYCODE_*; unicodeChar is
// the result of KeyEvent.getUnicodeChar() (0 for non-printing keys);
// meta is the KeyEvent.getMetaState() bitmask; source is the device
// source used to detect gamepad input; deviceID is the InputDevice id
// (reserved for multi-gamepad support).
func (w *Window) HandleRawKey(keyCode, unicodeChar, meta, source, deviceID int, down bool) {
	w.mu.Lock()
	handler := w.handler
	w.mu.Unlock()
	if handler == nil {
		return
	}

	// Gamepad-sourced key events short-circuit into the gamepad state
	// machine. Treat the keyCode as a gamepad button; fall through to
	// keyboard dispatch only if the code doesn't map to any button.
	if IsGamepadSource(source) {
		if handleRawGamepadKey(handler, deviceID, keyCode, down) {
			return
		}
	}

	key := mapAndroidKeyCode(keyCode)
	if key == platform.KeyUnknown {
		return
	}
	pa := platform.ActionRelease
	if down {
		pa = platform.ActionPress
	}

	handler.OnKeyEvent(platform.KeyEvent{
		Key:    key,
		Action: pa,
		Mods:   mapAndroidMetaState(meta),
	})

	if down && unicodeChar > 0 {
		handler.OnCharEvent(rune(unicodeChar))
	}
}

// HandleRawGamepadAxis routes an analog-stick or trigger sample.
// deviceID is the InputDevice id; axis is one of the Android
// MotionEvent.AXIS_* constants we care about (X, Y, Z, RZ, HAT_X,
// HAT_Y, LTRIGGER, RTRIGGER); value is the normalized axis value from
// getAxisValue.
func (w *Window) HandleRawGamepadAxis(deviceID, axis int, value float32) {
	w.mu.Lock()
	handler := w.handler
	w.mu.Unlock()
	if handler == nil {
		return
	}
	handleRawGamepadAxis(handler, deviceID, axis, value)
}

// HandleRawGamepadConnection updates gamepad-presence state when Android
// reports an InputDevice change (added/removed).
func (w *Window) HandleRawGamepadConnection(deviceID int, connected bool) {
	w.mu.Lock()
	handler := w.handler
	w.mu.Unlock()
	if handler == nil {
		return
	}
	if !connected {
		currentGamepad = gamepadState{}
		handler.OnGamepadEvent(platform.GamepadEvent{ID: 0, Disconnected: true})
	}
	// Connection is implicit: the first axis/button event activates state.
}
