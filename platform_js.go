//go:build js

package futurerender

import (
	"github.com/michaelraines/future-core/internal/platform"
	"github.com/michaelraines/future-core/internal/platform/web"
)

// newPlatformWindow creates a browser-based window backed by a canvas element.
func newPlatformWindow() platform.Window {
	return web.New()
}
