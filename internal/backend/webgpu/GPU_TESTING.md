# WebGPU Backend — GPU Testing Checklist

The WebGPU GPU pipeline is fully wired but requires hardware validation
with `libwgpu_native` installed at runtime. This document tracks what
needs to be tested and how.

## Prerequisites

Install wgpu-native for your platform:

- **macOS**: `libwgpu_native.dylib` (e.g. `brew install wgpu-native`)
- **Linux**: `libwgpu_native.so` in `LD_LIBRARY_PATH`
- **Windows**: `wgpu_native.dll` in `PATH`

Download from: https://github.com/gfx-rs/wgpu-native/releases

### Configuring the library path

Set `WGPU_NATIVE_LIB_PATH` to point to the library file or the directory
containing it. If unset, the dynamic linker's default search is used.

```bash
# Exact file path
export WGPU_NATIVE_LIB_PATH=/opt/homebrew/Cellar/wgpu-native/27.0.4.0/lib/libwgpu_native.dylib

# Directory containing the library
export WGPU_NATIVE_LIB_PATH=/opt/homebrew/Cellar/wgpu-native/27.0.4.0/lib
```

## Test Tiers

### Tier 1 — Device Init (no rendering)

Validates adapter/device creation via `purego.NewCallback`:

```bash
go test ./internal/backend/webgpu/ -run TestDeviceInit -v
```

**What this tests:**
- `wgpu.Init()` loads `libwgpu_native`
- `InstanceRequestAdapterSync` callback fires and returns valid adapter
- `AdapterRequestDeviceSync` callback fires and returns valid device
- Queue, default texture, uniform ring buffer created
- Dispose releases all handles without crash

**Expected result:** PASS (currently skips if library not found)

### Tier 2 — Clear + ReadPixels

Validates the offscreen rendering path end-to-end:

```bash
go test ./internal/backend/webgpu/ -run TestWebGPUGPUClearAndRead -v
```

**Status:** Implemented in `webgpu_gpu_test.go` as `TestWebGPUGPUClearAndRead`.
Clears to red, calls `ReadScreen()`, verifies pixel values with tolerance for
potential channel swaps.

### Tier 3 — Shader Compilation

Validates GLSL→WGSL translation + `wgpuDeviceCreateShaderModule`:

```bash
go test ./internal/backend/webgpu/ -run TestWebGPUGPUShaderCompile -v
```

**Status:** Implemented in `webgpu_gpu_test.go` as `TestWebGPUGPUShaderCompile`.
Creates shader with standard sprite GLSL, calls `compile()`, verifies both
`vertexModule` and `fragmentModule` are non-zero.

### Tier 4 — Draw a Solid-Color Quad

Full pipeline: shader → pipeline → vertex buffer → draw → readback:

```bash
go test ./internal/backend/webgpu/ -run TestWebGPUGPUDrawGreenQuad -v
go test ./internal/backend/webgpu/ -run TestWebGPUGPUDrawWithSubmit -v
```

**Status:** Implemented in `webgpu_gpu_test.go` as `TestWebGPUGPUDrawGreenQuad`
(direct draw path) and `TestWebGPUGPUDrawWithSubmit` (full BeginFrame→Draw→
EndFrame→ReadScreen engine path).

**What these validate:**
- Pipeline creation (bind group layouts, pipeline layout, render pipeline)
- Vertex buffer binding
- Uniform ring buffer writes
- Draw call execution
- Pixel readback matches expected color

### Tier 5 — Conformance Suite (GPU mode)

Run the 10-scene conformance suite without the `soft` tag:

```bash
go test ./internal/backend/webgpu/ -run TestConformanceWebGPU -v
```

This exercises the full GPU pipeline for all scenes:
clear, triangles, vertex colors, textured quads, blending, scissor,
orthographic projection, multiple draw calls.

### Tier 6 — Visual Test (Windowed Presentation)

Validates surface/swapchain presentation:

```bash
./scripts/visual-test.sh -m gpu -b webgpu -e sprite
```

**What this validates:**
- SurfaceFactory creates a valid surface
- SurfaceConfigure with FIFO present mode
- BeginFrame acquires surface texture
- EndFrame presents to screen
- Multi-frame rendering loop

### Tier 7 — Window Resize

Validates resize handling:

1. Run a visual test
2. Resize the window manually
3. Rendering should adapt without crash (surface reconfigured)

### Tier 8 — Browser WebGPU (WASM)

Validates the `_js.go` browser path via `GOOS=js GOARCH=wasm`:

```bash
# 1. Build WASM binary
GOOS=js GOARCH=wasm go build -o main.wasm ./cmd/yourapp/

# 2. Copy the Go WASM exec support JS
cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" .

# 3. Serve with a local HTTP server (WebGPU requires HTTPS or localhost)
# 4. Open in Chrome/Edge (WebGPU enabled by default) or Firefox Nightly
```

**What this validates:**
- `navigator.gpu` availability detection
- Async `requestAdapter()` / `requestDevice()` via Promise callbacks
- `GPUCanvasContext.configure()` for presentation
- GLSL→WGSL shader translation + `device.createShaderModule()`
- `GPURenderPipeline` creation with bind group layouts
- Texture/buffer creation and upload via `queue.writeTexture/writeBuffer`
- Full render pass: command encoder → render pass → draw → submit
- Canvas presentation

**Browser requirements:**
- Chrome 113+ or Edge 113+ (WebGPU enabled by default)
- Firefox Nightly with `dom.webgpu.enabled` flag
- Safari Technology Preview (partial support)

## Known Limitations

- **Conformance golden images**: GPU mode may produce slightly different
  pixel values than the soft rasterizer due to GPU floating-point behavior.
  The ±3 tolerance should handle this, but new goldens may be needed.

- **Y-flip**: WebGPU uses top-left origin (same as Vulkan). The projection
  matrix may need Y-flip in `SetUniformMat4` for correct rendering.
  Currently not implemented — validate during Tier 4 testing.

- **GLSL→WGSL translator coverage**: The translator handles the standard
  sprite shader patterns used by the engine. Advanced shaders using
  built-in functions (sin, cos, mix, clamp, etc.) require the translator
  extensions. See the "Translator Limitations" section below.

## GLSL→WGSL Translator Limitations

The translator uses line-by-line regex matching. It handles the engine's
core sprite/text shader patterns but does not yet support:

| Feature | GLSL Example | Status |
|---|---|---|
| Basic types, attributes, uniforms, varyings | `uniform mat4 uProjection;` | Supported |
| Type constructors | `vec4(aPosition, 0.0, 1.0)` | Supported |
| Texture sampling | `texture(uTexture, vTexCoord)` | Supported |
| Local var declarations | `vec4 c = ...;` | Supported |
| Built-in math functions | `sin(x)`, `cos(x)`, `pow(x, y)` | Supported (pass-through) |
| Interpolation functions | `mix(a, b, t)`, `clamp(x, lo, hi)` | Supported (pass-through) |
| Vector functions | `length(v)`, `normalize(v)`, `dot(a, b)` | Supported (pass-through) |
| `discard` statement | `discard;` | Supported |
| Control flow | `if (x > 0) { ... }` | Supported (pass-through) |
| `mod()` function | `mod(x, y)` | Needs translation to `x % y` or `fract()` |
| Array uniforms | `uniform vec4 colors[16];` | Not supported |
| Custom functions | `float myFunc(float x) { ... }` | Not supported |
| `#define` / `#ifdef` | Preprocessor directives | Not supported |

**"Pass-through" means** the GLSL and WGSL names are identical, so no
translation is needed. WGSL supports `sin()`, `cos()`, `mix()`, `clamp()`,
`step()`, `smoothstep()`, `length()`, `normalize()`, `dot()`, `cross()`,
`pow()`, `sqrt()`, `abs()`, `floor()`, `ceil()`, `min()`, `max()`,
`fract()`, `sign()`, `exp()`, `log()`, `reflect()`, `refract()`,
`distance()`, and `discard` natively.

The only GLSL built-in that needs active translation is `mod(x, y)` →
WGSL `(x % y)` for float modulo, since WGSL uses the `%` operator.
