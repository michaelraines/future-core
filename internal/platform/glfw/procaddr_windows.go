//go:build windows

package glfw

import (
	"fmt"

	"golang.org/x/sys/windows"
)

var glfwDLL *windows.LazyDLL

// openGLFWLib loads glfw3.dll on Windows.
func openGLFWLib() error {
	glfwDLL = windows.NewLazyDLL("glfw3.dll")
	if err := glfwDLL.Load(); err != nil {
		return fmt.Errorf("failed to load glfw3.dll: %w", err)
	}
	return nil
}

// getGLFWProcAddr resolves a GLFW function symbol from glfw3.dll.
func getGLFWProcAddr(name string) (uintptr, error) {
	proc := glfwDLL.NewProc(name)
	if err := proc.Find(); err != nil {
		return 0, fmt.Errorf("symbol %s not found: %w", name, err)
	}
	return proc.Addr(), nil
}
