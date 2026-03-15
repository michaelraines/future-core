package dx12

import "github.com/michaelraines/future-render/internal/backend"

// DXGI_FORMAT equivalents.
const (
	dxgiFormatR8UNorm           = 61
	dxgiFormatR8G8B8A8UNorm     = 28
	dxgiFormatB8G8R8A8UNorm     = 87
	dxgiFormatR16G16B16A16Float = 10
	dxgiFormatR32G32B32A32Float = 2
	dxgiFormatD24UNormS8UInt    = 45
	dxgiFormatD32Float          = 40
)

// D3D12_HEAP_TYPE equivalents.
const (
	d3d12HeapTypeDefault  = 1
	d3d12HeapTypeUpload   = 2
	d3d12HeapTypeReadback = 3
)

// dxgiFormatFromBackend maps backend texture formats to DXGI_FORMAT.
func dxgiFormatFromBackend(f backend.TextureFormat) int {
	switch f {
	case backend.TextureFormatRGBA8:
		return dxgiFormatR8G8B8A8UNorm
	case backend.TextureFormatRGB8:
		return dxgiFormatR8G8B8A8UNorm // DX12 has no RGB8; use RGBA8
	case backend.TextureFormatR8:
		return dxgiFormatR8UNorm
	case backend.TextureFormatRGBA16F:
		return dxgiFormatR16G16B16A16Float
	case backend.TextureFormatRGBA32F:
		return dxgiFormatR32G32B32A32Float
	case backend.TextureFormatDepth24:
		return dxgiFormatD24UNormS8UInt
	case backend.TextureFormatDepth32F:
		return dxgiFormatD32Float
	default:
		return dxgiFormatR8G8B8A8UNorm
	}
}
