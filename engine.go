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

// ErrTermination is returned from Update() to cleanly exit the game loop.
var ErrTermination = errors.New("game terminated")

// RunGame starts the game loop with the given Game implementation.
// This function blocks until the game exits. It must be called from
// the main goroutine on platforms that require it (macOS, iOS).
func RunGame(game Game) error {
	e := newEngine(game)
	return e.run()
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
// This is determined by the FUTURE_RENDER_BACKEND environment variable
// or defaults to "auto" (selects the best available backend).
// Supported values: "auto", "opengl", "vulkan", "metal", "webgl",
// "webgpu", "dx12", "soft".
func Backend() string {
	return backendName()
}

// backendName returns the backend name from the environment or default.
func backendName() string {
	if v := os.Getenv("FUTURE_RENDER_BACKEND"); v != "" {
		return v
	}
	return "auto"
}

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
)

// getEngine returns the current engine, or nil if not initialized.
func getEngine() *engine { return globalEnginePtr.Load() }

// setEngine stores the engine atomically.
func setEngine(e *engine) { globalEnginePtr.Store(e) }

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
