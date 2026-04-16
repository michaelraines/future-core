// Package futurecoreview is the JNI-callable surface for the
// future-core Android engine when it is embedded in a host Java
// Activity via a gomobile-bound AAR.
//
// The host Activity (or its FutureCoreView wrapper, see
// cmd/futurecoremobile/_files/FutureCoreView.java) calls the
// functions in this package from the Android render thread and UI
// thread. Each function is a thin trampoline into the private
// futurerender engine — see engine_android_embedded.go and
// engine_android_embedded_api.go in the parent package.
//
// This package is NOT used by the pure-Go NativeActivity build
// (build tag: futurecore_nativeactivity). That path runs the engine
// directly via futurerender.RunGame.
//
// Platform split:
//
//	futurecoreview.go          — shared state (this file, all tags)
//	futurecoreview_android.go  — real engine-delegating impls (tag: android)
//	futurecoreview_stub.go     — no-op impls for non-android
//	                             (so unit tests and IDE tooling
//	                             work on desktop without build tags)
package futurecoreview

import (
	"sync"

	futurerender "github.com/michaelraines/future-core"
)

// Package-level state, shared by android and stub impls. mu guards
// mutations from both the render thread (Tick, SetSurface,
// ClearSurface) and the UI thread (Layout, Suspend, Resume, input
// dispatch) so host code can call us from any thread.
var (
	mu   sync.Mutex
	game futurerender.Game
	opts *futurerender.RunGameOptions

	// Current logical screen dimensions reported by the host via Layout.
	widthPx, heightPx int
	pixelsPerPt       float32
)
