// Package webgpu implements backend.Device targeting the WebGPU API.
//
// WebGPU is a next-generation cross-platform GPU API that runs natively
// (via Dawn or wgpu-native) and in browsers (via the WebGPU JS API). This
// backend models WebGPU concepts — GPUAdapter, GPUDevice, GPUQueue,
// GPURenderPipeline, GPUTexture, GPUBuffer, GPUCommandEncoder, bind groups,
// WGPUTextureFormat, and WGPUBufferUsage — while currently delegating
// actual rendering to the software rasterizer for conformance testing in
// any environment.
//
// The backend registers itself as "webgpu" in the backend registry.
//
// Key API-specific types:
//   - AdapterInfo: mirrors GPUAdapterInfo (vendor, architecture, backend type)
//   - BackendType: underlying GPU API used by wgpu (Vulkan, Metal, D3D12, etc.)
//   - Limits: mirrors GPUSupportedLimits (max texture size, bind groups, etc.)
//   - WGPU format/usage constants: map backend types to WebGPU enums
package webgpu
