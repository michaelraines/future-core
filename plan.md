# M9 Plan — Platform Backends: WebGL2, Vulkan, WebGPU

## Goal

Implement three new backends alongside the existing OpenGL 3.3 backend. All four
backends must pass the same conformance test suite. The engine selects backends
via `FUTURE_RENDER_BACKEND` env var or auto-detection based on platform.

## Scope

| Backend | Platform | Build Tag | Notes |
|---|---|---|---|
| WebGL2 | WASM (browser) | `GOOS=js GOARCH=wasm` | Canvas + requestAnimationFrame |
| Vulkan | Linux/Windows/Android | `vulkan` | purego, no CGo |
| WebGPU | WASM (browser) | `GOOS=js GOARCH=wasm` | Modern web, navigator.gpu |
| Software | All | (none) | Headless CI fallback, pure Go |

## Architecture

Each backend implements `backend.Device` + `backend.CommandEncoder`. Each
platform target implements `platform.Window`. The existing interfaces are
sufficient — no changes to `internal/backend/backend.go` or
`internal/platform/platform.go` unless a gap is discovered during implementation.

```
internal/backend/
├── opengl/        (existing, M1)
├── webgl/         (new — WebGL2 via syscall/js)
├── vulkan/        (new — Vulkan 1.1 via purego)
├── webgpu/        (new — WebGPU via syscall/js navigator.gpu)
└── software/      (new — pure Go CPU rasterizer for CI)

internal/platform/
├── glfw/          (existing, M1)
└── web/           (new — browser canvas, rAF, DOM events)
```

## Task Breakdown

### Phase 9a — Backend Conformance Test Suite

Before writing any new backend, define a shared test suite that validates any
`backend.Device` implementation against the interface contract. This prevents
each backend from being tested in isolation with different assumptions.

| Task | Notes |
|---|---|
| Create `internal/backend/conformance/` package | Table-driven tests parameterized by Device factory |
| Test: texture create/upload/read cycle | NewTexture → Upload → ReadPixels round-trip |
| Test: buffer create/upload cycle | NewBuffer → Upload for vertex, index, uniform |
| Test: shader compile (valid GLSL) | NewShader with minimal vert+frag |
| Test: pipeline create from shader+format | NewPipeline with Vertex2D format |
| Test: render target create/bind | NewRenderTarget → render to it → read pixels |
| Test: draw triangle to render target | Full pipeline: upload quad, draw, read back |
| Test: blend modes produce expected output | BlendSourceOver, BlendAdditive on known colors |
| Test: texture filtering (nearest vs linear) | Visual difference on scaled texture |
| Test: stencil operations | EvenOdd fill rule behavior |
| Test: viewport/scissor clipping | Draw outside scissor → verify clipped |
| Test: DeviceCapabilities returns valid values | MaxTextureSize > 0, etc. |
| Register OpenGL backend as first conformance target | Validates the test suite itself |

### Phase 9b — Software Rasterizer Backend

A minimal CPU-based backend for headless testing and CI. Does not need to be
fast — correctness is the only goal. This lets the conformance suite run in CI
without GPU access.

| Task | Notes |
|---|---|
| `internal/backend/software/device.go` | In-memory framebuffer, RGBA8 only |
| Texture: backed by `[]byte` slices | Upload/ReadPixels are memcpy |
| Buffer: backed by `[]byte` slices | Upload is memcpy |
| Shader: no-op compile (store source) | Uniform setters store values in a map |
| Pipeline: store descriptor | No GPU state to manage |
| RenderTarget: separate `[]byte` framebuffer | Color only, no depth (2D sufficient) |
| CommandEncoder: software triangle rasterizer | Scanline rasterizer with barycentric interpolation |
| Texture sampling (nearest) | Sample from texture byte slice |
| Blend modes (SourceOver at minimum) | Alpha compositing on CPU |
| Register as conformance target | Must pass all conformance tests |

### Phase 9c — WebGL2 Backend + Web Platform

Browser rendering via WebGL2 (OpenGL ES 3.0). Uses `syscall/js` for DOM and
WebGL API access. The platform shim handles canvas, requestAnimationFrame, and
DOM input events.

| Task | Notes |
|---|---|
| `internal/platform/web/window.go` | Canvas element, rAF loop, resize observer |
| Web input handling (keyboard, mouse, touch) | DOM event listeners → InputHandler events |
| `internal/backend/webgl/device.go` | WebGL2 rendering context from canvas |
| WebGL2 texture operations | gl.texImage2D, gl.texSubImage2D, gl.readPixels |
| WebGL2 buffer operations | gl.bufferData, gl.bufferSubData |
| WebGL2 shader compilation | gl.createShader, gl.compileShader, gl.linkProgram |
| WebGL2 pipeline state | Blend, depth, cull, stencil via gl.enable/gl.blendFunc |
| WebGL2 command encoder | BeginRenderPass → FBO bind, Draw → gl.drawElements |
| WebGL2 render targets | gl.createFramebuffer + gl.framebufferTexture2D |
| GLSL ES 300 shader translation | `#version 330 core` → `#version 300 es` + precision |
| `engine_web.go` (build: js,wasm) | Wire web Window + webgl Device, rAF-driven loop |
| Register as conformance target | Must pass all conformance tests (via wasm test runner or node) |

### Phase 9d — Vulkan Backend

Vulkan 1.1 via purego (no CGo). The most complex backend due to Vulkan's
explicit resource management. Uses GLFW's Vulkan surface support for window
integration.

| Task | Notes |
|---|---|
| `internal/vk/` — Vulkan function loader | purego bindings for Vulkan entry points |
| Instance + physical device selection | Enumerate GPUs, select discrete if available |
| Logical device + queue creation | Graphics + present queue families |
| Swapchain management | VkSwapchainKHR, image acquisition, present |
| `internal/backend/vulkan/device.go` | Device interface implementation |
| VkImage + VkImageView for textures | Staging buffer upload, layout transitions |
| VkBuffer for vertex/index/uniform | Device-local + staging pattern |
| SPIR-V shader compilation | GLSL → SPIR-V via runtime compiler or pre-compiled |
| VkPipeline creation | Graphics pipeline with vertex input, blend, depth |
| VkRenderPass + VkFramebuffer | Render target implementation |
| VkCommandBuffer recording | Command encoder maps to command buffer recording |
| Synchronization (fences, semaphores) | Frame-in-flight synchronization |
| GLFW Vulkan surface integration | `glfwCreateWindowSurface` via purego |
| `engine_vulkan.go` (build: vulkan) | Wire GLFW Window + Vulkan Device |
| Register as conformance target | Must pass all conformance tests |

### Phase 9e — WebGPU Backend

Modern web graphics via the WebGPU API (`navigator.gpu`). Uses `syscall/js` to
call the WebGPU JavaScript API. Shares the web platform shim from Phase 9c.

| Task | Notes |
|---|---|
| `internal/backend/webgpu/device.go` | GPUDevice from navigator.gpu.requestAdapter |
| WebGPU texture operations | GPUTexture, createTexture, writeTexture |
| WebGPU buffer operations | GPUBuffer, createBuffer, writeBuffer |
| WebGPU shader compilation | GPUShaderModule from WGSL source |
| GLSL → WGSL translation layer | Convert existing GLSL shaders to WGSL |
| WebGPU render pipeline | GPURenderPipeline with vertex/fragment/blend state |
| WebGPU command encoder | GPUCommandEncoder → GPURenderPassEncoder |
| WebGPU render targets | GPUTexture as color attachment |
| Bind group management | GPUBindGroup for textures + uniforms per draw |
| `engine_webgpu.go` (build: js,wasm) | Wire web Window + webgpu Device |
| Register as conformance target | Must pass all conformance tests |

### Phase 9f — Backend Selection + Integration

Wire up `FUTURE_RENDER_BACKEND` auto-detection and ensure all backends
are selectable at build time.

| Task | Notes |
|---|---|
| Auto-detection logic per platform | js/wasm → webgpu (fallback webgl), linux/windows → vulkan (fallback opengl) |
| `engine.go` backend registry | Map of backend name → factory function |
| Build tags for each backend | `glfw`, `vulkan`, `js` — ensure clean compilation |
| Update `engine_stub.go` for new backends | Stub entries for non-tagged builds |
| Update CI to test software backend | Conformance tests run headlessly |
| Update ROADMAP.md | Mark M9 tasks as done |

## Suggested Implementation Order

1. **9a** (conformance tests) — validates existing OpenGL, defines contract
2. **9b** (software rasterizer) — enables headless CI, validates conformance suite
3. **9c** (WebGL2 + web platform) — closest to existing OpenGL, reuses GLSL
4. **9e** (WebGPU) — shares web platform from 9c, modern API
5. **9d** (Vulkan) — most complex, benefits from lessons learned in 9c/9e
6. **9f** (integration) — wires everything together

## Key Design Decisions

### GLSL Translation Strategy

The engine currently generates GLSL 330 core (from Kage transpiler and raw GLSL
input). Each backend needs a compatible shader format:

| Backend | Shader Input | Translation Needed |
|---|---|---|
| OpenGL | GLSL 330 core | None (native) |
| WebGL2 | GLSL ES 300 | `#version` swap + `precision` qualifiers |
| Vulkan | SPIR-V | GLSL → SPIR-V compilation (glslang or runtime) |
| WebGPU | WGSL | GLSL → WGSL translation (new translator needed) |
| Software | GLSL 330 core | Interpreted or ignored (fixed-function) |

Options for SPIR-V/WGSL:
- **Pure Go GLSL→SPIR-V compiler** — significant undertaking, but keeps no-CGo promise
- **Embed pre-compiled SPIR-V** — compile at build time, ship both GLSL + SPIR-V
- **Runtime shaderc/naga via purego** — load compiler library at runtime
- **Extend Kage transpiler** — emit WGSL/SPIR-V directly alongside GLSL

Recommended: Extend the Kage transpiler to emit WGSL and SPIR-V directly. For
raw GLSL input (`NewShaderFromGLSL`), provide a lightweight GLSL→WGSL
translator and use a purego-loaded SPIR-V compiler for Vulkan.

### No CGo Constraint

All backends must use purego for native library access:
- Vulkan: load `libvulkan.so`/`vulkan-1.dll` via purego
- GLFW Vulkan surface: already purego-based, add `glfwCreateWindowSurface`
- SPIR-V compiler: load `libshaderc` via purego (optional runtime dep)

### Web Platform Shared Between WebGL2 and WebGPU

Both web backends share `internal/platform/web/` for window management, input,
and rAF loop. The backend-specific code is in `internal/backend/webgl/` and
`internal/backend/webgpu/`. Selection between them happens in `engine_web.go`
based on `navigator.gpu` availability.

## Risks

| Risk | Mitigation |
|---|---|
| GLSL→WGSL translation is non-trivial | Start with subset matching Kage output; extend Kage transpiler |
| Vulkan complexity (sync, memory) | Start with single-frame-in-flight, simplest valid setup |
| SPIR-V compilation without CGo | Use purego-loaded shaderc, or extend Kage to emit SPIR-V |
| WebGPU browser support gaps | WebGL2 fallback on unsupported browsers |
| Conformance test suite may miss edge cases | Add tests as bugs are found in new backends |
| Software rasterizer correctness | Only needs to match interface contract, not pixel-perfect |
