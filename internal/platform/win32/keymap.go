//go:build windows

package win32

import "github.com/michaelraines/future-render/internal/platform"

// Win32 virtual key codes → platform.Key.
var vkKeyMap = [256]platform.Key{
	0x08: platform.KeyBackspace,
	0x09: platform.KeyTab,
	0x0D: platform.KeyEnter,
	0x10: platform.KeyLeftShift,
	0x11: platform.KeyLeftControl,
	0x12: platform.KeyLeftAlt,
	0x13: platform.KeyPause,
	0x14: platform.KeyCapsLock,
	0x1B: platform.KeyEscape,
	0x20: platform.KeySpace,
	0x21: platform.KeyPageUp,
	0x22: platform.KeyPageDown,
	0x23: platform.KeyEnd,
	0x24: platform.KeyHome,
	0x25: platform.KeyLeft,
	0x26: platform.KeyUp,
	0x27: platform.KeyRight,
	0x28: platform.KeyDown,
	0x2C: platform.KeyPrintScreen,
	0x2D: platform.KeyInsert,
	0x2E: platform.KeyDelete,
	0x30: platform.Key0,
	0x31: platform.Key1,
	0x32: platform.Key2,
	0x33: platform.Key3,
	0x34: platform.Key4,
	0x35: platform.Key5,
	0x36: platform.Key6,
	0x37: platform.Key7,
	0x38: platform.Key8,
	0x39: platform.Key9,
	0x41: platform.KeyA,
	0x42: platform.KeyB,
	0x43: platform.KeyC,
	0x44: platform.KeyD,
	0x45: platform.KeyE,
	0x46: platform.KeyF,
	0x47: platform.KeyG,
	0x48: platform.KeyH,
	0x49: platform.KeyI,
	0x4A: platform.KeyJ,
	0x4B: platform.KeyK,
	0x4C: platform.KeyL,
	0x4D: platform.KeyM,
	0x4E: platform.KeyN,
	0x4F: platform.KeyO,
	0x50: platform.KeyP,
	0x51: platform.KeyQ,
	0x52: platform.KeyR,
	0x53: platform.KeyS,
	0x54: platform.KeyT,
	0x55: platform.KeyU,
	0x56: platform.KeyV,
	0x57: platform.KeyW,
	0x58: platform.KeyX,
	0x59: platform.KeyY,
	0x5A: platform.KeyZ,
	0x5B: platform.KeyLeftSuper,  // Left Windows key
	0x5C: platform.KeyRightSuper, // Right Windows key
	0x5D: platform.KeyMenu,       // Applications key
	0x60: platform.KeyKP0,
	0x61: platform.KeyKP1,
	0x62: platform.KeyKP2,
	0x63: platform.KeyKP3,
	0x64: platform.KeyKP4,
	0x65: platform.KeyKP5,
	0x66: platform.KeyKP6,
	0x67: platform.KeyKP7,
	0x68: platform.KeyKP8,
	0x69: platform.KeyKP9,
	0x6A: platform.KeyKPMultiply,
	0x6B: platform.KeyKPAdd,
	0x6D: platform.KeyKPSubtract,
	0x6E: platform.KeyKPDecimal,
	0x6F: platform.KeyKPDivide,
	0x70: platform.KeyF1,
	0x71: platform.KeyF2,
	0x72: platform.KeyF3,
	0x73: platform.KeyF4,
	0x74: platform.KeyF5,
	0x75: platform.KeyF6,
	0x76: platform.KeyF7,
	0x77: platform.KeyF8,
	0x78: platform.KeyF9,
	0x79: platform.KeyF10,
	0x7A: platform.KeyF11,
	0x7B: platform.KeyF12,
	0x90: platform.KeyNumLock,
	0x91: platform.KeyScrollLock,
	0xA0: platform.KeyLeftShift,
	0xA1: platform.KeyRightShift,
	0xA2: platform.KeyLeftControl,
	0xA3: platform.KeyRightControl,
	0xA4: platform.KeyLeftAlt,
	0xA5: platform.KeyRightAlt,
	0xBA: platform.KeySemicolon,
	0xBB: platform.KeyEqual,
	0xBC: platform.KeyComma,
	0xBD: platform.KeyMinus,
	0xBE: platform.KeyPeriod,
	0xBF: platform.KeySlash,
	0xC0: platform.KeyGraveAccent,
	0xDB: platform.KeyLeftBracket,
	0xDC: platform.KeyBackslash,
	0xDD: platform.KeyRightBracket,
	0xDE: platform.KeyApostrophe,
}

// mapVKKey converts a Win32 virtual key code to a platform.Key.
// For extended keys (like right Ctrl/Alt, arrow keys), the extended flag
// from lParam must be checked by the caller.
func mapVKKey(vk uint32) platform.Key {
	if int(vk) >= len(vkKeyMap) {
		return platform.KeyUnknown
	}
	k := vkKeyMap[vk]
	if k == 0 && vk != 0 {
		return platform.KeyUnknown
	}
	return k
}

// mapWin32Mods reads the current modifier key state via GetKeyState.
func mapWin32Mods() platform.Modifier {
	var mods platform.Modifier
	if getKeyStateDown(vkShift) {
		mods |= platform.ModShift
	}
	if getKeyStateDown(vkControl) {
		mods |= platform.ModControl
	}
	if getKeyStateDown(vkMenu) {
		mods |= platform.ModAlt
	}
	if getKeyStateDown(vkLWin) || getKeyStateDown(vkRWin) {
		mods |= platform.ModSuper
	}
	if getKeyStateToggled(vkCapital) {
		mods |= platform.ModCapsLock
	}
	if getKeyStateToggled(vkNumLock) {
		mods |= platform.ModNumLock
	}
	return mods
}
