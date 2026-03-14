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

## Milestone 2 — Image Rendering (Planned)

Goal: load an image from disk, draw it to the screen with transforms. This
validates the full Image → Batcher → Pipeline → GPU path.

| Task | Status | Notes |
|---|---|---|
| Texture creation from `image.RGBA` in OpenGL backend | Planned | Upload, bind, dispose |
| Default sprite shader (vertex + fragment GLSL) | Planned | Textured quad, vertex color multiply |
| Sprite pass implementation | Planned | Reads batches, issues draw calls |
| `Image.DrawImage()` wired to batcher | Planned | GeoM → vertex positions, UV from source |
| `Image.Fill()` wired to clear or fullscreen quad | Planned | |
| `Image.SubImage()` with correct UV mapping | Planned | Sprite sheet support |
| PNG/JPEG image loading utility | Planned | `asset/` package |
| `DrawImageOptions` — ColorScale, BlendMode, Filter | Planned | |
| `cmd/sprite/main.go` example | Planned | Load image, draw with rotation |

**Exit criteria**: a PNG sprite renders on screen with scale, rotation, and
alpha blending.

---

## Milestone 3 — DrawTriangles + Custom Geometry (Planned)

Goal: expose the low-level `DrawTriangles` API for custom vertex data.

| Task | Status | Notes |
|---|---|---|
| `Image.DrawTriangles()` wired end-to-end | Planned | Vertex + index data → batcher |
| FillRule support (NonZero, EvenOdd) | Planned | Stencil-based |
| `DrawTrianglesOptions` — Blend, Filter, FillRule | Planned | |
| Example: procedural mesh / starfield | Planned | |

**Exit criteria**: `DrawTriangles` renders custom shapes with correct winding
and blending.

---

## Milestone 4 — Input (Planned)

Goal: full keyboard, mouse, touch, and gamepad input parity with Ebitengine.

| Task | Status | Notes |
|---|---|---|
| Wire GLFW key callbacks → `internal/input` State | Planned | |
| Wire mouse button/move/scroll callbacks | Planned | |
| `IsKeyPressed`/`InputChars` connected to real state | Planned | |
| `CursorPosition`/`Wheel` connected | Planned | |
| Gamepad support via GLFW joystick API | Planned | |
| Touch support (desktop emulation + mobile) | Planned | |
| Cursor mode (visible/hidden/captured) | Planned | |
| `IsKeyJustPressed` (edge detection) | Planned | Already in internal/input |
| `cmd/input/main.go` example | Planned | Print key/mouse state |

**Exit criteria**: all public input functions return real platform state.

---

## Milestone 5 — Text Rendering (Planned)

Goal: render TTF/OTF text to Images, matching Ebitengine's `text/v2` package.

| Task | Status | Notes |
|---|---|---|
| TTF parsing and glyph rasterization | Planned | Pure Go or freetype dep |
| Font atlas generation and caching | Planned | `text/` package |
| `text.Draw()` API | Planned | Position, size, color, alignment |
| Unicode + line wrapping support | Planned | |
| Font face management (size, style) | Planned | |
| Example: FPS counter overlay | Planned | |

**Exit criteria**: render arbitrary Unicode text from TTF fonts at any size.

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
