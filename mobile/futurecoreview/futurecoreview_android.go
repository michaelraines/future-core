//go:build android

package futurecoreview

import (
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
			// Nothing we can do from Java's perspective — the engine
			// wasn't SetGame'd first. Leave surface unassigned; Tick
			// will also be a no-op until SetGame happens.
			return
		}
		bootstrapped = true
	}
	futurerender.AndroidSetSurface(uintptr(nativeWindow))
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
	// Best-effort: EnsureDevice may fail on the very first call if
	// the surface isn't bound yet. The next Layout or SetSurface call
	// will retry.
	_ = futurerender.AndroidEnsureDevice()
}

// Tick runs one frame. Called by the host's render thread once per
// Choreographer vsync callback.
func Tick() error {
	mu.Lock()
	defer mu.Unlock()
	if game == nil {
		return nil
	}
	return futurerender.AndroidTick()
}

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

// Input dispatch — Phase 2 will populate these. Phase 1 leaves them
// as no-ops so the AAR still exports matching Java signatures; input
// events hit Go but are dropped on the floor. This avoids
// regenerating the AAR between Phase 1 and Phase 2.

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
	// Phase 2
	_ = keyCode
	_ = unicodeChar
	_ = source
	_ = deviceID
}

// OnKeyUpOnAndroid forwards a key-release event.
func OnKeyUpOnAndroid(keyCode, source, deviceID int) {
	// Phase 2
	_ = keyCode
	_ = source
	_ = deviceID
}

// OnGamepadAxisChanged reports a gamepad analog axis change.
func OnGamepadAxisChanged(deviceID, axisID int, value float32) {
	_ = deviceID
	_ = axisID
	_ = value
}

// OnGamepadHatChanged reports a d-pad / hat-switch change.
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

// OnGamepadAdded registers a newly-connected gamepad.
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

// OnInputDeviceRemoved drops gamepad state on unplug.
func OnInputDeviceRemoved(deviceID int) {
	_ = deviceID
}
