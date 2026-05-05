//go:build android && !futurecore_nativeactivity

package futurerender

import (
	"errors"
	"sync"

	platandroid "github.com/michaelraines/future-core/internal/platform/android"
)

// errEmbeddedEngineUnset is returned from AndroidBootstrap /
// AndroidEnsureDevice when called before SetAndroidGame. Indicates
// a lifecycle ordering bug in the host Activity.
var errEmbeddedEngineUnset = errors.New(
	"futurerender: android embedded engine not set — " +
		"call SetAndroidGame before AndroidBootstrap")

// Public API used by the mobile/futurecoreview JNI trampolines to
// drive the engine from a host Java Activity. The embedded build
// doesn't own the main thread (Java does) so we expose package-level
// functions over a global *engine singleton instead of running the
// engine loop ourselves.
//
// Lifecycle the Java side is expected to call:
//
//	// Activity.onCreate
//	futurerender.SetAndroidGame(game)
//	futurerender.AndroidBootstrap()
//
//	// SurfaceHolder.surfaceCreated
//	futurerender.AndroidSetSurface(nativeWindow)  // ← from ANativeWindow_fromSurface
//
//	// SurfaceHolder.surfaceChanged
//	futurerender.AndroidLayout(widthPx, heightPx)
//	futurerender.AndroidEnsureDevice()
//
//	// Choreographer.doFrame → render thread
//	futurerender.AndroidTick()
//
//	// SurfaceHolder.surfaceDestroyed
//	futurerender.AndroidClearSurface()
//
//	// Activity.onPause / onResume
//	futurerender.AndroidSuspend() / AndroidResume()
//
//	// Activity.onDestroy
//	futurerender.AndroidDispose()

var (
	embeddedMu     sync.Mutex
	embeddedEngine *engine
	embeddedOpts   *RunGameOptions
)

// SetAndroidGame registers the game that AndroidTick will drive. If
// an engine already exists for a previous game, the game is swapped
// in place — the engine itself is reused so GPU resources aren't
// rebuilt. Safe to call from any thread; the engine's own methods
// are thread-safe where needed.
func SetAndroidGame(game Game) {
	embeddedMu.Lock()
	defer embeddedMu.Unlock()
	if embeddedEngine == nil {
		embeddedEngine = newPlatformEngine(game)
		// Register with globalEnginePtr so the public CurrentFPS /
		// CurrentTPS / CurrentBackend accessors (used by the host
		// app's debug HUD via libs/rendering/futurecore/window.go)
		// see the live engine instead of nil. Without this the
		// embedded Android path runs cleanly but every public
		// counter accessor returns 0.
		setEngine(embeddedEngine)
		return
	}
	embeddedEngine.game = game
}

// SetAndroidOptions stores RunGameOptions (orientation, initial size)
// that the first AndroidBootstrap call consults. Takes effect on the
// next engine creation; no-op if the engine is already running.
func SetAndroidOptions(opts *RunGameOptions) {
	embeddedMu.Lock()
	defer embeddedMu.Unlock()
	embeddedOpts = opts
	if embeddedEngine != nil && opts != nil {
		if opts.InitialWindowWidth > 0 {
			embeddedEngine.windowW = opts.InitialWindowWidth
		}
		if opts.InitialWindowHeight > 0 {
			embeddedEngine.windowH = opts.InitialWindowHeight
		}
	}
}

// AndroidBootstrap performs one-time platform window + input setup.
// Call from the host Activity's onCreate after SetAndroidGame.
// Returns any error the engine raises during window creation;
// subsequent calls are no-ops. Does NOT create the rendering device
// — that happens in AndroidEnsureDevice once a surface is available.
func AndroidBootstrap() error {
	embeddedMu.Lock()
	defer embeddedMu.Unlock()
	if embeddedEngine == nil {
		return errEmbeddedEngineUnset
	}
	return embeddedEngine.Bootstrap()
}

// AndroidSetSurface hands the host's current ANativeWindow pointer
// to the engine. The pointer must stay valid until AndroidClearSurface.
func AndroidSetSurface(nativeWindow uintptr) {
	embeddedMu.Lock()
	defer embeddedMu.Unlock()
	if embeddedEngine == nil {
		return
	}
	embeddedEngine.HandleSurface(nativeWindow)
}

// AndroidClearSurface releases the engine's ANativeWindow reference
// before the host calls ANativeWindow_release. The host MUST wait
// for this call to return before releasing the window.
func AndroidClearSurface() {
	embeddedMu.Lock()
	defer embeddedMu.Unlock()
	if embeddedEngine == nil {
		return
	}
	embeddedEngine.ClearSurface()
}

// AndroidLayout records host surface dimensions. Engine uses them
// on the next Tick to compute projection and viewport; they're also
// the trigger for AndroidEnsureDevice to succeed.
func AndroidLayout(widthPx, heightPx int) {
	embeddedMu.Lock()
	defer embeddedMu.Unlock()
	if embeddedEngine == nil || embeddedEngine.window == nil {
		return
	}
	embeddedEngine.window.SetSize(widthPx, heightPx)
}

// AndroidEnsureDevice creates the GPU rendering device if it hasn't
// been created yet. Called from the host's surfaceChanged after
// AndroidLayout has a valid size. Idempotent.
func AndroidEnsureDevice() error {
	embeddedMu.Lock()
	defer embeddedMu.Unlock()
	if embeddedEngine == nil {
		return errEmbeddedEngineUnset
	}
	return embeddedEngine.EnsureDevice()
}

// AndroidTick drives a single frame: Update(s) + Draw + present.
// Called from the host's render thread once per vsync. Returns
// ErrTermination if the game asked to stop; host should stop
// scheduling further ticks on seeing that error.
func AndroidTick() error {
	embeddedMu.Lock()
	defer embeddedMu.Unlock()
	if embeddedEngine == nil {
		return nil
	}
	return embeddedEngine.TickOnce()
}

// AndroidSuspend pauses the engine (Activity.onPause).
func AndroidSuspend() {
	embeddedMu.Lock()
	defer embeddedMu.Unlock()
	if embeddedEngine == nil {
		return
	}
	embeddedEngine.Suspend()
}

// AndroidResume resumes the engine (Activity.onResume).
func AndroidResume() {
	embeddedMu.Lock()
	defer embeddedMu.Unlock()
	if embeddedEngine == nil {
		return
	}
	embeddedEngine.Resume()
}

// AndroidOnContextLost drops the GPU device so the next
// AndroidEnsureDevice rebuilds it. Rare on Vulkan.
func AndroidOnContextLost() {
	embeddedMu.Lock()
	defer embeddedMu.Unlock()
	if embeddedEngine == nil {
		return
	}
	embeddedEngine.OnContextLost()
}

// AndroidDispose tears down the engine. Called from Activity.onDestroy.
// Subsequent Android* calls become no-ops until a fresh SetAndroidGame.
func AndroidDispose() {
	embeddedMu.Lock()
	defer embeddedMu.Unlock()
	if embeddedEngine == nil {
		return
	}
	embeddedEngine.Dispose()
	embeddedEngine = nil
}

// rawInputWindow returns the android.Window backing the embedded engine,
// or nil if no engine / window is set up. Callers must hold embeddedMu.
func rawInputWindow() *platandroid.Window {
	if embeddedEngine == nil || embeddedEngine.window == nil {
		return nil
	}
	w, _ := embeddedEngine.window.(*platandroid.Window)
	return w
}

// AndroidDispatchTouch routes a MotionEvent sample into the engine's
// input handler. action is one of android.MotionAction*; id is the
// pointer ID; x/y are in physical pixels.
func AndroidDispatchTouch(action, id int, x, y float32) {
	embeddedMu.Lock()
	w := rawInputWindow()
	embeddedMu.Unlock()
	if w != nil {
		w.HandleRawTouch(action, id, x, y)
	}
}

// AndroidDispatchKey routes a KeyEvent into the engine's input handler.
// keyCode is KeyEvent.getKeyCode(), unicodeChar is getUnicodeChar(0),
// meta is getMetaState(), source is getSource(), deviceID is
// getDeviceId(), and down is true for ACTION_DOWN.
func AndroidDispatchKey(keyCode, unicodeChar, meta, source, deviceID int, down bool) {
	embeddedMu.Lock()
	w := rawInputWindow()
	embeddedMu.Unlock()
	if w != nil {
		w.HandleRawKey(keyCode, unicodeChar, meta, source, deviceID, down)
	}
}

// AndroidDispatchGamepadAxis routes a MotionEvent axis value into
// the engine's gamepad state machine.
func AndroidDispatchGamepadAxis(deviceID, axis int, value float32) {
	embeddedMu.Lock()
	w := rawInputWindow()
	embeddedMu.Unlock()
	if w != nil {
		w.HandleRawGamepadAxis(deviceID, axis, value)
	}
}

// AndroidDispatchGamepadConnection reports a device add/remove event.
func AndroidDispatchGamepadConnection(deviceID int, connected bool) {
	embeddedMu.Lock()
	w := rawInputWindow()
	embeddedMu.Unlock()
	if w != nil {
		w.HandleRawGamepadConnection(deviceID, connected)
	}
}

// AndroidDeviceScale returns the pixel density for the current
// engine window. Returns 1.0 if no engine is configured.
func AndroidDeviceScale() float64 {
	embeddedMu.Lock()
	defer embeddedMu.Unlock()
	if embeddedEngine == nil {
		return 1.0
	}
	return embeddedEngine.deviceScaleFactor()
}
