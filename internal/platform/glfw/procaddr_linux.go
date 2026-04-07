//go:build linux || freebsd

package glfw

import (
	"fmt"

	"github.com/ebitengine/purego"
)

// glfwLib holds the loaded GLFW library handle.
var glfwLib uintptr

// openGLFWLib opens the system-installed GLFW shared library.
// Install on Debian/Ubuntu: sudo apt-get install libglfw3-dev
// Install on Fedora: sudo dnf install glfw-devel
// Install on Arch: sudo pacman -S glfw
func openGLFWLib() error {
	names := []string{"libglfw.so.3", "libglfw.so"}

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
	return fmt.Errorf("failed to load GLFW (install libglfw3-dev or equivalent): %w", firstErr)
}

// getGLFWProcAddr resolves a GLFW function symbol from the loaded library.
func getGLFWProcAddr(name string) (uintptr, error) {
	return purego.Dlsym(glfwLib, name)
}
