# Backend Implementation Status

Per-backend state of the world for planning future work. Each section covers
what's implemented, what's working, what's broken, and what remains.

Last updated: 2026-03-24

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
| WebGPU | Early / incomplete | Yes | 10/10 (soft) | Planned (WGSL) |
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
- Requires GLFW (vendored CGo on Linux, purego on macOS/Windows)

### Roadmap
- Consider OpenGL 4.x path for compute shader support
- Otherwise feature-complete for Phase 1

---

## Vulkan

**Package**: `internal/backend/vulkan/`
**Status**: GPU bindings in progress — clear works, draw pipeline broken
**Platform**: macOS (MoltenVK), Linux, Windows
**Bindings**: `internal/vk/vk.go` — 91 purego-bound Vulkan functions
**Shader**: `internal/shaderc/shaderc.go` — GLSL→SPIR-V via purego libshaderc
**GPU Tests**: `vulkan_gpu_test.go`

### Implemented (GPU mode)
- Vulkan instance creation with extension enumeration
- Physical device selection (prefers discrete GPU)
- Logical device + graphics queue
- Command pool + command buffer management
- Swapchain (`VkSwapchainKHR`) with image acquisition and presentation
- Surface creation: `vkCreateMetalSurfaceEXT` (macOS), `vkCreateWin32SurfaceKHR` (Windows)
- Texture creation (`VkImage` + `VkImageView` + `VkDeviceMemory`)
- Texture upload via staging buffer with layout transitions
- Texture readback via staging buffer with barriers
- Buffer creation (`VkBuffer` + `VkDeviceMemory`) with map/unmap
- Shader compilation: GLSL→SPIR-V via shaderc, `VkShaderModule` creation
- Uniform storage and binding (float, vec2, vec4, mat4, int)
- Descriptor set layout with 3 bindings (sampler, fragment UBO, vertex UBO)
- Descriptor pool allocation and updates
- Render pass management (begin/end with clear values)
- Viewport and scissor (dynamic state)
- Draw and DrawIndexed command recording
- Fence-based synchronization
- Default sampler (nearest-neighbor)
- Frame lifecycle (BeginFrame/EndFrame with swapchain acquire/present)

### Known Issues
- **`vkCreateGraphicsPipelines` SIGSEGVs** — the full graphics pipeline
  creation crashes. Likely a struct layout or pointer lifetime issue in the
  pipeline create info chain. This blocks all sprite/geometry rendering.
  Clear-only rendering works (background color renders correctly).
- MoltenVK on macOS: `VK_KHR_portability_enumeration` may not be available;
  extension availability is now checked before requesting.

### What Works End-to-End
- Window opens with Vulkan presentation (no GL involvement)
- Background clear color renders correctly
- Swapchain acquire/present cycle runs without crashes
- Shader compilation (GLSL→SPIR-V) succeeds
- Texture upload/readback via staging buffer

### What Doesn't Work Yet
- Sprite rendering (blocked by pipeline SIGSEGV)
- Any geometry drawing (same blocker)

### Roadmap
1. **Fix `vkCreateGraphicsPipelines` SIGSEGV** — debug struct alignment,
   pointer lifetimes in pipeline create info. This is the critical blocker.
2. Validate full sprite rendering pipeline end-to-end
3. Run conformance suite against GPU mode
4. Implement resize handling (swapchain recreation)
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
**Status**: GPU bindings early — basic structure, shader pipeline incomplete
**Platform**: Cross-platform (desktop via wgpu-native, browser via JS API)
**Bindings**: `internal/wgpu/wgpu.go` — 53 purego-bound functions
**GPU Tests**: None

### Implemented (GPU mode)
- `_gpu.go` files exist for all types (device, encoder, pipeline, shader,
  texture, buffer, render target)
- wgpu-native bindings cover: Instance, Adapter, Device, Queue, Surface,
  Swapchain, Texture, TextureView, Buffer, ShaderModule, BindGroup,
  Pipeline, CommandEncoder, RenderPassEncoder
- Basic device initialization structure
- Texture and buffer creation framework

### Known Issues
- Async adapter/device creation has placeholder code ("for now, set adapter
  directly" comment)
- Shader compilation path incomplete (WGSL source expected but no GLSL→WGSL
  translator exists yet)
- No GPU tests to validate any of the implementation

### Roadmap
1. Implement GLSL→WGSL translator (or integrate Naga via purego)
2. Complete async adapter/device initialization
3. Validate basic clear + readback cycle
4. Implement full draw pipeline with bind groups
5. Add GPU tests
6. Surface/swapchain integration for presentation
7. Browser path via `syscall/js` (`navigator.gpu`)

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
| GLSL 330 | WGSL | Not implemented | Planned |
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

3. **WebGPU** — Bindings exist but shader pipeline is the blocker. Needs
   GLSL→WGSL translator before any rendering can work.

4. **DirectX 12** — Most work remaining. Minimal GPU implementation, no
   shader pipeline, Windows-only testing constraint.

5. **WebGL2** — Soft-delegating is sufficient for now. Real implementation
   depends on the web platform shim which is a separate effort.
