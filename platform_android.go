//go:build android

package futurerender

import (
	"github.com/michaelraines/future-core/internal/platform"
	"github.com/michaelraines/future-core/internal/platform/android"
)

// newPlatformWindow creates the Android platform window backed by
// golang.org/x/mobile/app for lifecycle, input, and display management.
func newPlatformWindow() platform.Window {
	return android.New()
}
