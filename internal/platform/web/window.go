//go:build js

// Package web implements platform.Window for browser environments using
// syscall/js. It manages a canvas element, input events, and the
// requestAnimationFrame loop.
package web

import (
	"syscall/js"

	"github.com/michaelraines/future-core/internal/platform"
)

// Window implements platform.Window for the browser.
type Window struct {
	canvas  js.Value
	doc     js.Value
	handler platform.InputHandler

	width, height     int
	fbWidth, fbHeight int
	dpr               float64
	title             string
	fullscreen        bool
	shouldClose       bool

	// Previous mouse position for delta calculation.
	lastMouseX, lastMouseY float64
	mouseInited            bool

	// Registered JS event listeners (for cleanup).
	listeners []js.Func
}

// New creates a new browser window.
func New() *Window {
	return &Window{
		dpr: 1.0,
	}
}

// Create sets up the canvas element and registers event listeners.
func (w *Window) Create(cfg platform.WindowConfig) error {
	w.doc = js.Global().Get("document")
	w.title = cfg.Title
	w.width = cfg.Width
	w.height = cfg.Height

	// Get device pixel ratio.
	w.dpr = js.Global().Get("devicePixelRatio").Float()
	if w.dpr < 1 {
		w.dpr = 1
	}

	// Look for an existing canvas or create one.
	w.canvas = w.doc.Call("getElementById", "game-canvas")
	if w.canvas.IsNull() || w.canvas.IsUndefined() {
		w.canvas = w.doc.Call("createElement", "canvas")
		w.canvas.Set("id", "game-canvas")
		w.doc.Get("body").Call("appendChild", w.canvas)
	}

	w.canvas.Set("width", int(float64(cfg.Width)*w.dpr))
	w.canvas.Set("height", int(float64(cfg.Height)*w.dpr))
	w.canvas.Get("style").Set("width", js.ValueOf(cfg.Width).String()+"px")
	w.canvas.Get("style").Set("height", js.ValueOf(cfg.Height).String()+"px")

	w.fbWidth = int(float64(cfg.Width) * w.dpr)
	w.fbHeight = int(float64(cfg.Height) * w.dpr)

	if cfg.Title != "" {
		w.doc.Set("title", cfg.Title)
	}

	// Register input event listeners.
	w.registerInputListeners()

	return nil
}

// Destroy removes event listeners.
func (w *Window) Destroy() {
	for _, fn := range w.listeners {
		fn.Release()
	}
	w.listeners = nil
}

// ShouldClose returns whether the window should close.
func (w *Window) ShouldClose() bool { return w.shouldClose }

// PollEvents is a no-op — browser events are callback-driven.
func (w *Window) PollEvents() {}

// SwapBuffers is a no-op — the browser composites automatically.
func (w *Window) SwapBuffers() {}

// Size returns the logical window size based on the actual browser
// window dimensions (not the initial requested size).
func (w *Window) Size() (int, int) {
	iw := js.Global().Get("innerWidth")
	ih := js.Global().Get("innerHeight")
	if !iw.IsUndefined() && !ih.IsUndefined() {
		return iw.Int(), ih.Int()
	}
	return w.width, w.height
}

// FramebufferSize returns the physical framebuffer size based on
// the actual browser window dimensions and device pixel ratio.
func (w *Window) FramebufferSize() (int, int) {
	ww, wh := w.Size()
	return int(float64(ww) * w.dpr), int(float64(wh) * w.dpr)
}

// DevicePixelRatio returns the display scale factor.
func (w *Window) DevicePixelRatio() float64 { return w.dpr }

// SetTitle sets the document title.
func (w *Window) SetTitle(title string) {
	w.title = title
	w.doc.Set("title", title)
}

// SetSize resizes the canvas.
func (w *Window) SetSize(width, height int) {
	w.width = width
	w.height = height
	w.fbWidth = int(float64(width) * w.dpr)
	w.fbHeight = int(float64(height) * w.dpr)
	w.canvas.Set("width", w.fbWidth)
	w.canvas.Set("height", w.fbHeight)
}

// SetFullscreen toggles fullscreen via the Fullscreen API.
func (w *Window) SetFullscreen(fullscreen bool) {
	if fullscreen && !w.fullscreen {
		w.canvas.Call("requestFullscreen")
	} else if !fullscreen && w.fullscreen {
		w.doc.Call("exitFullscreen")
	}
	w.fullscreen = fullscreen
}

// IsFullscreen returns the fullscreen state.
func (w *Window) IsFullscreen() bool { return w.fullscreen }

// SetCursorVisible shows or hides the cursor.
func (w *Window) SetCursorVisible(visible bool) {
	if visible {
		w.canvas.Get("style").Set("cursor", "default")
	} else {
		w.canvas.Get("style").Set("cursor", "none")
	}
}

// SetCursorLocked locks or unlocks the pointer.
func (w *Window) SetCursorLocked(locked bool) {
	if locked {
		w.canvas.Call("requestPointerLock")
	} else {
		w.doc.Call("exitPointerLock")
	}
}

// NativeHandle returns the canvas as a JS value handle (not useful as uintptr).
func (w *Window) NativeHandle() uintptr { return 0 }

// SetInputHandler sets the handler that receives input events.
func (w *Window) SetInputHandler(handler platform.InputHandler) {
	w.handler = handler
}

// PollGamepads polls the Gamepad API.
func (w *Window) PollGamepads() {
	if w.handler == nil {
		return
	}
	navigator := js.Global().Get("navigator")
	gamepads := navigator.Call("getGamepads")
	if gamepads.IsNull() || gamepads.IsUndefined() {
		return
	}
	for i := 0; i < gamepads.Length(); i++ {
		gp := gamepads.Index(i)
		if gp.IsNull() || gp.IsUndefined() {
			continue
		}
		var evt platform.GamepadEvent
		evt.ID = i
		axes := gp.Get("axes")
		for a := 0; a < axes.Length() && a < 6; a++ {
			evt.Axes[a] = axes.Index(a).Float()
		}
		buttons := gp.Get("buttons")
		for b := 0; b < buttons.Length() && b < 16; b++ {
			evt.Buttons[b] = buttons.Index(b).Get("pressed").Bool()
		}
		w.handler.OnGamepadEvent(evt)
	}
}

// Canvas returns the underlying canvas js.Value for WebGPU context creation.
func (w *Window) Canvas() js.Value { return w.canvas }
