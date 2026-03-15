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
| Gamepad GLFW joystick polling | Deferred | Internal plumbing exists; GLFW polling not yet implemented |
| `cmd/input/main.go` example | Deferred | Needs text rendering (M5) to display state meaningfully |

**Exit criteria**: all public input functions return real platform state.

**Completed**: Public API fully wired to `internal/input.State`. Key mapping
table handles differing iota orderings between public and platform key
constants. GLFW cursor position callback now computes DX/DY deltas. Expanded
key set to cover full keyboard. Added `IsKeyJustPressed`/`IsKeyJustReleased`
to public API. 100% test coverage on `internal/input`, 99.4% on root package.

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
| Multi-line layout / word wrapping | Deferred | Caller splits lines manually |
| Text alignment (center, right) | Deferred | Future enhancement |
| Complex scripts (BiDi, ligatures) | Deferred | Needs `go-text/typesetting` |
| `cmd/text/main.go` example | Deferred | Needs GLFW build environment |
| `cmd/input/main.go` example | Deferred | Needs GLFW build environment |

**Exit criteria**: render arbitrary Unicode text from TTF fonts at any size.

**Completed**: Full `text/` package with `Face`, glyph cache, row-packed
RGBA8 atlas, and `Draw()` function. Glyphs flow through existing
`DrawImage` → `Batcher` → `SpritePass` pipeline with zero internal changes.
All glyphs from the same face auto-merge into 1-2 GPU draw calls via shared
atlas texture. Added `Image.WritePixels()` for incremental atlas uploads.

---

## Milestone 6 — Audio (Planned)

Goal: audio playback parity with Ebitengine's `audio` package.

| Task | Status | Notes |
|---|---|---|
| Audio context + player abstraction | Planned | `audio/` package |
| WAV decoding | Planned | |
| OGG Vorbis decoding | Planned | Pure Go |
| MP3 decoding | Planned | Pure Go |
| Streaming playback (large files) | Planned | |
| Volume, pause, seek, loop | Planned | |
| Multiple simultaneous players | Planned | |
| Example: sound effects + BGM | Planned | |

**Exit criteria**: play, pause, loop, and mix audio from WAV/OGG/MP3 sources.

---

## Milestone 7 — Shader System (Planned)

Goal: user-defined shaders beyond the built-in sprite shader.

| Task | Status | Notes |
|---|---|---|
| `Shader` public type wrapping backend shader | Planned | |
| GLSL vertex + fragment shader compilation | Planned | |
| Uniform binding API (float, vec2/4, mat4, texture) | Planned | |
| `Image.DrawRectShader()` equivalent | Planned | |
| Shader hot-reload for development | Planned | Optional, dev-only |
| Example: custom post-processing shader | Planned | |

**Exit criteria**: users can write and apply custom GLSL shaders to draw calls.

---

## Milestone 8 — Advanced 2D Features (Planned)

Goal: remaining Ebitengine 2D feature parity.

| Task | Status | Notes |
|---|---|---|
| Off-screen render targets (`NewImage` as target) | Planned | Already in backend types |
| ColorMatrix transformation | Planned | `ColorM` equivalent |
| Screen capture / `ReadPixels` | Planned | |
| `SetScreenClearedEveryFrame(false)` | Planned | Accumulation rendering |
| Window resize handling + `Layout` re-evaluation | Planned | |
| High-DPI / device scale factor | Planned | |
| Multiple windows (stretch goal) | Planned | |
| Context loss recovery (mobile/web) | Planned | Godot-inspired command replay |
| `FUTURE_RENDER_BACKEND` env var selection | Planned | |
| Vsync toggle at runtime | Planned | |

**Exit criteria**: all Ebitengine 2D examples can be ported with minimal
changes (import path swap + minor API adjustments).

---

## Milestone 9 — WebGL + Vulkan Backends (Planned)

Goal: WebGL and Vulkan are required compatibility targets alongside OpenGL.
All three backends must pass the same integration test suite.

| Task | Status | Notes |
|---|---|---|
| WebGL2 backend (`internal/backend/webgl/`) | Planned | WASM target, GOOS=js GOARCH=wasm |
| WebGL2 platform shim (canvas, requestAnimationFrame) | Planned | `internal/platform/web/` |
| Vulkan backend (`internal/backend/vulkan/`) | Planned | Linux/Windows/Android |
| Vulkan platform integration (surface creation) | Planned | GLFW Vulkan surface support |
| Backend conformance test suite | Planned | Shared tests all 3 backends must pass |
| `FUTURE_RENDER_BACKEND` runtime selection (opengl/webgl/vulkan/auto) | Planned | Auto-detect based on platform |
| Software rasterizer (testing) | Planned | Headless CI fallback |

---

## Milestone 10 — Additional Backends (Future)

Goal: Metal, WebGPU, and other platform-specific backends.

| Task | Status | Notes |
|---|---|---|
| Metal backend (`internal/backend/metal/`) | Future | macOS/iOS |
| WebGPU backend | Future | Modern web, successor to WebGL path |
| DirectX 12 backend | Future | Windows |

---

## Milestone 11 — 3D Rendering (Future)

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
