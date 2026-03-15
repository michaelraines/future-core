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

### Prerequisites

- Go 1.24+
- [golangci-lint](https://golangci-lint.run/welcome/install/) (for `make lint` and `make fix`)

### CI

GitHub Actions runs `make ci` on every push and PR to `main`. The workflow
lives at `.github/workflows/ci.yml` and runs: format check → vet → lint →
test → coverage check → test-race → build.

Linter configuration is in `.golangci.yml`. Key enabled linters beyond
defaults: `gocritic`, `revive`, `errname`, `errorlint`, `exhaustive`,
`goimports`, `misspell`, `prealloc`, `unparam`.

There are no external Go dependencies yet (`go.mod` has only the standard
library). When platform backends are added, they will use build tags
(`-tags glfw`).

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
   `internal/pipeline/`, `internal/input/` must remain pure Go. CGo is
   permitted only in `internal/backend/<impl>/` and `internal/platform/<impl>/`.

5. **Interfaces are defined by consumers, not implementors.** Follow Go
   interface design conventions. Keep interfaces small and focused.

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

### 5. Update Docs
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
| `internal/backend/` | Excluded | — | Interface definitions only; implementations tested via integration |
| `internal/platform/` | Excluded | — | Interface definitions only; implementations tested via integration |
| Public API (root) | 80% | 100% | Image, GeoM, DrawImage, options, type mapping |

### Testing GPU Code Without OpenGL

Use mock implementations of `backend.Device` and `backend.Texture` to test
GPU code paths in unit tests. See `image_test.go` for the established pattern:
`mockDevice`, `mockTexture`, and the `withMockRenderer` helper.

### Coverage Commands

```bash
make cover        # Print per-package coverage summary
make cover-check  # Enforce 80% minimum (part of CI)
make cover-html   # Generate HTML report at coverage.html
```
