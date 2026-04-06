//go:build js

package futurerender

import (
	"errors"
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
	preferred := []string{"webgpu", "soft"}
	dev, _, err := backend.Resolve(backendName(), preferred)
	if err != nil {
		return err
	}

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
		return e.textures[texID]
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
		return e.renderTargets[targetID]
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
func (e *engine) frame() {
	if e.loopErr != nil {
		return
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
	if tps > 0 {
		e.accumulator += delta
		for e.accumulator >= tickDuration {
			e.inputState.Update()
			if err := e.game.Update(); err != nil {
				if errors.Is(err, ErrTermination) {
					e.loopErr = err
					return
				}
				e.loopErr = err
				return
			}
			e.tickCount++
			e.accumulator -= tickDuration
		}
	} else {
		e.inputState.Update()
		if err := e.game.Update(); err != nil {
			e.loopErr = err
			return
		}
		e.tickCount++
	}

	// Draw. Use actual window size (which may differ from the requested
	// size if the browser resized the canvas) so Layout receives the
	// same dimensions as Ebitengine would provide.
	winW, winH := e.window.Size()
	e.fbW, e.fbH = e.window.FramebufferSize()
	screenW, screenH := e.game.Layout(winW, winH)

	screen := &Image{
		width: screenW, height: screenH,
		u0: 0, v0: 0, u1: 1, v1: 1,
	}
	e.game.Draw(screen)

	proj := fmath.Mat4Ortho(0, float64(screenW), float64(screenH), 0, -1, 1)
	e.spritePass.Projection = proj.Float32()

	e.device.BeginFrame()

	ctx := pipeline.NewPassContext(e.fbW, e.fbH)
	ctx.ScreenClearEnabled = IsScreenClearedEveryFrame()
	e.renderPipeline.Execute(e.encoder, ctx)

	e.device.EndFrame()

	// Present rendered pixels to the visible canvas via 2D ImageData.
	// This is the browser equivalent of the desktop GL presenter —
	// ReadScreen copies the soft/WebGPU offscreen buffer to CPU, then
	// putImageData blits it to the canvas.
	e.presentToCanvas()

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
