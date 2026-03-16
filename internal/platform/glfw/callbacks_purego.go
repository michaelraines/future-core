//go:build darwin || windows

package glfw

import (
	"github.com/ebitengine/purego"

	"github.com/michaelraines/future-render/internal/platform"
)

// installCallbacks registers GLFW event callbacks via purego.NewCallback.
func (w *Window) installCallbacks() {
	activeWindows[w.win] = w

	w.keyCB = purego.NewCallback(func(window uintptr, key, scancode, action, mods int32) {
		win := activeWindows[window]
		if win == nil || win.handler == nil {
			return
		}
		win.handler.OnKeyEvent(platform.KeyEvent{
			Key:    mapKey(key),
			Action: mapAction(action),
			Mods:   mapMods(mods),
		})
	})
	fnGlfwSetKeyCallback(w.win, w.keyCB)

	w.charCB = purego.NewCallback(func(window uintptr, codepoint uint32) {
		win := activeWindows[window]
		if win == nil || win.handler == nil {
			return
		}
		win.handler.OnCharEvent(rune(codepoint))
	})
	fnGlfwSetCharCallback(w.win, w.charCB)

	w.mouseButtonCB = purego.NewCallback(func(window uintptr, button, action, mods int32) {
		win := activeWindows[window]
		if win == nil || win.handler == nil {
			return
		}
		var x, y float64
		fnGlfwGetCursorPos(window, &x, &y)
		win.handler.OnMouseButtonEvent(platform.MouseButtonEvent{
			Button: platform.MouseButton(button),
			Action: mapAction(action),
			X:      x,
			Y:      y,
			Mods:   mapMods(mods),
		})
	})
	fnGlfwSetMouseButtonCallback(w.win, w.mouseButtonCB)

	w.cursorPosCB = purego.NewCallback(func(window uintptr, x, y float64) {
		win := activeWindows[window]
		if win == nil || win.handler == nil {
			return
		}
		var dx, dy float64
		if win.hasPrevCursor {
			dx = x - win.prevCursorX
			dy = y - win.prevCursorY
		}
		win.prevCursorX = x
		win.prevCursorY = y
		win.hasPrevCursor = true
		win.handler.OnMouseMoveEvent(platform.MouseMoveEvent{
			X: x, Y: y, DX: dx, DY: dy,
		})
	})
	fnGlfwSetCursorPosCallback(w.win, w.cursorPosCB)

	w.scrollCB = purego.NewCallback(func(window uintptr, xoff, yoff float64) {
		win := activeWindows[window]
		if win == nil || win.handler == nil {
			return
		}
		win.handler.OnMouseScrollEvent(platform.MouseScrollEvent{
			DX: xoff, DY: yoff,
		})
	})
	fnGlfwSetScrollCallback(w.win, w.scrollCB)

	w.framebufferCB = purego.NewCallback(func(window uintptr, width, height int32) {
		win := activeWindows[window]
		if win == nil || win.handler == nil {
			return
		}
		win.handler.OnResizeEvent(int(width), int(height))
	})
	fnGlfwSetFramebufferSizeCallback(w.win, w.framebufferCB)
}
