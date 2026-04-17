//go:build android

package android

import "github.com/michaelraines/future-core/internal/platform"

// Android MotionEvent action constants (the masked action, not the full
// actionIndex-encoded value). Mirrored from android.view.MotionEvent so the
// JNI bridge can pass primitives without pulling in x/mobile.
const (
	MotionActionDown        = 0
	MotionActionUp          = 1
	MotionActionMove        = 2
	MotionActionCancel      = 3
	MotionActionOutside     = 4
	MotionActionPointerDown = 5
	MotionActionPointerUp   = 6
)

// Android InputDevice source bit flags (subset — anything with the
// SOURCE_CLASS_JOYSTICK or SOURCE_GAMEPAD bit is treated as a gamepad).
const (
	SourceClassJoystick = 0x01000000
	SourceGamepad       = 0x00000401
)

// IsGamepadSource reports whether an Android input source code describes a
// gamepad or joystick device.
func IsGamepadSource(source int) bool {
	return source&SourceClassJoystick != 0 || source&SourceGamepad == SourceGamepad
}

// Android KeyEvent meta-state bit flags (subset).
const (
	MetaShiftOn    = 0x1
	MetaAltOn      = 0x2
	MetaSymOn      = 0x4
	MetaCtrlOn     = 0x1000
	MetaMetaOn     = 0x10000
	MetaCapsLockOn = 0x100000
	MetaNumLockOn  = 0x200000
)

// Android KeyEvent.KEYCODE_* subset we care about — mapped to platform.Key.
// Values are from android.view.KeyEvent. Anything not listed maps to
// platform.KeyUnknown.
var androidRawKeyMap = map[int]platform.Key{
	// Letters (KEYCODE_A=29 through KEYCODE_Z=54)
	29: platform.KeyA, 30: platform.KeyB, 31: platform.KeyC, 32: platform.KeyD,
	33: platform.KeyE, 34: platform.KeyF, 35: platform.KeyG, 36: platform.KeyH,
	37: platform.KeyI, 38: platform.KeyJ, 39: platform.KeyK, 40: platform.KeyL,
	41: platform.KeyM, 42: platform.KeyN, 43: platform.KeyO, 44: platform.KeyP,
	45: platform.KeyQ, 46: platform.KeyR, 47: platform.KeyS, 48: platform.KeyT,
	49: platform.KeyU, 50: platform.KeyV, 51: platform.KeyW, 52: platform.KeyX,
	53: platform.KeyY, 54: platform.KeyZ,

	// Digits (KEYCODE_0=7 through KEYCODE_9=16)
	7: platform.Key0, 8: platform.Key1, 9: platform.Key2, 10: platform.Key3,
	11: platform.Key4, 12: platform.Key5, 13: platform.Key6, 14: platform.Key7,
	15: platform.Key8, 16: platform.Key9,

	// Navigation / control
	66: platform.KeyEnter,     // KEYCODE_ENTER
	62: platform.KeySpace,     // KEYCODE_SPACE
	67: platform.KeyBackspace, // KEYCODE_DEL
	112: platform.KeyDelete,   // KEYCODE_FORWARD_DEL
	61: platform.KeyTab,       // KEYCODE_TAB
	111: platform.KeyEscape,   // KEYCODE_ESCAPE
	4: platform.KeyEscape,     // KEYCODE_BACK (treat as Escape)

	// Arrows
	19: platform.KeyUp,    // KEYCODE_DPAD_UP
	20: platform.KeyDown,  // KEYCODE_DPAD_DOWN
	21: platform.KeyLeft,  // KEYCODE_DPAD_LEFT
	22: platform.KeyRight, // KEYCODE_DPAD_RIGHT

	// Modifiers
	59: platform.KeyLeftShift,   // KEYCODE_SHIFT_LEFT
	60: platform.KeyRightShift,  // KEYCODE_SHIFT_RIGHT
	113: platform.KeyLeftControl,  // KEYCODE_CTRL_LEFT
	114: platform.KeyRightControl, // KEYCODE_CTRL_RIGHT
	57: platform.KeyLeftAlt,     // KEYCODE_ALT_LEFT
	58: platform.KeyRightAlt,    // KEYCODE_ALT_RIGHT
	117: platform.KeyLeftSuper,  // KEYCODE_META_LEFT
	118: platform.KeyRightSuper, // KEYCODE_META_RIGHT
}

// mapAndroidKeyCode converts an Android KeyEvent.KEYCODE_* integer to a
// platform.Key. Returns platform.KeyUnknown for unmapped codes.
func mapAndroidKeyCode(keyCode int) platform.Key {
	if k, ok := androidRawKeyMap[keyCode]; ok {
		return k
	}
	return platform.KeyUnknown
}

// mapAndroidMetaState converts an Android KeyEvent meta-state bitmask to
// platform.Modifier flags.
func mapAndroidMetaState(meta int) platform.Modifier {
	var mods platform.Modifier
	if meta&MetaShiftOn != 0 {
		mods |= platform.ModShift
	}
	if meta&MetaCtrlOn != 0 {
		mods |= platform.ModControl
	}
	if meta&MetaAltOn != 0 {
		mods |= platform.ModAlt
	}
	if meta&MetaMetaOn != 0 {
		mods |= platform.ModSuper
	}
	if meta&MetaCapsLockOn != 0 {
		mods |= platform.ModCapsLock
	}
	if meta&MetaNumLockOn != 0 {
		mods |= platform.ModNumLock
	}
	return mods
}

// androidGamepadButtonMap maps Android KeyEvent.KEYCODE_BUTTON_* values to
// the 16-slot button index used by platform.GamepadEvent. Layout matches
// gamepadButtonMap in gamepad.go:
//
//	[0]=A, [1]=B, [2]=X, [3]=Y, [4]=LB, [5]=RB, [6]=Options, [7]=Menu,
//	[8]=LThumb, [9]=RThumb, [10]=DUp, [11]=DDown, [12]=DLeft, [13]=DRight
var androidGamepadButtonMap = map[int]int{
	96: 0,  // KEYCODE_BUTTON_A
	97: 1,  // KEYCODE_BUTTON_B
	99: 2,  // KEYCODE_BUTTON_X
	100: 3, // KEYCODE_BUTTON_Y
	102: 4, // KEYCODE_BUTTON_L1
	103: 5, // KEYCODE_BUTTON_R1
	109: 6, // KEYCODE_BUTTON_SELECT
	108: 7, // KEYCODE_BUTTON_START
	106: 8, // KEYCODE_BUTTON_THUMBL
	107: 9, // KEYCODE_BUTTON_THUMBR
	19: 10, // KEYCODE_DPAD_UP
	20: 11, // KEYCODE_DPAD_DOWN
	21: 12, // KEYCODE_DPAD_LEFT
	22: 13, // KEYCODE_DPAD_RIGHT
}

// Android MotionEvent axis constants (subset — sticks and triggers).
const (
	AxisX        = 0
	AxisY        = 1
	AxisZ        = 11
	AxisRZ       = 14
	AxisHatX     = 15
	AxisHatY     = 16
	AxisLTrigger = 17
	AxisRTrigger = 18
)
