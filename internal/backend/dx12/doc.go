// Package dx12 implements backend.Device targeting DirectX 12.
//
// DirectX 12 is Windows' modern low-level GPU API, accessed via purego
// loading of d3d12.dll and dxgi.dll with COM vtable calls. This backend
// models DX12 concepts — DXGI adapter, D3D12 device, command queue/list,
// root signature, pipeline state objects (PSO), descriptor heaps,
// DXGI_FORMAT, D3D12_HEAP_TYPE, and feature levels — while currently
// delegating actual rendering to the software rasterizer for conformance
// testing in any environment.
//
// The backend registers itself as "dx12" in the backend registry.
//
// Key API-specific types:
//   - AdapterDesc: mirrors DXGI_ADAPTER_DESC1 (description, vendor, memory)
//   - FeatureLevel: represents D3D feature levels (11.0 through 12.2)
//   - DXGI_FORMAT constants: map backend.TextureFormat to DXGI format values
//   - Debug layer: enabled via DeviceConfig.Debug
package dx12
