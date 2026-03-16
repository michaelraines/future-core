// Command clear is a minimal smoke test for Future Render.
// It opens a window, clears it to cornflower blue, and exits on Escape.
//
// Build: go build ./cmd/clear
// Run:   ./clear
package main

import (
	"log"

	futurerender "github.com/michaelraines/future-render"
)

type clearGame struct{}

func (g *clearGame) Update() error {
	if futurerender.IsKeyPressed(futurerender.KeyEscape) {
		return futurerender.ErrTermination
	}
	return nil
}

func (g *clearGame) Draw(_ *futurerender.Image) {
	// M1: drawing is handled by the engine's clear pass.
	// The clear color will be set once the pipeline is fully wired.
}

func (g *clearGame) Layout(_, _ int) (screenW, screenH int) {
	return 640, 480
}

func main() {
	futurerender.SetWindowSize(800, 600)
	futurerender.SetWindowTitle("Future Render — Clear Test")
	if err := futurerender.RunGame(&clearGame{}); err != nil {
		log.Fatal(err)
	}
}
