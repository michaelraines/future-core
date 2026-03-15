//go:build glfw

// Command input demonstrates input state display: keyboard, mouse, and
// gamepad state rendered as text each frame.
//
// Build: go build -tags glfw ./cmd/input
// Run:   ./input
package main

import (
	"fmt"
	"log"
	"strings"

	futurerender "github.com/michaelraines/future-render"
	fmath "github.com/michaelraines/future-render/math"
	"github.com/michaelraines/future-render/text"
	"golang.org/x/image/font/gofont/goregular"
)

const (
	screenW = 640
	screenH = 480
)

// keyEntry pairs a Key constant with its display name.
type keyEntry struct {
	key  futurerender.Key
	name string
}

// allKeys lists all keys we poll each frame.
var allKeys = []keyEntry{
	{futurerender.KeyA, "A"}, {futurerender.KeyB, "B"}, {futurerender.KeyC, "C"},
	{futurerender.KeyD, "D"}, {futurerender.KeyE, "E"}, {futurerender.KeyF, "F"},
	{futurerender.KeyG, "G"}, {futurerender.KeyH, "H"}, {futurerender.KeyI, "I"},
	{futurerender.KeyJ, "J"}, {futurerender.KeyK, "K"}, {futurerender.KeyL, "L"},
	{futurerender.KeyM, "M"}, {futurerender.KeyN, "N"}, {futurerender.KeyO, "O"},
	{futurerender.KeyP, "P"}, {futurerender.KeyQ, "Q"}, {futurerender.KeyR, "R"},
	{futurerender.KeyS, "S"}, {futurerender.KeyT, "T"}, {futurerender.KeyU, "U"},
	{futurerender.KeyV, "V"}, {futurerender.KeyW, "W"}, {futurerender.KeyX, "X"},
	{futurerender.KeyY, "Y"}, {futurerender.KeyZ, "Z"},
	{futurerender.Key0, "0"}, {futurerender.Key1, "1"}, {futurerender.Key2, "2"},
	{futurerender.Key3, "3"}, {futurerender.Key4, "4"}, {futurerender.Key5, "5"},
	{futurerender.Key6, "6"}, {futurerender.Key7, "7"}, {futurerender.Key8, "8"},
	{futurerender.Key9, "9"},
	{futurerender.KeySpace, "Space"}, {futurerender.KeyEnter, "Enter"},
	{futurerender.KeyTab, "Tab"}, {futurerender.KeyBackspace, "Backspace"},
	{futurerender.KeyUp, "Up"}, {futurerender.KeyDown, "Down"},
	{futurerender.KeyLeft, "Left"}, {futurerender.KeyRight, "Right"},
	{futurerender.KeyLeftShift, "LShift"}, {futurerender.KeyRightShift, "RShift"},
	{futurerender.KeyLeftControl, "LCtrl"}, {futurerender.KeyRightControl, "RCtrl"},
	{futurerender.KeyLeftAlt, "LAlt"}, {futurerender.KeyRightAlt, "RAlt"},
	{futurerender.KeyF1, "F1"}, {futurerender.KeyF2, "F2"}, {futurerender.KeyF3, "F3"},
	{futurerender.KeyF4, "F4"}, {futurerender.KeyF5, "F5"}, {futurerender.KeyF6, "F6"},
	{futurerender.KeyF7, "F7"}, {futurerender.KeyF8, "F8"}, {futurerender.KeyF9, "F9"},
	{futurerender.KeyF10, "F10"}, {futurerender.KeyF11, "F11"}, {futurerender.KeyF12, "F12"},
}

type inputGame struct {
	face *text.Face
}

func (g *inputGame) Update() error {
	if futurerender.IsKeyPressed(futurerender.KeyEscape) {
		return futurerender.ErrTermination
	}
	return nil
}

func (g *inputGame) Draw(screen *futurerender.Image) {
	screen.Fill(futurerender.ColorFromRGBA(0.05, 0.05, 0.1, 1.0))

	if g.face == nil {
		return
	}

	lineH := g.face.Metrics().Height
	y := 20.0
	white := fmath.Color{R: 1, G: 1, B: 1, A: 1}
	header := fmath.Color{R: 0.9, G: 0.8, B: 0.3, A: 1}
	green := fmath.Color{R: 0.4, G: 1, B: 0.4, A: 1}
	dim := fmath.Color{R: 0.5, G: 0.5, B: 0.5, A: 1}

	draw := func(s string, c fmath.Color) {
		text.Draw(screen, s, g.face, 20, y, &text.DrawOptions{ColorScale: c})
		y += lineH
	}

	// Keyboard section.
	draw("=== Keyboard ===", header)

	var pressed []string
	for _, ke := range allKeys {
		if futurerender.IsKeyPressed(ke.key) {
			pressed = append(pressed, ke.name)
		}
	}
	if len(pressed) > 0 {
		draw("Pressed: "+strings.Join(pressed, ", "), green)
	} else {
		draw("Pressed: (none)", dim)
	}

	y += lineH * 0.5

	// Mouse section.
	draw("=== Mouse ===", header)

	mx, my := futurerender.CursorPosition()
	draw(fmt.Sprintf("Position: (%d, %d)", mx, my), white)

	var buttons []string
	if futurerender.IsMouseButtonPressed(futurerender.MouseButtonLeft) {
		buttons = append(buttons, "Left")
	}
	if futurerender.IsMouseButtonPressed(futurerender.MouseButtonRight) {
		buttons = append(buttons, "Right")
	}
	if futurerender.IsMouseButtonPressed(futurerender.MouseButtonMiddle) {
		buttons = append(buttons, "Middle")
	}
	if len(buttons) > 0 {
		draw("Buttons: "+strings.Join(buttons, ", "), green)
	} else {
		draw("Buttons: (none)", dim)
	}

	y += lineH * 0.5

	// Gamepad section.
	draw("=== Gamepad ===", header)

	gpIDs := futurerender.GamepadIDs()
	if len(gpIDs) == 0 {
		draw("No gamepad connected", dim)
	}
	for _, id := range gpIDs {
		draw(fmt.Sprintf("Gamepad %d:", id), white)

		// Show first 6 axes.
		for axis := 0; axis < 6; axis++ {
			val := futurerender.GamepadAxisValue(id, axis)
			draw(fmt.Sprintf("  Axis %d: %.3f", axis, val), white)
		}

		// Show first 16 buttons.
		var gpButtons []string
		for btn := 0; btn < 16; btn++ {
			if futurerender.IsGamepadButtonPressed(id, futurerender.GamepadButton(btn)) {
				gpButtons = append(gpButtons, fmt.Sprintf("%d", btn))
			}
		}
		if len(gpButtons) > 0 {
			draw("  Buttons: "+strings.Join(gpButtons, ", "), green)
		} else {
			draw("  Buttons: (none)", dim)
		}
	}

	y = screenH - 40
	draw("Press Escape to exit", dim)
}

func (g *inputGame) Layout(_, _ int) (int, int) {
	return screenW, screenH
}

func main() {
	futurerender.SetWindowSize(screenW, screenH)
	futurerender.SetWindowTitle("Future Render \u2014 Input Example")

	game := &inputGame{}
	wrapper := &lazyInitGame{game: game}

	if err := futurerender.RunGame(wrapper); err != nil {
		log.Fatal(err)
	}
}

// lazyInitGame wraps inputGame to initialize the font face on first Update,
// after the GPU device is ready.
type lazyInitGame struct {
	game   *inputGame
	inited bool
}

func (g *lazyInitGame) Update() error {
	if !g.inited {
		var err error
		g.game.face, err = text.NewFace(goregular.TTF, 16)
		if err != nil {
			return err
		}
		g.inited = true
	}
	return g.game.Update()
}

func (g *lazyInitGame) Draw(screen *futurerender.Image) {
	g.game.Draw(screen)
}

func (g *lazyInitGame) Layout(outsideWidth, outsideHeight int) (int, int) {
	return g.game.Layout(outsideWidth, outsideHeight)
}
