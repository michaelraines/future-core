//go:build js

package web

import (
	"syscall/js"

	"github.com/michaelraines/future-core/internal/platform"
)

// registerInputListeners sets up DOM event listeners for keyboard, mouse, and touch.
func (w *Window) registerInputListeners() {
	// Keyboard events on document (not canvas) to catch all key input.
	w.addListener(w.doc, "keydown", w.onKeyDown)
	w.addListener(w.doc, "keyup", w.onKeyUp)

	// Mouse events on canvas.
	w.addListener(w.canvas, "mousedown", w.onMouseDown)
	w.addListener(w.canvas, "mouseup", w.onMouseUp)
	w.addListener(w.canvas, "mousemove", w.onMouseMove)
	w.addListener(w.canvas, "wheel", w.onWheel)

	// Prevent context menu on right-click.
	w.addListener(w.canvas, "contextmenu", func(_ js.Value, args []js.Value) {
		if len(args) > 0 {
			args[0].Call("preventDefault")
		}
	})

	// Touch events on canvas.
	w.addListener(w.canvas, "touchstart", w.onTouchStart)
	w.addListener(w.canvas, "touchmove", w.onTouchMove)
	w.addListener(w.canvas, "touchend", w.onTouchEnd)
	w.addListener(w.canvas, "touchcancel", w.onTouchEnd)

	// Resize observer.
	w.addListener(js.Global(), "resize", w.onResize)
}

// addListener registers a JS event listener and tracks it for cleanup.
func (w *Window) addListener(target js.Value, event string, fn func(this js.Value, args []js.Value)) {
	jsFn := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		fn(this, args)
		return nil
	})
	target.Call("addEventListener", event, jsFn)
	w.listeners = append(w.listeners, jsFn)
}

// --- Keyboard ---

func (w *Window) onKeyDown(_ js.Value, args []js.Value) {
	if w.handler == nil || len(args) == 0 {
		return
	}
	evt := args[0]
	// Prevent default for game keys (arrows, space, tab) to avoid scrolling.
	code := evt.Get("code").String()
	if shouldPreventDefault(code) {
		evt.Call("preventDefault")
	}
	key := mapKeyCode(code)
	if key < 0 {
		return
	}
	action := platform.ActionPress
	if evt.Get("repeat").Bool() {
		action = platform.ActionRepeat
	}
	w.handler.OnKeyEvent(platform.KeyEvent{
		Key:    key,
		Action: action,
		Mods:   mapModifiers(evt),
	})
	// Character input for text.
	keyStr := evt.Get("key").String()
	if len(keyStr) == 1 {
		w.handler.OnCharEvent(rune(keyStr[0]))
	}
}

func (w *Window) onKeyUp(_ js.Value, args []js.Value) {
	if w.handler == nil || len(args) == 0 {
		return
	}
	evt := args[0]
	key := mapKeyCode(evt.Get("code").String())
	if key < 0 {
		return
	}
	w.handler.OnKeyEvent(platform.KeyEvent{
		Key:    key,
		Action: platform.ActionRelease,
		Mods:   mapModifiers(evt),
	})
}

// --- Mouse ---

func (w *Window) onMouseDown(_ js.Value, args []js.Value) {
	if w.handler == nil || len(args) == 0 {
		return
	}
	evt := args[0]
	x, y := w.canvasCoords(evt)
	w.handler.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: mapMouseButton(evt.Get("button").Int()),
		Action: platform.ActionPress,
		X:      x, Y: y,
		Mods: mapModifiers(evt),
	})
}

func (w *Window) onMouseUp(_ js.Value, args []js.Value) {
	if w.handler == nil || len(args) == 0 {
		return
	}
	evt := args[0]
	x, y := w.canvasCoords(evt)
	w.handler.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: mapMouseButton(evt.Get("button").Int()),
		Action: platform.ActionRelease,
		X:      x, Y: y,
		Mods: mapModifiers(evt),
	})
}

func (w *Window) onMouseMove(_ js.Value, args []js.Value) {
	if w.handler == nil || len(args) == 0 {
		return
	}
	evt := args[0]
	x, y := w.canvasCoords(evt)

	dx := evt.Get("movementX").Float()
	dy := evt.Get("movementY").Float()

	w.lastMouseX = x
	w.lastMouseY = y
	w.mouseInited = true

	w.handler.OnMouseMoveEvent(platform.MouseMoveEvent{
		X: x, Y: y,
		DX: dx, DY: dy,
	})
}

func (w *Window) onWheel(_ js.Value, args []js.Value) {
	if w.handler == nil || len(args) == 0 {
		return
	}
	evt := args[0]
	evt.Call("preventDefault")
	// Normalize scroll delta (browsers report different scales).
	dx := -evt.Get("deltaX").Float() / 100
	dy := -evt.Get("deltaY").Float() / 100
	w.handler.OnMouseScrollEvent(platform.MouseScrollEvent{DX: dx, DY: dy})
}

// --- Touch ---

func (w *Window) onTouchStart(_ js.Value, args []js.Value) {
	if w.handler == nil || len(args) == 0 {
		return
	}
	evt := args[0]
	evt.Call("preventDefault")
	touches := evt.Get("changedTouches")
	for i := 0; i < touches.Length(); i++ {
		t := touches.Index(i)
		x, y := w.touchCoords(t)
		w.handler.OnTouchEvent(platform.TouchEvent{
			ID:     t.Get("identifier").Int(),
			Action: platform.ActionPress,
			X:      x, Y: y,
			Pressure: t.Get("force").Float(),
		})
	}
}

func (w *Window) onTouchMove(_ js.Value, args []js.Value) {
	if w.handler == nil || len(args) == 0 {
		return
	}
	evt := args[0]
	evt.Call("preventDefault")
	touches := evt.Get("changedTouches")
	for i := 0; i < touches.Length(); i++ {
		t := touches.Index(i)
		x, y := w.touchCoords(t)
		w.handler.OnTouchEvent(platform.TouchEvent{
			ID:     t.Get("identifier").Int(),
			Action: platform.ActionRepeat, // Move = repeat
			X:      x, Y: y,
			Pressure: t.Get("force").Float(),
		})
	}
}

func (w *Window) onTouchEnd(_ js.Value, args []js.Value) {
	if w.handler == nil || len(args) == 0 {
		return
	}
	evt := args[0]
	touches := evt.Get("changedTouches")
	for i := 0; i < touches.Length(); i++ {
		t := touches.Index(i)
		x, y := w.touchCoords(t)
		w.handler.OnTouchEvent(platform.TouchEvent{
			ID:     t.Get("identifier").Int(),
			Action: platform.ActionRelease,
			X:      x, Y: y,
		})
	}
}

// --- Resize ---

func (w *Window) onResize(_ js.Value, _ []js.Value) {
	if w.handler == nil {
		return
	}
	// Update DPR in case the window moved to a different display.
	w.dpr = js.Global().Get("devicePixelRatio").Float()
	if w.dpr < 1 {
		w.dpr = 1
	}
	// Resize canvas to match window inner dimensions.
	innerW := js.Global().Get("innerWidth").Int()
	innerH := js.Global().Get("innerHeight").Int()
	if innerW > 0 && innerH > 0 {
		w.width = innerW
		w.height = innerH
		w.fbWidth = int(float64(innerW) * w.dpr)
		w.fbHeight = int(float64(innerH) * w.dpr)
		w.canvas.Set("width", w.fbWidth)
		w.canvas.Set("height", w.fbHeight)
		w.handler.OnResizeEvent(innerW, innerH)
	}
}

// --- Coordinate helpers ---

func (w *Window) canvasCoords(evt js.Value) (float64, float64) {
	rect := w.canvas.Call("getBoundingClientRect")
	x := evt.Get("clientX").Float() - rect.Get("left").Float()
	y := evt.Get("clientY").Float() - rect.Get("top").Float()
	return x, y
}

func (w *Window) touchCoords(touch js.Value) (float64, float64) {
	rect := w.canvas.Call("getBoundingClientRect")
	x := touch.Get("clientX").Float() - rect.Get("left").Float()
	y := touch.Get("clientY").Float() - rect.Get("top").Float()
	return x, y
}

// shouldPreventDefault returns true for key codes that should not trigger
// browser default behavior (scrolling, tab switching, etc.).
func shouldPreventDefault(code string) bool {
	switch code {
	case "ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight",
		"Space", "Tab", "Backspace":
		return true
	}
	return false
}

// mapModifiers extracts modifier flags from a keyboard/mouse event.
func mapModifiers(evt js.Value) platform.Modifier {
	var m platform.Modifier
	if evt.Get("shiftKey").Bool() {
		m |= platform.ModShift
	}
	if evt.Get("ctrlKey").Bool() {
		m |= platform.ModControl
	}
	if evt.Get("altKey").Bool() {
		m |= platform.ModAlt
	}
	if evt.Get("metaKey").Bool() {
		m |= platform.ModSuper
	}
	return m
}

// mapMouseButton converts a DOM button index to platform.MouseButton.
func mapMouseButton(button int) platform.MouseButton {
	switch button {
	case 0:
		return platform.MouseButtonLeft
	case 1:
		return platform.MouseButtonMiddle
	case 2:
		return platform.MouseButtonRight
	case 3:
		return platform.MouseButton4
	case 4:
		return platform.MouseButton5
	default:
		return platform.MouseButtonLeft
	}
}
