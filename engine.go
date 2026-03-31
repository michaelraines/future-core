// Package futurerender is a production-grade 2D/3D rendering engine for Go.
//
// The engine provides an API compatible with Ebitengine's game loop model:
// a Game interface with Update(), Draw(), and Layout() methods. The engine
// manages the window, input, audio, and rendering pipeline.
//
// Basic usage:
//
//	type MyGame struct{}
//
//	func (g *MyGame) Update() error { return nil }
//	func (g *MyGame) Draw(screen *futurerender.Image) {}
//	func (g *MyGame) Layout(outsideWidth, outsideHeight int) (int, int) {
//	    return 320, 240
//	}
//
//	func main() {
//	    futurerender.RunGame(&MyGame{})
//	}
package futurerender

import (
	"errors"
	"os"
	"sync"
	"sync/atomic"
)

// Game is the interface that game implementations must satisfy.
// This matches Ebitengine's Game interface for compatibility.
type Game interface {
	// Update is called every tick. Game logic goes here.
	// Return a non-nil error to terminate the game loop.
	Update() error

	// Draw is called every frame. Rendering goes here.
	// The screen image is the render target for the frame.
	Draw(screen *Image)

	// Layout accepts the outside (window) size and returns the logical
	// screen size. The engine scales the logical screen to fit the window.
	Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int)
}

// FocusHandler is an optional interface that Game implementations can
// satisfy to receive focus/blur notifications. On mobile, these correspond
// to the app being foregrounded (OnFocus) or backgrounded (OnBlur).
// Use OnBlur to save game state before the OS may terminate the app.
type FocusHandler interface {
	// OnFocus is called when the application gains focus (foregrounded).
	OnFocus()
	// OnBlur is called when the application loses focus (backgrounded).
	// Save any critical state here — on mobile the app may be killed.
	OnBlur()
}

// LifecycleHandler is an optional interface that Game implementations can
// satisfy to receive lifecycle notifications. OnDispose is called once
// when the game loop exits, before the engine shuts down.
type LifecycleHandler interface {
	// OnDispose is called once when the game loop is about to exit.
	// Use this to release external resources (save files, network connections).
	OnDispose()
}

// ErrTermination is returned from Update() to cleanly exit the game loop.
var ErrTermination = errors.New("game terminated")

// RunGameOptions configures engine behavior at startup. Pass to
// RunGameWithOptions to customize. A nil options value uses defaults.
type RunGameOptions struct {
	// ScreenOrientation locks the screen to a specific orientation.
	// Only effective on mobile platforms; ignored on desktop.
	// Default: OrientationDefault (system decides).
	ScreenOrientation Orientation

	// InitialWindowWidth overrides the default window width (800).
	// Set to 0 to use the default.
	InitialWindowWidth int

	// InitialWindowHeight overrides the default window height (600).
	// Set to 0 to use the default.
	InitialWindowHeight int

	// InitialWindowTitle overrides the default window title.
	// Set to "" to use the default ("Future Render").
	InitialWindowTitle string
}

// Orientation represents a screen orientation preference.
type Orientation int

// Orientation constants.
const (
	// OrientationDefault lets the system decide the orientation.
	OrientationDefault Orientation = iota
	// OrientationPortrait locks to portrait mode.
	OrientationPortrait
	// OrientationLandscape locks to landscape mode.
	OrientationLandscape
)

// RunGame starts the game loop with the given Game implementation.
// This function blocks until the game exits. It must be called from
// the main goroutine on platforms that require it (macOS, iOS).
func RunGame(game Game) error {
	e := newEngine(game)
	return e.run()
}

// RunGameWithOptions starts the game loop with the given Game and options.
// This function blocks until the game exits. It must be called from
// the main goroutine on platforms that require it (macOS, iOS).
func RunGameWithOptions(game Game, opts *RunGameOptions) error {
	applyRunGameOptions(opts)
	e := newEngine(game)
	return e.run()
}

// applyRunGameOptions applies the given options to the pending engine state.
func applyRunGameOptions(opts *RunGameOptions) {
	if opts == nil {
		return
	}
	if opts.InitialWindowWidth > 0 {
		pendingWindowWidth = opts.InitialWindowWidth
	}
	if opts.InitialWindowHeight > 0 {
		pendingWindowHeight = opts.InitialWindowHeight
	}
	if opts.InitialWindowTitle != "" {
		pendingWindowTitle = opts.InitialWindowTitle
	}
	pendingOrientation = opts.ScreenOrientation
}

// SetWindowSize sets the window size in logical pixels.
func SetWindowSize(width, height int) {
	pendingWindowWidth = width
	pendingWindowHeight = height
	if e := getEngine(); e != nil {
		e.setWindowSize(width, height)
	}
}

// SetWindowTitle sets the window title.
func SetWindowTitle(title string) {
	pendingWindowTitle = title
	if e := getEngine(); e != nil {
		e.setWindowTitle(title)
	}
}

// SetFullscreen sets fullscreen mode.
func SetFullscreen(fullscreen bool) {
	if e := getEngine(); e != nil {
		e.setFullscreen(fullscreen)
	}
}

// IsFullscreen returns whether the window is in fullscreen mode.
func IsFullscreen() bool {
	if e := getEngine(); e != nil {
		return e.isFullscreen()
	}
	return false
}

// SetMaxTPS sets the maximum ticks per second. The default is 60.
// Set to 0 for uncapped TPS (sync to frame rate).
func SetMaxTPS(tps int) {
	if tps < 0 {
		tps = 0
	}
	maxTPS.Store(int64(tps))
}

// MaxTPS returns the current maximum ticks per second.
func MaxTPS() int {
	return int(maxTPS.Load())
}

// SetVsyncEnabled enables or disables vertical synchronization.
func SetVsyncEnabled(enabled bool) {
	if e := getEngine(); e != nil {
		e.setVSync(enabled)
	}
}

// IsVsyncEnabled returns whether VSync is enabled.
func IsVsyncEnabled() bool {
	if e := getEngine(); e != nil {
		return e.isVSync()
	}
	return true
}

// CurrentFPS returns the current frames per second.
func CurrentFPS() float64 {
	if e := getEngine(); e != nil {
		return e.currentFPS()
	}
	return 0
}

// CurrentTPS returns the current ticks per second.
func CurrentTPS() float64 {
	if e := getEngine(); e != nil {
		return e.currentTPS()
	}
	return 0
}

// SetCursorMode sets the cursor visibility and lock mode.
func SetCursorMode(mode CursorMode) {
	if e := getEngine(); e != nil {
		e.setCursorMode(mode)
	}
}

// CursorMode constants.
type CursorMode int

// CursorMode constants.
const (
	CursorModeVisible  CursorMode = iota // Normal cursor
	CursorModeHidden                     // Hidden cursor
	CursorModeCaptured                   // Hidden and locked to window
)

// Backend returns the current rendering backend name.
// Before the engine is running, this returns the value of the
// FUTURE_CORE_BACKEND environment variable (or "auto").
// After the engine starts, it returns the actual resolved backend name
// (e.g. "opengl", "soft").
func Backend() string {
	if v := resolvedBackend.Load(); v != "" {
		return v
	}
	return backendName()
}

// backendName returns the backend name from the environment or default.
func backendName() string {
	if v := os.Getenv("FUTURE_CORE_BACKEND"); v != "" {
		return v
	}
	return "auto"
}

// resolvedBackend stores the name of the backend that was actually selected
// at engine startup. Empty until the engine resolves a backend.
var resolvedBackend syncString

// DeviceScaleFactor returns the device pixel ratio.
func DeviceScaleFactor() float64 {
	if e := getEngine(); e != nil {
		return e.deviceScaleFactor()
	}
	return 1.0
}

// SetScreenClearedEveryFrame controls whether the screen is cleared at the
// start of each frame. The default is true. When set to false, the previous
// frame's content is preserved (useful for paint-like applications).
func SetScreenClearedEveryFrame(cleared bool) {
	screenClearedEveryFrame.Store(cleared)
}

// IsScreenClearedEveryFrame returns whether the screen is cleared each frame.
func IsScreenClearedEveryFrame() bool {
	return screenClearedEveryFrame.Load()
}

// SetScreenOrientation requests a specific screen orientation.
// Only effective on mobile platforms; ignored on desktop.
func SetScreenOrientation(o Orientation) {
	pendingOrientation = o
	// On mobile, apply immediately if the engine is running.
	// The platform layer checks this value each frame.
}

// ScreenOrientation returns the current screen orientation preference.
func ScreenOrientation() Orientation {
	return pendingOrientation
}

// --- Soft keyboard API ---

// ShowSoftKeyboard requests the platform to show the software keyboard.
// Only effective on mobile platforms; no-op on desktop.
func ShowSoftKeyboard() {
	// Implemented per-platform. On Android, this sends a JNI call.
}

// HideSoftKeyboard requests the platform to hide the software keyboard.
// Only effective on mobile platforms; no-op on desktop.
func HideSoftKeyboard() {
	// Implemented per-platform. On Android, this sends a JNI call.
}

// --- Engine internals ---

var (
	globalEnginePtr atomic.Pointer[engine]
	maxTPS          atomic.Int64

	// screenClearedEveryFrame controls whether the screen is cleared each frame.
	screenClearedEveryFrame atomic.Bool

	// Pre-run configuration stored as package-level state so that
	// SetWindowSize/SetWindowTitle can be called before RunGame.
	pendingWindowTitle  = "Future Render"
	pendingWindowWidth  = 800
	pendingWindowHeight = 600
	pendingOrientation  Orientation
)

// getEngine returns the current engine, or nil if not initialized.
func getEngine() *engine { return globalEnginePtr.Load() }

// setEngine stores the engine atomically.
func setEngine(e *engine) { globalEnginePtr.Store(e) }

// syncString is a simple thread-safe string value.
type syncString struct {
	mu  sync.RWMutex
	val string
}

func (s *syncString) Store(v string) {
	s.mu.Lock()
	s.val = v
	s.mu.Unlock()
}

func (s *syncString) Load() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.val
}

func init() {
	maxTPS.Store(60)
	screenClearedEveryFrame.Store(true)
}

// engine is defined per-platform in engine_stub.go / engine_glfw.go.
// Common fields and methods are here, platform-specific in the build-tagged files.

func newEngine(game Game) *engine {
	e := newPlatformEngine(game)
	setEngine(e)
	return e
}
