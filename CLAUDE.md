# Future Render — Agent Directives

## Project Overview

Future Render is a production-grade 2D/3D rendering engine in pure Go. Phase 1
targets full 2D feature parity with Ebitengine. The architecture is designed
from day one to support 3D rendering in later phases without rewrites.

Key documents:
- `DESIGN.md` — architecture, layer diagram, API design rationale
- `RESEARCH.md` — survey of Ebitengine, Raylib, bgfx, wgpu, Godot, Bevy, Three.js
- `FUTURE_3D.md` — 3D integration plan and Phase 1 constraints
- `ROADMAP.md` — phased implementation plan (update as work progresses)
- `BACKENDS.md` — per-backend GPU implementation status, features, and roadmap
- `mobile/futurecoreview/CLAUDE.md` — Android JNI bridge: embedded-mode
  engine driver, input dispatch, engine_android build-tag split
- `cmd/futurecoremobile/CLAUDE.md` — Android AAR builder; overlay
  pipeline + case-insensitive-FS bug writeup

## Text Rendering

The `text/` package uses `go-text/typesetting` (HarfBuzz shaping) and `golang.org/x/image/vector.Rasterizer`
for glyph rendering — the same libraries as Ebitengine's text/v2 — to produce pixel-identical text output.
The `util/` package's `DebugPrint` uses an embedded `text.png` bitmap font copied from Ebitengine for matching
debug text output. Do not replace these with alternative text rendering approaches without understanding
the parity implications.

**TODO:** Eventually replace `util/text.png` with our own bitmap font to avoid carrying Ebitengine's asset.
For now it's fine — it gives us pixel-identical DebugPrint output which is valuable for parity testing.

### DPR-aware glyph rasterisation

`text.GoTextFace.drawGlyphs` queries `futurerender.DeviceScaleFactor()`
at draw time and rasterises glyphs at `face.Size × DPR` atlas pixels
(clamped 1..4), then applies a `1/oversample` scale on the draw
transform so glyphs display at logical size. Without this the
engine's logical-to-physical viewport scaling produces 2x-upscaled
blurry text at retina (DPR=2). The cache key in `glyphImageKey`
includes the oversample factor so DPR changes (window dragged
between displays) build a fresh atlas. Cross-backend — every backend
whose viewport runs at physical resolution while the projection
runs at logical benefits.

## Build & Test

All build, test, and lint operations are run via `make`. The default target
runs the full CI pipeline.

```bash
# Full CI pipeline: fmt → vet → lint → test → cover-check → build
make

# Individual targets
make fmt          # Check formatting (fails if files need gofmt)
make vet          # Run go vet
make lint         # Run golangci-lint
make test         # Run all tests
make test-race    # Run tests with race detector
make cover        # Run tests with coverage summary
make cover-check  # Enforce minimum 80% coverage per package (fails CI)
make cover-html   # Generate HTML coverage report (coverage.html)
make bench        # Run benchmarks (math, batch)
make build        # Build all packages
make fix          # Auto-fix formatting and lint issues
make clean        # Remove build artifacts
```

### Visual Testing

Use `scripts/visual-test.sh` to visually validate rendering. Two modes:

- **soft** — Uses the software rasterizer. Works everywhere, no GPU needed.
- **gpu** — Uses the platform's preferred GPU backend (auto-detected).
  Requires GPU hardware. Falls back to soft if no GPU is available.

```bash
./scripts/visual-test.sh -m soft -e sprite      # Soft rasterizer sprite test
./scripts/visual-test.sh -m gpu -e sprite       # GPU sprite test (auto backend)
./scripts/visual-test.sh -m gpu -b vulkan       # Force Vulkan backend
./scripts/visual-test.sh -m soft -e triangles   # Soft triangles test
```

Both modes render through the **full engine pipeline** (window creation,
backend init, render loop, frame capture). On headless Linux environments
(CI, cloud), the script auto-starts Xvfb as a virtual display.

Screenshots are saved to `testdata/visual/<mode>_<example>.png` (gitignored).

**How it works**: the engine supports headless capture via environment variables:
- `FUTURE_CORE_HEADLESS=N` — capture after N frames and exit
- `FUTURE_CORE_HEADLESS_OUTPUT=path.png` — output file path

GPU mode needs ~60 frames for macOS OpenGL context initialization;
soft mode works with fewer frames. The script defaults to 60.

### Debugging the Sprite Pass

When a rendering regression is hard to reproduce with visual testing alone
(e.g. "some draws are missing on WebGPU but the same code works on soft"),
env-gated tracers and a shader-dump test can dump per-frame metadata to
stderr. All tracers cap themselves at the first N frames so logs stay
small, and the hot-path cost when disabled is a single atomic-int
comparison.

**Diagnostic tools**

| Env var / command | Source | Dumps |
|---|---|---|
| `FUTURE_CORE_TRACE_BATCHES=N` | `internal/pipeline/trace.go` | The batcher's per-frame batch list after `Flush()`: target ID, texture ID, shader, filter, blend, vertex and index counts for each batch. Useful for confirming what the batcher actually handed to the sprite pass. |
| `FUTURE_CORE_TRACE_PASSES=N` | `internal/pipeline/trace.go` | Every `BeginRenderPass` call the sprite pass makes, with target ID, load action, viewport, and clear color. Useful for catching "the same target is entered twice per frame" and "the load action is wrong" bugs. |
| `FUTURE_CORE_TRACE_WEBGPU=N` | `internal/backend/webgpu/trace_js.go` | Every browser-WebGPU encoder call for the first N frames: `BeginRenderPass`, `SetPipeline` (with blend + format), `bindUniforms` (with dynamic offset and first 8 bytes of payload), `SetTexture`, `DrawIndexed`, `Flush`. WASM-only. Use when the batch/pass traces show the right sequence but pixels still look wrong — this dumps the actual WebGPU calls leaving the encoder, so you can diff against a hand-written baseline (e.g. `parity-tests/wgsl-lighting-demo/`). |
| `FUTURE_CORE_DIAG_LOGS=N` | `internal/backend/webgpu/encoder_js.go`, `pipeline_js.go` | Per-call-site `console.log` budget (N entries total per run, shared across `BeginRenderPass`, `Flush`, and `createRenderPipeline`). Coarser than `FUTURE_CORE_TRACE_WEBGPU` — no per-frame structure — but catches early-init paths where the tracer's frame counter never advances (e.g. blank canvas, missing screen pass, silent pipeline creation). Emits to browser devtools via `console.log`. The always-on `uncapturederror` handler installed in `device_js.go:Init` catches WebGPU validation errors that would otherwise be swallowed (attachment size mismatches, pipeline/pass format incompatibility); it was how the always-on depth-stencil approach was diagnosed. |
| `go test -v -run TestDumpPointLightWGSL ./internal/shadertranslate/` | `internal/shadertranslate/tooling_dump_wgsl_test.go` | Prints every stage of the Kage → GLSL → WGSL translation for the lighting point-light shader, plus the combined uniform layout and the final post-merge WGSL actually sent to `createShaderModule`. Use when you suspect the translator is emitting something different from what you think it is. |
| `FUTURE_CORE_TRACE_TEXT=1` | (text package) | Logs glyph creation, draw positions, target sizes, and color scale values. Diagnoses invisible text by confirming glyphs are rasterized and DrawImage is called. |
| `FUTURE_CORE_NO_AA=1` | (image.go) | Bypasses `drawTrianglesAA` entirely, routing all `AntiAlias=true` draws through the aliased path. Useful for confirming whether a visual bug is caused by the AA buffer lifecycle. |
| `FUTURE_CORE_AA_SCALE=1` | (image.go) | Uses 1x AA buffers instead of 2x. Keeps the AA pipeline active but eliminates supersample quality. Isolates buffer-size-related issues. |
| `FUTURE_CORE_NO_ATLAS=1` | (sprite_atlas.go) | Disables sprite atlas packing. Each `NewImageFromImage` gets its own texture. Isolates atlas-related UV or texture-sharing bugs. |
| `FUTURE_CORE_VK_READSCREEN_DUMP=1` | `internal/backend/vulkan/device_gpu.go` | Vulkan-only. After every `ReadScreen`, prints 9 pixel values (3x3 grid across the captured image) to stderr. Answers "is the captured PNG all-white because the GPU image is actually all-white, or because readback is broken?" — which was the crux of the scene-selector triage. Free when unset; zero allocations in the hot path. |
| `FUTURE_CORE_VK_DUMP_SHADERS=<dir \| 1>` | `internal/backend/vulkan/shader_debug.go` | Vulkan-only. Writes each compiled Kage shader's GLSL source to `<dir>/<sha1>.vert.glsl` and `<sha1>.frag.glsl` on first compile (deduped by content hash). `1` or `true` dumps to `$TMPDIR`. Confirms what shaderc actually compiled — useful when Kage emission, shaderir changes, or build-time injection is in doubt. Pair with `spirv-dis` on the resulting `.spv` for a full IR trace. Was the tool that proved the Kage→GLSL path for light shaders matches the hand-written baseline during the varying-interpolation hunt. |
| `FUTURE_CORE_VK_UNIFORM_PROBE=<name>` | `internal/backend/vulkan/shader_debug.go` | Vulkan-only. On every `packUniformBuffer`, if a uniform field matches `<name>`, logs its byte offset, size, and packed bytes to stderr. Lets you confirm a uniform value reaches the GPU at the offset the SPIR-V expects — the probe that caught the `[3]float32` / vec3-std140 bug in the lighting demo. Bytes are hex, easiest to eyeball by decoding as little-endian float32 (`cd cc cc 3e` → 0.4). |
| `FUTURE_CORE_VK_TRACE_PIPELINES=1` | `internal/backend/vulkan/shader_debug.go` | Vulkan-only. Logs each `VkPipeline` creation and each `SetPipeline` call with vertex/fragment source hashes (matching `FUTURE_CORE_VK_DUMP_SHADERS`'s filenames) + the VkPipeline handle. Cross-references which pipeline is bound at which draw, useful when suspecting wrong pipeline-module pairing (e.g. built-in sprite vertex getting paired with a Kage fragment module). Output like `vulkan: pipeline bind pip=46176183296 vert=118a71f46a1d frag=a154e707c211`; grep-friendly. |

Typical debugging session — capture both traces for one frame of the
`rttest` repro program:

```bash
cd future-core
go build -o rttest ./cmd/rttest
FUTURE_CORE_TRACE_BATCHES=1 FUTURE_CORE_TRACE_PASSES=1 \
  WGPU_NATIVE_LIB_PATH=/opt/homebrew/lib \
  FUTURE_CORE_BACKEND=webgpu \
  FUTURE_CORE_HEADLESS=1 \
  FUTURE_CORE_HEADLESS_OUTPUT=/tmp/rttest.png \
  ./rttest 2>&1 | head -40
```

Expected output (fragment):
```
=== frame 1: 6 batches ===
  batch[0] target=2 texture=1 shader=0 filter=0 blend=1 verts=4 indices=6
  batch[1] target=2 texture=0 shader=0 filter=0 blend=1 verts=20 indices=30
  batch[2] target=0 texture=2 shader=0 filter=0 blend=1 verts=4 indices=6
  ...
[frame 1] BeginRenderPass target=2 load=load viewport=48x48 clear=[0.00 0.00 0.00 0.00]
[frame 1] BeginRenderPass target=0 load=clear viewport=256x256 clear=[0.00 0.00 0.00 1.00]
[frame 1] BeginRenderPass target=3 load=load viewport=48x48 clear=[0.00 0.00 0.00 0.00]
[frame 1] BeginRenderPass target=0 load=load viewport=256x256 clear=[0.00 0.00 0.00 0.00]
```

Reading the pass-boundary trace: target=0 is the screen/surface; non-zero
targets are offscreen render targets. A `load=clear` on target=0 is only
correct on the FIRST pass that touches the screen each frame — subsequent
screen passes must use `load=load` or they wipe prior screen content. The
sprite pass enforces this via a per-frame `screenCleared` flag.

In the browser, `os.Stderr` output is routed to the JavaScript console, so
the same env vars work in WASM as long as you set them on `go.env` in
`wasm_exec.js` before `go.run()`:

```js
const go = new Go();
go.env = go.env || {};
go.env.FUTURE_CORE_TRACE_BATCHES = "3";
go.env.FUTURE_CORE_TRACE_PASSES = "3";
```

Both tracers have zero cost when the env vars are unset: they resolve an
integer limit once at package init and the hot path checks a simple
integer comparison.

**Diagnostic workflow for "why is some draw missing" bugs:** capture
`FUTURE_CORE_TRACE_BATCHES` output from (a) a minimal program that
reproduces the bug and (b) a full program that's known to work on the
same backend. Diff the batcher command streams. Structural differences
(e.g., how often `target=0` is re-entered, which targets are allocated
mid-frame, batch ordering) usually point straight at the bug. This was
how the sprite-pass screen-re-entry bug was found.

### Profiling

Set `FUTURE_CORE_PPROF=:6060` to start a pprof HTTP server alongside the
engine. Only available on desktop builds (no-op in WASM; use Chrome
DevTools → Performance tab for browser profiling).

**Helper scripts** (`scripts/profile.sh` and `scripts/profile-compare.sh`)
automate the full capture-and-compare workflow:

```bash
# Capture a named profile (5s CPU + allocs + heap):
./scripts/profile.sh -n baseline -d 5

# ... make changes ...

# Capture the optimized variant on a different port:
./scripts/profile.sh -n optimized -d 5 -P 6061

# Compare side-by-side with a structured delta table:
./scripts/profile-compare.sh testdata/profiles/baseline testdata/profiles/optimized
```

`profile.sh` builds the target program, starts it headless with pprof,
captures CPU + allocation + heap profiles, generates text summaries, and
saves everything to `testdata/profiles/<name>/`. The profile directory is
gitignored. Run `./scripts/profile.sh -h` for all options.

**Manual workflows** (when you need finer control):

```bash
# Start any program with pprof:
FUTURE_CORE_PPROF=:6060 FUTURE_CORE_BACKEND=webgpu ./myprogram

# CPU profile (30 seconds):
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Heap allocations:
go tool pprof http://localhost:6060/debug/pprof/allocs

# Goroutine dump:
curl http://localhost:6060/debug/pprof/goroutine?debug=2

# Diff two captures interactively:
go tool pprof -diff_base=baseline/cpu.pb.gz optimized/cpu.pb.gz
```

For WASM browser profiling, use Chrome DevTools → Performance tab. The
Go runtime in WASM exposes GC pauses and goroutine activity through the
standard Chrome profiler.

### Prerequisites

- Go 1.24+
- [golangci-lint](https://golangci-lint.run/welcome/install/) (for `make lint` and `make fix`)
- GLFW 3 system library (for desktop builds):
  - Debian/Ubuntu: `sudo apt-get install libglfw3-dev`
  - Fedora: `sudo dnf install glfw-devel`
  - Arch: `sudo pacman -S glfw`
  - macOS (Homebrew): `brew install glfw`
  - Windows: place `glfw3.dll` in PATH

### CI

GitHub Actions runs `make ci` on every push and PR to `main`. The workflow
lives at `.github/workflows/ci.yml` and runs: format check → vet → lint →
test → coverage check → test-race → build.

Linter configuration is in `.golangci.yml`. Key enabled linters beyond
defaults: `gocritic`, `revive`, `errname`, `errorlint`, `exhaustive`,
`goimports`, `misspell`, `prealloc`, `unparam`.

There are no external Go dependencies yet (`go.mod` has only the standard
library). Desktop platform code (GLFW, OpenGL) is gated by OS-based build
constraints (`//go:build darwin || linux || freebsd || windows`) and compiles
automatically on desktop platforms — no `-tags` flags needed.

### `make lint` Path Resolution (fixed)

Earlier revisions reported typechecking errors like "stat
.../github.com/.../internal/platform: directory not found" from
`golangci-lint` and exited with code 7. Root cause: the Makefile derived
`MODULE` from `go list -m`, which — when a parent `go.work` is present —
emits both `future` and `future-core`, and `$(shell …)` captured only the
first line (`future`). The `sed` that converts `<module>/<pkg>` to
`./<pkg>` then silently no-op'd and golangci-lint received fully
qualified import paths it couldn't resolve. Fixed by parsing `go.mod`
directly:

```make
MODULE := $(shell awk '/^module / {print $$2; exit}' go.mod)
```

### gopls LSP noise on android/386

gopls cross-compiles against every GOOS/GOARCH combo it recognises, so
diagnostics tagged `[android,386]` may appear in editors for files that
import CGo-requiring deps (ebiten, x/mobile, chromedp). This project
officially targets `android/arm64`, not `android/386` — neither `make`,
`make build-android`, CI, nor any committed build step touches android/386.
The diagnostics are IDE-only noise: ignore them unless you are
intentionally adding android/386 support.

### Known CI Limitation: Audio Packages Excluded

The `audio/` package depends on `github.com/ebitengine/oto/v3`, which uses
CGo and requires ALSA development headers (`libasound2-dev` / `alsa.pc`) on
Linux. These headers are not installed in the CI environment, so **all audio
packages (`audio/`, `audio/mp3/`, `audio/vorbis/`, `audio/wav/`) are excluded** from the
default `make` targets (vet, lint, test, build, coverage).

The exclusion is implemented in the `Makefile` via the `PKGS` and `LINT_PATHS`
variables, which filter out packages matching `/audio`. The CI workflow
(`.github/workflows/ci.yml`) delegates linting to `make lint` so it respects
the same exclusion.

**To resolve this in the future**, choose one of:
1. **Install ALSA headers in CI** — add `sudo apt-get install -y libasound2-dev`
   to the workflow, then remove the `grep -v /audio` filters from the Makefile.
2. **Use a build tag** — gate audio packages behind `//go:build audio`, so they
   are excluded by default and only built/tested with `-tags audio`.
3. **Use a pure-Go audio backend** — replace `oto/v3` with a backend that
   doesn't require CGo, eliminating the system dependency entirely.

Until resolved, to test audio locally you need ALSA headers installed:
```bash
# Ubuntu/Debian
sudo apt-get install libasound2-dev

# Then test audio packages directly
go test ./audio/...
```

## Architecture Rules

These are non-negotiable. Violating them creates technical debt that compounds.

1. **Layer direction is strictly downward.** No package may import from a layer
   above it. The layers top-to-bottom: public API (`engine.go`, `image.go`,
   `input.go`) → `internal/pipeline` → `internal/batch` →
   `internal/backend` → `internal/platform`.

2. **Backend types never leak to game code.** The public API uses
   engine-specific types (`BlendMode`, `Filter`) that map to internal backend
   types. Users never import `internal/`.

3. **No 2D-only assumptions in internal layers.** The backend, pipeline, and
   batch systems must work for both 2D and 3D. Read `FUTURE_3D.md` "What
   Phase 1 Must NOT Do" before changing internal packages.

4. **No CGo in core packages.** `math/`, `internal/batch/`,
   `internal/pipeline/`, `internal/input/` must remain pure Go. All native
   API bindings (Vulkan, Metal, DX12, WebGPU, OpenGL, Cocoa, Win32, GLFW)
   use **purego** for dynamic symbol loading — no CGo. The entire project
   is CGo-free (except for the `audio/` package which uses `oto/v3`).
   GLFW is loaded as a system shared library on all platforms
   (`libglfw3-dev` on Debian/Ubuntu, `libglfw.3.dylib` on macOS,
   `glfw3.dll` on Windows).

5. **Interfaces are defined by consumers, not implementors.** Follow Go
   interface design conventions. Keep interfaces small and focused.

## Multi-Backend Architecture

Seven backends implement the `backend.Device` and `backend.CommandEncoder`
interfaces. Read `internal/backend/CLAUDE.md` for detailed backend
development guidance.

### Backend Registry

All backends self-register via `init()` in their `register.go` files using
`backend.Register(name, factory)`. The engine selects a backend via the
`FUTURE_CORE_BACKEND` env var (values: `opengl`, `webgl`, `vulkan`,
`metal`, `webgpu`, `dx12`, `soft`, `auto`).

### Soft-Delegation Pattern

Five backends (webgl, vulkan, metal, webgpu, dx12) delegate rendering to
the software rasterizer (`internal/backend/soft/`). This lets all backends
pass the 10-scene conformance suite in CI without GPU hardware. Each backend
wraps soft types and adds API-specific constants/types for the target GPU API.

**When converting a soft-delegating backend to real GPU bindings**: replace
the `inner` delegation in each method with actual GPU API calls. The type
structure, registration, conformance tests, and coverage are already in place.

### Conformance Testing

Every backend must pass `conformance.RunAll(t, dev, enc)` which renders
10 canonical scenes and compares pixel output against golden PNGs (±3
tolerance). Golden images are auto-generated on first run. See
`internal/backend/conformance/conformance.go` for the full scene list.

### Backend Coverage

| Backend | Package | Coverage | Conformance |
|---|---|---|---|
| Software | `internal/backend/soft/` | 91% | 10/10 |
| OpenGL | `internal/backend/opengl/` | (build-tagged) | N/A in CI |
| WebGL2 | `internal/backend/webgl/` | 92% | 10/10 |
| Vulkan | `internal/backend/vulkan/` | 92% | 10/10 |
| Metal | `internal/backend/metal/` | 90% | 10/10 |
| WebGPU | `internal/backend/webgpu/` | 92% | 10/10 |
| DirectX 12 | `internal/backend/dx12/` | 90% | 10/10 |

## Development Workflow

Follow this cycle for every change:

### 1. Understand Before Changing
- Read the relevant source files before modifying them
- Check `DESIGN.md` to understand where the change fits architecturally
- Check `FUTURE_3D.md` constraints if touching internal packages

### 2. Implement
- Make the minimal change needed
- Prefer editing existing files over creating new ones
- Don't add features, abstractions, or "improvements" beyond what was asked
- No empty files, placeholder packages, or premature abstractions

### 3. Test & Lint
- Run `make` after every change (runs fmt, vet, lint, test, cover-check, build)
- If iterating quickly, use `make test` alone, then `make` before committing
- **All changes require test coverage.** Aim for 100% on new code; the CI
  enforces a minimum of 80% per package. Use `make cover` to check.
- Use mock devices/interfaces to test GPU code paths without OpenGL
  (see `mockDevice` in `image_test.go` for the pattern)
- All checks must pass before committing

### 4. Verify Build
- `make build` ensures everything compiles (included in `make`)
- If adding platform-specific code, verify build tags work

### 5. Verify CI Parity Before Push
- **Do not push unless you are confident CI will pass.** CI runs with
  `TAGS=soft` and X11 development headers installed. Before pushing, run:
  ```bash
  make TAGS=soft fmt
  make TAGS=soft vet
  make TAGS=soft lint
  make TAGS=soft test
  make TAGS=soft cover-check
  make TAGS=soft build
  ```
- If GLFW is not installed locally, install it first:
  ```bash
  sudo apt-get install -y libglfw3-dev
  ```
- The CI workflow (`.github/workflows/ci.yml`) is the source of truth.
  Review it before pushing to ensure your changes match what CI validates.
- **Never assume local `make` passing is sufficient.** The CI uses
  `-tags soft` which changes which files are compiled.

### 5b. Cross-Backend Regression Check (MANDATORY for anything that
touches rendering)

Any change that touches the rendering pipeline — `internal/backend/*`,
`internal/pipeline/*`, `internal/batch/*`, `image.go`, `renderer.go`,
the sprite shaders, the vector package, or any ColorScale / blend /
premultiplication path — **must be verified to not regress any
backend the change didn't intend to affect.** A fix for WebGPU that
silently breaks Vulkan is a worse outcome than no fix at all.

Required verification before commit:

```bash
# 1. Soft backend conformance (10-scene golden-image suite, CI-equivalent)
cd future-core && make TAGS=soft test

# 2. Native backend conformance on the host platform (GPU-hardware tests)
go test ./internal/backend/webgpu/... ./internal/backend/vulkan/... \
       ./internal/backend/metal/...   ./internal/backend/opengl/... \
       ./internal/backend/webgl/...   ./internal/backend/dx12/...

# 3. Cross-backend parity on the future app (broadest visual signal)
cd .. && node parity-tests/native/runner.mjs
```

The native parity runner at `parity-tests/native/runner.mjs` orchestrates
the `future` desktop binary across backends with WebGPU as the
reference. Scenarios live in `parity-tests/native/scenarios/`. A
backend that was previously passing must continue to pass — if your
change drops a previously-green backend, treat it as a ship-blocker
unless the drop is the explicit intent and documented in the PR.

**When a backend you didn't touch regresses:** stop, revert the last
change, and investigate. Rendering changes have non-local effects via
shared abstractions (ColorScale / blend / FillRule / stencil state).
The cost of shipping a silent cross-backend regression (debugging it
later, blame landing on an unrelated PR) vastly outweighs the cost of
re-running the parity suite before pushing.

If a backend in the parity matrix is known-broken on the dev host
(e.g. DX12 on macOS, OpenGL in a headless container), note that in the
PR description and rely on CI to cover it — but don't skip the
backends the host *can* run.

### 6. Update Docs
- Update `ROADMAP.md` when completing milestone tasks
- Don't create new markdown files unless explicitly asked

### Loop: make → fix → make
If any check fails, fix the issue and re-run `make`. Don't commit broken
code. Don't skip tests. Don't use `-count=0` or other tricks to hide
failures. Use `make fix` to auto-fix formatting and lint issues.

## Code Style

- Standard Go formatting enforced by `gofmt` and `goimports` (via golangci-lint)
- Error returns use `(T, error)` pattern, not panics
- Exported types and functions have doc comments
- Internal packages use `internal/` path convention
- Test files are `*_test.go` in the same package
- Benchmarks use `Benchmark*` naming convention

## Test Writing Rules

**All tests use [testify](https://github.com/stretchr/testify) with `require`
(Must-style) assertions.** This is non-negotiable.

### Required patterns

1. **Use `require` (not `assert`) for all assertions.** `require` stops the
   test immediately on failure, preventing cascading errors and nil panics.

   ```go
   import "github.com/stretchr/testify/require"

   func TestSomething(t *testing.T) {
       result, err := DoThing()
       require.NoError(t, err)
       require.Equal(t, expected, result)
   }
   ```

2. **Never use raw `t.Errorf` / `t.Fatalf` / `if` checks.** Always use
   `require.*` functions instead.

   ```go
   // BAD — do not do this
   if got != want {
       t.Errorf("got %v, want %v", got, want)
   }

   // GOOD
   require.Equal(t, want, got)
   ```

3. **Use `require.InEpsilon` or `require.InDelta` for float comparisons.**

   ```go
   require.InDelta(t, 1.0, result, 1e-6)
   require.InEpsilon(t, expected, actual, 1e-6)
   ```

4. **Table-driven tests** use `t.Run` with `require`:

   ```go
   tests := []struct{ name string; in, want float64 }{
       {"positive", 2, 4},
       {"zero", 0, 0},
   }
   for _, tt := range tests {
       t.Run(tt.name, func(t *testing.T) {
           require.InDelta(t, tt.want, compute(tt.in), 1e-9)
       })
   }
   ```

5. **Test function naming**: `Test<Type><Method>` or `Test<Function>`, e.g.
   `TestVec2Add`, `TestMat4Inverse`, `TestNewImage`.

### Forbidden patterns

- `t.Errorf`, `t.Fatalf`, `t.Error`, `t.Fatal` — use `require.*` instead
- `if got != want { t.Errorf(...) }` — use `require.Equal`
- `assert.*` — use `require.*` (fail immediately, not at end)
- Manual epsilon comparisons — use `require.InDelta` / `require.InEpsilon`

## Naming Conventions

- Public API types match Ebitengine where applicable: `Game`, `Image`,
  `GeoM`, `DrawImageOptions`, `Vertex`, `Key`, `MouseButton`
- Math types use short names: `Vec2`, `Vec3`, `Mat3`, `Mat4`, `Quat`
- Backend interfaces use GPU terminology: `Device`, `Texture`, `Buffer`,
  `Shader`, `Pipeline`, `CommandEncoder`
- Platform interfaces: `Window`, `InputHandler`

## Commit Messages

- Use imperative mood: "Add sprite pass" not "Added sprite pass"
- First line under 72 characters
- Reference the milestone when relevant: "M2: wire DrawImage to batcher"

## Common Pitfalls

- **Don't hardcode orthographic projection** in pipeline internals — projection
  matrix must be a parameter
- **Don't assume Vertex2D is the only format** — batcher and pipeline must
  support arbitrary vertex formats
- **Don't tie render targets to screen size** — off-screen targets can be any
  dimension
- **Don't remove depth/3D fields** from backend types even though Phase 1
  doesn't use them
- **Don't merge pipeline and backend layers** — their separation is essential
  for 3D
- **Don't add Ebitengine as a dependency** — this is a clean-room implementation
- **Don't request Vulkan extensions without checking availability** — on macOS
  (MoltenVK), `VK_KHR_portability_enumeration` may not be present. Always use
  `vk.EnumerateInstanceExtensionProperties()` to check before requesting.
- **Vulkan deferred execution** — unlike OpenGL/soft where draws execute
  immediately, Vulkan records into command buffers. Vertex/index/uniform
  buffers must use ring-buffer offsets so each draw references distinct data.
  Overwriting a buffer between recording and submission corrupts all draws.
- **Vulkan descriptor pools must outlive GPU execution** — don't destroy/reset
  the pool in `EndRenderPass`. Defer to `BeginFrame` (after fence wait).
  Destroying early causes the GPU to read freed descriptors (zeros).
- **Never leave debug code in `bindUniforms`** — filling the UBO with identity
  matrices or debug patterns overwrites actual uniform data every frame.
- **Only Vulkan and WebGPU use `NoGL`** — Metal and DX12 still use the GL
  presenter (soft-delegation → ReadScreen → GL blit). Setting `needsNoGL`
  for non-Vulkan/WebGPU backends breaks their display path.
- **Always use `scripts/visual-test.sh`** for rendering validation, not
  `go run` directly. The script handles headless capture via env vars.
- **WebGPU has three build modes** — soft (CI), native GPU (`_gpu.go` via
  wgpu-native/purego), and browser (`_js.go` via `syscall/js`/`navigator.gpu`).
  Soft files use tag `(!(desktop) && !js) || soft`; GPU files use
  `desktop && !soft`; JS files use `js && !soft`. All three must compile when
  modifying the webgpu package. Verify with: `go build -tags soft`,
  `go build`, `GOOS=js GOARCH=wasm go build`.
- **WebGPU uniform ring buffer** — the native GPU path uses a 16 KB persistent
  buffer with 256-byte-aligned cursor, reset per-frame in `BeginFrame`. The
  browser path creates per-draw temporary buffers (JS GC handles cleanup).
- **WebGPU shader translation** — GLSL→WGSL via `internal/shadertranslate/wgsl.go`.
  Most GLSL built-ins pass through unchanged (WGSL has identical names).
  `mod(x, y)` needs active translation to `(x % y)`. Kage image builtins
  (`imageSrc0At`, etc.) are emitted as WGSL helper functions with
  `textureSampleLevel`. The translator does not support array uniforms or
  custom function definitions. WGSL output is validated by `naga-cli` in
  CI (see `wgsl_naga_test.go`).
  **2D assumptions in the translator** (must be revisited for 3D):
  - `textureSampleLevel(..., 0.0)` hardcodes LOD=0 everywhere. For 3D
    mipmapped textures, use `textureSample` (automatic LOD from
    derivatives) or compute LOD explicitly.
  - All textures are assumed `texture_2d<f32>`. 3D will need
    `texture_3d`, `texture_cube`, and `texture_depth_2d`.
  - Vertex format is hardcoded to `Vertex2D` (pos, uv, color). 3D
    will need normals, tangents, bone weights.
- **Known test requirement**: WebGPU native GPU tests require `libwgpu_native`
  at runtime. See `internal/backend/webgpu/GPU_TESTING.md` for the 7-tier
  validation checklist.
- **wgpu-native v27 API breaks** — All enums shifted by +1 (old `0` values are
  now `Undefined`; e.g. `LoadOp_Clear` is `2` not `1`, `TriangleList` is `4`
  not `3`). `WGPUFlags` (TextureUsage, BufferUsage, ColorWriteMask, ShaderStage)
  changed from `uint32` to `uint64`. Descriptor structs gained `StringView label`
  fields (16 bytes, not `uintptr`). Always cross-reference enum/struct values
  against the installed header: `grep "WGPUFoo_" /opt/homebrew/Cellar/wgpu-native/*/include/webgpu.h`.
- **wgpu-native struct-by-value on ARM64** — `wgpuInstanceRequestAdapter`,
  `wgpuAdapterRequestDevice`, and `wgpuBufferMapAsync` take `CallbackInfo`
  structs by value (>16 bytes). On ARM64, these are passed by hidden pointer.
  Use `purego.SyscallN` with `uintptr(unsafe.Pointer(&info))`, not flattened
  fields. `purego.RegisterFunc` cannot handle struct-by-value parameters.
- **wgpu-native library path** — Set `WGPU_NATIVE_LIB_PATH` env var to point
  to the directory or file path of `libwgpu_native.dylib`. On macOS with
  Homebrew: `WGPU_NATIVE_LIB_PATH=/opt/homebrew/Cellar/wgpu-native/*/lib`.
- **WebGPU `DepthSlice` must be `0xFFFFFFFF`** for 2D render targets — v27
  interprets `0` as "depth slice 0 of a 3D texture", causing validation errors.
  Set `DepthSlice: 0xFFFFFFFF` (WGPU_DEPTH_SLICE_UNDEFINED) in
  `RenderPassColorAttachment`.
- **WebGPU `TextureAspect` must be `1` (All)** — v27 changed `All` from `0`
  to `1`. Using `0` (now `Undefined`) silently produces zero-filled readbacks.
- **WebGPU `TextureDimension` 2D is `2`** — v27 changed from `1` to `2`.
  Using `1` (now `1D`) causes "Dimension Y exceeds limit" errors.
- **Use `make build` not `go build ./...`** — the Makefile handles build
  tag filtering and package exclusions (e.g., audio).
- **WebGPU browser performance** — the JS path has several critical
  optimizations: (1) direct canvas presentation via `GPUCanvasContext`
  (skip `ReadScreen`+`putImageData`), (2) 64 KB uniform ring buffer with
  `hasDynamicOffset` bind groups, (3) texture bind group cache by
  `(textureID, filter)`, (4) pre-allocated `Uint32Array`/`Uint8Array` for
  per-draw JS calls. When modifying the WebGPU JS encoder, preserve these
  patterns — per-draw `createBuffer` or `createBindGroup` calls cause severe
  FPS drops, especially on mobile.
- **Sprite atlas** — `NewImageFromImage` automatically packs small images
  (≤256px) into shared atlas textures (`sprite_atlas.go`). Atlased images
  share a `textureID`, so the batcher merges their draws. Atlased images do
  not support `WritePixels`/`Set`/`ReadPixels` (no-ops). Disable with
  `SetSpriteAtlasEnabled(false)`. Tests that inspect per-image textures
  should disable atlasing in their setup (see `withMockRenderer` pattern).
- **`Fill(transparent)` with SourceOver is a no-op** — `src*0 + dst*1 = dst`,
  so `Image.Clear()` does NOT reset a render target's backing texture.
  Fresh RTs read undefined memory on first `LoadActionLoad`. To actually
  zero a target, upload zeros via `texture.Upload(zeros, 0)` (bypasses
  blending) or overwrite every pixel with an opaque draw.
- **Offscreen render targets start with undefined contents** — the sprite
  pass uses `LoadActionLoad` for non-screen targets (the per-frame
  `screenCleared` guard only covers `target=0`). Code that allocates an
  RT mid-frame and reads it without first covering every needed pixel
  will read uninitialized memory.
- **Don't `Dispose()` a mid-frame-allocated render target synchronously** —
  the batcher still holds draw commands referencing it. When the sprite
  pass runs `Flush()` it will resolve a nil render target and pass it to
  `BeginRenderPass`, which on native WebGPU panics with "no color
  attachments". Use `renderer.deferDispose(img)` instead; the engine
  drains the queue via `disposeDeferred()` after `EndFrame()`.
- **Anti-aliasing via `drawTrianglesAA`** — `DrawTrianglesOptions.AntiAlias`
  routes into a per-`Image` 2x-supersample buffer (`aaBuffer`), sized to
  the full image and persistent across frames (matching Ebitengine's
  `bigOffscreenBuffer`). Flushed back via a linear-filtered downsample
  quad at sync points (any public method that reads/writes the target's
  texture). After each flush, `pendingClearTracker.Request()` registers
  a clear so the next AA draw starts clean — this is a **counter**, not
  a boolean, because the AA buffer can be flushed and re-entered 17+
  times per frame. `Clear()` uses `RequestOnce()` (idempotent) to avoid
  double-clearing when called multiple times per frame (e.g.,
  `imageClearWrapper` + `Frame.Draw`). A blend change triggers a flush.
  `Clear()` on the parent sets `aaBufferNeedsClear` so the buffer is
  cleared before the next AA draw. Sub-images can't own a buffer and
  fall back to aliased rendering.
- **`DrawTriangles` with `src=nil` uses `whiteTextureID`** — untextured
  draws (e.g., vector fills/strokes with vertex colors) must bind the
  white texture so vertex colors pass through unmodified. Using `texID=0`
  causes the sprite pass to skip texture binding (since `lastTextureID`
  starts at 0), leaving whatever texture was previously bound.
- **Sprite pass sets filter BEFORE texture** — `SetTextureFilter` must be
  called before `SetTexture` because the WebGPU encoder reads the current
  filter when creating the texture bind group. If reversed, the bind group
  uses the stale filter value (e.g., AA downsample gets nearest instead
  of linear).
- **`pendingClearTracker` uses Request vs RequestOnce** — `Request()`
  accumulates (each AA flush needs its own clear). `RequestOnce()` is
  idempotent (multiple `Clear()` calls per frame should produce one
  clear, not N). Using the wrong method causes either stale AA content
  (boolean collapse) or over-clearing (background wipe). Both bugs are
  covered by `renderer_test.go`.

## Test Coverage Requirements

**All changes require test coverage.** This is enforced by CI.

- **Target: 100%** — Aim for full coverage on every new function and branch.
- **Minimum: 80%** — CI fails if any package with test files drops below 80%.
  This is enforced by `make cover-check`, which runs as part of `make` / `make ci`.
- **No untested code ships.** If you add a function, add tests for it. If you
  modify a function, verify its existing tests still cover the changed paths.

### Per-Package Guidelines

| Package | Minimum | Target | Notes |
|---|---|---|---|
| `math/` | 80% | 100% | Pure functions, easy to test exhaustively |
| `internal/batch/` | 80% | 100% | Core optimization logic, must be correct |
| `internal/pipeline/` | 80% | 100% | Test pass ordering, context, sprite pass |
| `internal/input/` | 80% | 100% | Test state transitions, edge detection |
| `internal/backend/` | 80% | — | Interface definitions + registry; minimal tests |
| `internal/backend/soft/` | 80% | 100% | CPU rasterizer + Device impl; reference backend for conformance |
| `internal/backend/conformance/` | 80% | 100% | Golden-image test framework; exercises full pipeline |
| `internal/backend/webgl/` | 80% | 100% | WebGL2 soft-delegating backend; conformance + unit tests |
| `internal/backend/vulkan/` | 80% | 100% | Vulkan soft-delegating backend; conformance + unit tests |
| `internal/backend/metal/` | 80% | 100% | Metal soft-delegating backend; conformance + unit tests |
| `internal/backend/webgpu/` | 80% | 100% | WebGPU triple-mode backend (soft/native GPU/browser JS); conformance + unit tests; see `GPU_TESTING.md` |
| `internal/backend/dx12/` | 80% | 100% | DirectX 12 soft-delegating backend; conformance + unit tests |
| `internal/platform/` | Excluded | — | Interface definitions only; implementations tested via integration |
| Public API (root) | 80% | 100% | Image, GeoM, DrawImage, options, type mapping |

### Testing GPU Code Without OpenGL

Use mock implementations of `backend.Device` and `backend.Texture` to test
GPU code paths in unit tests. See `image_test.go` for the established pattern:
`mockDevice`, `mockTexture`, and the `withMockRenderer` helper.

### Conformance Testing (Golden Images)

The golden-image conformance framework in `internal/backend/conformance/`
verifies that any `backend.Device` implementation produces correct pixel
output. It renders 10 canonical scenes and compares against reference PNG
images with a per-channel tolerance of ±3.

**Running conformance tests:**
```bash
go test ./internal/backend/conformance/ -v   # Run against soft backend
```

**Adding a new backend to conformance:**
```go
// In your_backend_test.go:
func TestConformance(t *testing.T) {
    dev := yourbackend.New()
    require.NoError(t, dev.Init(backend.DeviceConfig{
        Width: conformance.SceneSize, Height: conformance.SceneSize,
    }))
    defer dev.Dispose()
    conformance.RunAll(t, dev, dev.Encoder())
}
```

**Updating golden images** (after intentional rasterizer changes):
```bash
rm internal/backend/conformance/testdata/golden/*.png
go test ./internal/backend/conformance/ -v   # Regenerates all goldens
```

**On failure**, the framework saves `_actual.png` and `_diff.png` artifacts
in `testdata/golden/diff/` for visual debugging.

**Test scenes** cover: clear, solid triangles, vertex-color interpolation,
textured quads, blend modes (source-over, additive), scissor clipping, and
orthographic projection.

### Coverage Commands

```bash
make cover        # Print per-package coverage summary
make cover-check  # Enforce 80% minimum (part of CI)
make cover-html   # Generate HTML report at coverage.html
```
