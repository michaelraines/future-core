//go:build darwin || linux || freebsd

package gl

import (
	"fmt"
	"runtime"

	"github.com/ebitengine/purego"
)

// lib holds the loaded OpenGL library handle.
var lib uintptr

// openGLLib opens the platform-specific OpenGL shared library using purego.
func openGLLib() error {
	var names []string
	switch runtime.GOOS {
	case "darwin":
		names = []string{"/System/Library/Frameworks/OpenGL.framework/OpenGL"}
	default: // linux, freebsd, etc.
		names = []string{"libGL.so.1", "libGL.so"}
	}

	var firstErr error
	for _, name := range names {
		h, err := purego.Dlopen(name, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err == nil {
			lib = h
			return nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return fmt.Errorf("failed to load OpenGL: %w", firstErr)
}

// getProcAddr resolves an OpenGL function symbol from the loaded library.
func getProcAddr(name string) (uintptr, error) {
	return purego.Dlsym(lib, name)
}
