//go:build windows

// Package win32 implements platform.Window using the Win32 API via syscalls.
// No CGo required — all system calls go through golang.org/x/sys/windows or
// syscall.SyscallN.
package win32

import (
	"syscall"
	"unsafe"
)

// ---------------------------------------------------------------------------
// DLL handles and procedure addresses
// ---------------------------------------------------------------------------

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	opengl32 = syscall.NewLazyDLL("opengl32.dll")
)

// user32 procedures.
var (
	procRegisterClassExW         = user32.NewProc("RegisterClassExW")
	procCreateWindowExW          = user32.NewProc("CreateWindowExW")
	procDestroyWindow            = user32.NewProc("DestroyWindow")
	procShowWindow               = user32.NewProc("ShowWindow")
	procUpdateWindow             = user32.NewProc("UpdateWindow")
	procDefWindowProcW           = user32.NewProc("DefWindowProcW")
	procPeekMessageW             = user32.NewProc("PeekMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")
	procPostQuitMessage          = user32.NewProc("PostQuitMessage")
	procGetClientRect            = user32.NewProc("GetClientRect")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procSetWindowTextW           = user32.NewProc("SetWindowTextW")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
	procGetDC                    = user32.NewProc("GetDC")
	procReleaseDC                = user32.NewProc("ReleaseDC")
	procSetWindowLongPtrW        = user32.NewProc("SetWindowLongPtrW")
	procGetWindowLongPtrW        = user32.NewProc("GetWindowLongPtrW")
	procAdjustWindowRectEx       = user32.NewProc("AdjustWindowRectEx")
	procGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	procShowCursor               = user32.NewProc("ShowCursor")
	procSetCapture               = user32.NewProc("SetCapture")
	procReleaseCapture           = user32.NewProc("ReleaseCapture")
	procClipCursor               = user32.NewProc("ClipCursor")
	procGetCursorPos             = user32.NewProc("GetCursorPos")
	procScreenToClient           = user32.NewProc("ScreenToClient")
	procSetCursorPos             = user32.NewProc("SetCursorPos")
	procLoadCursorW              = user32.NewProc("LoadCursorW")
	procGetMonitorInfoW          = user32.NewProc("GetMonitorInfoW")
	procMonitorFromWindow        = user32.NewProc("MonitorFromWindow")
	procChangeDisplaySettingsExW = user32.NewProc("ChangeDisplaySettingsExW")
	procGetKeyState              = user32.NewProc("GetKeyState")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procSetFocus                 = user32.NewProc("SetFocus")
	procTrackMouseEvent          = user32.NewProc("TrackMouseEvent")
	procSetCursor                = user32.NewProc("SetCursor")
)

// kernel32 procedures.
var (
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

// gdi32 procedures.
var (
	procChoosePixelFormat   = gdi32.NewProc("ChoosePixelFormat")
	procSetPixelFormat      = gdi32.NewProc("SetPixelFormat")
	procSwapBuffers         = gdi32.NewProc("SwapBuffers")
	procDescribePixelFormat = gdi32.NewProc("DescribePixelFormat")
)

// opengl32 procedures.
var (
	procWglCreateContext  = opengl32.NewProc("wglCreateContext")
	procWglDeleteContext  = opengl32.NewProc("wglDeleteContext")
	procWglMakeCurrent    = opengl32.NewProc("wglMakeCurrent")
	procWglGetProcAddress = opengl32.NewProc("wglGetProcAddress")
)

// ---------------------------------------------------------------------------
// Win32 constants
// ---------------------------------------------------------------------------

const (
	// Window styles.
	wsOverlapped       = 0x00000000
	wsCaption          = 0x00C00000
	wsSysMenu          = 0x00080000
	wsThickFrame       = 0x00040000
	wsMinimizeBox      = 0x00020000
	wsMaximizeBox      = 0x00010000
	wsOverlappedWindow = wsOverlapped | wsCaption | wsSysMenu | wsThickFrame | wsMinimizeBox | wsMaximizeBox
	wsPopup            = 0x80000000
	wsVisible          = 0x10000000

	// Extended window styles.
	wsExAppWindow = 0x00040000

	// Window messages.
	wmDestroy       = 0x0002
	wmSize          = 0x0005
	wmClose         = 0x0010
	wmKeyDown       = 0x0100
	wmKeyUp         = 0x0101
	wmChar          = 0x0102
	wmSysKeyDown    = 0x0104
	wmSysKeyUp      = 0x0105
	wmMouseMove     = 0x0200
	wmLButtonDown   = 0x0201
	wmLButtonUp     = 0x0202
	wmRButtonDown   = 0x0204
	wmRButtonUp     = 0x0205
	wmMButtonDown   = 0x0207
	wmMButtonUp     = 0x0208
	wmMouseWheel    = 0x020A
	wmMouseHWheel   = 0x020E
	wmXButtonDown   = 0x020B
	wmXButtonUp     = 0x020C
	wmMouseLeave    = 0x02A3
	wmSetCursor     = 0x0020
	wmDPIChanged    = 0x02E0
	wmEnterSizeMove = 0x0231
	wmExitSizeMove  = 0x0232

	// PeekMessage flags.
	pmRemove = 0x0001

	// ShowWindow commands.
	swShow = 5

	// SetWindowPos flags.
	swpNoZOrder = 0x0004
	swpNoMove   = 0x0002
	swpNoSize   = 0x0001

	// GetSystemMetrics indices.
	smCxScreen = 0
	smCyScreen = 1

	// MonitorFromWindow flags.
	monitorDefaultToNearest = 0x00000002

	// PIXELFORMATDESCRIPTOR flags.
	pfdDrawToWindow  = 0x00000004
	pfdSupportOpenGL = 0x00000020
	pfdDoubleBuffer  = 0x00000001
	pfdTypeRGBA      = 0

	// ChangeDisplaySettings flags.
	cdsFullscreen = 0x00000004

	// GWLP indices (negative values need special handling via gwlpToUintptr).
	gwlpUserData = -21
	gwlpStyle    = -16

	// Cursor IDs.
	idcArrow = 32512

	// Mouse wheel delta.
	wheelDelta = 120

	// TrackMouseEvent flags.
	tmeLeave = 0x00000002

	// Virtual key codes for modifier checks.
	vkShift   = 0x10
	vkControl = 0x11
	vkMenu    = 0x12 // Alt
	vkLWin    = 0x5B
	vkRWin    = 0x5C
	vkCapital = 0x14 // Caps Lock
	vkNumLock = 0x90

	// Key state mask.
	keyStateDown = 0x8000
)

// ---------------------------------------------------------------------------
// Win32 structures
// ---------------------------------------------------------------------------

// wndClassExW is the WNDCLASSEXW structure.
type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

// point is the POINT structure.
type point struct {
	X, Y int32
}

// rect is the RECT structure.
type rect struct {
	Left, Top, Right, Bottom int32
}

// msg is the MSG structure.
type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

// pixelFormatDescriptor is the PIXELFORMATDESCRIPTOR structure.
type pixelFormatDescriptor struct {
	Size           uint16
	Version        uint16
	Flags          uint32
	PixelType      uint8
	ColorBits      uint8
	RedBits        uint8
	RedShift       uint8
	GreenBits      uint8
	GreenShift     uint8
	BlueBits       uint8
	BlueShift      uint8
	AlphaBits      uint8
	AlphaShift     uint8
	AccumBits      uint8
	AccumRedBits   uint8
	AccumGreenBits uint8
	AccumBlueBits  uint8
	AccumAlphaBits uint8
	DepthBits      uint8
	StencilBits    uint8
	AuxBuffers     uint8
	LayerType      uint8
	Reserved       uint8
	LayerMask      uint32
	VisibleMask    uint32
	DamageMask     uint32
}

// monitorInfoExW is the MONITORINFOEXW structure.
type monitorInfoExW struct {
	Size     uint32
	Monitor  rect
	WorkArea rect
	Flags    uint32
	Device   [32]uint16
}

// trackMouseEventStruct is the TRACKMOUSEEVENT structure.
type trackMouseEventStruct struct {
	Size      uint32
	Flags     uint32
	HwndTrack uintptr
	HoverTime uint32
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// utf16Ptr converts a Go string to a UTF-16 pointer for Win32 APIs.
func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

// loWord extracts the low 16 bits of a uintptr.
func loWord(l uintptr) int16 {
	return int16(l & 0xFFFF)
}

// hiWord extracts the high 16 bits of a uintptr.
func hiWord(l uintptr) int16 {
	return int16((l >> 16) & 0xFFFF)
}

// getModuleHandle returns the module handle for the current executable.
func getModuleHandle() uintptr {
	ret, _, _ := procGetModuleHandleW.Call(0)
	return ret
}

// getKeyStateDown returns true if the specified virtual key is currently pressed.
func getKeyStateDown(vk int) bool {
	ret, _, _ := procGetKeyState.Call(uintptr(vk))
	return ret&keyStateDown != 0
}

// getKeyStateToggled returns true if the specified toggle key (Caps Lock,
// Num Lock, Scroll Lock) is currently active. Toggle state is in bit 0.
func getKeyStateToggled(vk int) bool {
	ret, _, _ := procGetKeyState.Call(uintptr(vk))
	return ret&0x0001 != 0
}

// makeIntResource converts an integer resource ID to a pointer.
func makeIntResource(id uintptr) *uint16 {
	return (*uint16)(unsafe.Pointer(id))
}

// negIndex converts a negative int constant to the uintptr representation
// that Win32 SetWindowLongPtr/GetWindowLongPtr expect. The int32 → int
// conversion sign-extends on 64-bit, then int → uintptr preserves the
// sign-extended bit pattern.
func negIndex(n int32) uintptr {
	return uintptr(int(n))
}
