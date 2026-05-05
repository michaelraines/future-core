//go:build !android

package futurecoreview

import (
	futurerender "github.com/michaelraines/future-core"
)

// Non-android stub implementations. These match the android-tagged
// public API exactly so callers (tests, IDE tooling, cross-platform
// code) compile on any host. None of the functions do anything
// useful — the real engine only exists on android.
//
// This file exists so:
//   1. `go test ./mobile/futurecoreview/...` works on macOS / Linux
//      without build tags.
//   2. `go build ./...` sweeps don't surface missing functions.
//   3. gomobile bind runs on the host (GOOS=linux/darwin) during its
//      own tool compile; it needs these signatures to exist in the
//      package graph before it cross-compiles for android.

// SetGame records the game for the android path; no-op on non-android.
func SetGame(g futurerender.Game) {
	mu.Lock()
	defer mu.Unlock()
	game = g
}

// SetOptions records options; no-op on non-android.
func SetOptions(o *futurerender.RunGameOptions) {
	mu.Lock()
	defer mu.Unlock()
	opts = o
}

// SetSurface — android-only; no-op elsewhere.
func SetSurface(nativeWindow int64) {
	_ = nativeWindow
}

// ClearSurface — android-only; no-op elsewhere.
func ClearSurface() {}

// Layout records dimensions for subsequent DeviceScale reads.
func Layout(wpx, hpx int, ppp float32) {
	mu.Lock()
	defer mu.Unlock()
	widthPx = wpx
	heightPx = hpx
	pixelsPerPt = ppp
}

// Tick — android-only; no-op elsewhere.
func Tick() error { return nil }

// Suspend — android-only; no-op elsewhere.
func Suspend() error { return nil }

// Resume — android-only; no-op elsewhere.
func Resume() error { return nil }

// OnContextLost — android-only; no-op elsewhere.
func OnContextLost() {}

// DeviceScale returns the last Layout-reported pixels-per-point.
func DeviceScale() float64 {
	mu.Lock()
	defer mu.Unlock()
	return float64(pixelsPerPt)
}

// RequestedOrientation — android-only; returns 0 (default) on host
// builds where there is no engine running.
func RequestedOrientation() int { return 0 }

// Input dispatch stubs.

// UpdateTouchesOnAndroid — android-only; no-op elsewhere.
func UpdateTouchesOnAndroid(action, id int, x, y float32) {
	_ = action
	_ = id
	_ = x
	_ = y
}

// OnKeyDownOnAndroid — android-only; no-op elsewhere.
func OnKeyDownOnAndroid(keyCode, unicodeChar, meta, source, deviceID int) {
	_ = keyCode
	_ = unicodeChar
	_ = meta
	_ = source
	_ = deviceID
}

// OnKeyUpOnAndroid — android-only; no-op elsewhere.
func OnKeyUpOnAndroid(keyCode, meta, source, deviceID int) {
	_ = keyCode
	_ = meta
	_ = source
	_ = deviceID
}

// OnGamepadAxisChanged — android-only; no-op elsewhere.
func OnGamepadAxisChanged(deviceID, axisID int, value float32) {
	_ = deviceID
	_ = axisID
	_ = value
}

// OnGamepadHatChanged — android-only; no-op elsewhere.
func OnGamepadHatChanged(deviceID, hatID, xValue, yValue int) {
	_ = deviceID
	_ = hatID
	_ = xValue
	_ = yValue
}

// OnGamepadButton — android-only; no-op elsewhere.
func OnGamepadButton(deviceID, keyCode int, pressed bool) {
	_ = deviceID
	_ = keyCode
	_ = pressed
}

// OnGamepadAdded — android-only; no-op elsewhere.
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

// OnInputDeviceRemoved — android-only; no-op elsewhere.
func OnInputDeviceRemoved(deviceID int) {
	_ = deviceID
}
