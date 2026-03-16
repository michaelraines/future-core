//go:build darwin

package cocoa

import (
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/michaelraines/future-render/internal/platform"
)

// ---------------------------------------------------------------------------
// Keyboard handlers
// ---------------------------------------------------------------------------

// keyDownHandler is called by FRContentView's keyDown: method.
var keyDownHandler = func(self objc.ID, _ objc.SEL, event objc.ID) {
	w := getWindowFromView(self)
	if w == nil || w.handler == nil {
		return
	}
	keyCode := objc.Send[uint16](event, selKeyCode)
	flags := objc.Send[uint64](event, selModifierFlags)

	w.handler.OnKeyEvent(platform.KeyEvent{
		Key:    mapMacKey(keyCode),
		Action: platform.ActionPress,
		Mods:   mapMacMods(flags),
	})

	// Also dispatch character input.
	chars := event.Send(selCharacters)
	if chars != 0 {
		utf8Ptr := objc.Send[uintptr](chars, selUTF8String)
		if utf8Ptr != 0 {
			str := goString(utf8Ptr)
			for _, r := range str {
				w.handler.OnCharEvent(r)
			}
		}
	}
}

// keyUpHandler is called by FRContentView's keyUp: method.
var keyUpHandler = func(self objc.ID, _ objc.SEL, event objc.ID) {
	w := getWindowFromView(self)
	if w == nil || w.handler == nil {
		return
	}
	keyCode := objc.Send[uint16](event, selKeyCode)
	flags := objc.Send[uint64](event, selModifierFlags)

	w.handler.OnKeyEvent(platform.KeyEvent{
		Key:    mapMacKey(keyCode),
		Action: platform.ActionRelease,
		Mods:   mapMacMods(flags),
	})
}

// flagsChangedHandler handles modifier key state changes (shift, ctrl, etc.).
var flagsChangedHandler = func(self objc.ID, _ objc.SEL, event objc.ID) {
	w := getWindowFromView(self)
	if w == nil || w.handler == nil {
		return
	}
	keyCode := objc.Send[uint16](event, selKeyCode)
	flags := objc.Send[uint64](event, selModifierFlags)

	// Determine press vs release by checking if the modifier bit is set.
	action := platform.ActionPress
	key := mapMacKey(keyCode)
	switch key {
	case platform.KeyLeftShift, platform.KeyRightShift:
		if flags&nsEventModifierFlagShift == 0 {
			action = platform.ActionRelease
		}
	case platform.KeyLeftControl, platform.KeyRightControl:
		if flags&nsEventModifierFlagControl == 0 {
			action = platform.ActionRelease
		}
	case platform.KeyLeftAlt, platform.KeyRightAlt:
		if flags&nsEventModifierFlagOption == 0 {
			action = platform.ActionRelease
		}
	case platform.KeyLeftSuper, platform.KeyRightSuper:
		if flags&nsEventModifierFlagCommand == 0 {
			action = platform.ActionRelease
		}
	case platform.KeyCapsLock:
		if flags&nsEventModifierFlagCapsLock == 0 {
			action = platform.ActionRelease
		}
	}

	w.handler.OnKeyEvent(platform.KeyEvent{
		Key:    key,
		Action: action,
		Mods:   mapMacMods(flags),
	})
}

// ---------------------------------------------------------------------------
// Mouse button handlers
// ---------------------------------------------------------------------------

// mouseDownHandler handles left mouse button press.
var mouseDownHandler = func(self objc.ID, _ objc.SEL, event objc.ID) {
	handleMouseButton(self, event, platform.MouseButtonLeft, platform.ActionPress)
}

// mouseUpHandler handles left mouse button release.
var mouseUpHandler = func(self objc.ID, _ objc.SEL, event objc.ID) {
	handleMouseButton(self, event, platform.MouseButtonLeft, platform.ActionRelease)
}

// rightMouseDownHandler handles right mouse button press.
var rightMouseDownHandler = func(self objc.ID, _ objc.SEL, event objc.ID) {
	handleMouseButton(self, event, platform.MouseButtonRight, platform.ActionPress)
}

// rightMouseUpHandler handles right mouse button release.
var rightMouseUpHandler = func(self objc.ID, _ objc.SEL, event objc.ID) {
	handleMouseButton(self, event, platform.MouseButtonRight, platform.ActionRelease)
}

// otherMouseDownHandler handles middle/other mouse button press.
var otherMouseDownHandler = func(self objc.ID, _ objc.SEL, event objc.ID) {
	btn := objc.Send[int32](event, selButtonNumber)
	handleMouseButton(self, event, mapMouseButton(btn), platform.ActionPress)
}

// otherMouseUpHandler handles middle/other mouse button release.
var otherMouseUpHandler = func(self objc.ID, _ objc.SEL, event objc.ID) {
	btn := objc.Send[int32](event, selButtonNumber)
	handleMouseButton(self, event, mapMouseButton(btn), platform.ActionRelease)
}

func handleMouseButton(self objc.ID, event objc.ID, button platform.MouseButton, action platform.Action) {
	w := getWindowFromView(self)
	if w == nil || w.handler == nil {
		return
	}
	loc := objc.Send[CGPoint](event, selLocationInWindow)
	flags := objc.Send[uint64](event, selModifierFlags)

	// Convert from Cocoa's bottom-left origin to top-left origin.
	frame := objc.Send[CGRect](w.contentView, selFrame)
	y := frame.Size.Height - loc.Y

	w.handler.OnMouseButtonEvent(platform.MouseButtonEvent{
		Button: button,
		Action: action,
		X:      loc.X,
		Y:      y,
		Mods:   mapMacMods(flags),
	})
}

func mapMouseButton(btn int32) platform.MouseButton {
	switch btn {
	case 0:
		return platform.MouseButtonLeft
	case 1:
		return platform.MouseButtonRight
	case 2:
		return platform.MouseButtonMiddle
	case 3:
		return platform.MouseButton4
	case 4:
		return platform.MouseButton5
	default:
		return platform.MouseButtonMiddle
	}
}

// ---------------------------------------------------------------------------
// Mouse move handler
// ---------------------------------------------------------------------------

// mouseMovedHandler handles all mouse movement (moved, dragged).
var mouseMovedHandler = func(self objc.ID, _ objc.SEL, event objc.ID) {
	w := getWindowFromView(self)
	if w == nil || w.handler == nil {
		return
	}
	loc := objc.Send[CGPoint](event, selLocationInWindow)

	// Convert from Cocoa's bottom-left origin to top-left origin.
	frame := objc.Send[CGRect](w.contentView, selFrame)
	x := loc.X
	y := frame.Size.Height - loc.Y

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
}

// ---------------------------------------------------------------------------
// Scroll wheel handler
// ---------------------------------------------------------------------------

// scrollWheelHandler handles scroll wheel events.
var scrollWheelHandler = func(self objc.ID, _ objc.SEL, event objc.ID) {
	w := getWindowFromView(self)
	if w == nil || w.handler == nil {
		return
	}

	// Use precise scrolling deltas if available (trackpad), otherwise deltaX/Y (mouse wheel).
	var sx, sy float64
	hasPrecise := objc.Send[bool](event, selHasPreciseScrollingDeltas)
	if hasPrecise {
		sx = objc.Send[float64](event, selScrollingDeltaX)
		sy = objc.Send[float64](event, selScrollingDeltaY)
	} else {
		sx = objc.Send[float64](event, selDeltaX)
		sy = objc.Send[float64](event, selDeltaY)
	}

	w.handler.OnMouseScrollEvent(platform.MouseScrollEvent{
		DX: sx, DY: sy,
	})
}

// ---------------------------------------------------------------------------
// Helper: C string → Go string
// ---------------------------------------------------------------------------

// goString converts a C string pointer to a Go string without copying via CGo.
func goString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	// Find the length by scanning for null terminator.
	var length int
	for {
		b := *(*byte)(unsafe.Pointer(ptr + uintptr(length)))
		if b == 0 {
			break
		}
		length++
		if length > 4096 { // safety limit
			break
		}
	}
	if length == 0 {
		return ""
	}
	return unsafe.String((*byte)(unsafe.Pointer(ptr)), length)
}
