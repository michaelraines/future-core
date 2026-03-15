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
│   ├── backend/            # Graphics backend interface
│   │   ├── backend.go      # Device, Texture, Buffer, Shader, Pipeline,
│   │   │                   # CommandEncoder, RenderTarget interfaces
│   │   ├── types.go        # VertexFormat, BlendMode, TextureFormat, etc.
│   │   ├── opengl/         # (Phase 1 implementation)
│   │   ├── metal/          # (Future)
│   │   └── webgl/          # (Future)
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
| `Texture` | 5 | GPU texture: upload, query size/format, dispose |
| `Buffer` | 4 | GPU buffer: upload data, query size, dispose |
| `Shader` | 7 | Compiled shader: set uniforms, dispose |
| `RenderTarget` | 4 | Off-screen framebuffer: attachments, dispose |
| `Pipeline` | 1 | Pre-compiled render state: dispose |
| `CommandEncoder` | 10 | Record and submit draw commands |

### Why Not a Single Large Interface

Following the Go interface design principle: interfaces are defined by consumers, not
implementors. Each interface represents a single concern. A backend that doesn't support
render targets (e.g., a software rasterizer for testing) only needs to stub `RenderTarget`.

### Backend Selection

Required backends: OpenGL 3.3+ (desktop), WebGL2 (web/WASM), Vulkan (desktop/Android).
Future: Metal, WebGPU, DirectX 12.

Selection is compile-time via build tags and runtime via environment variable:
```
FUTURE_RENDER_BACKEND=opengl|webgl|vulkan|auto
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
- **Integration tests** (future): Render known scenes, compare against golden images.
  Will use a headless OpenGL context or software rasterizer.
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
