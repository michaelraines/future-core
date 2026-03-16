//go:build darwin

package futurerender

import (
	"github.com/michaelraines/future-render/internal/platform"
	"github.com/michaelraines/future-render/internal/platform/cocoa"
)

// newPlatformWindow creates the native macOS Cocoa window (no GLFW needed).
func newPlatformWindow() platform.Window {
	return cocoa.New()
}
