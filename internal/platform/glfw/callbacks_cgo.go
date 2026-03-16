//go:build linux || freebsd

package glfw

/*
#cgo CFLAGS: -Icglfw
#define GLFW_INCLUDE_NONE
#include "glfw3.h"

// Forward declarations for Go callback functions.
extern void goGLFWKeyCallback(GLFWwindow*, int, int, int, int);
extern void goGLFWCharCallback(GLFWwindow*, unsigned int);
extern void goGLFWMouseButtonCallback(GLFWwindow*, int, int, int);
extern void goGLFWCursorPosCallback(GLFWwindow*, double, double);
extern void goGLFWScrollCallback(GLFWwindow*, double, double);
extern void goGLFWFramebufferSizeCallback(GLFWwindow*, int, int);

// C helper functions that register the Go callbacks with GLFW.
static void cSetKeyCallback(GLFWwindow* w) {
    glfwSetKeyCallback(w, goGLFWKeyCallback);
}
static void cSetCharCallback(GLFWwindow* w) {
    glfwSetCharCallback(w, goGLFWCharCallback);
}
static void cSetMouseButtonCallback(GLFWwindow* w) {
    glfwSetMouseButtonCallback(w, goGLFWMouseButtonCallback);
}
static void cSetCursorPosCallback(GLFWwindow* w) {
    glfwSetCursorPosCallback(w, goGLFWCursorPosCallback);
}
static void cSetScrollCallback(GLFWwindow* w) {
    glfwSetScrollCallback(w, goGLFWScrollCallback);
}
static void cSetFramebufferSizeCallback(GLFWwindow* w) {
    glfwSetFramebufferSizeCallback(w, goGLFWFramebufferSizeCallback);
}
*/
import "C"

import (
	"unsafe"

	"github.com/michaelraines/future-render/internal/platform"
)

//export goGLFWKeyCallback
func goGLFWKeyCallback(window *C.GLFWwindow, key, scancode, action, mods C.int) {
	win := activeWindows[uintptr(unsafe.Pointer(window))]
	if win == nil || win.handler == nil {
		return
	}
	win.handler.OnKeyEvent(platform.KeyEvent{
		Key:    mapKey(int32(key)),
		Action: mapAction(int32(action)),
		Mods:   mapMods(int32(mods)),
	})
}

//export goGLFWCharCallback
func goGLFWCharCallback(window *C.GLFWwindow, codepoint C.uint) {
	win := activeWindows[uintptr(unsafe.Pointer(window))]
	if win == nil || win.handler == nil {
		return
	}
	win.handler.OnCharEvent(rune(codepoint))
}

//export goGLFWMouseButtonCallback
func goGLFWMouseButtonCallback(window *C.GLFWwindow, button, action, mods C.int) {
	win := activeWindows[uintptr(unsafe.Pointer(window))]
	if win == nil || win.handler == nil {
		return
	}
	var x, y C.double
	C.glfwGetCursorPos(window, &x, &y)
	win.handler.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: platform.MouseButton(button),
		Action: mapAction(int32(action)),
		X:      float64(x),
		Y:      float64(y),
		Mods:   mapMods(int32(mods)),
	})
}

//export goGLFWCursorPosCallback
func goGLFWCursorPosCallback(window *C.GLFWwindow, x, y C.double) {
	win := activeWindows[uintptr(unsafe.Pointer(window))]
	if win == nil || win.handler == nil {
		return
	}
	gx, gy := float64(x), float64(y)
	var dx, dy float64
	if win.hasPrevCursor {
		dx = gx - win.prevCursorX
		dy = gy - win.prevCursorY
	}
	win.prevCursorX = gx
	win.prevCursorY = gy
	win.hasPrevCursor = true
	win.handler.OnMouseMoveEvent(platform.MouseMoveEvent{
		X: gx, Y: gy, DX: dx, DY: dy,
	})
}

//export goGLFWScrollCallback
func goGLFWScrollCallback(window *C.GLFWwindow, xoff, yoff C.double) {
	win := activeWindows[uintptr(unsafe.Pointer(window))]
	if win == nil || win.handler == nil {
		return
	}
	win.handler.OnMouseScrollEvent(platform.MouseScrollEvent{
		DX: float64(xoff), DY: float64(yoff),
	})
}

//export goGLFWFramebufferSizeCallback
func goGLFWFramebufferSizeCallback(window *C.GLFWwindow, width, height C.int) {
	win := activeWindows[uintptr(unsafe.Pointer(window))]
	if win == nil || win.handler == nil {
		return
	}
	win.handler.OnResizeEvent(int(width), int(height))
}

// installCallbacks registers GLFW event callbacks via CGo exports.
func (w *Window) installCallbacks() {
	activeWindows[w.win] = w
	cwin := (*C.GLFWwindow)(unsafe.Pointer(w.win)) //nolint:govet // CGo interop
	C.cSetKeyCallback(cwin)
	C.cSetCharCallback(cwin)
	C.cSetMouseButtonCallback(cwin)
	C.cSetCursorPosCallback(cwin)
	C.cSetScrollCallback(cwin)
	C.cSetFramebufferSizeCallback(cwin)
}
