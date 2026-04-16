//go:build android

package futurerender

import (
	"errors"
	"fmt"
	"time"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
	"github.com/michaelraines/future-core/internal/input"
	"github.com/michaelraines/future-core/internal/pipeline"
	"github.com/michaelraines/future-core/internal/platform"
	fmath "github.com/michaelraines/future-core/math"

	// Register backends available on Android.
	_ "github.com/michaelraines/future-core/internal/backend/soft"
	_ "github.com/michaelraines/future-core/internal/backend/vulkan"
)

// This file holds the shared Android engine type and helpers. The
// per-frame driver loop lives in one of two sibling files chosen by
// build tag:
//
//   engine_android_nativeactivity.go  — build tag: android && futurecore_nativeactivity
//                                       Pure-Go NativeActivity path via
//                                       golang.org/x/mobile/app.Main. Used by
//                                       gomobile-BUILT APKs where Go owns the
//                                       Android process.
//
//   engine_android_embedded.go        — build tag: android && !futurecore_nativeactivity  (default)
//                                       gomobile-BOUND AAR path where a host
//                                       Java Activity owns the process and
//                                       drives TickOnce() per frame from a
//                                       render thread via JNI. See the
//                                       mobile/futurecoreview package for the
//                                       JNI-callable surface.

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
    // Clamp RGB to alpha so (1,1,1,<1) vertex colors stay correctly
    // premultiplied. Reconstruct vec4 in one expression because WGSL
    // (other future-core backends) rejects swizzle assignment. See
    // engine_js.go for the full explanation.
    c = vec4(min(c.rgb, vec3(c.a)), c.a);
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

	// Frame-loop state shared between the NativeActivity and embedded
	// drivers. Both call TickOnce() per frame; the state here survives
	// across invocations.
	lastTime    time.Time
	accumulator time.Duration
	frameCount  int
	tickCount   int
	fpsTimer    time.Time

	// deviceInitialized gates the first TickOnce call — rendering
	// resources are created lazily once the host surface is known.
	deviceInitialized bool
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

	// Wire texture resolver. The miss path surfaces a one-shot warning
	// when a previously-disposed ID is looked up — see renderer.go.
	sp.ResolveTexture = func(texID uint32) backend.Texture {
		if tex, ok := e.textures[texID]; ok {
			return tex
		}
		e.rend.warnStaleIDOnce(texID, "texture")
		return nil
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
		if rt, ok := e.renderTargets[targetID]; ok {
			return rt
		}
		e.rend.warnStaleIDOnce(targetID, "render target")
		return nil
	}
	sp.ConsumePendingClear = func(targetID uint32) bool {
		return e.rend.pendingClears.Consume(targetID)
	}
	sp.ApplyUniforms = func(shader backend.Shader, uniforms map[string]any) {
		for name, val := range uniforms {
			applyUniformValue(shader, name, val)
		}
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

// TickOnce executes a single frame of the Android game loop: fixed-
// timestep updates, one Draw call, one swapchain present. Called once
// per vsync by whichever driver owns the process — runAndroid's
// paint.Event handler in the NativeActivity path, or the JNI render
// thread in the embedded path. Returns ErrTermination if the game
// asked to quit, or any other error to halt the loop.
//
// The caller is responsible for ensuring initDevice has succeeded
// before invoking TickOnce.
func (e *engine) TickOnce() error {
	if !e.deviceInitialized || e.window == nil {
		return nil
	}

	now := time.Now()
	if e.lastTime.IsZero() {
		e.lastTime = now
		e.fpsTimer = now
	}
	delta := now.Sub(e.lastTime)
	e.lastTime = now

	tps := MaxTPS()
	tickDuration := time.Duration(0)
	if tps > 0 {
		tickDuration = time.Second / time.Duration(tps)
	}

	// Fixed-timestep update. See engine_desktop.go for the
	// input-ordering rationale (Poll → BeginTick → game → EndTick).
	// Android events are pushed into inputState before TickOnce runs.
	if tps > 0 {
		e.accumulator += delta
		for e.accumulator >= tickDuration {
			e.window.PollEvents()
			e.window.PollGamepads()
			e.inputState.BeginTick()
			if err := e.game.Update(); err != nil {
				if errors.Is(err, ErrTermination) {
					return err
				}
				return fmt.Errorf("android: game update: %w", err)
			}
			e.inputState.EndTick()
			e.tickCount++
			e.accumulator -= tickDuration
		}
	} else {
		e.window.PollEvents()
		e.window.PollGamepads()
		e.inputState.BeginTick()
		if err := e.game.Update(); err != nil {
			if errors.Is(err, ErrTermination) {
				return err
			}
			return fmt.Errorf("android: game update: %w", err)
		}
		e.inputState.EndTick()
		e.tickCount++
	}

	// Draw.
	fbW, fbH := e.window.FramebufferSize()
	screenW, screenH := e.game.Layout(e.window.Size())

	screen := &Image{
		width: screenW, height: screenH,
		u0: 0, v0: 0, u1: 1, v1: 1,
	}
	e.game.Draw(screen)

	// Compute orthographic projection for the logical screen.
	proj := fmath.Mat4Ortho(0, float64(screenW), float64(screenH), 0, -1, 1)
	e.spritePass.Projection = proj.Float32()

	e.device.BeginFrame()
	ctx := pipeline.NewPassContext(fbW, fbH)
	ctx.ScreenClearEnabled = IsScreenClearedEveryFrame()
	e.renderPipeline.Execute(e.encoder, ctx)
	e.device.EndFrame()

	// Release any deferred AA buffers.
	if e.rend != nil {
		e.rend.disposeDeferred()
	}

	// Update FPS/TPS counters every second.
	e.frameCount++
	if time.Since(e.fpsTimer) >= time.Second {
		e.fpsValue = float64(e.frameCount)
		e.tpsValue = float64(e.tickCount)
		e.frameCount = 0
		e.tickCount = 0
		e.fpsTimer = time.Now()
	}

	return nil
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
		unregisterTexture: func(id uint32) {
			delete(e.textures, id)
		},
		registerShader: func(id uint32, shader *Shader) {
			e.shaders[id] = shader
		},
		registerRenderTarget: func(id uint32, rt backend.RenderTarget) {
			e.renderTargets[id] = rt
		},
		unregisterRenderTarget: func(id uint32) {
			delete(e.renderTargets, id)
		},
		pendingClears: newPendingClearTracker(),
	}
	e.rend = rend
	setRenderer(rend)

	// Create rendering resources.
	if err := e.initRenderResources(); err != nil {
		return err
	}
	e.deviceInitialized = true
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

func (e *engine) setWindowResizable(_ bool) {}

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
