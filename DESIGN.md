# Future Render — Design Document

## Overview

Future Render is a production-grade rendering engine written in pure Go. Phase 1 achieves
full 2D feature parity with Ebitengine. The architecture is designed from day one to support
3D rendering in future phases.

---

## Architecture

### Layered Design

Dependencies flow strictly downward. No layer may reach upward. Cross-layer
communication uses interfaces, not concrete types.

```
┌──────────────────────────────────┐
│         Game / App Layer         │  engine.go, image.go, input.go
│                                  │  Public API: Game interface, Image, GeoM
├──────────────────────────────────┤
│        Scene / Graph Layer       │  (Phase 2+: scene graph, 2D sprite trees)
│                                  │  Phase 1: flat draw list from Draw()
├──────────────────────────────────┤
│      Render Pipeline Layer       │  internal/pipeline/
│                                  │  Ordered passes with declared I/O
├──────────────────────────────────┤
│         Batch Layer              │  internal/batch/
│                                  │  Draw call sorting and merging
├──────────────────────────────────┤
│      Backend Abstraction Layer   │  internal/backend/
│                                  │  Device, Texture, Buffer, Shader,
│                                  │  Pipeline, CommandEncoder interfaces
├──────────────────────────────────┤
│        Platform / OS Layer       │  internal/platform/
│                                  │  Window, input events, timing
└──────────────────────────────────┘
```

### Package Structure

```
future-render/
├── engine.go               # Public Game interface, RunGame(), global state
├── image.go                # Public Image type, DrawImage, DrawTriangles
├── input.go                # Public input query API
├── math/                   # Pure Go math: Vec2/3/4, Mat3/4, Quat, Color,
│   │                       # Rect, AABB, Frustum, Ray, ColorMatrix
│   ├── vec2.go
│   ├── vec3.go
│   ├── vec4.go
│   ├── mat3.go
│   ├── mat4.go
│   ├── quaternion.go
│   ├── color.go
│   ├── geom.go             # Rect, AABB, Frustum, Plane, Ray
│   └── util.go             # Clamp, Lerp, etc.
├── internal/
│   ├── backend/            # Graphics backend interface + registry
│   │   ├── backend.go      # Device, Texture, Buffer, Shader, Pipeline,
│   │   │                   # CommandEncoder, RenderTarget interfaces
│   │   ├── types.go        # VertexFormat, BlendMode, TextureFormat, etc.
│   │   ├── registry.go     # Backend factory registry: Register/Create/Available
│   │   ├── conformance/    # Golden-image conformance test framework (10 scenes)
│   │   ├── soft/           # Software rasterizer — reference backend, no GPU needed
│   │   ├── opengl/         # OpenGL 3.3+ via purego (desktop OS constraint)
│   │   ├── webgl/          # WebGL2 — soft-delegating, ready for syscall/js
│   │   ├── vulkan/         # Vulkan — soft-delegating, ready for purego libvulkan
│   │   ├── metal/          # Metal — soft-delegating, ready for purego objc_msgSend
│   │   ├── webgpu/         # WebGPU — soft-delegating, ready for wgpu-native
│   │   └── dx12/           # DirectX 12 — soft-delegating, ready for purego d3d12.dll
│   ├── pipeline/           # Render pass definitions and execution
│   │   └── pass.go         # Pass interface, Pipeline, PassContext
│   ├── batch/              # Draw call batching and sorting
│   │   └── batch.go        # Batcher, Vertex2D, DrawCommand, Batch
│   ├── input/              # Input state aggregation
│   │   └── input.go        # State manager, InputHandler implementation
│   └── platform/           # OS/window per-platform
│       ├── platform.go     # Window, InputHandler interfaces
│       └── keys.go         # Key constants
├── asset/                  # Asset loading, caching, embedding
├── image/                  # Engine image type internals
├── shader/                 # Shader compilation and management
├── text/                   # Font loading and text rendering
├── audio/                  # Public audio API
└── cmd/                    # Example programs and tooling
```

### Rationale for Package Boundaries

- **`math/` is exported**: Users need vectors, matrices, colors for game logic.
  Pure Go, no CGo, no dependencies.
- **`internal/backend/` is internal**: Backend types must never leak to game code.
  The public API uses engine-specific types (BlendMode, Filter) that map internally.
- **`internal/pipeline/` is internal**: The render pipeline is an implementation detail.
  Users interact through `Image.DrawImage()` and `Image.DrawTriangles()`.
- **`internal/batch/` is internal**: Batching is transparent to the user.
- **`internal/platform/` is internal**: Window and input implementation details are hidden.

---

## Backend Abstraction

### Interface Design

The backend is defined by 7 interfaces (see `internal/backend/backend.go`):

| Interface | Methods | Purpose |
|---|---|---|
| `Device` | 10 | GPU context: create resources, begin/end frame |
| `Texture` | 7 | GPU texture: upload, upload region, read pixels, query size/format, dispose |
| `Buffer` | 4 | GPU buffer: upload data, query size, dispose |
| `Shader` | 7 | Compiled shader: set uniforms (float, vec2, vec4, mat4, int, block), dispose |
| `RenderTarget` | 5 | Off-screen framebuffer: color/depth attachments, size, dispose |
| `Pipeline` | 1 | Pre-compiled render state: dispose |
| `CommandEncoder` | 14 | Record and submit draw commands, bind state, set viewport/scissor |

### Why Not a Single Large Interface

Following the Go interface design principle: interfaces are defined by consumers, not
implementors. Each interface represents a single concern. A backend that doesn't support
render targets (e.g., a software rasterizer for testing) only needs to stub `RenderTarget`.

### Backend Selection

Seven backends are implemented, each registered via `init()` in the backend
registry (`internal/backend/registry.go`):

| Backend | Package | Status | Platform |
|---|---|---|---|
| Software | `soft/` | Production — CPU rasterizer, headless CI reference | All |
| OpenGL 3.3 | `opengl/` | Production — purego, no CGo | Desktop (darwin/linux/freebsd) |
| WebGL2 | `webgl/` | Soft-delegating — ready for syscall/js | Browser (WASM) |
| Vulkan | `vulkan/` | Soft-delegating — ready for purego libvulkan | Linux/Windows/Android |
| Metal | `metal/` | Soft-delegating — ready for purego objc_msgSend | macOS/iOS |
| WebGPU | `webgpu/` | Soft-delegating — ready for wgpu-native | Cross-platform |
| DirectX 12 | `dx12/` | Soft-delegating — ready for purego d3d12.dll | Windows |

"Soft-delegating" backends wrap the software rasterizer so conformance tests
pass in any environment. When real GPU bindings are added, only the delegation
layer needs replacement — the type structure and API surface are already in place.

Selection is compile-time via OS-based build constraints and runtime via
environment variable:
```
FUTURE_RENDER_BACKEND=opengl|webgl|vulkan|metal|webgpu|dx12|soft|auto
```

The `backend.Create(name)` function looks up the named factory in the registry.
`backend.Available()` returns all registered backend names.

### Auto-Detection

When `FUTURE_RENDER_BACKEND=auto` (the default), `backend.Resolve` iterates a
platform-specific preference list and returns the first registered backend:

| Platform | Preference Order |
|---|---|
| macOS | Metal → Vulkan → OpenGL → Software |
| Windows | DirectX 12 → Vulkan → OpenGL → Software |
| Linux / FreeBSD | Vulkan → OpenGL → Software |
| Browser (WASM) | WebGPU → WebGL2 → Software |
| Other | OpenGL → Software |

### Backend Feature Comparison

All backends implement the same 7 interfaces (Device, Texture, Buffer, Shader,
RenderTarget, Pipeline, CommandEncoder). The table below summarizes platform
availability, GPU API binding status, and shader language.

| Backend | Platform | GPU Binding | Shader Language | Conformance |
|---|---|---|---|---|
| Software | All | N/A (CPU) | N/A | 10/10 |
| OpenGL 3.3 | Desktop (Linux, Windows, macOS) | purego (Unix) / x/sys/windows — production | GLSL 330 core | N/A (GPU) |
| WebGL2 | Browser (WASM) | Soft-delegating | GLSL ES 3.00 | 10/10 |
| Vulkan | Linux, Windows, Android | Soft-delegating | SPIR-V (planned) | 10/10 |
| Metal | macOS, iOS | Soft-delegating | MSL (planned) | 10/10 |
| WebGPU | Cross-platform, Browser | Soft-delegating | WGSL (planned) | 10/10 |
| DirectX 12 | Windows | Soft-delegating | HLSL (planned) | 10/10 |

**Capability matrix** (reported by `DeviceCapabilities`):

| Capability | Soft | OpenGL | WebGL2 | Vulkan | Metal | WebGPU | DX12 |
|---|---|---|---|---|---|---|---|
| Max Texture Size | 4096 | GPU-dependent | 4096 | GPU-dependent | GPU-dependent | GPU-dependent | GPU-dependent |
| Render Targets | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| Instanced Draw | No | Yes | Yes | Yes | Yes | Yes | Yes |
| Compute Shaders | No | No | No | Yes | Yes | Yes | Yes |
| MSAA | No | Yes | Yes | Yes | Yes | Yes | Yes |
| Float16 Textures | No | Yes | Yes | Yes | Yes | Yes | Yes |

**Soft-delegating** means the backend currently delegates all rendering to the
software rasterizer for CI testability. The type structure, API constants, and
conformance scaffolding are in place — converting to real GPU bindings requires
replacing the `inner` delegation in each method with actual GPU API calls.

Use `cmd/backends` to list registered backends and query their capabilities:
```
go build ./cmd/backends && ./backends
```

---

## Render Pipeline

### Pass-Based Architecture

Rendering is a sequence of passes, not a single draw function. Each pass:
- Declares input resources (textures, buffers, render targets)
- Declares output resources
- Has a deterministic `Execute` function with no hidden state

Phase 1 passes:
1. **Clear Pass** — Clear the default framebuffer
2. **Sprite Pass** — Batch and draw all 2D sprites
3. **Present Pass** — Scale logical screen to window size

Future passes (additive, no rewrite needed):
- Shadow map pass
- Geometry pass (deferred)
- Lighting pass
- Post-processing pass
- UI overlay pass

### Draw Call Batching

The batcher (`internal/batch/`) sorts draw commands by a composite key:
`(ShaderID, BlendMode, TextureID, Depth)`

Commands sharing the same key are merged: vertices and indices are concatenated,
index values are offset. This minimizes GPU state changes.

The sort uses Go's `sort.Slice` for simplicity. If profiling shows this is a bottleneck,
radix sort (like bgfx) can be substituted without changing the interface.

---

## Public API Design

### Game Interface

```go
type Game interface {
    Update() error
    Draw(screen *Image)
    Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int)
}
```

Identical to Ebitengine. This enables migration with a thin adapter.

### Image Operations

- `NewImage(w, h)` — Create blank image
- `img.DrawImage(src, opts)` — Draw with GeoM transform, ColorScale, BlendMode
- `img.DrawTriangles(vertices, indices, src, opts)` — Low-level triangle drawing
- `img.Fill(color)` — Clear to solid color
- `img.SubImage(rect)` — Sprite sheet region

### Transform (GeoM)

GeoM wraps a `math.Mat3` with convenience methods:
- `Translate(tx, ty)`, `Scale(sx, sy)`, `Rotate(angle)`, `Skew(sx, sy)`
- `Concat(other)`, `Reset()`, `Apply(x, y)`

Operations compose left-to-right: `g.Scale(2,2); g.Translate(10,0)` scales then translates.

---

## Concurrency Model

- The game loop runs on the main goroutine (required by macOS/iOS).
- `Update()` and `Draw()` are called sequentially from the main loop — no concurrency.
- Input events are delivered via callbacks from the platform layer, buffered, and
  consumed during `Update()`.
- Asset loading can use goroutines with `context.Context` for cancellation.
- GPU resource creation/destruction is deferred to the render thread.

---

## Error Handling

- All resource creation returns `(T, error)`. No panics in library code.
- `Update()` returns `error` — returning non-nil stops the game loop.
- `futurerender.Termination` is the sentinel for clean exit.
- Backend errors during rendering are logged, not panicked — a dropped frame is better
  than a crash.

---

## Dependencies

Phase 1 dependencies (all small, auditable):

| Dependency | Purpose | Justification |
|---|---|---|
| Standard library only (Phase 1 core) | Math, sync, time | No external deps for core logic |
| `ebitengine/purego` | C FFI without CGo | Runtime loading of OpenGL and GLFW shared libs |

No dependency on Ebitengine or any of its sub-packages.

---

## Testing Strategy

**Coverage policy**: All changes require tests. Target 100%, CI enforces 80%
minimum per package via `make cover-check`. See `CLAUDE.md` for details.

- **Unit tests**: `math/`, `internal/batch/`, public API (Image, GeoM, DrawImage).
- **Mock-based tests**: GPU code paths tested with mock `backend.Device` and
  `backend.Texture` implementations (see `image_test.go`).
- **Benchmarks**: Vec2 ops, Mat4 multiply/inverse, quaternion slerp, batch flush.
- **Conformance tests** (`internal/backend/conformance/`): Golden-image integration
  tests that render 10 canonical scenes through any `backend.Device` and compare
  pixel output against reference PNGs (±3 tolerance per channel). The software
  rasterizer (`internal/backend/soft/`) serves as the reference implementation.
  New backends call `conformance.RunAll(t, dev, enc)` to verify correctness.
  On failure, `_actual.png` and `_diff.png` artifacts are saved for debugging.
  All 7 backends (soft, opengl, webgl, vulkan, metal, webgpu, dx12) pass the
  full 10-scene conformance suite.
- **Fuzz tests** (future): Asset parsers (image, font, audio).

---

## Alternatives Considered

### ECS vs Game Interface

Bevy's ECS approach provides excellent parallelism but adds significant complexity.
Go's goroutines + channels already provide good concurrency primitives. The Game interface
is simpler, familiar to Go developers, and proven by Ebitengine's large user base.

### Deferred vs Forward Rendering

Deferred rendering limits material flexibility (fixed G-buffer layout). Forward+ with
clustered light culling (Godot's approach) preserves material freedom while scaling to
many lights. Phase 1 uses simple forward rendering; Phase 2+ can add clustered culling.

### Automatic vs Manual Texture Atlasing

Ebitengine's automatic atlas is convenient but limits control. We choose manual texture
management with an optional atlas utility, giving users explicit control over GPU memory.

### Immediate-Mode vs Retained-Mode API

Raylib's immediate mode is simple but limits batching optimization. We use retained-mode
(Image objects, draw lists) with automatic batching, which allows sort-based optimization
while keeping the API simple.
