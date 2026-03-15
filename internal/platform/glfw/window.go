//go:build glfw

// Package glfw implements the platform.Window interface using GLFW,
// loaded at runtime via purego (no CGo required).
package glfw

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/michaelraines/future-render/internal/platform"
)

func init() {
	// GLFW must be called from the main thread.
	runtime.LockOSThread()
}

// Window implements platform.Window using GLFW via purego.
type Window struct {
	win            uintptr // GLFWwindow*
	handler        platform.InputHandler
	fullscreen     bool
	savedX, savedY int
	savedW, savedH int

	// Cursor tracking for delta computation.
	prevCursorX, prevCursorY float64
	hasPrevCursor            bool

	// Prevent callback pointers from being GC'd.
	keyCB         uintptr
	charCB        uintptr
	mouseButtonCB uintptr
	cursorPosCB   uintptr
	scrollCB      uintptr
	framebufferCB uintptr

	// Track previously connected gamepads for disconnect detection.
	connectedGamepads [glfwJoystickLast + 1]bool
}

// New creates a new GLFW window (uninitialized — call Create to open it).
func New() *Window {
	return &Window{}
}

// Create creates and shows the GLFW window.
func (w *Window) Create(cfg platform.WindowConfig) error {
	if err := initGLFWAPI(); err != nil {
		return fmt.Errorf("glfw api: %w", err)
	}

	if fnGlfwInit() == 0 {
		return fmt.Errorf("glfw init failed")
	}

	// Request OpenGL 3.3 core profile.
	fnGlfwWindowHint(glfwContextVersionMajor, 3)
	fnGlfwWindowHint(glfwContextVersionMinor, 3)
	fnGlfwWindowHint(glfwOpenGLProfile, glfwOpenGLCoreProfile)
	fnGlfwWindowHint(glfwOpenGLForwardCompat, glfwTrue)

	if cfg.Resizable {
		fnGlfwWindowHint(glfwResizable, glfwTrue)
	} else {
		fnGlfwWindowHint(glfwResizable, glfwFalse)
	}

	if !cfg.Decorated {
		fnGlfwWindowHint(glfwDecorated, glfwFalse)
	}

	var monitor uintptr
	width, height := int32(cfg.Width), int32(cfg.Height)
	if cfg.Fullscreen {
		monitor = fnGlfwGetPrimaryMonitor()
		mode := getVideoMode(fnGlfwGetVideoMode(monitor))
		width = mode.Width
		height = mode.Height
		w.fullscreen = true
	}

	win := fnGlfwCreateWindow(width, height, cStr(cfg.Title), monitor, 0)
	if win == 0 {
		fnGlfwTerminate()
		return fmt.Errorf("glfw create window failed")
	}
	w.win = win
	fnGlfwMakeContextCurrent(win)

	if cfg.VSync {
		fnGlfwSwapInterval(1)
	} else {
		fnGlfwSwapInterval(0)
	}

	w.installCallbacks()
	return nil
}

// Destroy closes the window and terminates GLFW.
func (w *Window) Destroy() {
	if w.win != 0 {
		fnGlfwDestroyWindow(w.win)
		w.win = 0
	}
	fnGlfwTerminate()
}

// ShouldClose returns whether the window close has been requested.
func (w *Window) ShouldClose() bool {
	return fnGlfwWindowShouldClose(w.win) != 0
}

// PollEvents processes pending window events.
func (w *Window) PollEvents() {
	fnGlfwPollEvents()
}

// SwapBuffers swaps front and back buffers.
func (w *Window) SwapBuffers() {
	fnGlfwSwapBuffers(w.win)
}

// Size returns the window size in screen coordinates.
func (w *Window) Size() (width, height int) {
	var ww, hh int32
	fnGlfwGetWindowSize(w.win, &ww, &hh)
	return int(ww), int(hh)
}

// FramebufferSize returns the framebuffer size in pixels.
func (w *Window) FramebufferSize() (width, height int) {
	var ww, hh int32
	fnGlfwGetFramebufferSize(w.win, &ww, &hh)
	return int(ww), int(hh)
}

// DevicePixelRatio returns the ratio of physical to logical pixels.
func (w *Window) DevicePixelRatio() float64 {
	var fbW, winW int32
	fnGlfwGetFramebufferSize(w.win, &fbW, nil)
	fnGlfwGetWindowSize(w.win, &winW, nil)
	if winW == 0 {
		return 1.0
	}
	return float64(fbW) / float64(winW)
}

// SetTitle sets the window title.
func (w *Window) SetTitle(title string) {
	fnGlfwSetWindowTitle(w.win, cStr(title))
}

// SetSize sets the window size in screen coordinates.
func (w *Window) SetSize(width, height int) {
	fnGlfwSetWindowSize(w.win, int32(width), int32(height))
}

// SetFullscreen toggles fullscreen mode.
func (w *Window) SetFullscreen(fullscreen bool) {
	if fullscreen == w.fullscreen {
		return
	}
	w.fullscreen = fullscreen
	if fullscreen {
		var x, y int32
		fnGlfwGetWindowPos(w.win, &x, &y)
		w.savedX, w.savedY = int(x), int(y)
		var sw, sh int32
		fnGlfwGetWindowSize(w.win, &sw, &sh)
		w.savedW, w.savedH = int(sw), int(sh)
		monitor := fnGlfwGetPrimaryMonitor()
		mode := getVideoMode(fnGlfwGetVideoMode(monitor))
		fnGlfwSetWindowMonitor(w.win, monitor, 0, 0, mode.Width, mode.Height, mode.RefreshRate)
	} else {
		fnGlfwSetWindowMonitor(w.win, 0, int32(w.savedX), int32(w.savedY), int32(w.savedW), int32(w.savedH), 0)
	}
}

// IsFullscreen returns whether the window is fullscreen.
func (w *Window) IsFullscreen() bool {
	return w.fullscreen
}

// SetCursorVisible shows or hides the cursor.
func (w *Window) SetCursorVisible(visible bool) {
	if visible {
		fnGlfwSetInputMode(w.win, glfwCursorMode, glfwCursorNormal)
	} else {
		fnGlfwSetInputMode(w.win, glfwCursorMode, glfwCursorHidden)
	}
}

// SetCursorLocked locks or unlocks the cursor.
func (w *Window) SetCursorLocked(locked bool) {
	if locked {
		fnGlfwSetInputMode(w.win, glfwCursorMode, glfwCursorDisabled)
	} else {
		fnGlfwSetInputMode(w.win, glfwCursorMode, glfwCursorNormal)
	}
}

// NativeHandle returns the GLFW window pointer as a uintptr.
func (w *Window) NativeHandle() uintptr {
	return w.win
}

// SetInputHandler sets the handler for input events.
func (w *Window) SetInputHandler(handler platform.InputHandler) {
	w.handler = handler
}

// PollGamepads queries GLFW for connected joysticks and dispatches
// GamepadEvent to the input handler. GLFW joystick input uses a polling
// API rather than callbacks, so this must be called each frame.
func (w *Window) PollGamepads() {
	if w.handler == nil {
		return
	}
	for jid := int32(glfwJoystick1); jid <= int32(glfwJoystickLast); jid++ {
		present := fnGlfwJoystickPresent(jid) != 0

		if !present {
			// Send disconnect event if previously connected.
			if w.connectedGamepads[jid] {
				w.connectedGamepads[jid] = false
				w.handler.OnGamepadEvent(platform.GamepadEvent{
					ID:           int(jid),
					Disconnected: true,
				})
			}
			continue
		}

		w.connectedGamepads[jid] = true

		var event platform.GamepadEvent
		event.ID = int(jid)

		// Read axes.
		var axisCount int32
		axesPtr := fnGlfwGetJoystickAxes(jid, &axisCount)
		if axesPtr != 0 && axisCount > 0 {
			n := int(axisCount)
			if n > len(event.Axes) {
				n = len(event.Axes)
			}
			for i := 0; i < n; i++ {
				event.Axes[i] = float64(*(*float32)(
					unsafe.Pointer(axesPtr + uintptr(i)*unsafe.Sizeof(float32(0))),
				))
			}
		}

		// Read buttons.
		var buttonCount int32
		buttonsPtr := fnGlfwGetJoystickButtons(jid, &buttonCount)
		if buttonsPtr != 0 && buttonCount > 0 {
			n := int(buttonCount)
			if n > len(event.Buttons) {
				n = len(event.Buttons)
			}
			for i := 0; i < n; i++ {
				b := *(*byte)(unsafe.Pointer(buttonsPtr + uintptr(i)))
				event.Buttons[i] = b == byte(glfwPress)
			}
		}

		w.handler.OnGamepadEvent(event)
	}
}

// activeWindows maps GLFW window handles to Window instances for callbacks.
var activeWindows = map[uintptr]*Window{}

// installCallbacks registers GLFW event callbacks via purego.NewCallback.
func (w *Window) installCallbacks() {
	activeWindows[w.win] = w

	w.keyCB = purego.NewCallback(func(window uintptr, key, scancode, action, mods int32) {
		win := activeWindows[window]
		if win == nil || win.handler == nil {
			return
		}
		win.handler.OnKeyEvent(platform.KeyEvent{
			Key:    mapKey(key),
			Action: mapAction(action),
			Mods:   mapMods(mods),
		})
	})
	fnGlfwSetKeyCallback(w.win, w.keyCB)

	w.charCB = purego.NewCallback(func(window uintptr, codepoint uint32) {
		win := activeWindows[window]
		if win == nil || win.handler == nil {
			return
		}
		win.handler.OnCharEvent(rune(codepoint))
	})
	fnGlfwSetCharCallback(w.win, w.charCB)

	w.mouseButtonCB = purego.NewCallback(func(window uintptr, button, action, mods int32) {
		win := activeWindows[window]
		if win == nil || win.handler == nil {
			return
		}
		var x, y float64
		fnGlfwGetCursorPos(window, &x, &y)
		win.handler.OnMouseButtonEvent(platform.MouseButtonEvent{
			Button: platform.MouseButton(button),
			Action: mapAction(action),
			X:      x,
			Y:      y,
			Mods:   mapMods(mods),
		})
	})
	fnGlfwSetMouseButtonCallback(w.win, w.mouseButtonCB)

	w.cursorPosCB = purego.NewCallback(func(window uintptr, x, y float64) {
		win := activeWindows[window]
		if win == nil || win.handler == nil {
			return
		}
		var dx, dy float64
		if win.hasPrevCursor {
			dx = x - win.prevCursorX
			dy = y - win.prevCursorY
		}
		win.prevCursorX = x
		win.prevCursorY = y
		win.hasPrevCursor = true
		win.handler.OnMouseMoveEvent(platform.MouseMoveEvent{
			X: x, Y: y, DX: dx, DY: dy,
		})
	})
	fnGlfwSetCursorPosCallback(w.win, w.cursorPosCB)

	w.scrollCB = purego.NewCallback(func(window uintptr, xoff, yoff float64) {
		win := activeWindows[window]
		if win == nil || win.handler == nil {
			return
		}
		win.handler.OnMouseScrollEvent(platform.MouseScrollEvent{
			DX: xoff, DY: yoff,
		})
	})
	fnGlfwSetScrollCallback(w.win, w.scrollCB)

	w.framebufferCB = purego.NewCallback(func(window uintptr, width, height int32) {
		win := activeWindows[window]
		if win == nil || win.handler == nil {
			return
		}
		win.handler.OnResizeEvent(int(width), int(height))
	})
	fnGlfwSetFramebufferSizeCallback(w.win, w.framebufferCB)
}

// --- Key mapping ---

func mapKey(k int32) platform.Key {
	switch k {
	case glfwKeySpace:
		return platform.KeySpace
	case glfwKeyApostrophe:
		return platform.KeyApostrophe
	case glfwKeyComma:
		return platform.KeyComma
	case glfwKeyMinus:
		return platform.KeyMinus
	case glfwKeyPeriod:
		return platform.KeyPeriod
	case glfwKeySlash:
		return platform.KeySlash
	case glfwKey0:
		return platform.Key0
	case glfwKey1:
		return platform.Key1
	case glfwKey2:
		return platform.Key2
	case glfwKey3:
		return platform.Key3
	case glfwKey4:
		return platform.Key4
	case glfwKey5:
		return platform.Key5
	case glfwKey6:
		return platform.Key6
	case glfwKey7:
		return platform.Key7
	case glfwKey8:
		return platform.Key8
	case glfwKey9:
		return platform.Key9
	case glfwKeyA:
		return platform.KeyA
	case glfwKeyB:
		return platform.KeyB
	case glfwKeyC:
		return platform.KeyC
	case glfwKeyD:
		return platform.KeyD
	case glfwKeyE:
		return platform.KeyE
	case glfwKeyF:
		return platform.KeyF
	case glfwKeyG:
		return platform.KeyG
	case glfwKeyH:
		return platform.KeyH
	case glfwKeyI:
		return platform.KeyI
	case glfwKeyJ:
		return platform.KeyJ
	case glfwKeyK:
		return platform.KeyK
	case glfwKeyL:
		return platform.KeyL
	case glfwKeyM:
		return platform.KeyM
	case glfwKeyN:
		return platform.KeyN
	case glfwKeyO:
		return platform.KeyO
	case glfwKeyP:
		return platform.KeyP
	case glfwKeyQ:
		return platform.KeyQ
	case glfwKeyR:
		return platform.KeyR
	case glfwKeyS:
		return platform.KeyS
	case glfwKeyT:
		return platform.KeyT
	case glfwKeyU:
		return platform.KeyU
	case glfwKeyV:
		return platform.KeyV
	case glfwKeyW:
		return platform.KeyW
	case glfwKeyX:
		return platform.KeyX
	case glfwKeyY:
		return platform.KeyY
	case glfwKeyZ:
		return platform.KeyZ
	case glfwKeyEscape:
		return platform.KeyEscape
	case glfwKeyEnter:
		return platform.KeyEnter
	case glfwKeyTab:
		return platform.KeyTab
	case glfwKeyBackspace:
		return platform.KeyBackspace
	case glfwKeyRight:
		return platform.KeyRight
	case glfwKeyLeft:
		return platform.KeyLeft
	case glfwKeyDown:
		return platform.KeyDown
	case glfwKeyUp:
		return platform.KeyUp
	case glfwKeyLeftShift, glfwKeyRightShift:
		return platform.KeyLeftShift
	case glfwKeyLeftCtrl, glfwKeyRightCtrl:
		return platform.KeyLeftControl
	case glfwKeyLeftAlt, glfwKeyRightAlt:
		return platform.KeyLeftAlt
	case glfwKeyF1:
		return platform.KeyF1
	case glfwKeyF2:
		return platform.KeyF2
	case glfwKeyF3:
		return platform.KeyF3
	case glfwKeyF4:
		return platform.KeyF4
	case glfwKeyF5:
		return platform.KeyF5
	case glfwKeyF6:
		return platform.KeyF6
	case glfwKeyF7:
		return platform.KeyF7
	case glfwKeyF8:
		return platform.KeyF8
	case glfwKeyF9:
		return platform.KeyF9
	case glfwKeyF10:
		return platform.KeyF10
	case glfwKeyF11:
		return platform.KeyF11
	case glfwKeyF12:
		return platform.KeyF12
	default:
		return platform.KeyUnknown
	}
}

// mapAction converts a GLFW action to a platform.Action.
func mapAction(a int32) platform.Action {
	switch a {
	case glfwPress:
		return platform.ActionPress
	case glfwRelease:
		return platform.ActionRelease
	case glfwRepeat:
		return platform.ActionRepeat
	default:
		return platform.ActionRelease
	}
}

// mapMods converts GLFW modifier keys to platform.Modifier.
func mapMods(m int32) platform.Modifier {
	var mods platform.Modifier
	if m&glfwModShift != 0 {
		mods |= platform.ModShift
	}
	if m&glfwModControl != 0 {
		mods |= platform.ModControl
	}
	if m&glfwModAlt != 0 {
		mods |= platform.ModAlt
	}
	if m&glfwModSuper != 0 {
		mods |= platform.ModSuper
	}
	return mods
}
