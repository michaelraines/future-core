//go:build glfw

package futurerender

import (
	"errors"
	"time"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/backend/opengl"
	"github.com/michaelraines/future-render/internal/batch"
	"github.com/michaelraines/future-render/internal/input"
	"github.com/michaelraines/future-render/internal/pipeline"
	"github.com/michaelraines/future-render/internal/platform"
	glfwplatform "github.com/michaelraines/future-render/internal/platform/glfw"
	fmath "github.com/michaelraines/future-render/math"
)

const (
	maxBatchVertices = 65536
	maxBatchIndices  = 65536 * 6
)

// Default sprite shader source (GLSL 330 core).
const spriteVertexShader = `#version 330 core

layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;

uniform mat4 uProjection;

out vec2 vTexCoord;
out vec4 vColor;

void main() {
    vTexCoord = aTexCoord;
    vColor = aColor;
    gl_Position = uProjection * vec4(aPosition, 0.0, 1.0);
}
`

const spriteFragmentShader = `#version 330 core

in vec2 vTexCoord;
in vec4 vColor;

uniform sampler2D uTexture;
uniform mat4 uColorBody;
uniform vec4 uColorTranslation;

out vec4 fragColor;

void main() {
    vec4 c = texture(uTexture, vTexCoord) * vColor;
    fragColor = uColorBody * c + uColorTranslation;
}
`

type engine struct {
	game       Game
	fpsValue   float64
	tpsValue   float64
	window     platform.Window
	device     backend.Device
	encoder    backend.CommandEncoder
	inputState *input.State

	// Rendering resources.
	rend           *renderer
	spriteShader   backend.Shader
	spritePipeline backend.Pipeline
	whiteTexture   backend.Texture
	spritePass     *pipeline.SpritePass
	renderPipeline *pipeline.Pipeline

	// Texture registry: maps texture IDs to backend textures.
	textures map[uint32]backend.Texture

	// Shader registry: maps shader IDs to Shader objects for SpritePass lookup.
	shaders map[uint32]*Shader

	// Render target registry: maps target IDs to backend render targets.
	renderTargets map[uint32]backend.RenderTarget

	// Window config state.
	windowTitle string
	windowW     int
	windowH     int
}

func newPlatformEngine(game Game) *engine {
	return &engine{
		game:          game,
		windowTitle:   pendingWindowTitle,
		windowW:       pendingWindowWidth,
		windowH:       pendingWindowHeight,
		textures:      make(map[uint32]backend.Texture),
		shaders:       make(map[uint32]*Shader),
		renderTargets: make(map[uint32]backend.RenderTarget),
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

// registerTexture adds a texture to the engine's registry for lookup by ID.
func (e *engine) registerTexture(id uint32, tex backend.Texture) {
	e.textures[id] = tex
}

// initRenderResources creates the default shader, pipeline, and white texture.
func (e *engine) initRenderResources() error {
	dev := e.device

	// 1x1 white texture for untextured draws.
	tex, err := dev.NewTexture(backend.TextureDescriptor{
		Width:  1,
		Height: 1,
		Format: backend.TextureFormatRGBA8,
		Filter: backend.FilterNearest,
		WrapU:  backend.WrapClamp,
		WrapV:  backend.WrapClamp,
		Data:   []byte{255, 255, 255, 255},
	})
	if err != nil {
		return err
	}
	e.whiteTexture = tex
	e.rend.whiteTextureID = e.rend.allocTextureID()
	e.registerTexture(e.rend.whiteTextureID, tex)

	// Default sprite shader.
	sh, err := dev.NewShader(backend.ShaderDescriptor{
		VertexSource:   spriteVertexShader,
		FragmentSource: spriteFragmentShader,
		Attributes:     batch.Vertex2DFormat().Attributes,
	})
	if err != nil {
		return err
	}
	e.spriteShader = sh

	// Default sprite pipeline.
	pip, err := dev.NewPipeline(backend.PipelineDescriptor{
		Shader:       sh,
		VertexFormat: batch.Vertex2DFormat(),
		BlendMode:    backend.BlendSourceOver,
		DepthTest:    false,
		DepthWrite:   false,
		CullMode:     backend.CullNone,
		Primitive:    backend.PrimitiveTriangles,
	})
	if err != nil {
		return err
	}
	e.spritePipeline = pip

	// Sprite pass.
	sp, err := pipeline.NewSpritePass(pipeline.SpritePassConfig{
		Device:      dev,
		Batcher:     e.rend.batcher,
		Pipeline:    pip,
		Shader:      sh,
		MaxVertices: maxBatchVertices,
		MaxIndices:  maxBatchIndices,
	})
	if err != nil {
		return err
	}
	e.spritePass = sp

	// Wire texture resolver.
	sp.ResolveTexture = func(texID uint32) backend.Texture {
		return e.textures[texID]
	}

	// Wire shader resolver.
	sp.ResolveShader = func(shaderID uint32) *pipeline.ShaderInfo {
		s, ok := e.shaders[shaderID]
		if !ok || s == nil {
			return nil
		}
		return &pipeline.ShaderInfo{
			Shader:   s.backend,
			Pipeline: s.pipeline,
		}
	}

	// Wire render target resolver.
	sp.ResolveRenderTarget = func(targetID uint32) backend.RenderTarget {
		return e.renderTargets[targetID]
	}

	// Build render pipeline.
	e.renderPipeline = pipeline.New()
	e.renderPipeline.AddPass(sp)

	return nil
}

// disposeRenderResources releases all rendering resources.
func (e *engine) disposeRenderResources() {
	if e.spritePass != nil {
		e.spritePass.Dispose()
	}
	if e.spritePipeline != nil {
		e.spritePipeline.Dispose()
	}
	if e.spriteShader != nil {
		e.spriteShader.Dispose()
	}
	if e.whiteTexture != nil {
		e.whiteTexture.Dispose()
	}
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

	// Initialize renderer (shared state for Image API).
	rend := &renderer{
		device:  dev,
		batcher: batch.NewBatcher(maxBatchVertices, maxBatchIndices),
		registerTexture: func(id uint32, tex backend.Texture) {
			e.textures[id] = tex
		},
		registerShader: func(id uint32, shader *Shader) {
			e.shaders[id] = shader
		},
		registerRenderTarget: func(id uint32, rt backend.RenderTarget) {
			e.renderTargets[id] = rt
		},
	}
	e.rend = rend
	setRenderer(rend)

	// Create rendering resources (shaders, pipeline, sprite pass).
	if err := e.initRenderResources(); err != nil {
		return err
	}
	defer e.disposeRenderResources()

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
				win.PollGamepads()
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
			win.PollGamepads()
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

		screen := &Image{
			width: screenW, height: screenH,
			u0: 0, v0: 0, u1: 1, v1: 1,
		}
		e.game.Draw(screen)

		// Compute orthographic projection for the logical screen.
		proj := fmath.Mat4Ortho(0, float64(screenW), float64(screenH), 0, -1, 1)
		e.spritePass.Projection = proj.Float32()

		// Begin render pass: clear or preserve based on user setting.
		loadAction := backend.LoadActionClear
		if !IsScreenClearedEveryFrame() {
			loadAction = backend.LoadActionLoad
		}
		e.encoder.BeginRenderPass(backend.RenderPassDescriptor{
			ClearColor:  [4]float32{0, 0, 0, 1},
			ClearDepth:  1.0,
			LoadAction:  loadAction,
			StoreAction: backend.StoreActionStore,
		})
		e.encoder.SetViewport(backend.Viewport{
			X: 0, Y: 0, Width: fbW, Height: fbH,
		})

		// Execute the render pipeline (sprite pass flushes batcher).
		ctx := pipeline.NewPassContext(fbW, fbH)
		e.renderPipeline.Execute(e.encoder, ctx)

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
		e.window.SetCursorLocked(false)
		e.window.SetCursorVisible(false)
	case CursorModeCaptured:
		e.window.SetCursorVisible(false)
		e.window.SetCursorLocked(true)
	default: // CursorModeVisible
		e.window.SetCursorLocked(false)
		e.window.SetCursorVisible(true)
	}
}

func (e *engine) deviceScaleFactor() float64 {
	if e.window != nil {
		return e.window.DevicePixelRatio()
	}
	return 1.0
}
