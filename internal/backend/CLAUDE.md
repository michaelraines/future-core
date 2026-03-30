# Backend Development Guide

This directory contains the graphics backend abstraction and all backend
implementations. Read this before modifying any backend code.

## Architecture

```
internal/backend/
├── backend.go          # 7 interfaces: Device, Texture, Buffer, Shader,
│                       # RenderTarget, Pipeline, CommandEncoder
├── types.go            # Enums: BlendMode, TextureFormat, TextureFilter,
│                       # BufferUsage, IndexFormat, PrimitiveType, etc.
├── registry.go         # Factory registry: Register/Create/Available
├── conformance/        # Golden-image test framework (10 scenes)
├── soft/               # Software rasterizer — reference implementation
├── opengl/             # OpenGL 3.3+ (purego, build tag: glfw)
├── webgl/              # WebGL2 (soft-delegating)
├── vulkan/             # Vulkan (soft-delegating)
├── metal/              # Metal (soft-delegating)
├── webgpu/             # WebGPU (soft-delegating)
└── dx12/               # DirectX 12 (soft-delegating)
```

## Dual-Mode Architecture

Four backends (vulkan, metal, webgpu, dx12) have **GPU mode** implementations
controlled by build tags. WebGPU additionally has a **browser mode** via
`syscall/js`. WebGL has separate `_js.go` files for browser rendering.

### Soft-Delegation Mode (CI)

When compiled without GPU support (CI, `-tags soft`, or non-matching platform),
backends delegate all rendering to the software rasterizer (`soft/`):

1. Each backend wraps `soft.Device` and `soft.Encoder()` internally
2. Each type (Texture, Buffer, etc.) wraps the corresponding `backend.*`
   interface returned by the soft device
3. The encoder unwraps wrapper types before delegating to the soft encoder
4. Conformance tests pass end-to-end in CI without any GPU hardware

**Unwrapping Pattern (Critical)**: The encoder must unwrap wrapper types
before delegating to the soft encoder:

```go
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
    if p, ok := pipeline.(*Pipeline); ok {
        e.inner.SetPipeline(p.inner)
        return
    }
    e.inner.SetPipeline(pipeline)
}
```

### GPU Mode (Desktop)

When compiled on desktop platforms without the `soft` tag, `_gpu.go` files
provide real GPU API implementations. Each backend has:

- `device_gpu.go` — GPU device init, resource creation, frame lifecycle
- `encoder_gpu.go` — command recording via GPU API
- `pipeline_gpu.go` — graphics pipeline state objects
- `shader_gpu.go` — shader compilation (GLSL→native format)
- `texture_gpu.go` — GPU texture management, upload, readback
- `buffer_gpu.go` — GPU buffer management
- `render_target_gpu.go` — render target / framebuffer management

**Build tags**:

| Backend | GPU mode | Browser mode | Soft fallback |
|---|---|---|---|
| Vulkan | `(darwin \|\| linux \|\| freebsd \|\| windows) && !soft` | — | `!(desktop) \|\| soft` |
| Metal | `darwin && !soft` | — | `!darwin \|\| soft` |
| WebGPU | `desktop && !soft` | `js && !soft` | `(!(desktop) && !js) \|\| soft` |
| DX12 | `windows && !soft` | — | `!windows \|\| soft` |
| WebGL | — | `js` (only) | `!js` |

WebGPU is the only backend with **three** build modes. The `_js.go` files use
`syscall/js` to call the browser `navigator.gpu` API directly. Verify all
three compile when modifying the webgpu package:
```bash
go build -tags soft ./internal/backend/webgpu/         # Soft
go build ./internal/backend/webgpu/                    # Native GPU
GOOS=js GOARCH=wasm go build ./internal/backend/webgpu/ # Browser
```

**Native API bindings** (all purego, no CGo):

| Backend | Bindings Package | Functions | Shader Pipeline |
|---|---|---|---|
| Vulkan | `internal/vk/` | 91 | GLSL→SPIR-V via `internal/shaderc/` (purego libshaderc) |
| Metal | `internal/mtl/` | 56 | GLSL→MSL via `internal/shadertranslate/msl.go` (pure Go) |
| WebGPU | `internal/wgpu/` | 60 | GLSL→WGSL via `internal/shadertranslate/wgsl.go` (pure Go) |
| DX12 | `internal/d3d12/` | 39 | Planned (HLSL) |

See `BACKENDS.md` at the project root for detailed per-backend status and roadmap.

## Adding a New Backend

1. Create `internal/backend/<name>/`
2. Implement all 7 interfaces (see method counts below)
3. Create `register.go` with `init()` calling `backend.Register("<name>", ...)`
4. Create `device_test.go` with `TestConformance<Name>` calling `conformance.RunAll`
5. Create `types.go` with API-specific constants and mapping functions
6. Create `types_test.go` with table-driven tests for all mapping functions
7. Run `make` — all checks must pass including 80% coverage minimum

### Interface Method Counts

| Interface | Methods | Key Methods |
|---|---|---|
| Device | 10 | Init, Dispose, BeginFrame, EndFrame, NewTexture, NewBuffer, NewShader, NewRenderTarget, NewPipeline, Capabilities |
| Texture | 7 | Upload, UploadRegion, ReadPixels, Width, Height, Format, Dispose |
| Buffer | 4 | Upload, UploadRegion, Size, Dispose |
| Shader | 7 | SetUniformFloat, SetUniformVec2, SetUniformVec4, SetUniformMat4, SetUniformInt, SetUniformBlock, Dispose |
| RenderTarget | 5 | ColorTexture, DepthTexture, Width, Height, Dispose |
| Pipeline | 1 | Dispose |
| CommandEncoder | 14 | BeginRenderPass, EndRenderPass, SetPipeline, SetVertexBuffer, SetIndexBuffer, SetTexture, SetTextureFilter, SetStencil, SetColorWrite, SetViewport, SetScissor, Draw, DrawIndexed, Flush |

## Conformance Testing

Every backend must pass the 10-scene conformance suite:

```go
func TestConformance<Name>(t *testing.T) {
    dev := <name>.New()
    require.NoError(t, dev.Init(backend.DeviceConfig{
        Width:  conformance.SceneSize,
        Height: conformance.SceneSize,
    }))
    defer dev.Dispose()
    enc := dev.Encoder()
    conformance.RunAll(t, dev, enc)
}
```

**Scenes**: clear_red, clear_green, triangle_red, triangle_vertex_colors,
textured_quad, blend_source_over, blend_additive, scissor_rect,
ortho_projection, multiple_triangles.

**Golden images** are auto-generated on first run and stored in each
backend's `testdata/golden/` directory. On failure, `_actual.png` and
`_diff.png` artifacts are saved in `testdata/golden/diff/`.

**Tolerance**: ±3 per channel (accounts for float rounding across backends).

## Backend Registry

Backends self-register via `init()`:

```go
func init() {
    backend.Register("mybackend", func() backend.Device { return New() })
}
```

The engine selects backends via `FUTURE_CORE_BACKEND` env var or
`backend.Create(name)`. `backend.Available()` lists all registered backends.

**Important**: Each backend's `register.go` must be imported (directly or
transitively) for registration to occur. The registration is unconditional
(no build tags) for soft-delegating backends.

## API-Specific Type Mapping

Each backend defines constants mirroring the target GPU API's enums and
provides mapping functions:

- **WebGL2**: GL format constants, buffer target constants, `ContextAttributes`
- **Vulkan**: VkFormat, VkBufferUsageFlags, VkImageUsageFlags, API version macros, `InstanceCreateInfo`, `PhysicalDeviceInfo`
- **Metal**: MTLPixelFormat, MTLTextureUsage, MTLStorageMode, `FeatureSet`
- **WebGPU**: WGPUTextureFormat, WGPUBufferUsage, `AdapterInfo`, `BackendType`, `Limits`
- **DirectX 12**: DXGI_FORMAT, D3D12_HEAP_TYPE, `FeatureLevel`, `AdapterDesc`

These mappings exist so that when real GPU bindings are added, the correct
API-specific values are already defined and tested.

## Common Pitfalls

- **Don't skip type unwrapping in the encoder.** The soft encoder uses type
  assertions to access internal `soft.*` types. If you pass a `webgl.Texture`
  to the soft encoder's `SetTexture`, it will silently fail.
- **Don't forget `Encoder()` method on Device.** The conformance framework
  needs both `dev` and `enc` passed separately.
- **Don't mix GPU and soft code in the same file.** GPU implementations go
  in `_gpu.go` files with appropriate build tags; soft-delegation in the
  untagged files.
- **Don't request Vulkan extensions without checking availability.** On
  macOS (MoltenVK), use `vk.EnumerateInstanceExtensionProperties()` first.
- **Don't modify `conformance/` golden images** unless the soft rasterizer
  itself changes. All backends must produce identical output.

## Vulkan GPU Development Gotchas

- **Descriptor pool lifetime**: Pools must survive until `BeginFrame`'s fence
  wait confirms the GPU finished. Use `vkResetDescriptorPool` (not destroy)
  for performance.
- **Buffer ring-buffers**: Vertex, index, and uniform buffers use persistently
  mapped memory with ring-buffer write cursors. Each `Upload` appends at an
  increasing offset; `SetVertexBuffer`/`SetIndexBuffer` bind at `lastWriteOffset`.
- **UBO alignment**: Descriptor buffer offsets must be multiples of 256
  (`uniformAlignOffset`). Use the full aligned range in descriptor writes.
- **Y-flip**: Vulkan Y-down vs OpenGL Y-up. The Vulkan `SetUniformMat4`
  negates row 1 of `uProjection` (column-major indices 1, 5, 9, 13).
- **Cocoa NoGL path**: `FramebufferSize()` returns logical (not physical)
  size to match GL behavior. Don't set `contentsScale` on the CAMetalLayer.
- **Struct sizes**: All Vulkan FFI structs verified against C equivalents in
  `internal/vk/vk_test.go`. Run `TestStructSizes` after adding new structs.
- **Swapchain format**: MoltenVK typically offers B8G8R8A8. Vulkan handles
  the RGBA→BGRA mapping in hardware — no shader swizzle needed.

## WebGPU GPU Development Notes

The WebGPU backend has the most complete GPU pipeline after OpenGL. Key
implementation details:

- **Shader translation**: GLSL→WGSL via `internal/shadertranslate/wgsl.go`.
  The translator extracts uniform layout for std140 packing. Most GLSL
  built-ins (`sin`, `cos`, `mix`, `clamp`, etc.) pass through unchanged.
  `mod(x, y)` is translated to `(x % y)`.
- **Bind group architecture**: Group 0 = uniforms (vertex+fragment visibility),
  Group 1 = texture + sampler (fragment visibility). Layouts are cached on
  the Pipeline; the Encoder reuses them.
- **Uniform ring buffer** (native path): 16 KB persistent GPU buffer with
  256-byte-aligned cursor. Reset per-frame in `BeginFrame`, advances per-draw.
  Eliminates per-draw buffer allocation.
- **Surface/presentation** (native path): `DeviceConfig.SurfaceFactory` creates
  a `wgpu::Surface`; configured with FIFO present mode. `BeginFrame` acquires
  surface texture; `EndFrame` presents. Auto-reconfigures on stale/lost surface.
- **Browser path**: `_js.go` files use `syscall/js` to call `navigator.gpu`.
  Async adapter/device creation via Promise callbacks. `GPUCanvasContext` for
  presentation. Per-draw uniform buffers (JS GC handles cleanup).
- **Resize**: `Device.Resize(w, h)` reconfigures the surface or recreates
  the offscreen texture.
- **GPU testing checklist**: See `internal/backend/webgpu/GPU_TESTING.md` for
  the 7-tier validation plan (device init → visual presentation).

## Coverage Requirements

- Minimum: 80% (CI-enforced via `make cover-check`)
- Target: 90%+ (all current backends achieve this)
- The conformance test alone covers most Device/Encoder paths
- Add unit tests for type mapping functions, error paths, and API-specific logic
