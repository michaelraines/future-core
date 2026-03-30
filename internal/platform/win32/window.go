//go:build windows

package win32

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/michaelraines/future-core/internal/platform"
)

func init() {
	runtime.LockOSThread()
}

// Window implements platform.Window using the Win32 API.
type Window struct {
	hwnd      uintptr
	hdc       uintptr
	hglrc     uintptr
	hInstance uintptr
	className *uint16

	handler     platform.InputHandler
	shouldClose bool
	fullscreen  bool
	noGL        bool

	// Saved geometry for fullscreen restore.
	savedStyle uint32
	savedRect  rect

	// Window size in logical pixels (cached from WM_SIZE).
	width, height int

	// Cursor state.
	cursorHidden bool
	cursorLocked bool

	// Cursor tracking for delta computation.
	prevCursorX, prevCursorY float64
	hasPrevCursor            bool

	// Track whether we've requested mouse leave tracking.
	trackingMouse bool
}

// New creates a new Win32 window (uninitialized — call Create to open it).
func New() *Window {
	return &Window{}
}

// windowMap maps HWND → Window for use in the window procedure.
var windowMap = map[uintptr]*Window{}

// Create creates and shows a Win32 window with an OpenGL context.
func (w *Window) Create(cfg platform.WindowConfig) error {
	initDPIAwareness()
	w.hInstance = getModuleHandle()

	// Register window class.
	w.className = utf16Ptr("FutureRenderWindow")
	wc := wndClassExW{
		Size:      uint32(unsafe.Sizeof(wndClassExW{})),
		Style:     0x0003, // CS_HREDRAW | CS_VREDRAW
		WndProc:   syscall.NewCallback(wndProc),
		Instance:  w.hInstance,
		Cursor:    loadCursor(idcArrow),
		ClassName: w.className,
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return fmt.Errorf("RegisterClassExW failed: %w", err)
	}

	// Calculate window rect for desired client area.
	style := uintptr(wsOverlappedWindow)
	if !cfg.Resizable {
		style &^= wsThickFrame | wsMaximizeBox
	}
	if !cfg.Decorated {
		style = wsPopup | wsVisible
	}

	r := rect{
		Left: 0, Top: 0,
		Right: int32(cfg.Width), Bottom: int32(cfg.Height),
	}
	procAdjustWindowRectEx.Call(
		uintptr(unsafe.Pointer(&r)),
		style,
		0, // no menu
		uintptr(wsExAppWindow),
	)

	winWidth := r.Right - r.Left
	winHeight := r.Bottom - r.Top

	// Center on screen.
	screenW, _, _ := procGetSystemMetrics.Call(smCxScreen)
	screenH, _, _ := procGetSystemMetrics.Call(smCyScreen)
	x := (int32(screenW) - winWidth) / 2
	y := (int32(screenH) - winHeight) / 2

	titlePtr := utf16Ptr(cfg.Title)
	hwnd, _, err := procCreateWindowExW.Call(
		uintptr(wsExAppWindow),
		uintptr(unsafe.Pointer(w.className)),
		uintptr(unsafe.Pointer(titlePtr)),
		style,
		uintptr(x), uintptr(y),
		uintptr(winWidth), uintptr(winHeight),
		0, 0, // parent, menu
		w.hInstance,
		0, // lpParam
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW failed: %w", err)
	}
	w.hwnd = hwnd
	w.width = cfg.Width
	w.height = cfg.Height

	// Store window pointer for wndProc lookup.
	windowMap[hwnd] = w
	procSetWindowLongPtrW.Call(hwnd, negIndex(gwlpUserData), uintptr(unsafe.Pointer(w)))

	w.noGL = cfg.NoGL

	if !cfg.NoGL {
		// Initialize OpenGL.
		w.hdc, _, _ = procGetDC.Call(hwnd)
		if w.hdc == 0 {
			w.Destroy()
			return fmt.Errorf("GetDC failed")
		}

		pfd := pixelFormatDescriptor{
			Size:        uint16(unsafe.Sizeof(pixelFormatDescriptor{})),
			Version:     1,
			Flags:       pfdDrawToWindow | pfdSupportOpenGL | pfdDoubleBuffer,
			PixelType:   pfdTypeRGBA,
			ColorBits:   32,
			DepthBits:   24,
			StencilBits: 8,
		}

		pixelFmt, _, _ := procChoosePixelFormat.Call(w.hdc, uintptr(unsafe.Pointer(&pfd)))
		if pixelFmt == 0 {
			w.Destroy()
			return fmt.Errorf("ChoosePixelFormat failed")
		}

		ret, _, _ := procSetPixelFormat.Call(w.hdc, pixelFmt, uintptr(unsafe.Pointer(&pfd)))
		if ret == 0 {
			w.Destroy()
			return fmt.Errorf("SetPixelFormat failed")
		}

		w.hglrc, _, _ = procWglCreateContext.Call(w.hdc)
		if w.hglrc == 0 {
			w.Destroy()
			return fmt.Errorf("wglCreateContext failed")
		}

		ret, _, _ = procWglMakeCurrent.Call(w.hdc, w.hglrc)
		if ret == 0 {
			w.Destroy()
			return fmt.Errorf("wglMakeCurrent failed")
		}

		// Enable VSync via wglSwapIntervalEXT if available.
		if cfg.VSync {
			w.setSwapInterval(1)
		} else {
			w.setSwapInterval(0)
		}
	}

	// Show the window.
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)
	procSetFocus.Call(hwnd)

	if cfg.Fullscreen {
		w.SetFullscreen(true)
	}

	return nil
}

// setSwapInterval tries to set vsync via WGL_EXT_swap_control.
func (w *Window) setSwapInterval(interval int) {
	name, _ := syscall.BytePtrFromString("wglSwapIntervalEXT")
	addr, _, _ := procWglGetProcAddress.Call(uintptr(unsafe.Pointer(name)))
	if addr == 0 {
		return
	}
	syscall.SyscallN(addr, uintptr(interval))
}

// loadCursor loads a system cursor by ID.
func loadCursor(id uintptr) uintptr {
	ret, _, _ := procLoadCursorW.Call(0, id)
	return ret
}

// Destroy closes the window and releases resources.
func (w *Window) Destroy() {
	if w.hglrc != 0 {
		procWglMakeCurrent.Call(0, 0)
		procWglDeleteContext.Call(w.hglrc)
		w.hglrc = 0
	}
	if w.hdc != 0 {
		procReleaseDC.Call(w.hwnd, w.hdc)
		w.hdc = 0
	}
	if w.hwnd != 0 {
		delete(windowMap, w.hwnd)
		procDestroyWindow.Call(w.hwnd)
		w.hwnd = 0
	}
}

// ShouldClose returns whether the window close has been requested.
func (w *Window) ShouldClose() bool {
	return w.shouldClose
}

// PollEvents processes pending Win32 messages.
func (w *Window) PollEvents() {
	var m msg
	for {
		ret, _, _ := procPeekMessageW.Call(
			uintptr(unsafe.Pointer(&m)),
			0, 0, 0,
			pmRemove,
		)
		if ret == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// SwapBuffers swaps the OpenGL front and back buffers. No-op when NoGL is set.
func (w *Window) SwapBuffers() {
	if w.noGL || w.hdc == 0 {
		return
	}
	procSwapBuffers.Call(w.hdc)
}

// Size returns the window client area size in logical pixels.
func (w *Window) Size() (int, int) {
	if w.hwnd == 0 {
		return 0, 0
	}
	var r rect
	procGetClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&r)))
	return int(r.Right - r.Left), int(r.Bottom - r.Top)
}

// FramebufferSize returns the framebuffer size in physical pixels.
// On Windows with DPI scaling, this may differ from Size().
func (w *Window) FramebufferSize() (int, int) {
	// For standard DPI awareness, client rect is already in physical pixels.
	return w.Size()
}

// DevicePixelRatio returns the ratio of physical to logical pixels.
func (w *Window) DevicePixelRatio() float64 {
	if w.hwnd == 0 {
		return 1.0
	}
	dpi := getDPIForWindow(w.hwnd)
	if dpi == 0 {
		return 1.0
	}
	return float64(dpi) / float64(defaultDPI)
}

// SetTitle sets the window title.
func (w *Window) SetTitle(title string) {
	if w.hwnd == 0 {
		return
	}
	procSetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(utf16Ptr(title))))
}

// SetSize sets the window client area size in logical pixels.
func (w *Window) SetSize(width, height int) {
	if w.hwnd == 0 {
		return
	}
	style, _, _ := procGetWindowLongPtrW.Call(w.hwnd, negIndex(gwlpStyle))
	r := rect{Right: int32(width), Bottom: int32(height)}
	procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&r)), style, 0, uintptr(wsExAppWindow))
	procSetWindowPos.Call(
		w.hwnd, 0,
		0, 0,
		uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
		swpNoMove|swpNoZOrder,
	)
}

// SetFullscreen toggles fullscreen mode.
func (w *Window) SetFullscreen(fullscreen bool) {
	if w.hwnd == 0 || fullscreen == w.fullscreen {
		return
	}
	w.fullscreen = fullscreen

	if fullscreen {
		// Save current style and rect.
		w.savedStyle = uint32(w.getStyle())
		procGetWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&w.savedRect)))

		// Get monitor info.
		monitor, _, _ := procMonitorFromWindow.Call(w.hwnd, monitorDefaultToNearest)
		mi := monitorInfoExW{Size: uint32(unsafe.Sizeof(monitorInfoExW{}))}
		procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&mi)))

		// Remove decorations and set to monitor size.
		procSetWindowLongPtrW.Call(w.hwnd, negIndex(gwlpStyle), uintptr(wsPopup|wsVisible))
		procSetWindowPos.Call(
			w.hwnd, 0,
			uintptr(mi.Monitor.Left), uintptr(mi.Monitor.Top),
			uintptr(mi.Monitor.Right-mi.Monitor.Left),
			uintptr(mi.Monitor.Bottom-mi.Monitor.Top),
			swpNoZOrder,
		)
	} else {
		// Restore original style and position.
		procSetWindowLongPtrW.Call(w.hwnd, negIndex(gwlpStyle), uintptr(w.savedStyle))
		procSetWindowPos.Call(
			w.hwnd, 0,
			uintptr(w.savedRect.Left), uintptr(w.savedRect.Top),
			uintptr(w.savedRect.Right-w.savedRect.Left),
			uintptr(w.savedRect.Bottom-w.savedRect.Top),
			swpNoZOrder,
		)
	}
}

func (w *Window) getStyle() uintptr {
	ret, _, _ := procGetWindowLongPtrW.Call(w.hwnd, negIndex(gwlpStyle))
	return ret
}

// IsFullscreen returns whether the window is in fullscreen mode.
func (w *Window) IsFullscreen() bool {
	return w.fullscreen
}

// SetCursorVisible shows or hides the cursor.
func (w *Window) SetCursorVisible(visible bool) {
	if visible == !w.cursorHidden {
		return
	}
	if visible {
		procShowCursor.Call(1)
		w.cursorHidden = false
	} else {
		procShowCursor.Call(0)
		w.cursorHidden = true
	}
}

// SetCursorLocked locks or unlocks the cursor to the window client area.
func (w *Window) SetCursorLocked(locked bool) {
	if locked == w.cursorLocked {
		return
	}
	w.cursorLocked = locked
	if locked {
		var r rect
		procGetClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&r)))
		// Convert client rect to screen coordinates.
		var topLeft, bottomRight point
		topLeft.X = r.Left
		topLeft.Y = r.Top
		bottomRight.X = r.Right
		bottomRight.Y = r.Bottom
		clientToScreen(w.hwnd, &topLeft)
		clientToScreen(w.hwnd, &bottomRight)
		clipRect := rect{
			Left: topLeft.X, Top: topLeft.Y,
			Right: bottomRight.X, Bottom: bottomRight.Y,
		}
		procClipCursor.Call(uintptr(unsafe.Pointer(&clipRect)))
	} else {
		procClipCursor.Call(0)
	}
}

func clientToScreen(hwnd uintptr, pt *point) {
	// ClientToScreen is just ScreenToClient in reverse, but we can use the
	// actual ClientToScreen function.
	proc := user32.NewProc("ClientToScreen")
	proc.Call(hwnd, uintptr(unsafe.Pointer(pt)))
}

// NativeHandle returns the HWND as a uintptr.
func (w *Window) NativeHandle() uintptr {
	return w.hwnd
}

// SetInputHandler sets the handler for input events.
func (w *Window) SetInputHandler(handler platform.InputHandler) {
	w.handler = handler
}

// PollGamepads queries XInput for connected controllers and dispatches events.
func (w *Window) PollGamepads() {
	if w.handler == nil {
		return
	}
	pollGamepadsXInput(w.handler)
}

// ---------------------------------------------------------------------------
// Window procedure
// ---------------------------------------------------------------------------

func wndProc(hwnd uintptr, umsg uint32, wParam, lParam uintptr) uintptr {
	w := windowMap[hwnd]
	if w == nil {
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wParam, lParam)
		return ret
	}

	switch umsg {
	case wmClose:
		w.shouldClose = true
		return 0

	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0

	case wmSize:
		width := int(loWord(lParam))
		height := int(hiWord(lParam))
		w.width = width
		w.height = height
		if w.handler != nil {
			w.handler.OnResizeEvent(width, height)
		}
		return 0

	case wmKeyDown, wmSysKeyDown:
		if w.handler != nil {
			vk := uint32(wParam)
			action := platform.ActionPress
			// Bit 30 of lParam: 1 = key was down before, so this is a repeat.
			if lParam&(1<<30) != 0 {
				action = platform.ActionRepeat
			}
			w.handler.OnKeyEvent(platform.KeyEvent{
				Key:    mapVKKey(vk),
				Action: action,
				Mods:   mapWin32Mods(),
			})
		}
		// Let DefWindowProc handle syskeys (Alt+F4, etc.).
		if umsg == wmSysKeyDown {
			break
		}
		return 0

	case wmKeyUp, wmSysKeyUp:
		if w.handler != nil {
			w.handler.OnKeyEvent(platform.KeyEvent{
				Key:    mapVKKey(uint32(wParam)),
				Action: platform.ActionRelease,
				Mods:   mapWin32Mods(),
			})
		}
		if umsg == wmSysKeyUp {
			break
		}
		return 0

	case wmChar:
		if w.handler != nil {
			w.handler.OnCharEvent(rune(wParam))
		}
		return 0

	case wmLButtonDown:
		w.handleMouseButton(lParam, platform.MouseButtonLeft, platform.ActionPress)
		return 0

	case wmLButtonUp:
		w.handleMouseButton(lParam, platform.MouseButtonLeft, platform.ActionRelease)
		return 0

	case wmRButtonDown:
		w.handleMouseButton(lParam, platform.MouseButtonRight, platform.ActionPress)
		return 0

	case wmRButtonUp:
		w.handleMouseButton(lParam, platform.MouseButtonRight, platform.ActionRelease)
		return 0

	case wmMButtonDown:
		w.handleMouseButton(lParam, platform.MouseButtonMiddle, platform.ActionPress)
		return 0

	case wmMButtonUp:
		w.handleMouseButton(lParam, platform.MouseButtonMiddle, platform.ActionRelease)
		return 0

	case wmXButtonDown:
		btn := platform.MouseButton4
		if hiWord(wParam) == 2 {
			btn = platform.MouseButton5
		}
		w.handleMouseButton(lParam, btn, platform.ActionPress)
		return 1 // Must return TRUE for XBUTTON messages

	case wmXButtonUp:
		btn := platform.MouseButton4
		if hiWord(wParam) == 2 {
			btn = platform.MouseButton5
		}
		w.handleMouseButton(lParam, btn, platform.ActionRelease)
		return 1

	case wmMouseMove:
		if w.handler != nil {
			x := float64(loWord(lParam))
			y := float64(hiWord(lParam))
			var dx, dy float64
			if w.hasPrevCursor {
				dx = x - w.prevCursorX
				dy = y - w.prevCursorY
			}
			w.prevCursorX = x
			w.prevCursorY = y
			w.hasPrevCursor = true

			w.handler.OnMouseMoveEvent(platform.MouseMoveEvent{
				X: x, Y: y, DX: dx, DY: dy,
			})

			// Request WM_MOUSELEAVE tracking if not yet active.
			if !w.trackingMouse {
				tme := trackMouseEventStruct{
					Size:      uint32(unsafe.Sizeof(trackMouseEventStruct{})),
					Flags:     tmeLeave,
					HwndTrack: hwnd,
				}
				procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
				w.trackingMouse = true
			}
		}
		return 0

	case wmMouseLeave:
		w.trackingMouse = false
		return 0

	case wmMouseWheel:
		if w.handler != nil {
			delta := float64(hiWord(wParam)) / float64(wheelDelta)
			w.handler.OnMouseScrollEvent(platform.MouseScrollEvent{
				DX: 0, DY: delta,
			})
		}
		return 0

	case wmMouseHWheel:
		if w.handler != nil {
			delta := float64(hiWord(wParam)) / float64(wheelDelta)
			w.handler.OnMouseScrollEvent(platform.MouseScrollEvent{
				DX: delta, DY: 0,
			})
		}
		return 0

	case wmSetCursor:
		// When cursor is hidden, prevent Windows from resetting it on mouse move.
		if w.cursorHidden && loWord(lParam) == 1 { // HTCLIENT = 1
			procSetCursor.Call(0) // NULL cursor
			return 1              // TRUE — we handled it
		}

	case wmDPIChanged:
		// lParam points to a RECT with the suggested new window position/size.
		if lParam != 0 {
			suggested := (*rect)(unsafe.Pointer(lParam))
			procSetWindowPos.Call(
				hwnd, 0,
				uintptr(suggested.Left), uintptr(suggested.Top),
				uintptr(suggested.Right-suggested.Left),
				uintptr(suggested.Bottom-suggested.Top),
				swpNoZOrder,
			)
		}
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wParam, lParam)
	return ret
}

func (w *Window) handleMouseButton(lParam uintptr, button platform.MouseButton, action platform.Action) {
	if w.handler == nil {
		return
	}
	x := float64(loWord(lParam))
	y := float64(hiWord(lParam))
	w.handler.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: button,
		Action: action,
		X:      x,
		Y:      y,
		Mods:   mapWin32Mods(),
	})
}
