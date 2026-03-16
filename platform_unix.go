//go:build linux || freebsd

package futurerender

import (
	"github.com/michaelraines/future-render/internal/platform"
	glfwplatform "github.com/michaelraines/future-render/internal/platform/glfw"
)

// newPlatformWindow creates a GLFW window (vendored C source, compiled via CGo).
func newPlatformWindow() platform.Window {
	return glfwplatform.New()
}
