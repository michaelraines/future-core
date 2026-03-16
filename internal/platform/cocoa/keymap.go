//go:build darwin

package cocoa

import "github.com/michaelraines/future-render/internal/platform"

// macOS virtual key codes → platform.Key.
// Reference: Events.h (Carbon) / HIToolbox/Events.h
var macKeyMap = [256]platform.Key{
	0x00: platform.KeyA,
	0x01: platform.KeyS,
	0x02: platform.KeyD,
	0x03: platform.KeyF,
	0x04: platform.KeyH,
	0x05: platform.KeyG,
	0x06: platform.KeyZ,
	0x07: platform.KeyX,
	0x08: platform.KeyC,
	0x09: platform.KeyV,
	0x0B: platform.KeyB,
	0x0C: platform.KeyQ,
	0x0D: platform.KeyW,
	0x0E: platform.KeyE,
	0x0F: platform.KeyR,
	0x10: platform.KeyY,
	0x11: platform.KeyT,
	0x12: platform.Key1,
	0x13: platform.Key2,
	0x14: platform.Key3,
	0x15: platform.Key4,
	0x16: platform.Key6,
	0x17: platform.Key5,
	0x18: platform.KeyEqual,
	0x19: platform.Key9,
	0x1A: platform.Key7,
	0x1B: platform.KeyMinus,
	0x1C: platform.Key8,
	0x1D: platform.Key0,
	0x1E: platform.KeyRightBracket,
	0x1F: platform.KeyO,
	0x20: platform.KeyU,
	0x21: platform.KeyLeftBracket,
	0x22: platform.KeyI,
	0x23: platform.KeyP,
	0x24: platform.KeyEnter,
	0x25: platform.KeyL,
	0x26: platform.KeyJ,
	0x27: platform.KeyApostrophe,
	0x28: platform.KeyK,
	0x29: platform.KeySemicolon,
	0x2A: platform.KeyBackslash,
	0x2B: platform.KeyComma,
	0x2C: platform.KeySlash,
	0x2D: platform.KeyN,
	0x2E: platform.KeyM,
	0x2F: platform.KeyPeriod,
	0x30: platform.KeyTab,
	0x31: platform.KeySpace,
	0x32: platform.KeyGraveAccent,
	0x33: platform.KeyBackspace,
	0x35: platform.KeyEscape,
	0x38: platform.KeyLeftShift,
	0x39: platform.KeyCapsLock,
	0x3A: platform.KeyLeftAlt, // Option
	0x3B: platform.KeyLeftControl,
	0x3C: platform.KeyRightShift,
	0x3D: platform.KeyRightAlt, // Right Option
	0x3E: platform.KeyRightControl,
	0x37: platform.KeyLeftSuper,  // Command
	0x36: platform.KeyRightSuper, // Right Command
	0x41: platform.KeyKPDecimal,
	0x43: platform.KeyKPMultiply,
	0x45: platform.KeyKPAdd,
	0x47: platform.KeyNumLock, // Clear on Mac keyboards
	0x4B: platform.KeyKPDivide,
	0x4C: platform.KeyKPEnter,
	0x4E: platform.KeyKPSubtract,
	0x51: platform.KeyKPEqual,
	0x52: platform.KeyKP0,
	0x53: platform.KeyKP1,
	0x54: platform.KeyKP2,
	0x55: platform.KeyKP3,
	0x56: platform.KeyKP4,
	0x57: platform.KeyKP5,
	0x58: platform.KeyKP6,
	0x59: platform.KeyKP7,
	0x5B: platform.KeyKP8,
	0x5C: platform.KeyKP9,
	0x60: platform.KeyF5,
	0x61: platform.KeyF6,
	0x62: platform.KeyF7,
	0x63: platform.KeyF3,
	0x64: platform.KeyF8,
	0x65: platform.KeyF9,
	0x67: platform.KeyF11,
	0x69: platform.KeyPrintScreen, // F13 → PrintScreen
	0x6D: platform.KeyF10,
	0x6B: platform.KeyScrollLock, // F14 → ScrollLock
	0x6F: platform.KeyF12,
	0x71: platform.KeyPause,  // F15 → Pause
	0x72: platform.KeyInsert, // Help key
	0x73: platform.KeyHome,
	0x74: platform.KeyPageUp,
	0x75: platform.KeyDelete,
	0x76: platform.KeyF4,
	0x77: platform.KeyEnd,
	0x78: platform.KeyF2,
	0x79: platform.KeyPageDown,
	0x7A: platform.KeyF1,
	0x7B: platform.KeyLeft,
	0x7C: platform.KeyRight,
	0x7D: platform.KeyDown,
	0x7E: platform.KeyUp,
}

// mapMacKey converts a macOS virtual key code to a platform.Key.
func mapMacKey(keyCode uint16) platform.Key {
	if int(keyCode) >= len(macKeyMap) {
		return platform.KeyUnknown
	}
	k := macKeyMap[keyCode]
	if k == 0 && keyCode != 0 {
		return platform.KeyUnknown
	}
	return k
}

// mapMacMods converts macOS NSEvent modifier flags to platform.Modifier.
func mapMacMods(flags uint64) platform.Modifier {
	var mods platform.Modifier
	if flags&nsEventModifierFlagShift != 0 {
		mods |= platform.ModShift
	}
	if flags&nsEventModifierFlagControl != 0 {
		mods |= platform.ModControl
	}
	if flags&nsEventModifierFlagOption != 0 {
		mods |= platform.ModAlt
	}
	if flags&nsEventModifierFlagCommand != 0 {
		mods |= platform.ModSuper
	}
	if flags&nsEventModifierFlagCapsLock != 0 {
		mods |= platform.ModCapsLock
	}
	return mods
}
