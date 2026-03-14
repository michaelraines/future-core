//go:build glfw

// Package glfw implements the platform.Window interface using GLFW.
package glfw

import (
	"fmt"
	"runtime"

	"github.com/go-gl/glfw/v3.3/glfw"

	"github.com/michaelraines/future-render/internal/platform"
)

func init() {
	// GLFW must be called from the main thread.
	runtime.LockOSThread()
}

// Window implements platform.Window using GLFW.
type Window struct {
	win            *glfw.Window
	handler        platform.InputHandler
	fullscreen     bool
	savedX, savedY int
	savedW, savedH int
}

// New creates a new GLFW window (uninitialized — call Create to open it).
func New() *Window {
	return &Window{}
}

// Create creates and shows the GLFW window.
func (w *Window) Create(cfg platform.WindowConfig) error {
	if err := glfw.Init(); err != nil {
		return fmt.Errorf("glfw init: %w", err)
	}

	// Request OpenGL 3.3 core profile.
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

	if cfg.Resizable {
		glfw.WindowHint(glfw.Resizable, glfw.True)
	} else {
		glfw.WindowHint(glfw.Resizable, glfw.False)
	}

	if !cfg.Decorated {
		glfw.WindowHint(glfw.Decorated, glfw.False)
	}

	var monitor *glfw.Monitor
	width, height := cfg.Width, cfg.Height
	if cfg.Fullscreen {
		monitor = glfw.GetPrimaryMonitor()
		mode := monitor.GetVideoMode()
		width = mode.Width
		height = mode.Height
		w.fullscreen = true
	}

	win, err := glfw.CreateWindow(width, height, cfg.Title, monitor, nil)
	if err != nil {
		glfw.Terminate()
		return fmt.Errorf("glfw create window: %w", err)
	}
	w.win = win
	win.MakeContextCurrent()

	if cfg.VSync {
		glfw.SwapInterval(1)
	} else {
		glfw.SwapInterval(0)
	}

	w.installCallbacks()
	return nil
}

// Destroy closes the window and terminates GLFW.
func (w *Window) Destroy() {
	if w.win != nil {
		w.win.Destroy()
		w.win = nil
	}
	glfw.Terminate()
}

// ShouldClose returns whether the window close has been requested.
func (w *Window) ShouldClose() bool {
	return w.win.ShouldClose()
}

// PollEvents processes pending window events.
func (w *Window) PollEvents() {
	glfw.PollEvents()
}

// SwapBuffers swaps front and back buffers.
func (w *Window) SwapBuffers() {
	w.win.SwapBuffers()
}

// Size returns the window size in screen coordinates.
func (w *Window) Size() (width, height int) {
	return w.win.GetSize()
}

// FramebufferSize returns the framebuffer size in pixels.
func (w *Window) FramebufferSize() (width, height int) {
	return w.win.GetFramebufferSize()
}

// DevicePixelRatio returns the ratio of physical to logical pixels.
func (w *Window) DevicePixelRatio() float64 {
	fbW, _ := w.win.GetFramebufferSize()
	winW, _ := w.win.GetSize()
	if winW == 0 {
		return 1.0
	}
	return float64(fbW) / float64(winW)
}

// SetTitle sets the window title.
func (w *Window) SetTitle(title string) {
	w.win.SetTitle(title)
}

// SetSize sets the window size in screen coordinates.
func (w *Window) SetSize(width, height int) {
	w.win.SetSize(width, height)
}

// SetFullscreen toggles fullscreen mode.
func (w *Window) SetFullscreen(fullscreen bool) {
	if fullscreen == w.fullscreen {
		return
	}
	w.fullscreen = fullscreen
	if fullscreen {
		w.savedX, w.savedY = w.win.GetPos()
		w.savedW, w.savedH = w.win.GetSize()
		monitor := glfw.GetPrimaryMonitor()
		mode := monitor.GetVideoMode()
		w.win.SetMonitor(monitor, 0, 0, mode.Width, mode.Height, mode.RefreshRate)
	} else {
		w.win.SetMonitor(nil, w.savedX, w.savedY, w.savedW, w.savedH, 0)
	}
}

// IsFullscreen returns whether the window is fullscreen.
func (w *Window) IsFullscreen() bool {
	return w.fullscreen
}

// SetCursorVisible shows or hides the cursor.
func (w *Window) SetCursorVisible(visible bool) {
	if visible {
		w.win.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
	} else {
		w.win.SetInputMode(glfw.CursorMode, glfw.CursorHidden)
	}
}

// SetCursorLocked locks or unlocks the cursor.
func (w *Window) SetCursorLocked(locked bool) {
	if locked {
		w.win.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
	} else {
		w.win.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
	}
}

// NativeHandle returns the GLFW window pointer as a uintptr.
func (w *Window) NativeHandle() uintptr {
	return uintptr(0) // GLFW doesn't expose a raw handle; context is already current
}

// SetInputHandler sets the handler for input events.
func (w *Window) SetInputHandler(handler platform.InputHandler) {
	w.handler = handler
}

// installCallbacks registers GLFW event callbacks.
func (w *Window) installCallbacks() {
	w.win.SetKeyCallback(func(_ *glfw.Window, key glfw.Key, _ int, action glfw.Action, mods glfw.ModifierKey) {
		if w.handler == nil {
			return
		}
		w.handler.OnKeyEvent(platform.KeyEvent{
			Key:    mapKey(key),
			Action: mapAction(action),
			Mods:   mapMods(mods),
		})
	})

	w.win.SetMouseButtonCallback(func(_ *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		if w.handler == nil {
			return
		}
		x, y := w.win.GetCursorPos()
		w.handler.OnMouseButtonEvent(platform.MouseButtonEvent{
			Button: platform.MouseButton(button),
			Action: mapAction(action),
			X:      x,
			Y:      y,
			Mods:   mapMods(mods),
		})
	})

	w.win.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) {
		if w.handler == nil {
			return
		}
		w.handler.OnMouseMoveEvent(platform.MouseMoveEvent{
			X: x, Y: y,
		})
	})

	w.win.SetScrollCallback(func(_ *glfw.Window, xoff, yoff float64) {
		if w.handler == nil {
			return
		}
		w.handler.OnMouseScrollEvent(platform.MouseScrollEvent{
			DX: xoff, DY: yoff,
		})
	})

	w.win.SetFramebufferSizeCallback(func(_ *glfw.Window, width, height int) {
		if w.handler == nil {
			return
		}
		w.handler.OnResizeEvent(width, height)
	})
}

// mapKey converts a GLFW key to a platform.Key.
func mapKey(k glfw.Key) platform.Key {
	switch k {
	case glfw.KeySpace:
		return platform.KeySpace
	case glfw.KeyApostrophe:
		return platform.KeyApostrophe
	case glfw.KeyComma:
		return platform.KeyComma
	case glfw.KeyMinus:
		return platform.KeyMinus
	case glfw.KeyPeriod:
		return platform.KeyPeriod
	case glfw.KeySlash:
		return platform.KeySlash
	case glfw.Key0:
		return platform.Key0
	case glfw.Key1:
		return platform.Key1
	case glfw.Key2:
		return platform.Key2
	case glfw.Key3:
		return platform.Key3
	case glfw.Key4:
		return platform.Key4
	case glfw.Key5:
		return platform.Key5
	case glfw.Key6:
		return platform.Key6
	case glfw.Key7:
		return platform.Key7
	case glfw.Key8:
		return platform.Key8
	case glfw.Key9:
		return platform.Key9
	case glfw.KeyA:
		return platform.KeyA
	case glfw.KeyB:
		return platform.KeyB
	case glfw.KeyC:
		return platform.KeyC
	case glfw.KeyD:
		return platform.KeyD
	case glfw.KeyE:
		return platform.KeyE
	case glfw.KeyF:
		return platform.KeyF
	case glfw.KeyG:
		return platform.KeyG
	case glfw.KeyH:
		return platform.KeyH
	case glfw.KeyI:
		return platform.KeyI
	case glfw.KeyJ:
		return platform.KeyJ
	case glfw.KeyK:
		return platform.KeyK
	case glfw.KeyL:
		return platform.KeyL
	case glfw.KeyM:
		return platform.KeyM
	case glfw.KeyN:
		return platform.KeyN
	case glfw.KeyO:
		return platform.KeyO
	case glfw.KeyP:
		return platform.KeyP
	case glfw.KeyQ:
		return platform.KeyQ
	case glfw.KeyR:
		return platform.KeyR
	case glfw.KeyS:
		return platform.KeyS
	case glfw.KeyT:
		return platform.KeyT
	case glfw.KeyU:
		return platform.KeyU
	case glfw.KeyV:
		return platform.KeyV
	case glfw.KeyW:
		return platform.KeyW
	case glfw.KeyX:
		return platform.KeyX
	case glfw.KeyY:
		return platform.KeyY
	case glfw.KeyZ:
		return platform.KeyZ
	case glfw.KeyEscape:
		return platform.KeyEscape
	case glfw.KeyEnter:
		return platform.KeyEnter
	case glfw.KeyTab:
		return platform.KeyTab
	case glfw.KeyBackspace:
		return platform.KeyBackspace
	case glfw.KeyRight:
		return platform.KeyRight
	case glfw.KeyLeft:
		return platform.KeyLeft
	case glfw.KeyDown:
		return platform.KeyDown
	case glfw.KeyUp:
		return platform.KeyUp
	case glfw.KeyLeftShift, glfw.KeyRightShift:
		return platform.KeyLeftShift
	case glfw.KeyLeftControl, glfw.KeyRightControl:
		return platform.KeyLeftControl
	case glfw.KeyLeftAlt, glfw.KeyRightAlt:
		return platform.KeyLeftAlt
	case glfw.KeyF1:
		return platform.KeyF1
	case glfw.KeyF2:
		return platform.KeyF2
	case glfw.KeyF3:
		return platform.KeyF3
	case glfw.KeyF4:
		return platform.KeyF4
	case glfw.KeyF5:
		return platform.KeyF5
	case glfw.KeyF6:
		return platform.KeyF6
	case glfw.KeyF7:
		return platform.KeyF7
	case glfw.KeyF8:
		return platform.KeyF8
	case glfw.KeyF9:
		return platform.KeyF9
	case glfw.KeyF10:
		return platform.KeyF10
	case glfw.KeyF11:
		return platform.KeyF11
	case glfw.KeyF12:
		return platform.KeyF12
	default:
		return platform.KeyUnknown
	}
}

// mapAction converts a GLFW action to a platform.Action.
func mapAction(a glfw.Action) platform.Action {
	switch a {
	case glfw.Press:
		return platform.ActionPress
	case glfw.Release:
		return platform.ActionRelease
	case glfw.Repeat:
		return platform.ActionRepeat
	default:
		return platform.ActionRelease
	}
}

// mapMods converts GLFW modifier keys to platform.Modifier.
func mapMods(m glfw.ModifierKey) platform.Modifier {
	var mods platform.Modifier
	if m&glfw.ModShift != 0 {
		mods |= platform.ModShift
	}
	if m&glfw.ModControl != 0 {
		mods |= platform.ModControl
	}
	if m&glfw.ModAlt != 0 {
		mods |= platform.ModAlt
	}
	if m&glfw.ModSuper != 0 {
		mods |= platform.ModSuper
	}
	return mods
}
