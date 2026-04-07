//go:build (linux || freebsd) && !android

package futurerender

import (
	"github.com/michaelraines/future-core/internal/platform"
	glfwplatform "github.com/michaelraines/future-core/internal/platform/glfw"
)

// newPlatformWindow creates a GLFW window (system library loaded via purego).
func newPlatformWindow() platform.Window {
	return glfwplatform.New()
}
