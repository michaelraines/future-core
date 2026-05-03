//go:build js

package futurerender

import (
	"errors"
	"fmt"
	goimage "image"
	"os"
	"syscall/js"
	"time"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
	"github.com/michaelraines/future-core/internal/input"
	"github.com/michaelraines/future-core/internal/pipeline"
	"github.com/michaelraines/future-core/internal/platform"
	fmath "github.com/michaelraines/future-core/math"

	// Register backends available in the browser.
	_ "github.com/michaelraines/future-core/internal/backend/soft"
	_ "github.com/michaelraines/future-core/internal/backend/webgl"
	_ "github.com/michaelraines/future-core/internal/backend/webgpu"
)

const (
	maxBatchVertices = 65536
	maxBatchIndices  = 65536 * 6
)

// Default sprite shader source (GLSL 330 core — translated to WGSL at runtime).
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
    // Standard premultiplied-alpha sprite shader: sample texture,
    // modulate by vertex color, apply color matrix. No rgb-vs-alpha
    // clamp — callers are responsible for supplying correctly
    // premultiplied vertex colors (ColorScale.ScaleAlpha now scales
    // all four channels, matching Ebitengine; the vector package
    // either passes through premultiplied vertex colors or lets
    // DrawTriangles premultiply straight ones). An earlier revision
    // did rgb=min(rgb,a) here to compensate for a broken
    // ScaleAlpha, which silently clamped any channel where straight
    // RGB exceeded scaled A — turning bright stroke colors into
    // dim-gray garbage. Removed once ScaleAlpha was fixed at the
    // source.
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

	// Registries.
	textures      map[uint32]backend.Texture
	shaders       map[uint32]*Shader
	renderTargets map[uint32]backend.RenderTarget

	// Window config state.
	windowTitle string
	windowW     int
	windowH     int
	fbW, fbH    int // physical framebuffer dimensions

	// Canvas presentation (2D readback path).
	pixelBuf []byte
	ctx2d    js.Value

	// Frame timing.
	lastTime    time.Time
	accumulator time.Duration
	frameCount  int
	tickCount   int
	fpsTimer    time.Time

	// Error from the game loop (stored for reporting).
	loopErr error
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

func (e *engine) registerTexture(id uint32, tex backend.Texture) {
	e.textures[id] = tex
}

func (e *engine) run() error {
	// Allow ?backend=<name> in the URL to override FUTURE_CORE_BACKEND
	// for the parity harness. Browser test runners can set this without
	// rebuilding the WASM, so the same artefact serves WebGPU and WebGL
	// captures from one server. Falls through to the env var (and
	// ultimately "auto") when the param is missing or empty.
	if loc := js.Global().Get("location"); !loc.IsUndefined() && !loc.IsNull() {
		if search := loc.Get("search"); !search.IsUndefined() && !search.IsNull() {
			params := js.Global().Get("URLSearchParams").New(search)
			b := params.Call("get", "backend")
			if !b.IsNull() && !b.IsUndefined() && b.String() != "" {
				_ = os.Setenv("FUTURE_CORE_BACKEND", b.String())
			}
		}
	}

	// Create platform window.
	win := newPlatformWindow()

	winCfg := platform.DefaultWindowConfig()
	if e.windowTitle != "" {
		winCfg.Title = e.windowTitle
	}
	if e.windowW > 0 {
		winCfg.Width = e.windowW
	}
	if e.windowH > 0 {
		winCfg.Height = e.windowH
	}
	if err := win.Create(winCfg); err != nil {
		return err
	}
	e.window = win

	// Resolve and create backend.
	//
	// Default chain: WebGPU first (richest feature set, native dynamic
	// uniforms), then WebGL2 (broadly available — no separate flag, no
	// ORIGIN_TRIAL needed, runs on Safari and older Chrome/Firefox), and
	// the soft rasterizer as the last-resort fallback. Override via
	// FUTURE_CORE_BACKEND=webgl (or =webgpu) to pin a specific backend
	// for parity comparison.
	preferred := []string{"webgpu", "webgl", "soft"}
	dev, resolvedName, err := backend.Resolve(backendName(), preferred)
	if err != nil {
		return err
	}
	// Stash the resolved backend so callers of Backend() / BackendName()
	// see the concrete name (e.g. "webgpu") instead of the literal user
	// request "auto" — the debug overlay's [futurecore/<backend>] tag
	// otherwise reads "[futurecore/auto]" on browser builds.
	resolvedBackend.Store(resolvedName)

	e.fbW, e.fbH = win.FramebufferSize()
	if err := dev.Init(backend.DeviceConfig{
		Width: e.fbW, Height: e.fbH, VSync: true,
	}); err != nil {
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

	// Set up input.
	inputState := input.New()
	win.SetInputHandler(inputState)
	e.inputState = inputState

	// Initialize timing.
	e.lastTime = time.Now()
	e.fpsTimer = time.Now()

	// Start the requestAnimationFrame loop.
	e.startRAFLoop()

	// Block forever — the browser event loop drives everything.
	//
	// Note: synchronous waits on JS Promises from inside a RAF callback
	// (e.g. awaiting GPUBuffer.mapAsync) deadlock Go's runtime. The JS
	// event loop can't tick until the callback returns, so the promise
	// never resolves, so the waiter never wakes, and Go declares
	// "all goroutines are asleep - deadlock!". Changing this `select {}`
	// to a channel receive or ticker-based wait doesn't help — the
	// runtime's deadlock detector fires either way. GPU→CPU readback on
	// WebGPU browser must be done without awaiting the map promise (see
	// Image.cpuPixels cache for the sync fast path; AtlasPacker composites
	// directly on GPU to avoid the roundtrip entirely).
	select {}
}

// initRenderResources creates the default shader, pipeline, and white texture.
func (e *engine) initRenderResources() error {
	dev := e.device

	// 1x1 white texture for untextured draws.
	tex, err := dev.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1,
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

	sp.ResolveTexture = func(texID uint32) backend.Texture {
		if tex, ok := e.textures[texID]; ok {
			return tex
		}
		e.rend.warnStaleIDOnce(texID, "texture")
		return nil
	}
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

// startRAFLoop registers a requestAnimationFrame callback that drives the game loop.
func (e *engine) startRAFLoop() {
	var rafFn js.Func
	rafFn = js.FuncOf(func(_ js.Value, _ []js.Value) interface{} {
		e.frame()
		js.Global().Call("requestAnimationFrame", rafFn)
		return nil
	})
	js.Global().Call("requestAnimationFrame", rafFn)
}

// frame executes one frame of the game loop.
// frameTiming controls per-phase timing output to the JS console via
// console.time/console.timeEnd. Enabled by FUTURE_CORE_FRAME_TIMING=1.
// Zero-cost when disabled.
var frameTiming = os.Getenv("FUTURE_CORE_FRAME_TIMING") != ""

func (e *engine) frame() {
	if e.loopErr != nil {
		return
	}

	var frameStart time.Time
	if frameTiming {
		frameStart = time.Now()
	}

	now := time.Now()
	delta := now.Sub(e.lastTime)
	e.lastTime = now

	tps := MaxTPS()
	tickDuration := time.Duration(0)
	if tps > 0 {
		tickDuration = time.Second / time.Duration(tps)
	}

	// Fixed-timestep update.
	//
	// Async JS events accumulate into e.inputState between rAF
	// callbacks (scrollDX, chars, key-down edges). BeginTick must
	// happen BEFORE game.Update so per-key duration counters are
	// bumped and the game reads a non-zero duration on the first
	// tick of a new press. EndTick must happen AFTER game.Update so
	// scroll / character / mouse-delta accumulators survive into the
	// game's read — otherwise clearing them at tick start zeros the
	// events the game is trying to consume.
	if tps > 0 {
		e.accumulator += delta
		for e.accumulator >= tickDuration {
			e.inputState.BeginTick()
			if err := e.game.Update(); err != nil {
				if errors.Is(err, ErrTermination) {
					e.loopErr = err
					return
				}
				e.loopErr = err
				return
			}
			e.inputState.EndTick()
			e.tickCount++
			e.accumulator -= tickDuration
		}
	} else {
		e.inputState.BeginTick()
		if err := e.game.Update(); err != nil {
			e.loopErr = err
			return
		}
		e.inputState.EndTick()
		e.tickCount++
	}

	// Sync the canvas pixel buffer to the current CSS size × DPI.
	// This handles browser resizes and ensures the framebuffer matches
	// the displayed canvas, just as Ebitengine does each frame.
	type canvasSyncer interface{ SyncCanvasSize() }
	if cs, ok := e.window.(canvasSyncer); ok {
		cs.SyncCanvasSize()
	}

	// Draw. Use actual canvas CSS size so Layout receives the same
	// dimensions as Ebitengine would provide.
	winW, winH := e.window.Size()
	e.fbW, e.fbH = e.window.FramebufferSize()
	screenW, screenH := e.game.Layout(winW, winH)

	// Resize the backend's internal screen render target if needed.
	if resizer, ok := e.device.(interface{ ResizeScreen(int, int) }); ok {
		resizer.ResizeScreen(e.fbW, e.fbH)
	}

	screen := &Image{
		width: screenW, height: screenH,
		u0: 0, v0: 0, u1: 1, v1: 1,
		bounds: goimage.Rect(0, 0, screenW, screenH),
	}

	var drawStart time.Time
	if frameTiming {
		drawStart = time.Now()
	}
	e.game.Draw(screen)
	var drawDur time.Duration
	if frameTiming {
		drawDur = time.Since(drawStart)
	}

	proj := fmath.Mat4Ortho(0, float64(screenW), float64(screenH), 0, -1, 1)
	e.spritePass.Projection = proj.Float32()

	e.device.BeginFrame()

	var execStart time.Time
	if frameTiming {
		execStart = time.Now()
	}
	ctx := pipeline.NewPassContext(e.fbW, e.fbH)
	ctx.ScreenClearEnabled = IsScreenClearedEveryFrame()
	ctx.ScreenHasStencil = e.device.Capabilities().SupportsStencil
	e.renderPipeline.Execute(e.encoder, ctx)
	var execDur time.Duration
	if frameTiming {
		execDur = time.Since(execStart)
	}

	e.device.EndFrame()

	// Release any deferred AA buffers now that the sprite pass has
	// consumed all references to them.
	if e.rend != nil {
		e.rend.disposeDeferred()
	}

	// When the device renders directly to a GPUCanvasContext the browser
	// composites automatically on queue.submit — no CPU readback needed.
	// Fall back to the ReadScreen+putImageData path for backends that
	// render to an offscreen buffer (e.g. the soft rasterizer).
	type canvasPresenter interface{ PresentsToCanvas() bool }
	if cp, ok := e.device.(canvasPresenter); !ok || !cp.PresentsToCanvas() {
		e.presentToCanvas()
	}

	// Per-frame timing summary (env: FUTURE_CORE_FRAME_TIMING=1).
	// Logged once per second to avoid flooding the console.
	if frameTiming && e.frameCount == 0 {
		frameDur := time.Since(frameStart)
		fmt.Fprintf(os.Stderr,
			"[frame-timing] draw=%s execute=%s total=%s\n",
			drawDur.Round(time.Microsecond),
			execDur.Round(time.Microsecond),
			frameDur.Round(time.Microsecond))
	}

	// FPS tracking.
	e.frameCount++
	if time.Since(e.fpsTimer) >= time.Second {
		e.fpsValue = float64(e.frameCount)
		e.tpsValue = float64(e.tickCount)
		e.frameCount = 0
		e.tickCount = 0
		e.fpsTimer = time.Now()
	}
}

// presentToCanvas copies rendered pixels to the visible canvas via ReadScreen + putImageData.
func (e *engine) presentToCanvas() {
	w, h := e.fbW, e.fbH
	size := w * h * 4
	if len(e.pixelBuf) != size {
		e.pixelBuf = make([]byte, size)
	}

	if !e.device.ReadScreen(e.pixelBuf) {
		return
	}

	// Lazy-init 2D context for the canvas.
	if e.ctx2d.IsUndefined() || e.ctx2d.IsNull() {
		canvas := js.Global().Get("document").Call("getElementById", "game-canvas")
		if canvas.IsNull() || canvas.IsUndefined() {
			return
		}
		e.ctx2d = canvas.Call("getContext", "2d")
		if e.ctx2d.IsNull() || e.ctx2d.IsUndefined() {
			return
		}
	}

	imgData := e.ctx2d.Call("createImageData", w, h)
	arr := js.Global().Get("Uint8ClampedArray").New(size)
	js.CopyBytesToJS(arr, e.pixelBuf)
	imgData.Get("data").Call("set", arr)
	e.ctx2d.Call("putImageData", imgData, 0, 0)
}

func (e *engine) setWindowSize(w, h int) {
	e.windowW = w
	e.windowH = h
}

func (e *engine) setWindowTitle(title string) {
	e.windowTitle = title
	js.Global().Get("document").Set("title", title)
}

func (e *engine) setFullscreen(fs bool)      {}
func (e *engine) isFullscreen() bool         { return false }
func (e *engine) setVSync(_ bool)            {}
func (e *engine) isVSync() bool              { return true }
func (e *engine) currentFPS() float64        { return e.fpsValue }
func (e *engine) currentTPS() float64        { return e.tpsValue }
func (e *engine) setCursorMode(_ CursorMode) {}
func (e *engine) deviceScaleFactor() float64 {
	dpr := js.Global().Get("devicePixelRatio").Float()
	if dpr < 1 {
		return 1
	}
	return dpr
}
func (e *engine) setWindowResizable(_ bool) {}
