//go:build (darwin || linux || freebsd || windows) && !android

package futurerender

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
	"github.com/michaelraines/future-core/internal/gl"
	"github.com/michaelraines/future-core/internal/input"
	"github.com/michaelraines/future-core/internal/pipeline"
	"github.com/michaelraines/future-core/internal/platform"
	fmath "github.com/michaelraines/future-core/math"

	// Register backends so they are available for selection.
	_ "github.com/michaelraines/future-core/internal/backend/dx12"
	_ "github.com/michaelraines/future-core/internal/backend/metal"
	_ "github.com/michaelraines/future-core/internal/backend/opengl"
	_ "github.com/michaelraines/future-core/internal/backend/soft"
	_ "github.com/michaelraines/future-core/internal/backend/vulkan"
	_ "github.com/michaelraines/future-core/internal/backend/webgpu"
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

	// Screen presenter: blits non-GL backend output to the GL framebuffer.
	presenter *gl.Presenter

	// noGL is true when the backend uses its own presentation (Vulkan, Metal, DX12).
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
	sp.ConsumePendingClear = func(targetID uint32) bool {
		if e.rend.pendingClears[targetID] {
			delete(e.rend.pendingClears, targetID)
			return true
		}
		return false
	}

	// Build render pipeline.
	e.renderPipeline = pipeline.New()
	e.renderPipeline.AddPass(sp)

	// Check if the backend needs a screen presenter (non-GL backends).
	// Pass nil to probe without reading pixels. Skip if noGL since the
	// GL presenter requires an OpenGL context.
	if !e.noGL && dev.ReadScreen(nil) {
		e.presenter = &gl.Presenter{}
	}

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
	if e.presenter != nil {
		e.presenter.Dispose()
	}
}

func (e *engine) run() error {
	// Check for headless screenshot-and-exit mode.
	headless := getHeadlessConfig()

	// Resolve backend name BEFORE creating the window so we can determine
	// whether the window needs an OpenGL context.
	preferred := preferredBackends()
	resolvedName := resolveBackendName(backendName(), preferred)
	// Vulkan and WebGPU have their own swapchain presentation. Metal and
	// DX12 still use the GL presenter (soft-delegation → ReadScreen → GL blit).
	needsNoGL := resolvedName == "vulkan" || resolvedName == "webgpu"
	e.noGL = needsNoGL

	// Create platform window (selected per OS via build tags).
	win := newPlatformWindow()
	e.window = win

	winCfg := e.windowConfig()
	winCfg.NoGL = needsNoGL
	if err := win.Create(winCfg); err != nil {
		return err
	}
	defer win.Destroy()

	// Initialize GL functions only if we have an OpenGL context.
	if !needsNoGL {
		if err := gl.Init(); err != nil {
			return fmt.Errorf("gl init: %w", err)
		}
	}

	// Create the backend device.
	dev, resolved, err := backend.Resolve(backendName(), preferred)
	if err != nil {
		return fmt.Errorf("backend selection: %w", err)
	}
	resolvedBackend.Store(resolved)

	// Build DeviceConfig with SurfaceFactory if the window supports Vulkan surfaces.
	fbW, fbH := win.FramebufferSize()
	devCfg := backend.DeviceConfig{
		Width:  fbW,
		Height: fbH,
		VSync:  true,
	}
	// In headless mode, skip swapchain creation so the backend renders to its
	// offscreen target, which supports ReadScreen for screenshot capture.
	if needsNoGL && headless == nil {
		if resolvedName == "webgpu" {
			// WebGPU uses wgpu.InstanceCreateSurface with the native layer,
			// not the Vulkan surface factory.
			if provider, ok := win.(platform.MetalLayerProvider); ok {
				devCfg.MetalLayer = provider.MetalLayer()
			}
		} else if creator, ok := win.(platform.VulkanSurfaceCreator); ok {
			devCfg.SurfaceFactory = func(instance uintptr) (uintptr, error) {
				return creator.CreateVulkanSurface(instance)
			}
		}
	}
	if err := dev.Init(devCfg); err != nil {
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
		pendingClears: make(map[uint32]bool),
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

		// Resize the backend's internal screen render target if needed.
		// Software and soft-delegating backends keep a fixed-size screen
		// buffer that must be recreated when the framebuffer dimensions change.
		if resizer, ok := e.device.(interface{ ResizeScreen(int, int) }); ok {
			resizer.ResizeScreen(fbW, fbH)
		}

		screen := &Image{
			width: screenW, height: screenH,
			u0: 0, v0: 0, u1: 1, v1: 1,
		}
		e.game.Draw(screen)

		// Compute orthographic projection for the logical screen.
		proj := fmath.Mat4Ortho(0, float64(screenW), float64(screenH), 0, -1, 1)
		e.spritePass.Projection = proj.Float32()

		// Begin frame: prepare the backend for command recording.
		e.device.BeginFrame()

		// Execute the render pipeline (sprite pass manages its own render
		// passes per target, including clearing the screen target).
		ctx := pipeline.NewPassContext(fbW, fbH)
		ctx.ScreenClearEnabled = IsScreenClearedEveryFrame()
		e.renderPipeline.Execute(e.encoder, ctx)

		// End frame: submit recorded commands to the GPU.
		e.device.EndFrame()

		// Dispose any images deferred during this frame (e.g. AA buffers
		// that were replaced mid-frame). Safe to release now because
		// the sprite pass has consumed all references.
		if e.rend != nil {
			e.rend.disposeDeferred()
		}

		// For non-GL backends: read rendered pixels and blit to the
		// window's GL framebuffer.
		if e.presenter != nil {
			if e.presenter.Tex == 0 {
				e.presenter = gl.InitPresenter(fbW, fbH)
			}
			e.presenter.Resize(fbW, fbH)
			if e.device.ReadScreen(e.presenter.Buf) {
				e.presenter.Present(e.presenter.Buf, fbW, fbH, fbW, fbH)
			}
		}

		// Headless screenshot-and-exit: capture after N frames (before
		// SwapBuffers so the back buffer still contains rendered content).
		frameCount++
		if headless != nil && frameCount >= headless.frames {
			if err := e.saveScreenshot(fbW, fbH, headless.output); err != nil {
				fmt.Fprintf(os.Stderr, "headless: %v\n", err)
				os.Exit(1) //nolint:gocritic // intentional force-exit; defers are expendable in headless capture mode
			}
			fmt.Printf("headless: captured frame %d → %s (%dx%d, backend=%s)\n",
				frameCount, headless.output, fbW, fbH, resolved)
			// Force exit — on macOS the Cocoa event loop may keep the
			// process alive even after window destruction.
			os.Exit(0)
		}

		win.SwapBuffers()

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

func (e *engine) setWindowResizable(_ bool) {}

// resolveBackendName determines the backend name that will be used, without
// actually creating the device. Used to decide window configuration (e.g. NoGL)
// before the window is created.
func resolveBackendName(name string, preferred []string) string {
	if name != "auto" {
		return name
	}
	for _, p := range preferred {
		if backend.IsRegistered(p) {
			return p
		}
	}
	return "soft" // fallback
}

// preferredBackends returns the platform-specific preferred backend order.
// OpenGL is currently preferred on all platforms because it is the only
// backend with a working GLSL shader pipeline. The GPU backends (Metal,
// Vulkan, DX12) compile by default and have presentation layers, but need
// a GLSL→MSL/SPIR-V translation step before they can render. Once shader
// cross-compilation is added, restore the native API preferences:
//
//	darwin:  metal, opengl, soft
//	windows: dx12, vulkan, opengl, soft
//	linux:   vulkan, opengl, soft
func preferredBackends() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"opengl", "metal", "soft"}
	case "windows":
		return []string{"opengl", "dx12", "vulkan", "soft"}
	case "linux", "freebsd":
		return []string{"opengl", "vulkan", "soft"}
	default:
		return []string{"opengl", "soft"}
	}
}
