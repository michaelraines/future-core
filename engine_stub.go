//go:build !(darwin || linux || freebsd || windows)

package futurerender

import (
	"errors"

	"github.com/michaelraines/future-render/internal/input"
)

type engine struct {
	game        Game
	fpsValue    float64
	tpsValue    float64
	windowTitle string
	windowW     int
	windowH     int
	inputState  *input.State
}

func newPlatformEngine(game Game) *engine {
	return &engine{
		game:        game,
		windowTitle: pendingWindowTitle,
		windowW:     pendingWindowWidth,
		windowH:     pendingWindowHeight,
	}
}

func (e *engine) run() error {
	return errors.New("engine: no platform backend available (build with -tags glfw)")
}

func (e *engine) setWindowSize(_, _ int)     {}
func (e *engine) setWindowTitle(_ string)    {}
func (e *engine) setFullscreen(_ bool)       {}
func (e *engine) isFullscreen() bool         { return false }
func (e *engine) setVSync(_ bool)            {}
func (e *engine) isVSync() bool              { return true }
func (e *engine) currentFPS() float64        { return e.fpsValue }
func (e *engine) currentTPS() float64        { return e.tpsValue }
func (e *engine) setCursorMode(_ CursorMode) {}
func (e *engine) deviceScaleFactor() float64 { return 1.0 }
