//go:build glfw

package futurerender

import (
	"errors"
	"time"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/backend/opengl"
	"github.com/michaelraines/future-render/internal/input"
	"github.com/michaelraines/future-render/internal/platform"
	glfwplatform "github.com/michaelraines/future-render/internal/platform/glfw"
)

type engine struct {
	game       Game
	fpsValue   float64
	tpsValue   float64
	window     platform.Window
	device     backend.Device
	encoder    backend.CommandEncoder
	inputState *input.State

	// Window config state.
	windowTitle string
	windowW     int
	windowH     int
}

func newPlatformEngine(game Game) *engine {
	return &engine{
		game:        game,
		windowTitle: pendingWindowTitle,
		windowW:     pendingWindowWidth,
		windowH:     pendingWindowHeight,
	}
}

func (e *engine) windowConfig() platform.WindowConfig {
	cfg := platform.DefaultWindowConfig()
	if e.windowTitle != "" {
		cfg.Title = e.windowTitle
	}
	if e.windowW > 0 {
		cfg.Width = e.windowW
	}
	if e.windowH > 0 {
		cfg.Height = e.windowH
	}
	return cfg
}

func (e *engine) run() error {
	// Create platform window.
	win := glfwplatform.New()
	e.window = win

	winCfg := e.windowConfig()
	if err := win.Create(winCfg); err != nil {
		return err
	}
	defer win.Destroy()

	// Initialize OpenGL backend.
	dev := opengl.New()
	fbW, fbH := win.FramebufferSize()
	if err := dev.Init(backend.DeviceConfig{
		Width:  fbW,
		Height: fbH,
		VSync:  true,
	}); err != nil {
		return err
	}

	e.device = dev
	e.encoder = dev.Encoder()

	// Set up input.
	inputState := input.New()
	win.SetInputHandler(inputState)
	e.inputState = inputState

	// Main loop: fixed-timestep update + variable-rate draw.
	tps := MaxTPS()
	tickDuration := time.Duration(0)
	if tps > 0 {
		tickDuration = time.Second / time.Duration(tps)
	}

	lastTime := time.Now()
	accumulator := time.Duration(0)

	// FPS/TPS tracking.
	frameCount := 0
	tickCount := 0
	fpsTimer := time.Now()

	for !win.ShouldClose() {
		now := time.Now()
		delta := now.Sub(lastTime)
		lastTime = now

		// Re-read TPS in case it changed.
		tps = MaxTPS()
		if tps > 0 {
			tickDuration = time.Second / time.Duration(tps)
		}

		// Fixed-timestep update.
		if tps > 0 {
			accumulator += delta
			for accumulator >= tickDuration {
				inputState.Update()
				win.PollEvents()
				if err := e.game.Update(); err != nil {
					if errors.Is(err, ErrTermination) {
						return nil
					}
					return err
				}
				tickCount++
				accumulator -= tickDuration
			}
		} else {
			// Uncapped: one update per frame.
			inputState.Update()
			win.PollEvents()
			if err := e.game.Update(); err != nil {
				if errors.Is(err, ErrTermination) {
					return nil
				}
				return err
			}
			tickCount++
		}

		// Draw.
		fbW, fbH = win.FramebufferSize()
		screenW, screenH := e.game.Layout(win.Size())

		screen := &Image{width: screenW, height: screenH}
		e.game.Draw(screen)

		// Clear the default framebuffer.
		e.encoder.BeginRenderPass(backend.RenderPassDescriptor{
			ClearColor:  [4]float32{0, 0, 0, 1},
			ClearDepth:  1.0,
			LoadAction:  backend.LoadActionClear,
			StoreAction: backend.StoreActionStore,
		})
		e.encoder.SetViewport(backend.Viewport{
			X: 0, Y: 0, Width: fbW, Height: fbH,
		})
		e.encoder.EndRenderPass()

		win.SwapBuffers()
		frameCount++

		// Update FPS/TPS counters every second.
		if time.Since(fpsTimer) >= time.Second {
			e.fpsValue = float64(frameCount)
			e.tpsValue = float64(tickCount)
			frameCount = 0
			tickCount = 0
			fpsTimer = time.Now()
		}
	}

	return nil
}

func (e *engine) setWindowSize(width, height int) {
	e.windowW = width
	e.windowH = height
	if e.window != nil {
		e.window.SetSize(width, height)
	}
}

func (e *engine) setWindowTitle(title string) {
	e.windowTitle = title
	if e.window != nil {
		e.window.SetTitle(title)
	}
}

func (e *engine) setFullscreen(fullscreen bool) {
	if e.window != nil {
		e.window.SetFullscreen(fullscreen)
	}
}

func (e *engine) isFullscreen() bool {
	if e.window != nil {
		return e.window.IsFullscreen()
	}
	return false
}

func (e *engine) setVSync(_ bool) {
	// Would need to store and apply at next frame.
}

func (e *engine) isVSync() bool { return true }

func (e *engine) currentFPS() float64 { return e.fpsValue }
func (e *engine) currentTPS() float64 { return e.tpsValue }

func (e *engine) setCursorMode(mode CursorMode) {
	if e.window == nil {
		return
	}
	switch mode {
	case CursorModeHidden:
		e.window.SetCursorVisible(false)
	case CursorModeCaptured:
		e.window.SetCursorLocked(true)
	default:
		e.window.SetCursorVisible(true)
	}
}

func (e *engine) deviceScaleFactor() float64 {
	if e.window != nil {
		return e.window.DevicePixelRatio()
	}
	return 1.0
}
