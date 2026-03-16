//go:build linux || freebsd

// This file provides the CGo-based GLFW API initialization for Linux/BSD.
// GLFW is compiled from vendored source in cglfw/ — no system GLFW install needed.
package glfw

/*
#cgo CFLAGS: -D_GLFW_X11 -D_GNU_SOURCE -Icglfw
#cgo linux LDFLAGS: -lm -ldl -lpthread -lrt -lX11 -lXrandr -lXi -lXcursor -lXinerama
#cgo freebsd LDFLAGS: -lm -lpthread -lX11 -lXrandr -lXi -lXcursor -lXinerama

#define GLFW_INCLUDE_NONE
#include "glfw3.h"
*/
import "C"

import "unsafe"

func initGLFWAPI() error {
	fnGlfwInit = func() int32 {
		return int32(C.glfwInit())
	}
	fnGlfwTerminate = func() {
		C.glfwTerminate()
	}
	fnGlfwWindowHint = func(hint, value int32) {
		C.glfwWindowHint(C.int(hint), C.int(value))
	}
	fnGlfwCreateWindow = func(width, height int32, title *byte, monitor, share uintptr) uintptr {
		return uintptr(unsafe.Pointer(C.glfwCreateWindow(
			C.int(width), C.int(height),
			(*C.char)(unsafe.Pointer(title)),
			(*C.GLFWmonitor)(unsafe.Pointer(monitor)), //nolint:govet // CGo interop
			(*C.GLFWwindow)(unsafe.Pointer(share)),    //nolint:govet // CGo interop
		)))
	}
	fnGlfwDestroyWindow = func(window uintptr) {
		C.glfwDestroyWindow((*C.GLFWwindow)(unsafe.Pointer(window))) //nolint:govet // CGo interop
	}
	fnGlfwWindowShouldClose = func(window uintptr) int32 {
		return int32(C.glfwWindowShouldClose((*C.GLFWwindow)(unsafe.Pointer(window)))) //nolint:govet // CGo interop
	}
	fnGlfwPollEvents = func() {
		C.glfwPollEvents()
	}
	fnGlfwSwapBuffers = func(window uintptr) {
		C.glfwSwapBuffers((*C.GLFWwindow)(unsafe.Pointer(window))) //nolint:govet // CGo interop
	}
	fnGlfwSwapInterval = func(interval int32) {
		C.glfwSwapInterval(C.int(interval))
	}
	fnGlfwMakeContextCurrent = func(window uintptr) {
		C.glfwMakeContextCurrent((*C.GLFWwindow)(unsafe.Pointer(window))) //nolint:govet // CGo interop
	}
	fnGlfwGetWindowSize = func(window uintptr, width, height *int32) {
		C.glfwGetWindowSize((*C.GLFWwindow)(unsafe.Pointer(window)), (*C.int)(unsafe.Pointer(width)), (*C.int)(unsafe.Pointer(height))) //nolint:govet // CGo interop
	}
	fnGlfwSetWindowSize = func(window uintptr, width, height int32) {
		C.glfwSetWindowSize((*C.GLFWwindow)(unsafe.Pointer(window)), C.int(width), C.int(height)) //nolint:govet // CGo interop
	}
	fnGlfwGetFramebufferSize = func(window uintptr, width, height *int32) {
		C.glfwGetFramebufferSize((*C.GLFWwindow)(unsafe.Pointer(window)), (*C.int)(unsafe.Pointer(width)), (*C.int)(unsafe.Pointer(height))) //nolint:govet // CGo interop
	}
	fnGlfwGetWindowPos = func(window uintptr, xpos, ypos *int32) {
		C.glfwGetWindowPos((*C.GLFWwindow)(unsafe.Pointer(window)), (*C.int)(unsafe.Pointer(xpos)), (*C.int)(unsafe.Pointer(ypos))) //nolint:govet // CGo interop
	}
	fnGlfwSetWindowTitle = func(window uintptr, title *byte) {
		C.glfwSetWindowTitle((*C.GLFWwindow)(unsafe.Pointer(window)), (*C.char)(unsafe.Pointer(title))) //nolint:govet // CGo interop
	}
	fnGlfwSetWindowMonitor = func(window uintptr, monitor uintptr, xpos, ypos, width, height, refreshRate int32) {
		C.glfwSetWindowMonitor( //nolint:govet // CGo interop
			(*C.GLFWwindow)(unsafe.Pointer(window)),
			(*C.GLFWmonitor)(unsafe.Pointer(monitor)),
			C.int(xpos), C.int(ypos), C.int(width), C.int(height), C.int(refreshRate),
		)
	}
	fnGlfwSetInputMode = func(window uintptr, mode, value int32) {
		C.glfwSetInputMode((*C.GLFWwindow)(unsafe.Pointer(window)), C.int(mode), C.int(value)) //nolint:govet // CGo interop
	}
	fnGlfwGetCursorPos = func(window uintptr, xpos, ypos *float64) {
		C.glfwGetCursorPos((*C.GLFWwindow)(unsafe.Pointer(window)), (*C.double)(unsafe.Pointer(xpos)), (*C.double)(unsafe.Pointer(ypos))) //nolint:govet // CGo interop
	}
	fnGlfwGetPrimaryMonitor = func() uintptr {
		return uintptr(unsafe.Pointer(C.glfwGetPrimaryMonitor()))
	}
	fnGlfwGetVideoMode = func(monitor uintptr) uintptr {
		return uintptr(unsafe.Pointer(C.glfwGetVideoMode((*C.GLFWmonitor)(unsafe.Pointer(monitor))))) //nolint:govet // CGo interop
	}

	// Callback setters are handled via CGo exports in callbacks_cgo.go.
	// Set them to no-ops here; installCallbacks uses C API directly.
	fnGlfwSetKeyCallback = func(_, _ uintptr) uintptr { return 0 }
	fnGlfwSetCharCallback = func(_, _ uintptr) uintptr { return 0 }
	fnGlfwSetMouseButtonCallback = func(_, _ uintptr) uintptr { return 0 }
	fnGlfwSetCursorPosCallback = func(_, _ uintptr) uintptr { return 0 }
	fnGlfwSetScrollCallback = func(_, _ uintptr) uintptr { return 0 }
	fnGlfwSetFramebufferSizeCallback = func(_, _ uintptr) uintptr { return 0 }

	// Joystick functions.
	fnGlfwJoystickPresent = func(jid int32) int32 {
		return int32(C.glfwJoystickPresent(C.int(jid)))
	}
	fnGlfwGetJoystickAxes = func(jid int32, count *int32) uintptr {
		return uintptr(unsafe.Pointer(C.glfwGetJoystickAxes(C.int(jid), (*C.int)(unsafe.Pointer(count))))) //nolint:govet // CGo interop
	}
	fnGlfwGetJoystickButtons = func(jid int32, count *int32) uintptr {
		return uintptr(unsafe.Pointer(C.glfwGetJoystickButtons(C.int(jid), (*C.int)(unsafe.Pointer(count))))) //nolint:govet // CGo interop
	}

	return nil
}

// cStr converts a Go string to a null-terminated byte pointer for C interop.
func cStr(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// getVideoMode reads the GLFWvidmode struct from a C pointer.
func getVideoMode(ptr uintptr) glfwVideoMode {
	vm := (*C.GLFWvidmode)(unsafe.Pointer(ptr)) //nolint:govet // CGo interop
	return glfwVideoMode{
		Width:       int32(vm.width),
		Height:      int32(vm.height),
		RedBits:     int32(vm.redBits),
		GreenBits:   int32(vm.greenBits),
		BlueBits:    int32(vm.blueBits),
		RefreshRate: int32(vm.refreshRate),
	}
}
