//go:build android && !futurecore_nativeactivity

package futurerender

import (
	"fmt"

	"github.com/michaelraines/future-core/internal/input"
	androidplatform "github.com/michaelraines/future-core/internal/platform/android"
)

// Embedded driver. Default Android build tag (android without
// futurecore_nativeactivity). Used by gomobile-BOUND AARs where a host
// Java Activity owns the process. The futurecore/mobile/futurecoreview
// package exposes the JNI-callable surface; those functions land here.
//
// Lifecycle contract:
//
//   1. Host Activity's onCreate instantiates FutureCoreView, which
//      creates a SurfaceView + SurfaceHolder.Callback. Java calls into
//      Go to set up the engine via a package-level SetGame in
//      mobile/futurecoreview (that package owns the *engine reference
//      and forwards to the methods below).
//   2. SurfaceHolder.surfaceCreated fires → Java calls HandleSurface(fd)
//      with the ANativeWindow pointer from ANativeWindow_fromSurface.
//   3. SurfaceHolder.surfaceChanged(format, w, h) → Java calls Layout.
//      On the first size-is-valid call, Bootstrap runs initDevice.
//   4. Choreographer vsync callbacks dispatch Tick on the render
//      thread → TickOnce runs one frame.
//   5. Host Activity onPause → Suspend; onResume → Resume.
//   6. SurfaceHolder.surfaceDestroyed fires → Java calls ClearSurface
//      and blocks on a latch until the render thread has drained.

// Bootstrap performs the one-time window + input setup that the
// NativeActivity path does inside runAndroid, factored out so the JNI
// bridge can invoke it explicitly. Safe to call multiple times; only
// the first call has effect.
func (e *engine) Bootstrap() error {
	if e.window != nil {
		return nil
	}
	// Vulkan handles its own presentation on Android.
	e.noGL = true

	win := newPlatformWindow()
	e.window = win

	winCfg := e.windowConfig()
	winCfg.NoGL = true
	if err := win.Create(winCfg); err != nil {
		return fmt.Errorf("android: window create: %w", err)
	}

	inputState := input.New()
	win.SetInputHandler(inputState)
	e.inputState = inputState
	return nil
}

// EnsureDevice initializes the rendering backend once surface dimensions
// are known. Called from Java's SurfaceHolder.surfaceChanged after the
// first size update. Idempotent — subsequent calls are no-ops unless
// OnContextLost has reset deviceInitialized.
func (e *engine) EnsureDevice() error {
	if e.deviceInitialized {
		return nil
	}
	if e.window == nil {
		return fmt.Errorf("android: EnsureDevice called before Bootstrap")
	}
	w, h := e.window.FramebufferSize()
	if w <= 0 || h <= 0 {
		return nil // wait for the next size update
	}
	return e.initDevice(e.window)
}

// HandleSurface passes a newly-created or replaced ANativeWindow
// handle (obtained via ANativeWindow_fromSurface on the Java side) to
// the platform window. The caller retains ownership; pair with
// ClearSurface before ANativeWindow_release.
func (e *engine) HandleSurface(nativeWindow uintptr) {
	if e.window == nil {
		return
	}
	if androidWin, ok := e.window.(*androidplatform.Window); ok {
		androidWin.SetNativeWindow(nativeWindow)
	}
}

// ClearSurface releases the ANativeWindow reference held by the
// platform window. Called from Java's SurfaceHolder.surfaceDestroyed
// BEFORE it unpins the render thread and releases the native window.
func (e *engine) ClearSurface() {
	if e.window == nil {
		return
	}
	if androidWin, ok := e.window.(*androidplatform.Window); ok {
		androidWin.SetNativeWindow(0)
	}
}

// Suspend is called from Java's onPause to pause the game loop. The
// render thread should stop scheduling Tick calls; game state persists.
func (e *engine) Suspend() {
	if fh, ok := e.game.(FocusHandler); ok {
		fh.OnBlur()
	}
}

// Resume is called from Java's onResume. Inverse of Suspend.
func (e *engine) Resume() {
	if fh, ok := e.game.(FocusHandler); ok {
		fh.OnFocus()
	}
}

// OnContextLost is called after a GPU context loss (rare on Vulkan —
// typically only after adb-force device-lost injection). Drops the
// cached device/renderer so the next EnsureDevice rebuilds them.
func (e *engine) OnContextLost() {
	e.disposeRenderResources()
	e.device = nil
	e.encoder = nil
	e.deviceInitialized = false
}

// Dispose is called from Java's onDestroy. Cleans up GPU resources
// and notifies the game. Further TickOnce calls are no-ops because
// deviceInitialized is cleared.
func (e *engine) Dispose() {
	if lh, ok := e.game.(LifecycleHandler); ok {
		lh.OnDispose()
	}
	e.disposeRenderResources()
	if e.window != nil {
		e.window.Destroy()
		e.window = nil
	}
	e.deviceInitialized = false
}

// run is a stub in the embedded build — the embedded driver does not
// own the process. Host Java Activity drives the loop via the JNI
// surface exported from mobile/futurecoreview. Calling this directly
// from application code is always a bug; it returns an error so
// futurerender.RunGame surfaces the issue rather than silently
// spinning.
func (e *engine) run() error {
	return fmt.Errorf("android (embedded build): RunGame is not supported; " +
		"use github.com/michaelraines/future-core/mobile/futurecoreview " +
		"and let the host Activity drive the frame loop. " +
		"For pure-Go NativeActivity builds, rebuild with -tags futurecore_nativeactivity")
}
