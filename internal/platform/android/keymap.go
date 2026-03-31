//go:build android

package android

import (
	mkey "golang.org/x/mobile/event/key"

	"github.com/michaelraines/future-core/internal/platform"
)

// mapKey converts a golang.org/x/mobile key code to a platform.Key.
func mapKey(code mkey.Code) platform.Key {
	if int(code) < len(androidKeyMap) {
		return androidKeyMap[code]
	}
	return platform.KeyUnknown
}

// androidKeyMap maps x/mobile key codes to platform.Key values.
// Reference: golang.org/x/mobile/event/key/key.go
var androidKeyMap = [256]platform.Key{
	mkey.CodeA: platform.KeyA,
	mkey.CodeB: platform.KeyB,
	mkey.CodeC: platform.KeyC,
	mkey.CodeD: platform.KeyD,
	mkey.CodeE: platform.KeyE,
	mkey.CodeF: platform.KeyF,
	mkey.CodeG: platform.KeyG,
	mkey.CodeH: platform.KeyH,
	mkey.CodeI: platform.KeyI,
	mkey.CodeJ: platform.KeyJ,
	mkey.CodeK: platform.KeyK,
	mkey.CodeL: platform.KeyL,
	mkey.CodeM: platform.KeyM,
	mkey.CodeN: platform.KeyN,
	mkey.CodeO: platform.KeyO,
	mkey.CodeP: platform.KeyP,
	mkey.CodeQ: platform.KeyQ,
	mkey.CodeR: platform.KeyR,
	mkey.CodeS: platform.KeyS,
	mkey.CodeT: platform.KeyT,
	mkey.CodeU: platform.KeyU,
	mkey.CodeV: platform.KeyV,
	mkey.CodeW: platform.KeyW,
	mkey.CodeX: platform.KeyX,
	mkey.CodeY: platform.KeyY,
	mkey.CodeZ: platform.KeyZ,

	mkey.Code1: platform.Key1,
	mkey.Code2: platform.Key2,
	mkey.Code3: platform.Key3,
	mkey.Code4: platform.Key4,
	mkey.Code5: platform.Key5,
	mkey.Code6: platform.Key6,
	mkey.Code7: platform.Key7,
	mkey.Code8: platform.Key8,
	mkey.Code9: platform.Key9,
	mkey.Code0: platform.Key0,

	mkey.CodeReturnEnter:        platform.KeyEnter,
	mkey.CodeEscape:             platform.KeyEscape,
	mkey.CodeDeleteBackspace:    platform.KeyBackspace,
	mkey.CodeTab:                platform.KeyTab,
	mkey.CodeSpacebar:           platform.KeySpace,
	mkey.CodeHyphenMinus:        platform.KeyMinus,
	mkey.CodeEqualSign:          platform.KeyEqual,
	mkey.CodeLeftSquareBracket:  platform.KeyLeftBracket,
	mkey.CodeRightSquareBracket: platform.KeyRightBracket,
	mkey.CodeBackslash:          platform.KeyBackslash,
	mkey.CodeSemicolon:          platform.KeySemicolon,
	mkey.CodeApostrophe:         platform.KeyApostrophe,
	mkey.CodeGraveAccent:        platform.KeyGraveAccent,
	mkey.CodeComma:              platform.KeyComma,
	mkey.CodeFullStop:           platform.KeyPeriod,
	mkey.CodeSlash:              platform.KeySlash,

	mkey.CodeCapsLock: platform.KeyCapsLock,

	mkey.CodeF1:  platform.KeyF1,
	mkey.CodeF2:  platform.KeyF2,
	mkey.CodeF3:  platform.KeyF3,
	mkey.CodeF4:  platform.KeyF4,
	mkey.CodeF5:  platform.KeyF5,
	mkey.CodeF6:  platform.KeyF6,
	mkey.CodeF7:  platform.KeyF7,
	mkey.CodeF8:  platform.KeyF8,
	mkey.CodeF9:  platform.KeyF9,
	mkey.CodeF10: platform.KeyF10,
	mkey.CodeF11: platform.KeyF11,
	mkey.CodeF12: platform.KeyF12,

	mkey.CodeInsert:        platform.KeyInsert,
	mkey.CodeHome:          platform.KeyHome,
	mkey.CodePageUp:        platform.KeyPageUp,
	mkey.CodeDeleteForward: platform.KeyDelete,
	mkey.CodeEnd:           platform.KeyEnd,
	mkey.CodePageDown:      platform.KeyPageDown,
	mkey.CodeRightArrow:    platform.KeyRight,
	mkey.CodeLeftArrow:     platform.KeyLeft,
	mkey.CodeDownArrow:     platform.KeyDown,
	mkey.CodeUpArrow:       platform.KeyUp,

	mkey.CodeKeypadNumLock:     platform.KeyNumLock,
	mkey.CodeKeypadSlash:       platform.KeyKPDivide,
	mkey.CodeKeypadAsterisk:    platform.KeyKPMultiply,
	mkey.CodeKeypadHyphenMinus: platform.KeyKPSubtract,
	mkey.CodeKeypadPlusSign:    platform.KeyKPAdd,
	mkey.CodeKeypadEnter:       platform.KeyKPEnter,
	mkey.CodeKeypad1:           platform.KeyKP1,
	mkey.CodeKeypad2:           platform.KeyKP2,
	mkey.CodeKeypad3:           platform.KeyKP3,
	mkey.CodeKeypad4:           platform.KeyKP4,
	mkey.CodeKeypad5:           platform.KeyKP5,
	mkey.CodeKeypad6:           platform.KeyKP6,
	mkey.CodeKeypad7:           platform.KeyKP7,
	mkey.CodeKeypad8:           platform.KeyKP8,
	mkey.CodeKeypad9:           platform.KeyKP9,
	mkey.CodeKeypad0:           platform.KeyKP0,
	mkey.CodeKeypadFullStop:    platform.KeyKPDecimal,
	mkey.CodeKeypadEqualSign:   platform.KeyKPEqual,

	mkey.CodeLeftControl:  platform.KeyLeftControl,
	mkey.CodeLeftShift:    platform.KeyLeftShift,
	mkey.CodeLeftAlt:      platform.KeyLeftAlt,
	mkey.CodeLeftGUI:      platform.KeyLeftSuper,
	mkey.CodeRightControl: platform.KeyRightControl,
	mkey.CodeRightShift:   platform.KeyRightShift,
	mkey.CodeRightAlt:     platform.KeyRightAlt,
	mkey.CodeRightGUI:     platform.KeyRightSuper,
}
