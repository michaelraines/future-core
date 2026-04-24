//go:build android

package futurecoreview

import (
	"log"
	"runtime/debug"

	futurerender "github.com/michaelraines/future-core"
)

// bootstrapped tracks whether AndroidBootstrap has run. Set on the
// first SetSurface call so subsequent surface-replace cycles don't
// re-run window creation.
var bootstrapped bool

// Android-specific implementations. Each function takes the package
// mutex, then forwards to futurerender's embedded-engine API. The
// futurerender side has its own locking, but we take the package
// mutex here too to serialize mutations against the shared state
// (game, opts, widthPx, ...).

// SetGame registers the game implementation that Tick will drive
// once the host Activity has provided a surface.
func SetGame(g futurerender.Game) {
	mu.Lock()
	defer mu.Unlock()
	game = g
	futurerender.SetAndroidGame(g)
}

// SetOptions stores engine options. Takes effect on next Bootstrap.
func SetOptions(o *futurerender.RunGameOptions) {
	mu.Lock()
	defer mu.Unlock()
	opts = o
	futurerender.SetAndroidOptions(o)
}

// SetSurface forwards an ANativeWindow pointer to the engine.
// Triggers Bootstrap on first call so the engine is ready to accept
// it. The int64 is the raw ANativeWindow* obtained via
// ANativeWindow_fromSurface on the Java side.
func SetSurface(nativeWindow int64) {
	mu.Lock()
	defer mu.Unlock()
	if !bootstrapped {
		if err := futurerender.AndroidBootstrap(); err != nil {
			log.Printf("futurecoreview.SetSurface: AndroidBootstrap failed: %v", err)
			return
		}
		log.Printf("futurecoreview.SetSurface: AndroidBootstrap ok")
		bootstrapped = true
	}
	futurerender.AndroidSetSurface(uintptr(nativeWindow))
	log.Printf("futurecoreview.SetSurface: nw=0x%x handed to engine", uintptr(nativeWindow))
}

// ClearSurface releases the engine's ANativeWindow reference.
// Blocks until the engine acknowledges — host must wait for this
// to return before calling ANativeWindow_release.
func ClearSurface() {
	mu.Lock()
	defer mu.Unlock()
	futurerender.AndroidClearSurface()
}

// Layout reports host surface dimensions + device density, then
// triggers EnsureDevice so the GPU swapchain builds against the new
// dimensions.
func Layout(wpx, hpx int, ppp float32) {
	mu.Lock()
	defer mu.Unlock()
	widthPx = wpx
	heightPx = hpx
	pixelsPerPt = ppp
	futurerender.AndroidLayout(wpx, hpx)
	if err := futurerender.AndroidEnsureDevice(); err != nil {
		log.Printf("futurecoreview.Layout: AndroidEnsureDevice returned: %v (retry on next Layout/SetSurface)", err)
	} else {
		log.Printf("futurecoreview.Layout: AndroidEnsureDevice ok (%dx%d ppp=%v)", wpx, hpx, ppp)
	}
}

// Tick runs one frame. Called by the host's render thread once per
// Choreographer vsync callback.
func Tick() error {
	mu.Lock()
	defer mu.Unlock()
	if game == nil {
		return nil
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("futurecoreview.Tick: PANIC %v\n%s", r, debug.Stack())
			// Re-panic so the host gets notified and the process dies
			// with a real tombstone instead of silently looping.
			panic(r)
		}
	}()
	err := futurerender.AndroidTick()
	if err != nil {
		tickErrCount++
		if tickErrCount%60 == 1 {
			log.Printf("futurecoreview.Tick: AndroidTick returned (seen %d times): %v", tickErrCount, err)
		}
	} else if tickOkCount == 0 {
		log.Printf("futurecoreview.Tick: first AndroidTick ok")
		tickOkCount++
	}
	return err
}

var (
	tickErrCount int
	tickOkCount  int
)

// Suspend pauses the engine (Activity.onPause).
func Suspend() error {
	mu.Lock()
	defer mu.Unlock()
	futurerender.AndroidSuspend()
	return nil
}

// Resume unpauses the engine (Activity.onResume).
func Resume() error {
	mu.Lock()
	defer mu.Unlock()
	futurerender.AndroidResume()
	return nil
}

// OnContextLost drops the GPU device. Next EnsureDevice rebuilds.
func OnContextLost() {
	mu.Lock()
	defer mu.Unlock()
	futurerender.AndroidOnContextLost()
}

// DeviceScale returns the host's pixels-per-point.
func DeviceScale() float64 {
	mu.Lock()
	defer mu.Unlock()
	return futurerender.AndroidDeviceScale()
}

// Input dispatch — routes Java-sourced MotionEvent / KeyEvent /
// InputDevice data through futurerender's embedded-engine API. Each
// function forwards to AndroidDispatch* which type-asserts the engine's
// window to *android.Window and calls its HandleRaw* method.

// UpdateTouchesOnAndroid forwards a touch sample to the engine.
// action is the masked MotionEvent action (0=DOWN, 1=UP, 2=MOVE,
// 3=CANCEL, 5=POINTER_DOWN, 6=POINTER_UP). id is the pointer ID;
// x/y are in physical pixels.
func UpdateTouchesOnAndroid(action, id int, x, y float32) {
	futurerender.AndroidDispatchTouch(action, id, x, y)
}

// OnKeyDownOnAndroid forwards a key-press event from dispatchKeyEvent.
func OnKeyDownOnAndroid(keyCode, unicodeChar, meta, source, deviceID int) {
	futurerender.AndroidDispatchKey(keyCode, unicodeChar, meta, source, deviceID, true)
}

// OnKeyUpOnAndroid forwards a key-release event from dispatchKeyEvent.
func OnKeyUpOnAndroid(keyCode, meta, source, deviceID int) {
	futurerender.AndroidDispatchKey(keyCode, 0, meta, source, deviceID, false)
}

// OnGamepadAxisChanged reports one axis from MotionEvent.getAxisValue
// (usually called once per axis per event). axisID is the Android
// MotionEvent.AXIS_* constant; value is the normalized axis value.
func OnGamepadAxisChanged(deviceID, axisID int, value float32) {
	futurerender.AndroidDispatchGamepadAxis(deviceID, axisID, value)
}

// OnGamepadHatChanged reports a d-pad / hat-switch change. The
// Android input system exposes the HAT as two axes (HAT_X / HAT_Y);
// we forward them through the same axis path.
func OnGamepadHatChanged(deviceID, _, xValue, yValue int) {
	// HAT values from Android are already in {-1, 0, +1}; the axis path
	// stores them as float64 in the GamepadEvent.Axes slots beyond the
	// stick/trigger layout. Treat them as axis 15/16 (MotionEvent.AXIS_HAT_X/Y).
	futurerender.AndroidDispatchGamepadAxis(deviceID, 15, float32(xValue))
	futurerender.AndroidDispatchGamepadAxis(deviceID, 16, float32(yValue))
}

// OnGamepadButton reports a button press or release. keyCode is the
// Android KeyEvent.KEYCODE_BUTTON_* value.
func OnGamepadButton(deviceID, keyCode int, pressed bool) {
	// Buttons share the dispatchKey path: Window.HandleRawKey detects
	// gamepad sources and routes to the gamepad state machine.
	const gamepadSource = 0x00000401 // SOURCE_GAMEPAD
	futurerender.AndroidDispatchKey(keyCode, 0, 0, gamepadSource, deviceID, pressed)
}

// OnGamepadAdded registers a newly-connected gamepad. For now the
// device registration is implicit — the first axis/button event
// activates the gamepad state — so this is a connection notification
// only.
func OnGamepadAdded(
	deviceID int,
	_ string,
	_, _ int,
	_ string,
	_, _, _, _ int,
) {
	futurerender.AndroidDispatchGamepadConnection(deviceID, true)
}

// OnInputDeviceRemoved drops gamepad state on unplug.
func OnInputDeviceRemoved(deviceID int) {
	futurerender.AndroidDispatchGamepadConnection(deviceID, false)
}
