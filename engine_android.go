//go:build android

package futurerender

import (
	"errors"
	"fmt"
	"runtime"
	"time"

	"golang.org/x/mobile/app"
	mkey "golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
	"golang.org/x/mobile/event/touch"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
	"github.com/michaelraines/future-core/internal/input"
	"github.com/michaelraines/future-core/internal/pipeline"
	"github.com/michaelraines/future-core/internal/platform"
	androidplatform "github.com/michaelraines/future-core/internal/platform/android"
	fmath "github.com/michaelraines/future-core/math"

	// Register backends available on Android.
	_ "github.com/michaelraines/future-core/internal/backend/soft"
	_ "github.com/michaelraines/future-core/internal/backend/vulkan"
)

const (
	maxBatchVertices = 65536
	maxBatchIndices  = 65536 * 6
)

// Default sprite shader source (GLSL 330 core — same as desktop; backends
// handle any translation needed for Vulkan SPIR-V).
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

	// noGL is always true on Android (Vulkan handles its own presentation).
	noGL bool

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
	// Android uses an inverted control flow: the OS drives the event loop
	// via app.Main, and we render in response to paint events.
	app.Main(func(a app.App) {
		e.runAndroid(a)
	})
	return nil
}

// runAndroid implements the Android event loop.
func (e *engine) runAndroid(a app.App) {
	// Vulkan handles its own presentation on Android.
	e.noGL = true

	// Create platform window.
	win := newPlatformWindow()
	e.window = win

	// Set the x/mobile app reference on the Android window.
	androidWin, _ := win.(*androidplatform.Window)
	if androidWin != nil {
		androidWin.SetApp(a)
	}

	winCfg := e.windowConfig()
	winCfg.NoGL = true
	if err := win.Create(winCfg); err != nil {
		fmt.Printf("android: window create: %v\n", err)
		return
	}
	defer win.Destroy()

	// Set up input.
	inputState := input.New()
	win.SetInputHandler(inputState)
	e.inputState = inputState

	var (
		deviceInit  bool
		lastTime    = time.Now()
		tps         = MaxTPS()
		accumulator time.Duration
		frameCount  int
		tickCount   int
		fpsTimer    = time.Now()
	)

	for ev := range a.Events() {
		switch ev := a.Filter(ev).(type) {
		case lifecycle.Event:
			if androidWin != nil {
				androidWin.HandleLifecycleEvent(ev)
			}
			// If the activity is dead, stop the loop.
			if ev.Crosses(lifecycle.StageDead) == lifecycle.CrossOn {
				return
			}

		case size.Event:
			if androidWin != nil {
				androidWin.HandleSizeEvent(ev)
			}

			// (Re-)initialize the backend if not yet done.
			if !deviceInit && ev.WidthPx > 0 && ev.HeightPx > 0 {
				if err := e.initDevice(win); err != nil {
					fmt.Printf("android: init device: %v\n", err)
					return
				}
				deviceInit = true
			}

		case touch.Event:
			if androidWin != nil {
				androidWin.HandleTouchEvent(ev)
			}

		case mkey.Event:
			if androidWin != nil {
				androidWin.HandleKeyEvent(ev)
			}

		case paint.Event:
			if !deviceInit {
				continue
			}
			if androidWin != nil && !androidWin.HandlePaintEvent(ev) {
				continue
			}

			// Process queued input events.
			now := time.Now()
			delta := now.Sub(lastTime)
			lastTime = now

			// Re-read TPS in case it changed.
			tps = MaxTPS()
			tickDuration := time.Duration(0)
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
							return
						}
						fmt.Printf("android: game update: %v\n", err)
						return
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
						return
					}
					fmt.Printf("android: game update: %v\n", err)
					return
				}
				tickCount++
			}

			// Draw.
			fbW, fbH := win.FramebufferSize()
			screenW, screenH := e.game.Layout(win.Size())

			screen := &Image{
				width: screenW, height: screenH,
				u0: 0, v0: 0, u1: 1, v1: 1,
			}
			e.game.Draw(screen)

			// Compute orthographic projection for the logical screen.
			proj := fmath.Mat4Ortho(0, float64(screenW), float64(screenH), 0, -1, 1)
			e.spritePass.Projection = proj.Float32()

			// Begin frame.
			e.device.BeginFrame()

			// Execute the render pipeline.
			ctx := pipeline.NewPassContext(fbW, fbH)
			ctx.ScreenClearEnabled = IsScreenClearedEveryFrame()
			e.renderPipeline.Execute(e.encoder, ctx)

			// End frame (submits to GPU + presents via Vulkan swapchain).
			e.device.EndFrame()

			// Update FPS/TPS counters every second.
			frameCount++
			if time.Since(fpsTimer) >= time.Second {
				e.fpsValue = float64(frameCount)
				e.tpsValue = float64(tickCount)
				frameCount = 0
				tickCount = 0
				fpsTimer = time.Now()
			}

			// Request continuous rendering.
			a.Publish()
		}
	}
}

// initDevice creates and initializes the rendering backend device.
func (e *engine) initDevice(win platform.Window) error {
	preferred := preferredBackends()
	dev, resolved, err := backend.Resolve(backendName(), preferred)
	if err != nil {
		return fmt.Errorf("backend selection: %w", err)
	}
	resolvedBackend.Store(resolved)

	fbW, fbH := win.FramebufferSize()
	devCfg := backend.DeviceConfig{
		Width:  fbW,
		Height: fbH,
		VSync:  true,
	}

	// Set up Vulkan surface factory if the window supports it.
	if creator, ok := win.(platform.VulkanSurfaceCreator); ok {
		devCfg.SurfaceFactory = func(instance uintptr) (uintptr, error) {
			return creator.CreateVulkanSurface(instance)
		}
	}

	if err := dev.Init(devCfg); err != nil {
		return err
	}

	e.device = dev
	e.encoder = dev.Encoder()

	// Initialize renderer.
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

	// Create rendering resources.
	return e.initRenderResources()
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

func (e *engine) setFullscreen(_ bool)       {}
func (e *engine) isFullscreen() bool         { return true }
func (e *engine) setVSync(_ bool)            {}
func (e *engine) isVSync() bool              { return true }
func (e *engine) currentFPS() float64        { return e.fpsValue }
func (e *engine) currentTPS() float64        { return e.tpsValue }
func (e *engine) setCursorMode(_ CursorMode) {}

func (e *engine) deviceScaleFactor() float64 {
	if e.window != nil {
		return e.window.DevicePixelRatio()
	}
	return 1.0
}

// preferredBackends returns Android's backend preference order.
// Vulkan is the primary GPU API on Android; soft rasterizer as fallback.
func preferredBackends() []string {
	return []string{"vulkan", "soft"}
}

// resolveBackendName determines the backend name that will be used.
func resolveBackendName(name string, preferred []string) string {
	if name != "auto" {
		return name
	}
	for _, p := range preferred {
		if backend.IsRegistered(p) {
			return p
		}
	}
	return "soft"
}

// Ensure Android main thread lock for Vulkan.
func init() {
	runtime.LockOSThread()
}
