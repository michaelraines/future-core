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
	// Compute logical size from physical pixels and density.
	if e.PixelsPerPt > 0 {
		w.width = int(float32(e.WidthPx) / e.PixelsPerPt)
		w.height = int(float32(e.HeightPx) / e.PixelsPerPt)
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
