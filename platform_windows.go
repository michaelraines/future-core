//go:build windows

package futurerender

import (
	"github.com/michaelraines/future-render/internal/platform"
	"github.com/michaelraines/future-render/internal/platform/win32"
)

// newPlatformWindow creates the native Win32 window (no GLFW needed).
func newPlatformWindow() platform.Window {
	return win32.New()
}
