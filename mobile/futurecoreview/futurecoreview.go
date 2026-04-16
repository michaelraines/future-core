// Package futurecoreview is the JNI-callable surface for the
// future-core Android engine when it is embedded in a host Java
// Activity via a gomobile-bound AAR.
//
// The host Activity (or its FutureCoreView wrapper) calls the
// functions in this package from the Android render thread and UI
// thread. Each function is a thin trampoline that forwards into the
// global *futurerender.engine created by SetGame. Functions are
// safe to call from any thread — the engine handles its own locking.
//
// This package is NOT used by the pure-Go NativeActivity build
// (build tag: futurecore_nativeactivity). That path runs the engine
// directly via futurerender.RunGame.
//
// Phase 0 status: all functions are stubs that only validate inputs
// and log. Real JNI wiring (SetSurface → ANativeWindow, Tick →
// engine.TickOnce, UpdateTouchesOnAndroid → HandleRawTouch) lands in
// Phase 1.
package futurecoreview

import (
	"sync"

	futurerender "github.com/michaelraines/future-core"
)

// state is the package-level mutable state. Guarded by mu. Touched
// from both the Android UI thread (Layout, Suspend, Resume, input
// dispatch) and the render thread (Tick, SetSurface, ClearSurface),
// so every access goes through the mutex.
var (
	mu   sync.Mutex
	game futurerender.Game
	opts *futurerender.RunGameOptions

	// Current logical screen dimensions reported by the host via Layout.
	widthPx, heightPx int
	pixelsPerPt       float32
)

// SetGame registers the game implementation that TickOnce will drive
// once the host Activity has provided a surface and the first Layout
// call has arrived. Typically called once from the AAR consumer's
// init — it's safe to call multiple times (last call wins) but only
// the most recent game is active.
func SetGame(g futurerender.Game) {
	mu.Lock()
	defer mu.Unlock()
	game = g
}

// SetOptions stores run-time options (screen orientation, window
// dimensions) that the engine consults on first initialization.
// Parallels futurerender.RunGameWithOptions for the embedded path.
// Passing nil clears any previously-set options.
func SetOptions(o *futurerender.RunGameOptions) {
	mu.Lock()
	defer mu.Unlock()
	opts = o
}

// SetSurface forwards an ANativeWindow pointer (obtained by the host
// via ANativeWindow_fromSurface) to the engine. The pointer must
// remain valid until ClearSurface is called. The host is responsible
// for calling ANativeWindow_release after ClearSurface returns.
//
// Phase 0 stub: records the handle for future Tick invocations but
// does not yet create the Vulkan swapchain.
func SetSurface(nativeWindow uintptr) {
	mu.Lock()
	defer mu.Unlock()
	// Phase 1: forward to engine.HandleSurface.
	_ = nativeWindow
}

// ClearSurface tells the engine the ANativeWindow is about to become
// invalid (SurfaceHolder.surfaceDestroyed). The host MUST block on
// this call until it returns before calling ANativeWindow_release,
// or Vulkan will present against freed memory.
//
// Phase 0 stub.
func ClearSurface() {
	// Phase 1: mu.Lock + engine.ClearSurface + drain render thread.
}

// Layout reports the current host surface dimensions and the device
// pixel density. Called from SurfaceHolder.surfaceChanged and any
// time the host view's size changes (rotation, keyboard open).
func Layout(wpx, hpx int, ppp float32) {
	mu.Lock()
	defer mu.Unlock()
	widthPx = wpx
	heightPx = hpx
	pixelsPerPt = ppp
}

// Tick runs one frame of the game loop. The host's render thread
// calls this from a Choreographer-driven callback. Returns any
// error the game's Update returned (including ErrTermination to ask
// the host to stop scheduling further ticks).
//
// Phase 0 stub: no-op. Phase 1: engine.TickOnce.
func Tick() error {
	mu.Lock()
	defer mu.Unlock()
	if game == nil {
		return nil
	}
	// Phase 1: engine.TickOnce()
	return nil
}

// Suspend is called from the host's onPause. The render thread
// should stop scheduling Tick calls. Game state persists.
func Suspend() error {
	mu.Lock()
	defer mu.Unlock()
	// Phase 1: engine.Suspend()
	return nil
}

// Resume is called from the host's onResume. Inverse of Suspend.
func Resume() error {
	mu.Lock()
	defer mu.Unlock()
	// Phase 1: engine.Resume()
	return nil
}

// OnContextLost informs the engine that the GPU device was lost and
// all GPU resources must be recreated on the next Tick. Rare on
// Vulkan; only relevant after an adb-injected device loss or driver
// reset. Android's surface lifecycle does NOT count as context loss
// — surfaceDestroyed is handled by ClearSurface.
func OnContextLost() {
	// Phase 1: mu.Lock + engine.OnContextLost()
}

// DeviceScale returns the current device pixels-per-point value. The
// host can use this to convert between Android dp and futurerender
// logical pixels.
func DeviceScale() float64 {
	mu.Lock()
	defer mu.Unlock()
	return float64(pixelsPerPt)
}

// Input dispatch — raw parameters come straight from MotionEvent /
// KeyEvent on the Java side. Phase 0 stubs; Phase 2 wires them
// through to the Android platform Window's new HandleRaw* helpers.

// UpdateTouchesOnAndroid forwards a touch event to the engine.
// action is AMOTION_EVENT_ACTION_* (0=DOWN, 1=UP, 2=MOVE, 3=CANCEL).
func UpdateTouchesOnAndroid(action, id int, x, y float32) {
	// Phase 2: window.HandleRawTouch
	_ = action
	_ = id
	_ = x
	_ = y
}

// OnKeyDownOnAndroid forwards a key-press event.
func OnKeyDownOnAndroid(keyCode, unicodeChar, source, deviceID int) {
	// Phase 2: window.HandleRawKey(..., down=true)
	_ = keyCode
	_ = unicodeChar
	_ = source
	_ = deviceID
}

// OnKeyUpOnAndroid forwards a key-release event.
func OnKeyUpOnAndroid(keyCode, source, deviceID int) {
	// Phase 2: window.HandleRawKey(..., down=false)
	_ = keyCode
	_ = source
	_ = deviceID
}

// OnGamepadAxisChanged reports a gamepad analog axis change.
func OnGamepadAxisChanged(deviceID, axisID int, value float32) {
	// Phase 2: window.HandleRawGamepadAxis
	_ = deviceID
	_ = axisID
	_ = value
}

// OnGamepadHatChanged reports a d-pad / hat-switch change. xValue
// and yValue are each -1, 0, or 1.
func OnGamepadHatChanged(deviceID, hatID, xValue, yValue int) {
	_ = deviceID
	_ = hatID
	_ = xValue
	_ = yValue
}

// OnGamepadButton reports a gamepad button press or release.
func OnGamepadButton(deviceID, keyCode int, pressed bool) {
	_ = deviceID
	_ = keyCode
	_ = pressed
}

// OnGamepadAdded registers a newly-connected gamepad. Name and
// descriptor are diagnostic strings. Counts tell the engine how many
// axes/hats the device exposes.
func OnGamepadAdded(
	deviceID int,
	name string,
	axisCount, hatCount int,
	descriptor string,
	vendorID, productID, buttonMask, axisMask int,
) {
	_ = deviceID
	_ = name
	_ = axisCount
	_ = hatCount
	_ = descriptor
	_ = vendorID
	_ = productID
	_ = buttonMask
	_ = axisMask
}

// OnInputDeviceRemoved drops gamepad state when the device is
// unplugged. deviceID matches the ID previously reported to
// OnGamepadAdded.
func OnInputDeviceRemoved(deviceID int) {
	_ = deviceID
}
