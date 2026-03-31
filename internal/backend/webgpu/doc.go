// Package webgpu implements backend.Device targeting the WebGPU API.
//
// WebGPU is a next-generation cross-platform GPU API that runs natively
// (via wgpu-native) and in browsers (via the WebGPU JS API). This backend
// has three build modes:
//
//   - Soft-delegation (CI / -tags soft): delegates to the software rasterizer
//     for conformance testing without GPU hardware.
//   - Native GPU (desktop, !soft): uses wgpu-native via purego bindings
//     (internal/wgpu/) for direct GPU rendering with surface presentation,
//     uniform ring buffer, sampler cache, and depth/stencil support.
//   - Browser (GOOS=js): uses syscall/js to call navigator.gpu directly
//     for WebGPU rendering in the browser, with GPUCanvasContext for
//     presentation and async adapter/device creation via Promises.
//
// Shader translation from GLSL 330 to WGSL is handled by the pure-Go
// translator in internal/shadertranslate/wgsl.go, shared by both the
// native and browser paths.
//
// The backend registers itself as "webgpu" in the backend registry.
//
// Key API-specific types:
//   - AdapterInfo: mirrors GPUAdapterInfo (vendor, architecture, backend type)
//   - BackendType: underlying GPU API used by wgpu (Vulkan, Metal, D3D12, etc.)
//   - Limits: mirrors GPUSupportedLimits (max texture size, bind groups, etc.)
//   - WGPU format/usage constants: map backend types to WebGPU enums
//
// See GPU_TESTING.md for the 8-tier validation checklist covering native
// and browser testing.
package webgpu
