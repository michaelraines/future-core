//go:build darwin

package cocoa

import (
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/michaelraines/future-core/internal/platform"
)

// ---------------------------------------------------------------------------
// GameController framework support
// ---------------------------------------------------------------------------
//
// macOS provides the GCController framework for gamepad input. It supports
// MFi, Xbox, DualShock, DualSense, and other Bluetooth/USB controllers.
// We load it via purego and use objc.Send to query controller state.

var (
	gameControllerFW uintptr
	gcLoaded         bool
	gcLoadAttempted  bool

	classGCController objc.Class

	selControllers        objc.SEL
	selExtendedGamepad    objc.SEL
	selCount              objc.SEL
	selObjectAtIndex      objc.SEL
	selPlayerIndex        objc.SEL
	selLeftThumbstick     objc.SEL
	selRightThumbstick    objc.SEL
	selLeftTrigger        objc.SEL
	selRightTrigger       objc.SEL
	selLeftShoulder       objc.SEL
	selRightShoulder      objc.SEL
	selButtonA            objc.SEL
	selButtonB            objc.SEL
	selButtonX            objc.SEL
	selButtonY            objc.SEL
	selDpad               objc.SEL
	selXAxis              objc.SEL
	selYAxis              objc.SEL
	selValue              objc.SEL
	selIsPressed          objc.SEL
	selButtonMenu         objc.SEL
	selButtonOptions      objc.SEL
	selLeftThumbstickBtn  objc.SEL
	selRightThumbstickBtn objc.SEL
	selUp                 objc.SEL
	selDown               objc.SEL
	selLeft               objc.SEL
	selRight              objc.SEL
)

func loadGameController() {
	if gcLoadAttempted {
		return
	}
	gcLoadAttempted = true

	var err error
	gameControllerFW, err = purego.Dlopen(
		"/System/Library/Frameworks/GameController.framework/GameController",
		purego.RTLD_LAZY|purego.RTLD_GLOBAL,
	)
	if err != nil {
		return
	}

	classGCController = objc.GetClass("GCController")
	if classGCController == 0 {
		return
	}

	selControllers = objc.RegisterName("controllers")
	selExtendedGamepad = objc.RegisterName("extendedGamepad")
	selCount = objc.RegisterName("count")
	selObjectAtIndex = objc.RegisterName("objectAtIndex:")
	selPlayerIndex = objc.RegisterName("playerIndex")
	selLeftThumbstick = objc.RegisterName("leftThumbstick")
	selRightThumbstick = objc.RegisterName("rightThumbstick")
	selLeftTrigger = objc.RegisterName("leftTrigger")
	selRightTrigger = objc.RegisterName("rightTrigger")
	selLeftShoulder = objc.RegisterName("leftShoulder")
	selRightShoulder = objc.RegisterName("rightShoulder")
	selButtonA = objc.RegisterName("buttonA")
	selButtonB = objc.RegisterName("buttonB")
	selButtonX = objc.RegisterName("buttonX")
	selButtonY = objc.RegisterName("buttonY")
	selDpad = objc.RegisterName("dpad")
	selXAxis = objc.RegisterName("xAxis")
	selYAxis = objc.RegisterName("yAxis")
	selValue = objc.RegisterName("value")
	selIsPressed = objc.RegisterName("isPressed")
	selButtonMenu = objc.RegisterName("buttonMenu")
	selButtonOptions = objc.RegisterName("buttonOptions")
	selLeftThumbstickBtn = objc.RegisterName("leftThumbstickButton")
	selRightThumbstickBtn = objc.RegisterName("rightThumbstickButton")
	selUp = objc.RegisterName("up")
	selDown = objc.RegisterName("down")
	selLeft = objc.RegisterName("left")
	selRight = objc.RegisterName("right")

	gcLoaded = true
}

// maxGamepads is the maximum number of gamepads we track.
const maxGamepads = 16

// connectedGamepads tracks which controller indices were connected last frame.
var connectedGamepads [maxGamepads]bool

// pollGamepadsGC queries GCController for connected controllers and dispatches
// GamepadEvent to the handler.
func pollGamepadsGC(handler platform.InputHandler) {
	loadGameController()
	if !gcLoaded {
		return
	}

	// [GCController controllers] returns an NSArray of GCController.
	controllers := cls(classGCController).Send(selControllers)
	if controllers == 0 {
		// Mark all previously connected as disconnected.
		for i := 0; i < maxGamepads; i++ {
			if connectedGamepads[i] {
				connectedGamepads[i] = false
				handler.OnGamepadEvent(platform.GamepadEvent{
					ID:           i,
					Disconnected: true,
				})
			}
		}
		return
	}

	count := objc.Send[int](controllers, selCount)
	if count > maxGamepads {
		count = maxGamepads
	}

	// Track which IDs are still present.
	var stillPresent [maxGamepads]bool

	for i := 0; i < count; i++ {
		controller := controllers.Send(selObjectAtIndex, uintptr(i))
		if controller == 0 {
			continue
		}

		// Use array index as the gamepad ID.
		id := i
		stillPresent[id] = true
		connectedGamepads[id] = true

		// Get extended gamepad profile.
		gamepad := controller.Send(selExtendedGamepad)
		if gamepad == 0 {
			// Controller doesn't support extended profile — send basic event.
			handler.OnGamepadEvent(platform.GamepadEvent{ID: id})
			continue
		}

		var event platform.GamepadEvent
		event.ID = id

		// Thumbsticks: axes 0-3.
		leftStick := gamepad.Send(selLeftThumbstick)
		if leftStick != 0 {
			event.Axes[0] = float64(readAxisValue(leftStick, selXAxis))
			event.Axes[1] = -float64(readAxisValue(leftStick, selYAxis)) // Invert Y
		}
		rightStick := gamepad.Send(selRightThumbstick)
		if rightStick != 0 {
			event.Axes[2] = float64(readAxisValue(rightStick, selXAxis))
			event.Axes[3] = -float64(readAxisValue(rightStick, selYAxis)) // Invert Y
		}

		// Triggers: axes 4-5.
		lt := gamepad.Send(selLeftTrigger)
		if lt != 0 {
			event.Axes[4] = float64(readElementValue(lt))
		}
		rt := gamepad.Send(selRightTrigger)
		if rt != 0 {
			event.Axes[5] = float64(readElementValue(rt))
		}

		// Buttons: [A, B, X, Y, LB, RB, Options, Menu, LThumb, RThumb, DUp, DDown, DLeft, DRight]
		event.Buttons[0] = readButtonPressed(gamepad, selButtonA)
		event.Buttons[1] = readButtonPressed(gamepad, selButtonB)
		event.Buttons[2] = readButtonPressed(gamepad, selButtonX)
		event.Buttons[3] = readButtonPressed(gamepad, selButtonY)
		event.Buttons[4] = readButtonPressed(gamepad, selLeftShoulder)
		event.Buttons[5] = readButtonPressed(gamepad, selRightShoulder)
		event.Buttons[6] = readButtonPressed(gamepad, selButtonOptions)
		event.Buttons[7] = readButtonPressed(gamepad, selButtonMenu)
		event.Buttons[8] = readButtonPressed(gamepad, selLeftThumbstickBtn)
		event.Buttons[9] = readButtonPressed(gamepad, selRightThumbstickBtn)

		// D-pad.
		dpad := gamepad.Send(selDpad)
		if dpad != 0 {
			event.Buttons[10] = readButtonPressedDirect(dpad, selUp)
			event.Buttons[11] = readButtonPressedDirect(dpad, selDown)
			event.Buttons[12] = readButtonPressedDirect(dpad, selLeft)
			event.Buttons[13] = readButtonPressedDirect(dpad, selRight)
		}

		handler.OnGamepadEvent(event)
	}

	// Send disconnect events for controllers that disappeared.
	for i := 0; i < maxGamepads; i++ {
		if connectedGamepads[i] && !stillPresent[i] {
			connectedGamepads[i] = false
			handler.OnGamepadEvent(platform.GamepadEvent{
				ID:           i,
				Disconnected: true,
			})
		}
	}
}

// readAxisValue reads the float value of a GCControllerAxisInput via its parent element.
func readAxisValue(element objc.ID, axisSel objc.SEL) float32 {
	axis := element.Send(axisSel)
	if axis == 0 {
		return 0
	}
	return objc.Send[float32](axis, selValue)
}

// readElementValue reads the float value of a GCControllerElement (trigger, button).
func readElementValue(element objc.ID) float32 {
	return objc.Send[float32](element, selValue)
}

// readButtonPressed checks if a button element on the gamepad is pressed.
func readButtonPressed(gamepad objc.ID, buttonSel objc.SEL) bool {
	btn := gamepad.Send(buttonSel)
	if btn == 0 {
		return false
	}
	return objc.Send[bool](btn, selIsPressed)
}

// readButtonPressedDirect checks isPressed on a direct element (e.g., dpad direction).
func readButtonPressedDirect(element objc.ID, dirSel objc.SEL) bool {
	dir := element.Send(dirSel)
	if dir == 0 {
		return false
	}
	return objc.Send[bool](dir, selIsPressed)
}
