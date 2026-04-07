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

	// Resize tracking — set by ResizeObserver, consumed by SyncCanvasSize.
	resizeDirty bool

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

	// Style the canvas to fill its container, matching Ebitengine's behavior.
	style := w.canvas.Get("style")
	style.Set("width", "100%")
	style.Set("height", "100%")
	style.Set("margin", "0px")
	style.Set("padding", "0px")
	style.Set("display", "block")
	style.Set("outline", "none")
	w.canvas.Set("tabIndex", 1)

	// Set initial pixel buffer size. SyncCanvasSize() will update this
	// each frame to match the actual CSS layout size × DPI.
	w.canvas.Set("width", int(float64(cfg.Width)*w.dpr))
	w.canvas.Set("height", int(float64(cfg.Height)*w.dpr))

	w.fbWidth = int(float64(cfg.Width) * w.dpr)
	w.fbHeight = int(float64(cfg.Height) * w.dpr)

	if cfg.Title != "" {
		w.doc.Set("title", cfg.Title)
	}

	// Register input event listeners.
	w.registerInputListeners()

	// Use a ResizeObserver to detect canvas resize instead of polling
	// the DOM every frame. SyncCanvasSize checks the dirty flag.
	w.resizeDirty = true // sync once on first frame
	resizeObserver := js.Global().Get("ResizeObserver")
	if !resizeObserver.IsUndefined() {
		cb := js.FuncOf(func(_ js.Value, _ []js.Value) interface{} {
			w.resizeDirty = true
			return nil
		})
		w.listeners = append(w.listeners, cb)
		observer := resizeObserver.New(cb)
		observer.Call("observe", w.canvas)
	}

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

// Size returns the logical (CSS) size of the canvas element.
func (w *Window) Size() (int, int) {
	if !w.canvas.IsUndefined() && !w.canvas.IsNull() {
		cw := w.canvas.Get("clientWidth")
		ch := w.canvas.Get("clientHeight")
		if !cw.IsUndefined() && !ch.IsUndefined() && cw.Int() > 0 && ch.Int() > 0 {
			return cw.Int(), ch.Int()
		}
	}
	return w.width, w.height
}

// FramebufferSize returns the physical framebuffer size (canvas pixel buffer).
func (w *Window) FramebufferSize() (int, int) {
	if !w.canvas.IsUndefined() && !w.canvas.IsNull() {
		pw := w.canvas.Get("width")
		ph := w.canvas.Get("height")
		if !pw.IsUndefined() && !ph.IsUndefined() && pw.Int() > 0 && ph.Int() > 0 {
			return pw.Int(), ph.Int()
		}
	}
	return w.fbWidth, w.fbHeight
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

// SyncCanvasSize updates the canvas pixel buffer to match its current
// CSS layout size × device pixel ratio. Only queries the DOM when the
// ResizeObserver fires, avoiding per-frame JS boundary crossings.
func (w *Window) SyncCanvasSize() {
	if !w.resizeDirty {
		return
	}
	w.resizeDirty = false

	cssW, cssH := w.Size()
	newDPR := js.Global().Get("devicePixelRatio").Float()
	if newDPR < 1 {
		newDPR = 1
	}
	w.dpr = newDPR
	fbW := int(float64(cssW) * w.dpr)
	fbH := int(float64(cssH) * w.dpr)
	if fbW != w.fbWidth || fbH != w.fbHeight {
		w.width = cssW
		w.height = cssH
		w.fbWidth = fbW
		w.fbHeight = fbH
		w.canvas.Set("width", fbW)
		w.canvas.Set("height", fbH)
	}
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
