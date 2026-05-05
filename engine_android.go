//go:build android

package futurerender

import (
	"errors"
	"fmt"
	goimage "image"
	"log"
	"time"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
	"github.com/michaelraines/future-core/internal/input"
	"github.com/michaelraines/future-core/internal/pipeline"
	"github.com/michaelraines/future-core/internal/platform"
	platandroid "github.com/michaelraines/future-core/internal/platform/android"
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

// Built-in sprite shader source comes from internal/builtin. Both
// stages are kept as GLSL 330 (for backends that consume GLSL — webgl,
// soft via shadertranslate) and as precompiled SPIR-V (for Vulkan on
// Android, where libshaderc is not available). createSpriteShader
// below picks the right path based on the device's preferred shader
// language.

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

	// noGL is true when the backend presents directly (Vulkan swapchain).
	// False when we need our SoftPresenter (soft backend → ANativeWindow
	// lock/blit/unlockAndPost). Set per-frame in TickOnce based on a
	// ReadScreen(nil) probe, the same pattern the desktop engine uses
	// to decide between direct-present and GL-presenter paths.
	noGL bool

	// softPresenter uploads CPU-rasterized frames directly to the
	// ANativeWindow when the resolved backend is the software
	// rasterizer. Created lazily after initDevice — nil means the
	// backend handles its own presentation (Vulkan swapchain).
	softPresenter  *platandroid.SoftPresenter
	softPresentBuf []byte

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

	// Default sprite shader. Prefers precompiled SPIR-V on Vulkan
	// (avoids shaderc, which is not available on Android); falls back
	// to GLSL on backends that don't take SPIR-V.
	sh, err := createSpriteShader(dev)
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
		bounds: goimage.Rect(0, 0, screenW, screenH),
	}
	e.game.Draw(screen)

	// Compute orthographic projection for the logical screen.
	proj := fmath.Mat4Ortho(0, float64(screenW), float64(screenH), 0, -1, 1)
	e.spritePass.Projection = proj.Float32()

	e.device.BeginFrame()
	ctx := pipeline.NewPassContext(fbW, fbH)
	ctx.ScreenClearEnabled = IsScreenClearedEveryFrame()
	ctx.ScreenHasStencil = e.device.Capabilities().SupportsStencil
	e.renderPipeline.Execute(e.encoder, ctx)
	e.device.EndFrame()

	// Release any deferred AA buffers.
	if e.rend != nil {
		e.rend.disposeDeferred()
	}

	// For CPU-rasterized backends (soft), copy the default color image
	// into the ANativeWindow via lock/unlockAndPost. Swapchain-enabled
	// backends (Vulkan) skip this — EndFrame already presented for them.
	if e.softPresenter != nil {
		need := fbW * fbH * 4
		if len(e.softPresentBuf) < need {
			e.softPresentBuf = make([]byte, need)
		}
		if e.device.ReadScreen(e.softPresentBuf[:need]) {
			if err := e.softPresenter.Present(fbW, fbH, e.softPresentBuf[:need]); err != nil {
				log.Printf("futurerender.TickOnce: soft present failed: %v", err)
			}
		}
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
	requested := backendName()

	// On the ranchu/goldfish Android emulator, gfxstream's guest Vulkan
	// driver has a QSRI sync-fd bug (exportSyncFdForQSRILocked fails
	// with EBADF after a few submits, and every subsequent Vulkan sync
	// primitive — WaitForFences, QueueWaitIdle, DeviceWaitIdle — hangs
	// forever waiting for retirement callbacks that never arrive). The
	// emulator's OpenGL ES path is on a different gfxstream code path
	// that works fine, which is how ebitenmobile-based AARs render on
	// the same AVD. We don't have a GLES backend (yet), but we can
	// take the same "skip Vulkan on emulator" approach by forcing the
	// software rasterizer with our ANativeWindow_lock presenter, which
	// talks to libandroid.so directly and never touches the broken
	// Vulkan driver. Real devices keep the Vulkan default.
	if requested == "auto" && platandroid.IsRanchuEmulator() {
		log.Printf("futurerender.initDevice: emulator detected — forcing soft backend + ANativeWindow present")
		requested = "soft"
	}

	dev, resolved, err := backend.Resolve(requested, preferred)
	if err != nil {
		return fmt.Errorf("backend selection: %w", err)
	}
	resolvedBackend.Store(resolved)

	fbW, fbH := win.FramebufferSize()
	_, hasSurface := win.(platform.VulkanSurfaceCreator)
	log.Printf("futurerender.initDevice: backend=%s fb=%dx%d hasVulkanSurface=%v",
		resolved, fbW, fbH, hasSurface)
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
		log.Printf("futurerender.initDevice: dev.Init failed: %v", err)
		return err
	}
	log.Printf("futurerender.initDevice: dev.Init ok backend=%s", resolved)

	e.device = dev
	e.encoder = dev.Encoder()

	// If the backend rasterizes on the CPU (soft), it can't present
	// itself — we need to copy its framebuffer into the ANativeWindow
	// each frame. Probe via ReadScreen(nil): backends that need a
	// presenter return true. Vulkan with a swapchain returns false.
	if dev.ReadScreen(nil) {
		aw, ok := win.(*platandroid.Window)
		if !ok {
			return fmt.Errorf("futurerender: soft backend requires android.Window, got %T", win)
		}
		sp, err := platandroid.NewSoftPresenter(aw)
		if err != nil {
			return fmt.Errorf("futurerender: soft presenter: %w", err)
		}
		e.softPresenter = sp
		e.noGL = false
		log.Printf("futurerender.initDevice: soft-backend ANativeWindow presenter installed")
	}

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
