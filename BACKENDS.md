# Backend Implementation Status

Per-backend state of the world for planning future work. Each section covers
what's implemented, what's working, what's broken, and what remains.

Last updated: 2026-03-30

---

## Parity Triage Workflow

This is the repeatable loop for catching a GPU backend up to WebGPU
parity against the `future` demo app. Apply it in order — each step
narrows the search space. Patterns established for Vulkan transfer
directly to Metal, DX12, and anything else that joins the family.

**Tools**: all in the workspace root, not this repo:
- `scripts/capture.sh` — single-backend headless PNG capture
- `scripts/parity-diff.sh` — pairwise pixel-diff with WebGPU reference
- Tracers: `FUTURE_CORE_TRACE_BATCHES`, `FUTURE_CORE_TRACE_PASSES`
  (see "Debugging the Sprite Pass" in [CLAUDE.md](CLAUDE.md))

### 1. Baseline the backend

```sh
scripts/capture.sh --backend <name> --scene scene-selector --frames 3
scripts/capture.sh --backend webgpu --scene scene-selector --frames 3
scripts/parity-diff.sh --scene scene-selector --ref webgpu --test <name>
```

If the PNG is far smaller than the WebGPU reference, the backend is
producing a uniform image (solid color). `parity-diff.sh` flags
"blank capture" before attempting a diff — that's the first signal.

### 2. Confirm the batcher + pass layers match

```sh
scripts/capture.sh --backend <name>  --scene scene-selector --frames 2 \
  --trace-passes 2 --trace-batches 2 --trace-log /tmp/<name>-trace.log
scripts/capture.sh --backend webgpu --scene scene-selector --frames 2 \
  --trace-passes 2 --trace-batches 2 --trace-log /tmp/webgpu-trace.log
diff /tmp/<name>-trace.log /tmp/webgpu-trace.log
```

If `diff` is empty, the pipeline / batcher / sprite-pass layers are
producing identical command streams — the divergence is inside the
backend's encoder or below. If `diff` is non-empty, start there — the
bug is in a higher layer responding to some backend-specific capability
flag.

### 3. Localize: conformance vs demo-app

Run the backend's conformance suite in isolation:

```sh
go test ./internal/backend/<name>/... -v -run TestConformance
```

If conformance passes, the basic render-pass / draw / readback path is
sound. The demo-app bug is specific to the `future` engine's usage
pattern (multi-RT composite, AA buffer lifecycle, atlas texture sampling,
etc.).

### 4. Instrument `ReadScreen`

Add a temporary `fmt.Fprintf` in the backend's `ReadScreen` that prints
first + center pixel of the staging buffer (gated by an env var so
it's zero-cost when unset). This cheaply tells you whether the bug is
in rendering (image contents are wrong) or in readback (image is fine,
readback is broken). For Vulkan it was the former.

### 5. Isolate offscreen-RT content

The sprite pass composites offscreen RTs onto the screen. Add readbacks
on each offscreen RT mid-frame via the device's `Texture.ReadPixels` to
see which target's content is wrong. Common patterns:

- **All white**: composite is sampling without the expected alpha mask
  or blending config is off (blend factors mapped wrong)
- **All black**: clear is landing but draws aren't reaching the attachment
  (pipeline not bound, viewport/scissor wrong, command buffer not
  recording)
- **Wrong colors**: channel swizzle mismatch, format mismatch between
  pipeline and attachment, or swapchain format confusion

### 6. Validation layers (Vulkan-specific, but the principle generalizes)

MoltenVK is permissive. Run the backend under a strict driver with
validation layers enabled — for Vulkan that's `scripts/capture.sh` run
from inside the lavapipe container (future-core `make
docker-vulkan-test`). Every `VUID-*` error is a likely bug MoltenVK
tolerated.

### 7. Document what you found

Update this file — the backend's "Known Issues" table stays honest
about what passes, what fails, and which root causes are identified vs
suspected.

---

## Overview

All seven backends implement `backend.Device` and `backend.CommandEncoder`.
Five backends (Vulkan, Metal, WebGPU, DX12, WebGL) have a **soft-delegation
fallback** for CI. Four of those (Vulkan, Metal, WebGPU, DX12) also have
**real GPU bindings** in `_gpu.go` files, selected by build tags.

| Backend | GPU Rendering | Soft Fallback | Conformance | Shader Pipeline |
|---|---|---|---|---|
| Software | N/A (CPU) | N/A | 10/10 | N/A |
| OpenGL 3.3 | Production | None | N/A (GPU) | GLSL 330 core |
| Vulkan | Clear works, draw broken | Yes | 10/10 (soft) | GLSL→SPIR-V (shaderc) |
| Metal | Clear + draw working | Yes | 10/10 (soft) | GLSL→MSL (pure Go) |
| WebGPU | Pipeline wired | Yes | 10/10 (soft) | GLSL→WGSL (pure Go) |
| DirectX 12 | Early / incomplete | Yes | 10/10 (soft) | Planned (HLSL) |
| WebGL2 | Soft-delegating only | Yes | 10/10 (soft) | GLSL ES 3.00 (stub) |

---

## Software Rasterizer

**Package**: `internal/backend/soft/`
**Status**: Production — reference implementation
**Coverage**: 91%

The CPU-based reference backend. All conformance golden images are generated
from this backend. Pure Go, no dependencies, runs everywhere.

### Implemented
- Half-space triangle rasterization with barycentric interpolation
- Vertex transform (MVP matrix)
- Nearest and linear texture sampling
- 5 blend modes (source-over, additive, multiply, screen, copy)
- Depth testing
- Scissor clipping
- Color matrix transform
- Full Device/Texture/Buffer/Shader/Pipeline/RenderTarget/CommandEncoder

### Limitations
- CPU-only, not suitable for real-time rendering at scale
- No compute shaders, no instancing
- Single-threaded rasterizer

### Roadmap
None — feature-complete for its purpose as a CI reference backend.

---

## OpenGL 3.3

**Package**: `internal/backend/opengl/`
**Status**: Production
**Platform**: Desktop (macOS, Linux, Windows) via GLFW
**Bindings**: `internal/gl/gl.go` — purego dynamic loading of OpenGL

The primary production backend. All rendering features work end-to-end:
sprites, text, custom shaders, render targets, blend modes, stencil.

### Implemented
- Full Device + CommandEncoder with real OpenGL calls
- Texture upload/readback via `glTexImage2D` / `glGetTexImage`
- VAO/VBO/IBO management
- Shader compilation (GLSL 330 core)
- Render targets via FBOs
- Sampler objects for per-draw texture filtering
- Stencil operations for fill rules (EvenOdd)
- VSync via GLFW swap interval

### Limitations
- No compute shaders (OpenGL 3.3 limitation)
- Not available on WASM/mobile
- Requires system GLFW library (loaded via purego on all platforms)

### Roadmap
- Consider OpenGL 4.x path for compute shader support
- Otherwise feature-complete for Phase 1

---

## Vulkan

**Package**: `internal/backend/vulkan/`
**Status**: Unit + conformance green; demo-app parity with WebGPU broken
**Platform**: macOS (MoltenVK), Linux, Windows
**Bindings**: `internal/vk/vk.go` — 91 purego-bound Vulkan functions
**Shader**: `internal/shaderc/shaderc.go` — GLSL→SPIR-V via purego libshaderc
**GPU Tests**: `vulkan_gpu_test.go`, `internal/backend/conformance`

### Implemented (GPU mode)
- Instance creation with extension enumeration (conditional on availability)
- Physical device selection, logical device + graphics queue
- Command pool + command buffer management
- Swapchain with image acquisition/presentation; `VK_ERROR_OUT_OF_DATE_KHR` recovery
- Surface creation: `vkCreateMetalSurfaceEXT` (macOS), `vkCreateWin32SurfaceKHR` (Windows)
- Texture create / upload / readback via staging, with barriers
- Idempotent `Texture.Dispose` with DeviceWaitIdle (fixes cross-frame SIGSEGV)
- Buffer ring-buffer with 1 MB host-visible uniform pool (grown from 16 KB)
- GLSL→SPIR-V compilation + `VkShaderModule`
- Descriptor set layout (sampler + fragment UBO + vertex UBO), pool reset in `resetFrame()`
- Render pass cache with dual Clear/Load variants per render target
- Per-RT `VkFramebuffer` with D24S8 depth-stencil attachment
- Graphics pipeline cache keyed by render pass (pipelines are bound at creation)
- Dynamic viewport / scissor / stencil reference
- Fence-based BeginFrame/EndFrame lifecycle
- Conformance suite (12 scenes) passing on soft-delegation path

### What Works End-to-End
- Full conformance suite (including fill-rule stencil scenes)
- `TestVulkanGPUDrawGreenQuad`, `TestVulkanGPUDrawWithSubmit` — unit path round-trips through ReadScreen correctly
- Single-triangle render + readback: green on black, pixel-level correct

### Demo-app parity status

Measured via `scripts/parity-diff.sh --test vulkan --ref webgpu` on
macOS MoltenVK against every scene in `future/cmd/driver/prepare/
providers.go` (22 scenes total, at `--frames 3` with the 5% diff
threshold the parity runner uses by default).

**Passing (14/22):**

| Scene | Vulkan vs WebGPU |
|---|---|
| `scene-selector` | 0.00% (with ~7-15% stochastic flicker; see below) |
| `input-actions-demo` | 0.01% |
| `pointer-demo` | 0.08% |
| `last-signal` | 0.11% |
| `controls-demo` | 0.17% |
| `keybinding-demo` | 0.22% |
| `console` | 0.42% |
| `particle-garden` | 0.87% |
| `platformer` | 0.97% |
| `cascade` | 1.61% |
| `rttest` (diagnostic) | 1.87% |
| `viewport-platformer` | 2.35% |
| `orb-drop` | 2.63% |
| `chipmunk` | 4.10% |

**Failing (8/22):**

| Scene | Diff | Symptom |
|---|---|---|
| `sprite-demo` | 5.57% | WebGPU side renders blank (Vulkan is correct here) |
| `vector-showcase` | 17.77% | 2 panels empty, 1 panel magenta (same bug class) |
| `responsive-layout` | 30.69% | Layout responds differently on Vulkan |
| `frame-layout` | 43.42% | Investigation needed |
| `bubble-pop` | 64.93% | Game RT renders solid magenta; HUD parity |
| `isometric-combat` | 66.08% | Investigation needed |
| `deep-cartography` | 68.96% | Investigation needed |
| `lighting` | 68.80% | Shader-driven lightmap renders with wrong tint |
| `woodland` | 99.91% | Renders whole-scene vs WebGPU's preview tiles |

**Aggregate:** 14/22 passing (64%) at the 5% diff threshold. Two of
the "failures" (sprite-demo, vector-showcase) have Vulkan rendering
the **correct** content while WebGPU renders blank/different — the
parity runner doesn't distinguish "Vulkan broken" from "WebGPU
broken", it just measures difference.

### Root causes fixed (this series)

1. **`SetTextureFilter` was a no-op** (`internal/backend/vulkan/encoder_gpu.go`).
   The filter parameter was dropped via `_ = filter` and bindUniforms
   always used a hard-coded Nearest sampler. Any caller that asked for
   `FilterLinear` silently got Nearest. Visible effect: the AA buffer
   downsample composite (which explicitly requests Linear for 2x→1x
   blending) was effectively 1:4 subsampled, corrupting any RT that
   used the AA path. `rttest`'s first offscreen RT read as magenta
   instead of red. Fixed by implementing a `samplerFor(filter)` cache
   on Device and recording the requested filter on the Encoder, which
   the descriptor-binding path consults.

2. **Descriptor pool exhausted at 16 sets per frame**
   (`ensureDescriptorPool`). Every `DrawIndexed` allocates one set
   via `bindUniforms`, but the pool was sized for 16 and
   `vk.AllocateDescriptorSet` returned an error that the encoder
   dropped silently — downstream draws kept the last successful
   descriptor set bound and sampled from whatever texture it pointed
   at. On scene-selector (~100 batches/frame) everything past draw 16
   composited from the same stale fallback texture, giving an
   all-white screen. Fixed by bumping the pool to 2048 sets (and
   proportional sampler/UBO counts); the pool is already reset per
   frame in `resetFrame()` so this is a per-frame budget.

### Remaining: scene-selector ~7% stochastic empty-frame flicker

Even with the DeviceWaitIdle synchronization fix, ~7% of scene-selector
captures come back as the bare screen clear + debug HUD (tiles don't
composite). Observations from triage:

- **Scene-specific**: cascade, particle-garden, bubble-pop HUD don't
  flicker. Only scene-selector (which allocates 20+ small offscreen
  RTs for tile previews) does.
- **Stochastic, not deterministic**: same `--frames 3` invocation gives
  different results across runs (4/5 content, 1/5 empty, etc.).
- **Trace-identical**: `FUTURE_CORE_TRACE_BATCHES` / `TRACE_PASSES`
  diff between a "good" capture and an "empty" capture is zero bytes.
  Same 98 batches per frame, same render-pass sequence. The divergence
  is below the encoder trace layer.
- **ReadScreen buffer confirms**: the `defaultColorImage` memory
  literally contains the flickered (empty) state on bad frames.
  Readback is correct; rendering wrote that state.
- **Narrowed NOT caused by**: sprite atlas (NO_ATLAS shows same rate),
  AA path (NO_AA makes it *worse*, 35% rate), frame count (all 1..100
  frames show similar rates).
- **Not a sync-fence gap**: DeviceWaitIdle in BeginFrame + ReadScreen
  reduces from ~20% to ~7%, but further sync (stronger subpass deps)
  doesn't help.

Tried but did NOT fix it:
  - Stronger subpass dependencies (AllCommands / MemoryRead-Write) —
    same rate.
  - Hoisting descriptor-set-layout creation to per-Pipeline from
    per-render-pass — same rate (mildly worse in one sample).
  - Disabling the sprite atlas (NO_ATLAS) — same rate.
  - Disabling AA (NO_AA) — rate *increases* to ~35%.

The remaining flicker is specifically scene-selector (and other
scenes with many small persistent offscreen RTs). Next candidate
angles: VkImage allocation ordering and memory aliasing among the
20+ tile RTs; MoltenVK's descriptor pool reset semantics (whether
reset actually releases descriptors synchronously on its Metal
translation layer).

### Remaining: bubble-pop game RT magenta

bubble-pop's 1024×768 game render target samples as solid
(255, 0, 255, 255). HUD (text, UI panels, controls) renders
correctly — indicating the draw / composite / sampler paths that
scene-selector exercises all work; something specific to bubble-pop's
game RT initialization or first-draw sequence stays broken.

Investigation so far:
- 11 batches target the game RT in frame 1 (trace), with different
  sub-RT textures (physics objects) as sources. So draws ARE issued
  to the target — it's not a "no draws land" issue.
- Magenta is the debug "missing texture" colour on many IHVs; MoltenVK
  initialises undefined memory to 0xFF on some paths, and the game
  RT's first fill might not be landing, leaving the initial contents
  visible through subsequent composites.
- Frame count doesn't matter (magenta persists at f=1..20).

Next step: add a per-RT `ReadPixels` diagnostic right after each
render pass ends to localise whether the game RT's memory is written
correctly but mis-sampled, or whether the writes themselves aren't
landing.

### Roadmap
1. Fix scene-selector residual ~7-15% flicker (candidate: VkImage
   allocation ordering / memory aliasing among many small RTs;
   investigate MoltenVK descriptor-pool reset semantics)
2. Fix bubble-pop game RT magenta (per-pass `ReadPixels` to localise
   whether writes land or sampling is wrong)
3. Fix MoltenVK-tolerated validation errors surfaced by lavapipe CI
   (missing `VkImageMemoryBarrier.sType` fields on some paths,
   missing `VK_BUFFER_USAGE_INDEX_BUFFER_BIT` on index streams —
   see `docker-compose.yml` for expected failures)
4. Resize handling (swapchain recreation; partially working)
5. Multi-frame-in-flight synchronization (currently single-buffered)
6. Device-local memory for vertex/index buffers (currently host-visible)

---

## Metal

**Package**: `internal/backend/metal/`
**Status**: GPU bindings in progress — clear + draw working in tests
**Platform**: macOS, iOS
**Bindings**: `internal/mtl/mtl.go` — 56 purego-bound Metal functions via `objc_msgSend`
**Shader**: `internal/shadertranslate/msl.go` — pure-Go GLSL→MSL translator (12KB)
**GPU Tests**: `metal_gpu_test.go` (ClearAndRead, ShaderCompile, DrawGreenQuad)

### Implemented (GPU mode)
- Metal device creation via `MTLCreateSystemDefaultDevice`
- Command queue and command buffer management
- Texture creation (`MTLTexture`) with upload and readback
- Buffer creation (`MTLBuffer`) with contents pointer access
- Shader compilation: GLSL→MSL (pure Go translator), then `newLibraryWithSource:`
- Render pipeline state (`MTLRenderPipelineState`) creation
- Render command encoder with draw commands
- Depth/stencil state objects
- CAMetalLayer integration for presentation
- Drawable acquisition and present

### Known Issues
- GPU test `TestMetalDrawGreenQuad` passes — basic rendering works
- Full sprite pipeline integration with engine loop not yet validated
- MSL translator covers common GLSL patterns but may miss edge cases

### What Works End-to-End
- Clear and ReadPixels cycle (GPU test passing)
- Shader compilation (GLSL→MSL→MTLLibrary)
- Draw a green quad (full pipeline: shader→pipeline→draw→readback)

### Roadmap
1. Validate full sprite rendering through engine loop (visual test)
2. Run conformance suite against GPU mode
3. Wire CAMetalLayer presentation into engine frame loop
4. Expand MSL translator coverage for all Kage built-in functions
5. Implement render targets (off-screen `MTLTexture` as color attachment)

---

## WebGPU

**Package**: `internal/backend/webgpu/`
**Status**: GPU pipeline wired — needs GPU hardware validation
**Platform**: Cross-platform (desktop via wgpu-native, browser via JS API)
**Bindings**: `internal/wgpu/wgpu.go` — 60 purego-bound functions
**Shader**: `internal/shadertranslate/wgsl.go` — pure-Go GLSL→WGSL translator
**GPU Tests**: None (needs wgpu-native library at runtime)

### Implemented (GPU mode)
- `_gpu.go` files for all types (device, encoder, pipeline, shader,
  texture, buffer, render target)
- wgpu-native bindings: Instance, Adapter, Device, Queue, Surface,
  Swapchain, Texture, TextureView, Buffer, ShaderModule, BindGroup,
  Pipeline, CommandEncoder, RenderPassEncoder
- **Synchronous adapter/device initialization** via `purego.NewCallback`
  (wgpu-native calls callbacks inline)
- **GLSL→WGSL shader translation** (`shadertranslate/wgsl.go`): vertex
  attributes, uniforms, varyings, texture sampling, local variable
  declarations, type constructor mapping
- **Uniform ring buffer**: 16 KB persistent GPU buffer with 256-byte-aligned
  cursor; reset per-frame in `BeginFrame`, advances per-draw. Eliminates
  per-draw buffer creation/destruction
- **Bind group layout caching**: pipeline creates group 0 (uniforms) and
  group 1 (texture + sampler) layouts once; encoder reuses them
- **Depth/stencil pipeline state**: `DepthStencilState` built from
  `PipelineDescriptor` fields when depth testing is enabled
- **Depth attachment**: `BeginRenderPass` wires the render target's depth
  texture view into `RenderPassDepthStencilAttachment`
- **Sampler cache**: device caches samplers by `FilterMode` (nearest/linear);
  `SetTextureFilter` records per-slot filter, used when binding textures
- **Resize handling**: `Resize(w, h)` reconfigures the surface (or recreates
  the offscreen texture); `BeginFrame` detects stale/lost surfaces and
  reconfigures automatically before retry
- **Surface/presentation**: `SurfaceFactory`-driven surface creation,
  `wgpuSurfaceConfigure` for presentation mode (VSync/FIFO), per-frame
  texture acquisition via `SurfaceGetCurrentTexture`, present in `EndFrame`
- **Frame lifecycle**: `BeginFrame` acquires surface texture + resets
  uniform cursor; `EndFrame` presents and releases the surface view
- Texture creation, upload, readback (staging buffer + map)
- Buffer creation, upload, sub-region upload
- Viewport, scissor, draw, drawIndexed
- ReadScreen via texture-to-buffer copy
- **Browser path** (`//go:build js`): full `syscall/js` implementation
  targeting the browser `navigator.gpu` API — `GPUDevice`, `GPUQueue`,
  `GPUCommandEncoder`, `GPURenderPassEncoder`, `GPUCanvasContext` for
  presentation, async adapter/device via Promise callbacks, GLSL→WGSL
  translation shared with native path

### Browser Performance Optimizations (Done)
- **Direct canvas presentation**: when `GPUCanvasContext` is available,
  rendering goes directly to the canvas texture — the CPU readback path
  (`ReadScreen` + `putImageData`) is bypassed entirely
- **Uniform ring buffer**: 64 KB persistent GPUBuffer with 256-byte-aligned
  sub-allocations; `hasDynamicOffset` bind group created once per pipeline
  change, reused across all draws with dynamic offsets
- **Texture bind group cache**: bind groups cached by `(textureID, filter)`;
  `SetTexture` becomes a single `setBindGroup` call on cache hit
- **JS object pooling**: pre-allocated `Uint32Array(1)` for dynamic offsets
  and `Uint8Array(256)` for uniform writes, avoiding per-draw typed-array
  allocation
- **ResizeObserver**: canvas size sync uses a dirty flag from
  `ResizeObserver` instead of querying DOM properties every frame

### Known Issues
- No GPU tests yet — requires `libwgpu_native.{so,dylib,dll}` at runtime
- SetStencil and SetColorWrite are no-ops (state baked into pipeline)
- GLSL→WGSL translator covers core sprite/text patterns and built-in
  math functions; does not support array uniforms or custom functions

### Roadmap
1. Validate clear + readback cycle on GPU hardware (native + browser)
2. Validate full sprite rendering pipeline with visual test
3. Run conformance suite against GPU mode
4. HTML harness example for browser WebGPU
5. Mobile browser profiling (iOS Safari, Android Chrome)
6. Batch color matrices into UBO array (requires shader + vertex format changes)

---

## DirectX 12

**Package**: `internal/backend/dx12/`
**Status**: GPU bindings early — minimal implementation
**Platform**: Windows only
**Bindings**: `internal/d3d12/d3d12.go` — 39 purego-bound functions via COM vtable dispatch
**GPU Tests**: None

### Implemented (GPU mode)
- `_gpu.go` files exist for all types
- COM vtable dispatch for D3D12 interfaces (Factory, Adapter, Device,
  CommandQueue, CommandAllocator, GraphicsCommandList, Resource,
  DescriptorHeap, Fence, PipelineState, RootSignature)
- Basic device initialization with adapter enumeration
- `pipeline_gpu.go` is minimal (~500 bytes) — placeholder only

### Known Issues
- Most GPU methods are skeletal — minimal actual D3D12 API calls
- No shader compilation path (HLSL)
- No GPU tests
- Can only be tested on Windows hardware

### Roadmap
1. Implement DXGI swap chain (`IDXGISwapChain4`)
2. Implement GLSL→HLSL translator (or GLSL→SPIR-V→HLSL via SPIRV-Cross)
3. Complete D3D12 root signature and pipeline state object creation
4. Implement command list recording for draw calls
5. Implement descriptor heap management (CBV/SRV/UAV, samplers)
6. Implement resource management (upload heaps, fence-based lifetime)
7. Add GPU tests
8. Validate clear + readback, then full sprite pipeline

---

## WebGL2

**Package**: `internal/backend/webgl/`
**Status**: Soft-delegating only — no real GPU bindings yet
**Platform**: Browser (WASM) via `syscall/js`
**Coverage**: 92%

### Implemented
- Full soft-delegation wrapper with correct type unwrapping
- API-specific type mapping (GL format constants, buffer targets)
- `ContextAttributes` for canvas creation
- `device_js.go` with `syscall/js` WebGL2 bindings (structure only)
- GLSL 330→GLSL ES 3.00 translator stub

### Roadmap
1. Web platform shim (`internal/platform/web/`) — canvas, rAF, DOM events
2. Wire `syscall/js` WebGL2 context to Device methods
3. Complete GLSL ES 3.00 shader translation
4. Touch and pointer event mapping
5. WASM build + HTML harness example

---

## Shader Pipeline Summary

| Source | Target | Implementation | Status |
|---|---|---|---|
| GLSL 330 | OpenGL 3.3 | Native (no translation) | Production |
| GLSL 330 | SPIR-V | `internal/shaderc/` (purego libshaderc) | Working |
| GLSL 330 | MSL | `internal/shadertranslate/msl.go` (pure Go) | Working |
| GLSL 330 | GLSL ES 3.00 | Stub translator in webgl/ | Planned |
| GLSL 330 | WGSL | `internal/shadertranslate/wgsl.go` (pure Go) | Working |
| GLSL 330 | HLSL | Not implemented | Planned |
| Kage | GLSL 330 | `internal/shaderir/` transpiler | Production |

---

## Priority Order for GPU Backend Completion

Based on current state and effort required:

1. **Vulkan** — Closest to working. Fix the pipeline SIGSEGV and the full
   rendering pipeline should come online. All other pieces (swapchain,
   textures, shaders, command recording) are implemented.

2. **Metal** — GPU tests already pass including draw. Needs engine loop
   integration and visual validation. Likely the first backend to achieve
   full GPU rendering after OpenGL.

3. **WebGPU** — GPU pipeline fully wired (shader translation, uniforms,
   bind groups, depth/stencil, sampler cache). Needs GPU hardware
   validation and surface/swapchain integration for presentation.

4. **DirectX 12** — Most work remaining. Minimal GPU implementation, no
   shader pipeline, Windows-only testing constraint.

5. **WebGL2** — Soft-delegating is sufficient for now. Real implementation
   depends on the web platform shim which is a separate effort.
