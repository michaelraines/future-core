//go:build windows

package win32

import (
	"math"
	"syscall"
	"unsafe"

	"github.com/michaelraines/future-render/internal/platform"
)

// ---------------------------------------------------------------------------
// XInput DLL + functions
// ---------------------------------------------------------------------------

// xinputDLL is loaded lazily — we try xinput1_4, then xinput1_3, then xinput9_1_0.
var (
	xinputDLL           *syscall.LazyDLL
	procXInputGetState  *syscall.LazyProc
	xinputLoaded        bool
	xinputLoadAttempted bool
)

func loadXInput() {
	if xinputLoadAttempted {
		return
	}
	xinputLoadAttempted = true

	names := []string{"xinput1_4.dll", "xinput1_3.dll", "xinput9_1_0.dll"}
	for _, name := range names {
		dll := syscall.NewLazyDLL(name)
		if err := dll.Load(); err == nil {
			xinputDLL = dll
			procXInputGetState = dll.NewProc("XInputGetState")
			xinputLoaded = true
			return
		}
	}
}

// ---------------------------------------------------------------------------
// XInput constants
// ---------------------------------------------------------------------------

const (
	xinputMaxControllers = 4

	// XINPUT_GAMEPAD button flags.
	xinputGamepadDPadUp        = 0x0001
	xinputGamepadDPadDown      = 0x0002
	xinputGamepadDPadLeft      = 0x0004
	xinputGamepadDPadRight     = 0x0008
	xinputGamepadStart         = 0x0010
	xinputGamepadBack          = 0x0020
	xinputGamepadLeftThumb     = 0x0040
	xinputGamepadRightThumb    = 0x0080
	xinputGamepadLeftShoulder  = 0x0100
	xinputGamepadRightShoulder = 0x0200
	xinputGamepadA             = 0x1000
	xinputGamepadB             = 0x2000
	xinputGamepadX             = 0x4000
	xinputGamepadY             = 0x8000

	// Dead zone thresholds (standard XInput values).
	xinputLeftThumbDeadZone  = 7849
	xinputRightThumbDeadZone = 8689
	xinputTriggerThreshold   = 30

	// Error codes.
	errorSuccess            = 0
	errorDeviceNotConnected = 1167
)

// ---------------------------------------------------------------------------
// XInput structures
// ---------------------------------------------------------------------------

// xinputGamepad matches the XINPUT_GAMEPAD structure.
type xinputGamepad struct {
	Buttons      uint16
	LeftTrigger  uint8
	RightTrigger uint8
	ThumbLX      int16
	ThumbLY      int16
	ThumbRX      int16
	ThumbRY      int16
}

// xinputState matches the XINPUT_STATE structure.
type xinputState struct {
	PacketNumber uint32
	Gamepad      xinputGamepad
}

// ---------------------------------------------------------------------------
// Gamepad polling
// ---------------------------------------------------------------------------

// connectedGamepads tracks which XInput controllers are currently connected.
var connectedGamepads [xinputMaxControllers]bool

// pollGamepadsXInput queries XInput for up to 4 connected controllers
// and dispatches GamepadEvent to the handler.
func pollGamepadsXInput(handler platform.InputHandler) {
	loadXInput()
	if !xinputLoaded {
		return
	}

	for i := uint32(0); i < xinputMaxControllers; i++ {
		var state xinputState
		ret, _, _ := procXInputGetState.Call(uintptr(i), uintptr(unsafe.Pointer(&state)))

		if ret != errorSuccess {
			// Send disconnect event if previously connected.
			if connectedGamepads[i] {
				connectedGamepads[i] = false
				handler.OnGamepadEvent(platform.GamepadEvent{
					ID:           int(i),
					Disconnected: true,
				})
			}
			continue
		}

		connectedGamepads[i] = true

		var event platform.GamepadEvent
		event.ID = int(i)

		// Map axes: [leftX, leftY, rightX, rightY, leftTrigger, rightTrigger]
		event.Axes[0] = normalizeThumbstick(state.Gamepad.ThumbLX, xinputLeftThumbDeadZone)
		event.Axes[1] = -normalizeThumbstick(state.Gamepad.ThumbLY, xinputLeftThumbDeadZone) // Invert Y
		event.Axes[2] = normalizeThumbstick(state.Gamepad.ThumbRX, xinputRightThumbDeadZone)
		event.Axes[3] = -normalizeThumbstick(state.Gamepad.ThumbRY, xinputRightThumbDeadZone) // Invert Y
		event.Axes[4] = normalizeTrigger(state.Gamepad.LeftTrigger)
		event.Axes[5] = normalizeTrigger(state.Gamepad.RightTrigger)

		// Map buttons — standard gamepad layout:
		// [A, B, X, Y, LB, RB, Back, Start, LThumb, RThumb, DPadUp, DPadDown, DPadLeft, DPadRight, -, -]
		gp := state.Gamepad.Buttons
		event.Buttons[0] = gp&xinputGamepadA != 0
		event.Buttons[1] = gp&xinputGamepadB != 0
		event.Buttons[2] = gp&xinputGamepadX != 0
		event.Buttons[3] = gp&xinputGamepadY != 0
		event.Buttons[4] = gp&xinputGamepadLeftShoulder != 0
		event.Buttons[5] = gp&xinputGamepadRightShoulder != 0
		event.Buttons[6] = gp&xinputGamepadBack != 0
		event.Buttons[7] = gp&xinputGamepadStart != 0
		event.Buttons[8] = gp&xinputGamepadLeftThumb != 0
		event.Buttons[9] = gp&xinputGamepadRightThumb != 0
		event.Buttons[10] = gp&xinputGamepadDPadUp != 0
		event.Buttons[11] = gp&xinputGamepadDPadDown != 0
		event.Buttons[12] = gp&xinputGamepadDPadLeft != 0
		event.Buttons[13] = gp&xinputGamepadDPadRight != 0

		handler.OnGamepadEvent(event)
	}
}

// normalizeThumbstick applies dead zone and returns a value in [-1, 1].
func normalizeThumbstick(value int16, deadZone int16) float64 {
	v := float64(value)
	dz := float64(deadZone)

	if math.Abs(v) < dz {
		return 0
	}

	// Remap from [deadZone, 32767] to [0, 1] (or [-32768, -deadZone] to [-1, 0]).
	maxVal := 32767.0
	if v < 0 {
		maxVal = 32768.0
	}

	sign := 1.0
	if v < 0 {
		sign = -1.0
		v = -v
	}

	return sign * (v - dz) / (maxVal - dz)
}

// normalizeTrigger returns a value in [0, 1] from a trigger byte.
func normalizeTrigger(value uint8) float64 {
	if value < xinputTriggerThreshold {
		return 0
	}
	return float64(value-xinputTriggerThreshold) / float64(255-xinputTriggerThreshold)
}
