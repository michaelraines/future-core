//go:build android && futurecore_nativeactivity

package futurerender

import (
	"errors"
	"fmt"
	"runtime"

	"golang.org/x/mobile/app"
	mkey "golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
	"golang.org/x/mobile/event/touch"

	"github.com/michaelraines/future-core/internal/input"
	androidplatform "github.com/michaelraines/future-core/internal/platform/android"
)

// NativeActivity driver. Selected with -tags futurecore_nativeactivity.
// Used by pure-Go Android APKs built via `gomobile build` where Go IS
// the Android app and app.Main owns the main thread. The default
// Android build tag (android without futurecore_nativeactivity) uses
// engine_android_embedded.go instead, which expects a host Java
// Activity to drive the frame loop via JNI.

// init pins the NativeActivity's main goroutine to the process main
// thread. Vulkan requires the surface to be used from the thread that
// created it; in the NativeActivity model that's the thread app.Main
// is running on, which is guaranteed to be the Java process main
// thread by x/mobile/app's bootstrap.
func init() {
	runtime.LockOSThread()
}

// run starts the x/mobile/app event pump. Blocks for the lifetime of
// the Activity. Called from futurerender.RunGame.
func (e *engine) run() error {
	app.Main(func(a app.App) {
		e.runAndroid(a)
	})
	return nil
}

// runAndroid is the NativeActivity event loop. It translates
// x/mobile/app events into calls on the engine's platform.Window,
// lazy-initializes the backend on the first size.Event, and invokes
// TickOnce on each paint.Event.
func (e *engine) runAndroid(a app.App) {
	// Vulkan handles its own presentation on Android.
	e.noGL = true

	// Create platform window.
	win := newPlatformWindow()
	e.window = win

	// Set the x/mobile app reference on the Android window.
	androidWin, _ := win.(*androidplatform.Window)
	if androidWin != nil {
		androidWin.SetApp(a)
	}

	winCfg := e.windowConfig()
	winCfg.NoGL = true
	if err := win.Create(winCfg); err != nil {
		fmt.Printf("android: window create: %v\n", err)
		return
	}
	defer win.Destroy()

	// Set up input.
	inputState := input.New()
	win.SetInputHandler(inputState)
	e.inputState = inputState

	for ev := range a.Events() {
		switch ev := a.Filter(ev).(type) {
		case lifecycle.Event:
			if androidWin != nil {
				androidWin.HandleLifecycleEvent(ev)
			}
			// Notify game of focus changes if it implements FocusHandler.
			if fh, ok := e.game.(FocusHandler); ok {
				switch ev.Crosses(lifecycle.StageFocused) {
				case lifecycle.CrossOn:
					fh.OnFocus()
				case lifecycle.CrossOff:
					fh.OnBlur()
				}
			}
			// If the activity is dead, notify and stop the loop.
			if ev.Crosses(lifecycle.StageDead) == lifecycle.CrossOn {
				if lh, ok := e.game.(LifecycleHandler); ok {
					lh.OnDispose()
				}
				return
			}

		case size.Event:
			if androidWin != nil {
				androidWin.HandleSizeEvent(ev)
			}
			// (Re-)initialize the backend if not yet done.
			if !e.deviceInitialized && ev.WidthPx > 0 && ev.HeightPx > 0 {
				if err := e.initDevice(win); err != nil {
					fmt.Printf("android: init device: %v\n", err)
					return
				}
			}

		case touch.Event:
			if androidWin != nil {
				androidWin.HandleTouchEvent(ev)
			}

		case mkey.Event:
			if androidWin != nil {
				androidWin.HandleKeyEvent(ev)
			}

		case paint.Event:
			if !e.deviceInitialized {
				continue
			}
			if androidWin != nil && !androidWin.HandlePaintEvent(ev) {
				continue
			}
			if err := e.TickOnce(); err != nil {
				if errors.Is(err, ErrTermination) {
					return
				}
				fmt.Printf("android: tick: %v\n", err)
				return
			}

			// Request continuous rendering.
			a.Publish()
		}
	}
}
