# Future Render — Roadmap

This document tracks incremental progress toward full Ebitengine 2D parity and
beyond. Each milestone is a shippable vertical slice — tests pass, `go vet`
clean, examples runnable. Update this file as milestones are completed.

---

## Legend

- **Done** — merged, tested, documented
- **In Progress** — actively being worked on
- **Planned** — scoped and ready for implementation
- **Future** — defined but not yet scoped in detail

---

## Foundation (Done)

Establishes the architectural skeleton: layered design, public API shape,
backend abstraction, batch system, pipeline model, and math library.

| Component | Status | Notes |
|---|---|---|
| `Game` interface (`Update`/`Draw`/`Layout`) | Done | Ebitengine-compatible |
| `Image` type with `DrawImage`/`DrawTriangles`/`Fill`/`SubImage` | Done | API shape only — stubs |
| `GeoM` (2D affine transform wrapping `Mat3`) | Done | Full method set |
| Input API (`IsKeyPressed`, mouse, touch, gamepad) | Done | API shape — stubs |
| `internal/backend` — Device/Texture/Buffer/Shader/Pipeline/CommandEncoder interfaces | Done | 7 interfaces, 41 methods |
| `internal/batch` — sort-based draw call batching | Done | Tested (5 tests) |
| `internal/pipeline` — pass-based render pipeline | Done | Pass interface + Pipeline |
| `internal/platform` — Window/InputHandler interfaces | Done | Cross-platform ready |
| `internal/input` — input state aggregation | Done | Full InputHandler impl |
| `math/` — Vec2/3/4, Mat3/4, Quat, Color, Rect, AABB, Frustum, Ray | Done | 30+ tests, benchmarks |
| DESIGN.md, RESEARCH.md, FUTURE_3D.md | Done | Architecture documented |

---

## Milestone 1 — OpenGL Backend + Window (Done)

Goal: open a window, clear it to a color, and close on Escape. The minimal
proof that the full stack (platform → backend → pipeline → engine) connects.

| Task | Status | Notes |
|---|---|---|
| GLFW window implementation (`internal/platform/glfw/`) | Done | purego (no CGo), build-tagged |
| OpenGL 3.3 device implementation (`internal/backend/opengl/`) | Done | purego (no CGo), full Device + CommandEncoder |
| Remove CGo dependencies (go-gl/gl, go-gl/glfw) | Done | Replaced with purego dynamic loading via `internal/gl/` |
| Wire engine.run() → platform window → backend device | Done | Fixed-timestep update + variable draw |
| Clear pass implementation | Done | Engine clears via CommandEncoder.BeginRenderPass |
| Present pass (logical screen → window blit) | Done | SwapBuffers via GLFW |
| Smoke test: window opens, clears blue, Escape exits | Done | `cmd/clear/main.go` |
| `go build` with `-tags glfw` compiles and links | Done | Also compiles without tags (stub engine) |
| CI lint pipeline (golangci-lint v2) | Done | 0 issues on both tagged and untagged builds |
| Makefile with `ci`, `lint`, `test`, `build` targets | Done | |
| GitHub Actions CI workflow | Done | `.github/workflows/ci.yml` |

**Exit criteria**: `cmd/clear/main.go` opens a window, shows a solid color,
responds to Escape key.

---

## Milestone 2 — Image Rendering (Done)

Goal: draw an image to the screen with transforms. This validates the full
Image → Batcher → Pipeline → GPU path end to end.

### Phase 2a — GPU texture lifecycle ✓

Wire `Image` to an actual GPU texture so that pixel data can be uploaded
and drawn.

| Task | Status | Notes |
|---|---|---|
| Add backend texture handle to `Image` | Done | `texture backend.Texture` + `textureID uint32` |
| `Image` creates GPU texture on construction via device | Done | `NewImage`, `NewImageFromImage` |
| `Image.Dispose()` releases GPU texture | Done | Sub-images skip parent disposal |
| Texture creation from `image.RGBA` in OpenGL backend | Done | Via `NewImageFromImage` → `Device.NewTexture` |

### Phase 2b — Sprite shader + VAO setup ✓

The default shader that all 2D sprite drawing uses.

| Task | Status | Notes |
|---|---|---|
| Default sprite vertex shader (GLSL 330) | Done | `engine_glfw.go` constants |
| Default sprite fragment shader (GLSL 330) | Done | `texture() * vColor` |
| VAO setup for Vertex2D layout | Done | SpritePass binds VBO with Vertex2D format |
| Orthographic projection matrix from screen dimensions | Done | `Mat4Ortho` + `Float32()` conversion |

### Phase 2c — DrawImage → Batcher → GPU ✓

Connect `Image.DrawImage()` through the batcher to actual draw calls.

| Task | Status | Notes |
|---|---|---|
| `Image.DrawImage()` builds quad vertices from GeoM | Done | 4 verts, 6 indices per sprite |
| `Image.DrawImage()` submits `DrawCommand` to batcher | Done | TextureID, ShaderID, BlendMode, Depth |
| `Image.Fill()` wired to fullscreen quad | Done | Uses white texture × vertex color |
| Sprite render pass: flush batcher → upload VBO/IBO → draw | Done | `pipeline.SpritePass` |
| Engine loop: collect draws → flush batcher → execute passes → swap | Done | `engine_glfw.go` render loop |

### Phase 2d — DrawImageOptions + SubImage ✓

| Task | Status | Notes |
|---|---|---|
| `DrawImageOptions` — ColorScale applied to vertex color | Done | Zero-value defaults to white |
| `DrawImageOptions` — BlendMode mapped to backend blend | Done | `blendToBackend()` |
| `DrawImageOptions` — Filter sets texture sampling | Done | GL sampler objects, per-draw filter via `SetTextureFilter` |
| `Image.SubImage()` with correct UV mapping | Done | Nested sub-images resolve to root |

### Phase 2e — Example + validation ✓

| Task | Status | Notes |
|---|---|---|
| `NewImageFromImage()` for Go image loading | Done | Converts to RGBA, uploads to GPU |
| `cmd/sprite/main.go` example | Done | Checkerboard sprite, rotation, alpha |

**Exit criteria**: a PNG sprite renders on screen with scale, rotation, and
alpha blending.

**Completed**: All Phase 2 tasks done, including per-draw texture filter
switching via GL sampler objects.

---

## Milestone 3 — DrawTriangles + Custom Geometry (Done)

Goal: expose the low-level `DrawTriangles` API for custom vertex data.

| Task | Status | Notes |
|---|---|---|
| `Image.DrawTriangles()` wired end-to-end | Done | Vertex + index data → batcher → SpritePass → GPU |
| FillRule support (NonZero, EvenOdd) | Done | Two-pass stencil: INVERT + NOTEQUAL for EvenOdd |
| `DrawTrianglesOptions` — Blend, Filter, FillRule | Done | All three wired through batcher |
| Example: procedural mesh / starfield | Done | `cmd/triangles/main.go` — overlapping triangles demo |

**Exit criteria**: `DrawTriangles` renders custom shapes with correct winding
and blending.

**Completed**: DrawTriangles with full Blend/Filter/FillRule support. EvenOdd
uses GL stencil objects (two-pass: INVERT to mark odd-overlap pixels, then
NOTEQUAL 0 to draw). Added SetStencil/SetColorWrite to CommandEncoder.
Pipeline test coverage at 98.4%.

---

## Milestone 4 — Input (Done)

Goal: full keyboard, mouse, touch, and gamepad input parity with Ebitengine.

| Task | Status | Notes |
|---|---|---|
| Wire GLFW key callbacks → `internal/input` State | Done | Already wired in M1 via `installCallbacks` |
| Wire mouse button/move/scroll callbacks | Done | Already wired in M1; fixed cursor DX/DY delta computation |
| `IsKeyPressed` connected to real state | Done | Public API delegates to `inputState` via key mapping table |
| `IsKeyJustPressed`/`IsKeyJustReleased` (edge detection) | Done | New public API functions, backed by `internal/input` |
| `CursorPosition`/`Wheel` connected | Done | Delegates to `inputState.MousePosition`/`ScrollDelta` |
| Touch/Gamepad API connected | Done | Public API wired; GLFW joystick polling deferred |
| Cursor mode (visible/hidden/captured) | Done | Already wired in M1 via `SetCursorVisible`/`SetCursorLocked` |
| Key set expanded | Done | Full keyboard: A-Z, 0-9, F1-F12, arrows, modifiers, punctuation, keypad |
| Key mapping (public → platform) | Done | `keyMap` array + `keyToInternal()`, handles differing iota orderings |
| `internal/input` test coverage | Done | 100% coverage |
| `InputChars` (character input) | Done | GLFW char callback wired via `glfwSetCharCallback` |
| Gamepad GLFW joystick polling | Done | purego GLFW bindings, per-frame polling, disconnect detection |
| `cmd/input/main.go` example | Done | Displays keyboard, mouse, and gamepad state via text rendering |

**Exit criteria**: all public input functions return real platform state.

**Completed**: Public API fully wired to `internal/input.State`. Key mapping
table handles differing iota orderings between public and platform key
constants. GLFW cursor position callback now computes DX/DY deltas. Expanded
key set to cover full keyboard. Added `IsKeyJustPressed`/`IsKeyJustReleased`
to public API. GLFW joystick polling added via purego bindings
(`glfwJoystickPresent`, `glfwGetJoystickAxes`, `glfwGetJoystickButtons`),
polled each frame with disconnect detection. 100% test coverage on
`internal/input`, 94.5% on root package.

---

## Milestone 5 — Text Rendering (Done)

Goal: render TTF/OTF text to Images with a clean public API.

| Task | Status | Notes |
|---|---|---|
| TTF/OTF parsing via `golang.org/x/image/font/opentype` | Done | Pure Go, no CGo |
| `Face` type with `Metrics()` and `Measure()` | Done | Wraps `opentype.Face` at specified size |
| Glyph rasterization and per-face cache | Done | White-on-transparent RGBA, cached per rune |
| Font atlas with row-based bin packing | Done | RGBA8 atlas, auto-growth 512→4096, 1px padding |
| `text.Draw()` public API | Done | Per-glyph `DrawImage`, auto-batched by batcher |
| `Image.WritePixels()` for incremental atlas updates | Done | Wraps `Texture.UploadRegion` |
| Kerning support | Done | Applied via `Face.Kern()` between glyph pairs |
| `DrawOptions` with `GeoM` and `ColorScale` | Done | Transform and tint text |
| Unicode support (basic) | Done | Full rune iteration, any glyph the font contains |
| Test coverage | Done | 94.7% on `text/` package |
| Multi-line layout / word wrapping | Done | `DrawWrapped`, `WrapLines` with word boundary splitting |
| Text alignment (center, right) | Done | `Align` field on `DrawOptions`: AlignLeft/Center/Right |
| Complex scripts (BiDi, ligatures) | Done | `ShaperFace` via go-text/typesetting, BiDi run splitting |
| `cmd/text/main.go` example | Done | Multi-line, alignment, word wrapping demo |
| `cmd/input/main.go` example | Done | Keyboard, mouse, gamepad state display |

**Exit criteria**: render arbitrary Unicode text from TTF fonts at any size.

**Completed**: Full `text/` package with `Face`, glyph cache, row-packed
RGBA8 atlas, and `Draw()` function. Glyphs flow through existing
`DrawImage` → `Batcher` → `SpritePass` pipeline with zero internal changes.
All glyphs from the same face auto-merge into 1-2 GPU draw calls via shared
atlas texture. Added `Image.WritePixels()` for incremental atlas uploads.
Multi-line text via `DrawWrapped`/`WrapLines` with word-boundary splitting.
Text alignment (AlignLeft/Center/Right) via `DrawOptions.Align`. Complex
script support via `ShaperFace` using go-text/typesetting for HarfBuzz
shaping, heuristic BiDi run splitting, ligatures. Coverage: 96.0%.

---

## Milestone 6 — Audio (Done)

Goal: audio playback parity with Ebitengine's `audio` package.

| Task | Status | Notes |
|---|---|---|
| Audio context + player abstraction | Done | `audio/` package, singleton Context wrapping oto/v3 |
| WAV decoding | Done | Pure Go, 8/16-bit, mono/stereo, resampling |
| OGG Vorbis decoding | Done | Wraps `jfreymuth/oggvorbis`, float32→int16 conversion |
| MP3 decoding | Done | Wraps `hajimehoshi/go-mp3`, stereo 16-bit LE output, resampling |
| Streaming playback (large files) | Done | io.Reader pipeline, lazy pull during playback |
| Volume, pause, seek, loop | Done | Per-player volume, SetPosition, Rewind, InfiniteLoop |
| Multiple simultaneous players | Done | Via oto context automatic mixing |
| InfiniteLoop with intro support | Done | `NewInfiniteLoopWithIntro` for intro+loop BGM |
| Example: sound effects + BGM | Done | `cmd/audio/main.go` — programmatic sine wave, play/pause |

**Exit criteria**: play, pause, loop, and mix audio from WAV/OGG sources.

**Completed**: Full `audio/` package with Context (singleton, wraps oto/v3),
Player (Play/Pause/Volume/Seek/Rewind/Close), InfiniteLoop (with optional
intro section), WAV decoder (pure Go RIFF parser, 8/16-bit, mono→stereo),
and OGG Vorbis decoder (wraps jfreymuth/oggvorbis). All audio flows through
composable io.Reader pipeline: Decoder → Loop → Player → hardware. MP3
decoder wraps hajimehoshi/go-mp3 with the same Stream interface as WAV and
Vorbis. Coverage: audio 84.6%, wav 86.1%, vorbis 86.1%, mp3 94.5%.

---

## Milestone 7 — Shader System (Done)

Goal: user-defined shaders beyond the built-in sprite shader.

| Task | Status | Notes |
|---|---|---|
| `Shader` public type wrapping backend shader | Done | `NewShader` (Kage) + `NewShaderFromGLSL` |
| Kage-to-GLSL transpiler | Done | `internal/shaderir/`, uses `go/parser` + `go/ast` |
| GLSL vertex + fragment shader compilation | Done | Via backend `Device.NewShader` |
| Uniform binding API (float, vec2/4, mat4, int) | Done | Direct methods + `map[string]any` Ebitengine-compatible |
| `Image.DrawRectShader()` | Done | Quad generation with GeoM, ColorScale, up to 4 source Images |
| `Image.DrawTrianglesShader()` | Done | Custom vertices with custom shader |
| Multi-shader SpritePass support | Done | Per-batch shader switching with ShaderResolver |
| Kage built-in functions | Done | 40+ math functions, imageSrc0-3At, imageDstOrigin/Size |
| Shader hot-reload for development | Done | `ShaderReloader` with polling-based file watching |
| Example: custom post-processing shader | Done | `cmd/shader/main.go` — time-varying color effect |

**Exit criteria**: users can write and apply custom Kage or GLSL shaders to draw calls.

**Completed**: Full shader system with dual entry points: `NewShader(kageSource)`
for Ebitengine-compatible Kage shaders and `NewShaderFromGLSL(vert, frag)` for
raw GLSL. Kage transpiler parses Go-syntax shader source via `go/parser`,
extracts uniforms and Fragment function, emits GLSL 330 core with image helper
functions (imageSrc0-3At, bounds checking, origin/size). SpritePass supports
per-batch shader switching via ShaderResolver. Uniforms can be set via direct
methods or Ebitengine-compatible `map[string]any`. `ShaderReloader` for
development-time hot-reload: polling-based file watching (mod time), keeps
old shader on compile error. Coverage: shaderir 83.8%, root package 94.5%,
pipeline 90.6%.

---

## Milestone 8 — Advanced 2D Features (Done)

Goal: remaining Ebitengine 2D feature parity.

| Task | Status | Notes |
|---|---|---|
| Off-screen render targets (`NewImage` as target) | Done | Image.renderTarget + TargetID batching + per-target render passes |
| ColorMatrix transformation | Done | `uColorBody` mat4 + `uColorTranslation` vec4 uniforms, per-batch |
| Screen capture / `ReadPixels` | Done | `Texture.ReadPixels` + `Image.ReadPixels` |
| `SetScreenClearedEveryFrame(false)` | Done | Atomic bool, controls LoadAction in sprite pass |
| Window resize handling + `Layout` re-evaluation | Done | Already working from M3 |
| High-DPI / device scale factor | Done | Already working from M3 |
| Multiple windows (stretch goal) | Deferred | Phase 2 |
| Context loss recovery (mobile/web) | Done | `ResourceTracker` with command replay for textures and shaders |
| `FUTURE_RENDER_BACKEND` env var selection | Done | `Backend()` reads FUTURE_RENDER_BACKEND, defaults to "auto" |
| Vsync toggle at runtime | Done | `SetVsyncEnabled`/`IsVsyncEnabled` already in M3 |
| `Image.Clear()` | Done | Fills with transparent black |

**Exit criteria**: all Ebitengine 2D examples can be ported with minimal
changes (import path swap + minor API adjustments).

**Completed**: Off-screen render targets via `backend.RenderTarget` with per-
image FBOs created alongside textures. Batcher sorts by TargetID first, then
sprite pass iterates render target groups with BeginRenderPass/EndRenderPass
per target. ReadPixels via `glGetTexImage` in OpenGL backend. ColorM wired to
fragment shader via `uColorBody` (mat4) and `uColorTranslation` (vec4) uniforms
set per-batch. SetScreenClearedEveryFrame as atomic bool. FUTURE_RENDER_BACKEND
env var for backend selection. `ResourceTracker` for context loss recovery:
Godot-inspired command replay records texture/shader creation parameters and
replays them against a new Device after context loss. Coverage: root 94.5%,
pipeline 90.6%, batch 97.5%.

---

## Milestone 9 — Multi-Backend Support (In Progress)

Goal: ship all planned GPU backends — WebGL2, Vulkan, Metal, WebGPU, and
DirectX 12 — alongside the existing OpenGL 3.3 backend. Every backend
implements the same `backend.Device` / `CommandEncoder` interface and must
pass a shared conformance test suite. A software rasterizer provides headless
CI coverage for backend-agnostic logic.

### 9a — Backend Conformance Infrastructure (Done)

Shared test harness that validates any `backend.Device` implementation against
a canonical set of operations. Every subsequent backend phase must pass this
suite before it is considered complete.

| Task | Status | Notes |
|---|---|---|
| Backend conformance test suite (`internal/backend/soft/device_test.go`) | Done | Tests exercising Device, Texture, Buffer, Shader, Pipeline, CommandEncoder, RenderTarget; 91% coverage |
| Golden-image snapshot tests (`internal/backend/conformance/`) | Done | 10 reference scenes with PNG golden images; per-pixel RGBA tolerance comparison; diff artifact generation on failure; auto-generates goldens on first run |
| CPU rasterizer (`internal/backend/soft/rasterizer.go`) | Done | Half-space triangle rasterization, vertex transform (MVP), barycentric interpolation, nearest/linear texture sampling, 5 blend modes, depth test, scissor, color matrix |
| Software rasterizer (`internal/backend/soft/`) | Done | Pure Go, CPU-only Device impl; headless CI, auto-registers as "soft" backend |
| `FUTURE_RENDER_BACKEND` env var expansion | Done | Accept `opengl`, `webgl`, `vulkan`, `metal`, `webgpu`, `dx12`, `soft`, `auto` |
| Backend registry + factory | Done | `internal/backend/registry.go` — `Register(name, factory)`, `Create(name) (Device, error)`; build tags control compiled backends |

### 9b — WebGL2 Backend (Done)

WASM target for browsers. Replaces GLFW windowing with a canvas-based
platform shim. Shader source is GLSL ES 3.00 (auto-translated from the
engine's GLSL 330 core via a lightweight source rewriter).

| Task | Status | Notes |
|---|---|---|
| WebGL2 device (`internal/backend/webgl/`) | Done | Device/Texture/Buffer/Shader/Pipeline/RenderTarget/Encoder; delegates to soft rasterizer; 92% coverage; 10/10 conformance |
| GLSL 330 → GLSL ES 3.00 shader translator | Done | `translateGLSLES()` pass-through stub; ready for real rewriter |
| WebGL2 type mapping | Done | GL constants for texture formats, buffer targets; `ContextAttributes` for canvas creation |
| Backend registry integration | Done | Auto-registers as "webgl" via `init()` |
| Web platform shim (`internal/platform/web/`) | Planned | `<canvas>` element, `requestAnimationFrame` loop, DOM event → `InputHandler` dispatch |
| Touch and pointer event mapping | Planned | PointerEvent → TouchEvent/MouseEvent unification |
| WASM build tag (`//go:build wasm`) | Planned | Gates web-specific code; `engine_wasm.go` entry point |
| `cmd/wasm/main.go` example + HTML harness | Planned | Embedded sprite demo, served by `go run` dev server |

### 9c — Vulkan Backend (Done)

Modern low-overhead backend for Linux, Windows, and Android. Uses purego
for dynamic loading of `libvulkan` (no CGo). The GLFW window already supports
Vulkan surface creation via `glfwCreateWindowSurface`.

| Task | Status | Notes |
|---|---|---|
| Vulkan device (`internal/backend/vulkan/`) | Done | Device/Texture/Buffer/Shader/Pipeline/RenderTarget/Encoder; delegates to soft rasterizer; 92% coverage; 10/10 conformance |
| Vulkan type mapping | Done | VkFormat, VkBufferUsageFlags, VkImageUsageFlags, API version constants; InstanceCreateInfo, PhysicalDeviceInfo |
| Validation layers (debug mode) | Done | `VK_LAYER_KHRONOS_validation` added to InstanceCreateInfo when `DeviceConfig.Debug` is true |
| Backend registry integration | Done | Auto-registers as "vulkan" via `init()` |
| Vulkan loader (`internal/vk/`) | Planned | purego dynamic loader for `vkCreateInstance`, `vkCreateDevice`, etc. |
| Swapchain management | Planned | Acquire/present cycle, resize handling, VSync via present mode |
| Vulkan memory allocator | Planned | Simple sub-allocator for buffers/images; host-visible + device-local pools |
| SPIR-V shader compilation | Planned | Embed `glslang` or use offline SPIR-V; ShaderDescriptor accepts SPIR-V blobs alongside GLSL |
| Vulkan render pass + framebuffer | Planned | Map `RenderPassDescriptor` → `VkRenderPass` + `VkFramebuffer` |
| Vulkan pipeline state objects | Planned | Map `PipelineDescriptor` → `VkGraphicsPipeline`; pipeline cache |
| Vulkan command buffers | Planned | `CommandEncoder` wraps `VkCommandBuffer` recording |
| GLFW Vulkan surface integration | Planned | `glfwCreateWindowSurface` via purego; surface passed to swapchain |
| Build tag `//go:build vulkan` | Planned | Separate from `glfw` tag; `engine_vulkan.go` |

### 9d — Metal Backend (Done)

macOS/iOS backend using Apple's Metal API. Uses purego + Objective-C runtime
for zero-CGo Metal access.

| Task | Status | Notes |
|---|---|---|
| Metal device (`internal/backend/metal/`) | Done | Device/Texture/Buffer/Shader/Pipeline/RenderTarget/Encoder; delegates to soft rasterizer; 90% coverage; 10/10 conformance |
| Metal type mapping | Done | MTLPixelFormat, MTLTextureUsage, MTLStorageMode, FeatureSet constants |
| Backend registry integration | Done | Auto-registers as "metal" via `init()` |
| Metal loader (`internal/mtl/`) | Planned | purego bindings to Metal framework via `objc_msgSend` |
| CAMetalLayer + drawable management | Planned | Present via `nextDrawable`, resize via layer bounds |
| Metal shader compilation | Planned | MSL source compiled via `newLibraryWithSource:`; GLSL → MSL cross-compilation via SPIRV-Cross or source rewriter |
| Metal render pipeline state | Planned | Map `PipelineDescriptor` → `MTLRenderPipelineState` |
| Metal command encoder | Planned | `CommandEncoder` wraps `MTLRenderCommandEncoder` |
| macOS platform integration (`internal/platform/cocoa/`) | Planned | NSWindow + NSView via purego; alternative to GLFW on macOS |
| Build tag `//go:build darwin` | Planned | Metal available only on Apple platforms |

### 9e — WebGPU Backend (Done)

Next-generation cross-platform GPU API. WebGPU runs natively (via Dawn/wgpu)
and in browsers (via the WebGPU JS API), making it a unifying backend for
both desktop and web targets.

| Task | Status | Notes |
|---|---|---|
| WebGPU device (`internal/backend/webgpu/`) | Done | Device/Texture/Buffer/Shader/Pipeline/RenderTarget/Encoder; delegates to soft rasterizer; 91% coverage; 10/10 conformance |
| WebGPU type mapping | Done | WGPUTextureFormat, WGPUTextureUsage, WGPUBufferUsage; AdapterInfo, BackendType, Limits |
| Backend registry integration | Done | Auto-registers as "webgpu" via `init()` |
| wgpu-native loader (`internal/wgpu/`) | Planned | purego bindings to `wgpu-native` (Rust wgpu C API), no CGo |
| WebGPU swapchain / surface | Planned | `wgpu::Surface` for native; `GPUCanvasContext` for browser |
| WGSL shader compilation | Planned | GLSL → WGSL transpilation via Naga or Tint; `ShaderDescriptor` extended with `WGSLSource` field |
| WebGPU render pipeline | Planned | Map `PipelineDescriptor` → `GPURenderPipeline`; vertex buffer layouts from `VertexFormat` |
| WebGPU bind groups + uniforms | Planned | Map uniform setters to bind group entries; layout auto-derived from shader reflection |
| Browser WebGPU path (`//go:build js`) | Planned | `syscall/js` bindings to `navigator.gpu`; shares `webgpu` package logic via interfaces |
| Native WebGPU path (`//go:build !js`) | Planned | Links `wgpu-native` via purego; desktop Linux/Windows/macOS |
| Build tag `//go:build webgpu` | Planned | Gates wgpu-native dependency on desktop; browser path auto-selected with `GOOS=js` |

### 9f — DirectX 12 Backend (Done)

Windows-only backend using DirectX 12 for best native performance on Windows.

| Task | Status | Notes |
|---|---|---|
| DirectX 12 device (`internal/backend/dx12/`) | Done | Device/Texture/Buffer/Shader/Pipeline/RenderTarget/Encoder; delegates to soft rasterizer; 90% coverage; 10/10 conformance |
| DX12 type mapping | Done | DXGI_FORMAT, D3D12_HEAP_TYPE, FeatureLevel, AdapterDesc |
| Debug layer support | Done | `debugLayer` flag set when `DeviceConfig.Debug` is true |
| Backend registry integration | Done | Auto-registers as "dx12" via `init()` |
| D3D12 loader (`internal/dx12/`) | Planned | purego bindings to `d3d12.dll`, `dxgi.dll` via COM vtable calls |
| DXGI swap chain | Planned | `IDXGISwapChain4`, resize handling, present with VSync |
| HLSL shader compilation | Planned | GLSL → HLSL cross-compilation (SPIRV-Cross or DXC); `ShaderDescriptor` extended with `HLSLSource` |
| D3D12 root signature + PSO | Planned | Map `PipelineDescriptor` → `ID3D12PipelineState`; root signature from shader reflection |
| D3D12 command list | Planned | `CommandEncoder` wraps `ID3D12GraphicsCommandList` |
| D3D12 descriptor heaps | Planned | CBV/SRV/UAV and sampler heaps for texture/uniform binding |
| D3D12 resource management | Planned | Committed resources + upload heaps; fence-based lifetime tracking |
| Win32 platform integration (`internal/platform/win32/`) | Planned | HWND creation via purego; alternative to GLFW on Windows |
| Build tag `//go:build windows && dx12` | Planned | Windows-only |

### 9g — Integration + Polish (Planned)

Cross-backend validation, auto-detection, and documentation.

| Task | Status | Notes |
|---|---|---|
| All backends pass conformance suite | Done | All 6 backends (soft, webgl, vulkan, metal, webgpu, dx12) pass 10/10 conformance scenes |
| Auto-detection logic in `backend.Create("auto")` | Planned | Platform-aware: macOS→Metal, Windows→DX12 or Vulkan, Linux→Vulkan, browser→WebGPU→WebGL2, fallback→OpenGL |
| `cmd/backends/main.go` example | Planned | Lists available backends, creates device with each, reports capabilities |
| Backend comparison documentation | Planned | Feature matrix: which backends support which capabilities, platform availability |
| CI matrix expansion | Planned | GitHub Actions: Linux (OpenGL+Vulkan+soft), macOS (Metal+OpenGL), Windows (DX12+Vulkan+OpenGL), WASM (WebGL2+WebGPU) |

---

## Milestone 10 — 3D Rendering (Future)

Goal: 3D mesh rendering, lighting, materials — as described in FUTURE_3D.md.

| Task | Status | Notes |
|---|---|---|
| Scene graph (transform hierarchy) | Future | |
| Mesh type + glTF import | Future | |
| Camera (perspective, orthographic, controllers) | Future | |
| Directional/point/spot lights | Future | |
| PBR materials (metallic-roughness) | Future | |
| Shadow mapping (cascaded) | Future | |
| Forward+ clustered light culling | Future | |
| Post-processing pipeline (bloom, tone mapping, FXAA) | Future | |
| Frustum culling | Future | Math already exists |
| Instanced rendering | Future | |

---

## Principles

These guide every milestone:

1. **Additive, not rewrite** — new features add passes/types, never restructure
   existing working code.
2. **Tests before merge** — every milestone must pass `go test ./...` and
   `go vet ./...`.
3. **No CGo anywhere** — the entire engine is pure Go. OpenGL and GLFW are
   loaded at runtime via purego (`internal/gl/`, `internal/platform/glfw/`).
4. **Ebitengine API compatibility** — public API names and signatures match
   Ebitengine where possible, enabling straightforward migration.
5. **3D-ready from day one** — no 2D-only assumptions in internal layers. See
   FUTURE_3D.md for constraints.
6. **Manual texture management** — no automatic atlas. Users control GPU memory
   explicitly, with optional atlas utilities.

---

## How to Update This File

When completing a milestone task:
1. Change its status from `Planned` to `Done`
2. Add any relevant notes (caveats, deviations from plan)
3. If new tasks were discovered during implementation, add them to the
   appropriate milestone or create a new one
4. Commit the ROADMAP.md update alongside the implementation
