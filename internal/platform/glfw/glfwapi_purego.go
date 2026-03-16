//go:build darwin || windows

// This file provides the purego-based GLFW API initialization for platforms
// that load GLFW as a shared library at runtime (macOS, Windows).
package glfw

import (
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
)

func initGLFWAPI() error {
	if err := openGLFWLib(); err != nil {
		return err
	}

	must := func(fn interface{}, name string) error {
		addr, serr := getGLFWProcAddr(name)
		if serr != nil {
			return fmt.Errorf("glfw: symbol %s: %w", name, serr)
		}
		purego.RegisterFunc(fn, addr)
		return nil
	}

	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnGlfwInit, "glfwInit"},
		{&fnGlfwTerminate, "glfwTerminate"},
		{&fnGlfwWindowHint, "glfwWindowHint"},
		{&fnGlfwCreateWindow, "glfwCreateWindow"},
		{&fnGlfwDestroyWindow, "glfwDestroyWindow"},
		{&fnGlfwWindowShouldClose, "glfwWindowShouldClose"},
		{&fnGlfwPollEvents, "glfwPollEvents"},
		{&fnGlfwSwapBuffers, "glfwSwapBuffers"},
		{&fnGlfwSwapInterval, "glfwSwapInterval"},
		{&fnGlfwMakeContextCurrent, "glfwMakeContextCurrent"},
		{&fnGlfwGetWindowSize, "glfwGetWindowSize"},
		{&fnGlfwSetWindowSize, "glfwSetWindowSize"},
		{&fnGlfwGetFramebufferSize, "glfwGetFramebufferSize"},
		{&fnGlfwGetWindowPos, "glfwGetWindowPos"},
		{&fnGlfwSetWindowTitle, "glfwSetWindowTitle"},
		{&fnGlfwSetWindowMonitor, "glfwSetWindowMonitor"},
		{&fnGlfwSetInputMode, "glfwSetInputMode"},
		{&fnGlfwGetCursorPos, "glfwGetCursorPos"},
		{&fnGlfwGetPrimaryMonitor, "glfwGetPrimaryMonitor"},
		{&fnGlfwGetVideoMode, "glfwGetVideoMode"},
		{&fnGlfwSetKeyCallback, "glfwSetKeyCallback"},
		{&fnGlfwSetCharCallback, "glfwSetCharCallback"},
		{&fnGlfwSetMouseButtonCallback, "glfwSetMouseButtonCallback"},
		{&fnGlfwSetCursorPosCallback, "glfwSetCursorPosCallback"},
		{&fnGlfwSetScrollCallback, "glfwSetScrollCallback"},
		{&fnGlfwSetFramebufferSizeCallback, "glfwSetFramebufferSizeCallback"},
		{&fnGlfwJoystickPresent, "glfwJoystickPresent"},
		{&fnGlfwGetJoystickAxes, "glfwGetJoystickAxes"},
		{&fnGlfwGetJoystickButtons, "glfwGetJoystickButtons"},
	} {
		if ferr := must(e.fn, e.name); ferr != nil {
			return ferr
		}
	}

	return nil
}

// cStr converts a Go string to a null-terminated byte pointer.
// Safe when passed directly as a purego function argument (pinned during call).
func cStr(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// getVideoMode reads the GLFWvidmode struct from a pointer.
func getVideoMode(ptr uintptr) glfwVideoMode {
	return *(*glfwVideoMode)(unsafe.Pointer(ptr)) //nolint:govet // purego interop: reading struct from C pointer
}
