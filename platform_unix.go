//go:build (linux || freebsd) && !android

package futurerender

import (
	"github.com/michaelraines/future-core/internal/platform"
	glfwplatform "github.com/michaelraines/future-core/internal/platform/glfw"
)

// newPlatformWindow creates a GLFW window (vendored C source, compiled via CGo).
func newPlatformWindow() platform.Window {
	return glfwplatform.New()
}
