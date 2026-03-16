//go:build darwin || linux || freebsd

package glfw

import (
	"fmt"
	"runtime"

	"github.com/ebitengine/purego"
)

// glfwLib holds the loaded GLFW library handle.
var glfwLib uintptr

// openGLFWLib opens the platform-specific GLFW shared library using purego.
func openGLFWLib() error {
	var names []string
	switch runtime.GOOS {
	case "darwin":
		names = []string{"libglfw.3.dylib", "libglfw.dylib"}
	default: // linux, freebsd, etc.
		names = []string{"libglfw.so.3", "libglfw.so"}
	}

	var firstErr error
	for _, name := range names {
		h, err := purego.Dlopen(name, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err == nil {
			glfwLib = h
			return nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return fmt.Errorf("failed to load GLFW: %w", firstErr)
}

// getGLFWProcAddr resolves a GLFW function symbol from the loaded library.
func getGLFWProcAddr(name string) (uintptr, error) {
	return purego.Dlsym(glfwLib, name)
}
