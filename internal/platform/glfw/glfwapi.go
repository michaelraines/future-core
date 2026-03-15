//go:build glfw

// This file provides pure Go GLFW bindings loaded at runtime via purego.
// No CGo is required. The GLFW shared library (libglfw.so on Linux,
// libglfw.dylib on macOS, glfw3.dll on Windows) must be available.
package glfw

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// ---------------------------------------------------------------------------
// GLFW constants
// ---------------------------------------------------------------------------

const (
	glfwTrue  = 1
	glfwFalse = 0

	// Window hints.
	glfwResizable           = 0x00020003
	glfwDecorated           = 0x00020005
	glfwContextVersionMajor = 0x00022002
	glfwContextVersionMinor = 0x00022003
	glfwOpenGLForwardCompat = 0x00022006
	glfwOpenGLProfile       = 0x00022008
	glfwOpenGLCoreProfile   = 0x00032001

	// Input modes.
	glfwCursorMode     = 0x00033001
	glfwCursorNormal   = 0x00034001
	glfwCursorHidden   = 0x00034002
	glfwCursorDisabled = 0x00034003

	// Key codes.
	glfwKeySpace        = 32
	glfwKeyApostrophe   = 39
	glfwKeyComma        = 44
	glfwKeyMinus        = 45
	glfwKeyPeriod       = 46
	glfwKeySlash        = 47
	glfwKey0            = 48
	glfwKey1            = 49
	glfwKey2            = 50
	glfwKey3            = 51
	glfwKey4            = 52
	glfwKey5            = 53
	glfwKey6            = 54
	glfwKey7            = 55
	glfwKey8            = 56
	glfwKey9            = 57
	glfwKeyA            = 65
	glfwKeyB            = 66
	glfwKeyC            = 67
	glfwKeyD            = 68
	glfwKeyE            = 69
	glfwKeyF            = 70
	glfwKeyG            = 71
	glfwKeyH            = 72
	glfwKeyI            = 73
	glfwKeyJ            = 74
	glfwKeyK            = 75
	glfwKeyL            = 76
	glfwKeyM            = 77
	glfwKeyN            = 78
	glfwKeyO            = 79
	glfwKeyP            = 80
	glfwKeyQ            = 81
	glfwKeyR            = 82
	glfwKeyS            = 83
	glfwKeyT            = 84
	glfwKeyU            = 85
	glfwKeyV            = 86
	glfwKeyW            = 87
	glfwKeyX            = 88
	glfwKeyY            = 89
	glfwKeyZ            = 90
	glfwKeyLeftBracket  = 91
	glfwKeyBackslash    = 92
	glfwKeyRightBracket = 93
	glfwKeyGraveAccent  = 96
	glfwKeyEscape       = 256
	glfwKeyEnter        = 257
	glfwKeyTab          = 258
	glfwKeyBackspace    = 259
	glfwKeyRight        = 262
	glfwKeyLeft         = 263
	glfwKeyDown         = 264
	glfwKeyUp           = 265
	glfwKeyPageUp       = 266
	glfwKeyPageDown     = 267
	glfwKeyHome         = 268
	glfwKeyEnd          = 269
	glfwKeyCapsLock     = 280
	glfwKeyScrollLock   = 281
	glfwKeyNumLock      = 282
	glfwKeyPrintScreen  = 283
	glfwKeyPause        = 284
	glfwKeyF1           = 290
	glfwKeyF2           = 291
	glfwKeyF3           = 292
	glfwKeyF4           = 293
	glfwKeyF5           = 294
	glfwKeyF6           = 295
	glfwKeyF7           = 296
	glfwKeyF8           = 297
	glfwKeyF9           = 298
	glfwKeyF10          = 299
	glfwKeyF11          = 300
	glfwKeyF12          = 301
	glfwKeyKP0          = 320
	glfwKeyKP1          = 321
	glfwKeyKP2          = 322
	glfwKeyKP3          = 323
	glfwKeyKP4          = 324
	glfwKeyKP5          = 325
	glfwKeyKP6          = 326
	glfwKeyKP7          = 327
	glfwKeyKP8          = 328
	glfwKeyKP9          = 329
	glfwKeyKPDecimal    = 330
	glfwKeyKPDivide     = 331
	glfwKeyKPMultiply   = 332
	glfwKeyKPSubtract   = 333
	glfwKeyKPAdd        = 334
	glfwKeyKPEnter      = 335
	glfwKeyKPEqual      = 336
	glfwKeyLeftShift    = 340
	glfwKeyLeftCtrl     = 341
	glfwKeyLeftAlt      = 342
	glfwKeyRightShift   = 344
	glfwKeyRightCtrl    = 345
	glfwKeyRightAlt     = 346
	glfwKeyRightSuper   = 347
	glfwKeyMenu         = 348
	glfwKeySemicolon    = 59
	glfwKeyEqual        = 61
	glfwKeyLeftSuper    = 343
	glfwKeyInsert       = 260
	glfwKeyDelete       = 261

	// Actions.
	glfwRelease = 0
	glfwPress   = 1
	glfwRepeat  = 2

	// Modifier keys.
	glfwModShift   = 0x0001
	glfwModControl = 0x0002
	glfwModAlt     = 0x0004
	glfwModSuper   = 0x0008

	// Joystick IDs.
	glfwJoystick1    = 0
	glfwJoystickLast = 15
)

// ---------------------------------------------------------------------------
// GLFW function variables
// ---------------------------------------------------------------------------

var (
	fnGlfwInit               func() int32
	fnGlfwTerminate          func()
	fnGlfwWindowHint         func(hint, value int32)
	fnGlfwCreateWindow       func(width, height int32, title *byte, monitor, share uintptr) uintptr
	fnGlfwDestroyWindow      func(window uintptr)
	fnGlfwWindowShouldClose  func(window uintptr) int32
	fnGlfwPollEvents         func()
	fnGlfwSwapBuffers        func(window uintptr)
	fnGlfwSwapInterval       func(interval int32)
	fnGlfwMakeContextCurrent func(window uintptr)
	fnGlfwGetWindowSize      func(window uintptr, width, height *int32)
	fnGlfwSetWindowSize      func(window uintptr, width, height int32)
	fnGlfwGetFramebufferSize func(window uintptr, width, height *int32)
	fnGlfwGetWindowPos       func(window uintptr, xpos, ypos *int32)
	fnGlfwSetWindowTitle     func(window uintptr, title *byte)
	fnGlfwSetWindowMonitor   func(window uintptr, monitor uintptr, xpos, ypos, width, height, refreshRate int32)
	fnGlfwSetInputMode       func(window uintptr, mode, value int32)
	fnGlfwGetCursorPos       func(window uintptr, xpos, ypos *float64)
	fnGlfwGetPrimaryMonitor  func() uintptr
	fnGlfwGetVideoMode       func(monitor uintptr) uintptr // returns *GLFWvidmode

	fnGlfwSetKeyCallback             func(window, cbfun uintptr) uintptr
	fnGlfwSetCharCallback            func(window, cbfun uintptr) uintptr
	fnGlfwSetMouseButtonCallback     func(window, cbfun uintptr) uintptr
	fnGlfwSetCursorPosCallback       func(window, cbfun uintptr) uintptr
	fnGlfwSetScrollCallback          func(window, cbfun uintptr) uintptr
	fnGlfwSetFramebufferSizeCallback func(window, cbfun uintptr) uintptr

	// Joystick/gamepad functions.
	fnGlfwJoystickPresent    func(jid int32) int32
	fnGlfwGetJoystickAxes    func(jid int32, count *int32) uintptr
	fnGlfwGetJoystickButtons func(jid int32, count *int32) uintptr
)

// glfwVideoMode mirrors the C GLFWvidmode struct.
type glfwVideoMode struct {
	Width       int32
	Height      int32
	RedBits     int32
	GreenBits   int32
	BlueBits    int32
	RefreshRate int32
}

// ---------------------------------------------------------------------------
// GLFW library loading
// ---------------------------------------------------------------------------

var glfwLib uintptr

func initGLFWAPI() error {
	var err error
	glfwLib, err = openGLFWLib()
	if err != nil {
		return err
	}

	must := func(fn interface{}, name string) error {
		addr, serr := purego.Dlsym(glfwLib, name)
		if serr != nil {
			return fmt.Errorf("glfw: symbol %s: %w", name, serr)
		}
		purego.RegisterFunc(fn, addr)
		return nil
	}

	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnGlfwInit, "glfwInit"},
		{&fnGlfwTerminate, "glfwTerminate"},
		{&fnGlfwWindowHint, "glfwWindowHint"},
		{&fnGlfwCreateWindow, "glfwCreateWindow"},
		{&fnGlfwDestroyWindow, "glfwDestroyWindow"},
		{&fnGlfwWindowShouldClose, "glfwWindowShouldClose"},
		{&fnGlfwPollEvents, "glfwPollEvents"},
		{&fnGlfwSwapBuffers, "glfwSwapBuffers"},
		{&fnGlfwSwapInterval, "glfwSwapInterval"},
		{&fnGlfwMakeContextCurrent, "glfwMakeContextCurrent"},
		{&fnGlfwGetWindowSize, "glfwGetWindowSize"},
		{&fnGlfwSetWindowSize, "glfwSetWindowSize"},
		{&fnGlfwGetFramebufferSize, "glfwGetFramebufferSize"},
		{&fnGlfwGetWindowPos, "glfwGetWindowPos"},
		{&fnGlfwSetWindowTitle, "glfwSetWindowTitle"},
		{&fnGlfwSetWindowMonitor, "glfwSetWindowMonitor"},
		{&fnGlfwSetInputMode, "glfwSetInputMode"},
		{&fnGlfwGetCursorPos, "glfwGetCursorPos"},
		{&fnGlfwGetPrimaryMonitor, "glfwGetPrimaryMonitor"},
		{&fnGlfwGetVideoMode, "glfwGetVideoMode"},
		{&fnGlfwSetKeyCallback, "glfwSetKeyCallback"},
		{&fnGlfwSetCharCallback, "glfwSetCharCallback"},
		{&fnGlfwSetMouseButtonCallback, "glfwSetMouseButtonCallback"},
		{&fnGlfwSetCursorPosCallback, "glfwSetCursorPosCallback"},
		{&fnGlfwSetScrollCallback, "glfwSetScrollCallback"},
		{&fnGlfwSetFramebufferSizeCallback, "glfwSetFramebufferSizeCallback"},
		{&fnGlfwJoystickPresent, "glfwJoystickPresent"},
		{&fnGlfwGetJoystickAxes, "glfwGetJoystickAxes"},
		{&fnGlfwGetJoystickButtons, "glfwGetJoystickButtons"},
	} {
		if ferr := must(e.fn, e.name); ferr != nil {
			return ferr
		}
	}

	return nil
}

func openGLFWLib() (uintptr, error) {
	var names []string
	switch runtime.GOOS {
	case "darwin":
		names = []string{"libglfw.3.dylib", "libglfw.dylib"}
	case "windows":
		names = []string{"glfw3.dll"}
	default: // linux, freebsd, etc.
		names = []string{"libglfw.so.3", "libglfw.so"}
	}

	var firstErr error
	for _, name := range names {
		h, err := purego.Dlopen(name, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err == nil {
			return h, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return 0, fmt.Errorf("failed to load GLFW: %w", firstErr)
}

// cStr converts a Go string to a null-terminated byte pointer.
// Safe when passed directly as a purego function argument (pinned during call).
func cStr(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// getVideoMode reads the GLFWvidmode struct from a pointer.
func getVideoMode(ptr uintptr) glfwVideoMode {
	return *(*glfwVideoMode)(unsafe.Pointer(ptr))
}
