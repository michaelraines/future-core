# Backend Implementation Status

Per-backend state of the world for planning future work. Each section covers
what's implemented, what's working, what's broken, and what remains.

Last updated: 2026-04-26

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
validation layers enabled. Two entry points:

- **Unit + conformance tests** under lavapipe + Khronos validation:
  `cd future-core && make docker-vulkan-test`. Fast PR-style gate
  for the parts of the backend that don't trip the open
  draw-path bugs.
- **Headless demo capture** under lavapipe:
  `future-meta/scripts/capture-lavapipe.sh --scene <name>` —
  builds the future binary inside the container against the
  workspace future-core (so uncommitted engine changes are
  exercised) and runs the binary headlessly under xvfb +
  lavapipe. Any `VUID-*` validation error or SIGSEGV from a
  strict-but-not-permissive driver path is a real bug
  MoltenVK silently tolerates.

The capture wrapper integrates with the parity harness too —
`scripts/parity-diff.sh --test lavapipe --scene X` will run the
`X` scene on lavapipe in Docker and pixel-diff against the
WebGPU host capture. Today the demo-app code crashes on lavapipe
on a missing graphics-pipeline-bind before `vkCmdDrawIndexed`
(VUID-vkCmdDrawIndexed-None-02700 — see open issues in the
Vulkan backend section); fixing that unlocks the full
strict-validation parity sweep.

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
| Vulkan | Production (20/22 demo scenes parity) | Yes | 10/10 (soft) | GLSL→SPIR-V (shaderc) |
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
**Status**: Unit + conformance green; demo-app parity at 20/22 scenes (1 real Vulkan bug, 1 game-state nondeterminism)
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

Last re-measured 2026-04-26 via `scripts/parity-diff.sh --test vulkan
--ref webgpu` (workspace tooling) on macOS MoltenVK against every
scene in `future/cmd/driver/prepare/providers.go`, at the scene's
configured frame count (60 default, 120 for scenes with WebGPU
device-init pacing) with the 5% diff threshold.

**Passing by parity-diff (20/22):**

| Scene | Diff | Notes |
|---|---|---|
| `scene-selector` | 0.01% | Flicker is gone in single-capture; long-soak re-test still pending |
| `bubble-pop` | 0.01% | Was 64.93% then 0.75% — RT init-clear (commit 359f00b) fully resolved the magenta game RT |
| `frame-layout` | 0.01% | Was 22.21% — sub-frame composite now lands |
| `deep-cartography` | 0.01% (f=120) | Was 57.69% at f=60 — purely a frame-count issue; WebGPU device-init stall hadn't completed |
| `cascade` | 0.01% | |
| `vector-showcase` | 0.33% | |
| `particle-garden` | 0.14% | |
| `orb-drop` | 0.01% | |
| `sprite-demo` | 3.98% | |
| `chipmunk` | 0.31% | |
| `platformer` | 0.04% | |
| `responsive-layout` | 0.01% | |
| `woodland` | 0.10% | |
| `input-actions-demo` | 0.01% | (legacy measurement) |
| `pointer-demo` | 0.08% | (legacy measurement) |
| `last-signal` | 0.11% | (legacy measurement) |
| `controls-demo` | 0.17% | (legacy measurement) |
| `keybinding-demo` | 0.22% | (legacy measurement) |
| `console` | 0.42% | (legacy measurement) |
| `rttest` (diagnostic) | 1.87% | (legacy measurement) |
| `viewport-platformer` | 2.35% | (legacy measurement) |

**Diff > 5% (2/22):**

| Scene | Diff | Actual status |
|---|---|---|
| `lighting` | 5.37% | Just over threshold. Suspected MoltenVK `[[position]]` interpolation bug on Kage-custom-shader light-quad pipelines after a `DrawImage→custom-shader` transition; rewrite of `vDstPos`→`gl_FragCoord` works for full-screen Kage passes but not for the per-light point/spot quads. See inline notes in `internal/backend/vulkan/shader_rewrite.go`. Disambiguating MoltenVK-vs-spec requires running the demo binary under lavapipe — current `docker-compose.yml` only runs unit/conformance tests. |
| `isometric-combat` | 52.15% (f=120) | **Game-state divergence, not pixel divergence.** `FUTURE_CORE_TRACE_BATCHES`/`TRACE_PASSES` diff between WebGPU and Vulkan is *empty* in the early frames — engine produces identical command streams — but by frame 120 the two backends show different player positions, different terrain decoration tile sets, and different procgen layouts. Root cause: each backend's run uses non-deterministic `math/rand` seeding in the demo's procgen + entity-spawn paths, so steady-state game state diverges even when rendering is byte-identical. Tracked in workspace as the `future/rand` deterministic-seeding effort. |

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

### Resolved during this series

- **scene-selector ~7% stochastic empty-frame flicker** — single-capture
  re-tests on 2026-04-26 show 0.01% diff with no flicker. A long-soak
  N=30 re-test is still warranted before declaring fully closed, but the
  bug as previously characterised is no longer reproducible.
- **bubble-pop game RT magenta** — fixed by RT init-clear (commit
  359f00b: zero-init RT VkImage at creation via one-shot render pass).
  Now passes at 0.01%.
- **frame-layout missing sub-frame panels** — passes at 0.01%. The
  nested-RT composite path now works.

### Demo-app lavapipe path: first finding (2026-04-26)

`docker-compose.yml` now has a `vulkan-parity` service that builds
and runs the future demo binary against lavapipe + validation layers.
Companion host wrappers: `future-meta/scripts/capture-lavapipe.sh`
and `parity-diff.sh --test lavapipe`.

First smoke-test capture (`scene-selector`, 30 frames) reproduces a
real Vulkan API misuse the encoder is doing, that MoltenVK
silently tolerates:

```
VUID-vkCmdDrawIndexed-None-02700: Must not call vkCmdDrawIndexed
on this command buffer while there is no
VK_PIPELINE_BIND_POINT_GRAPHICS pipeline bound.
```

Stack trace: `internal/vk.CmdDrawIndexed → encoder_gpu.go:430
(Encoder.DrawIndexed) → SpritePass.Execute → engine.run → main`.
The crash reproduces with `VK_INSTANCE_LAYERS` unset too, so
this isn't a validation-only signal — lavapipe itself rejects
the call sequence. MoltenVK tolerates it (presumably by binding
some default state or silently no-op'ing the draw).

This is the load-bearing first finding the lavapipe integration
exists to surface. Fixing it unblocks running the full demo-app
parity sweep against lavapipe.

### Roadmap
1. Fix the `vkCmdDrawIndexed` without graphics pipeline bound
   bug — root cause likely in the Encoder's pipeline-bind path
   or a code path that skips `SetPipeline` before issuing the
   first draw of a render pass. Lavapipe smoke test will go
   green once this lands.
2. Fix `lighting` 5.37% — either dig into MoltenVK `[[position]]`
   interpolation behaviour for Kage-custom-shader pipelines after
   `DrawImage→custom-shader` transitions, or run the demo binary
   under lavapipe + validation layers (now possible — see #1)
   to disambiguate spec-vs-MoltenVK
3. `isometric-combat` game-state divergence — RESOLVED by the
   `future/rand` + headless-deterministic-timestep work
   (parity now 0.7%); listed for history.
4. Fix MoltenVK-tolerated validation errors surfaced by lavapipe CI
   (missing `VkImageMemoryBarrier.sType` fields on some paths,
   missing `VK_BUFFER_USAGE_INDEX_BUFFER_BIT` on index streams —
   see `docker-compose.yml` for expected failures)
5. Resize handling (swapchain recreation; partially working)
6. Multi-frame-in-flight synchronization (currently single-buffered)
7. Device-local memory for vertex/index buffers (currently host-visible)

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
