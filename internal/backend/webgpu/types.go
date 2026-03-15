package webgpu

import "github.com/michaelraines/future-render/internal/backend"

// WGPUTextureFormat equivalents (matching wgpu-native enum values).
const (
	wgpuTextureFormatRGBA8UNorm   = 18
	wgpuTextureFormatBGRA8UNorm   = 24
	wgpuTextureFormatR8UNorm      = 1
	wgpuTextureFormatRGBA16Float  = 33
	wgpuTextureFormatRGBA32Float  = 36
	wgpuTextureFormatDepth24Plus  = 40
	wgpuTextureFormatDepth32Float = 42
)

// WGPUTextureUsage flags.
const (
	wgpuTextureUsageCopySrc          = 0x01
	wgpuTextureUsageCopyDst          = 0x02
	wgpuTextureUsageSampled          = 0x04
	wgpuTextureUsageStorage          = 0x08
	wgpuTextureUsageRenderAttachment = 0x10
)

// WGPUBufferUsage flags.
const (
	wgpuBufferUsageMapRead  = 0x0001
	wgpuBufferUsageMapWrite = 0x0002
	wgpuBufferUsageCopySrc  = 0x0004
	wgpuBufferUsageCopyDst  = 0x0008
	wgpuBufferUsageIndex    = 0x0010
	wgpuBufferUsageVertex   = 0x0020
	wgpuBufferUsageUniform  = 0x0040
	wgpuBufferUsageStorage  = 0x0080
)

// wgpuTextureFormatFromBackend maps backend texture formats to WGPUTextureFormat.
func wgpuTextureFormatFromBackend(f backend.TextureFormat) int {
	switch f {
	case backend.TextureFormatRGBA8:
		return wgpuTextureFormatRGBA8UNorm
	case backend.TextureFormatRGB8:
		return wgpuTextureFormatRGBA8UNorm // WebGPU has no RGB8; use RGBA8
	case backend.TextureFormatR8:
		return wgpuTextureFormatR8UNorm
	case backend.TextureFormatRGBA16F:
		return wgpuTextureFormatRGBA16Float
	case backend.TextureFormatRGBA32F:
		return wgpuTextureFormatRGBA32Float
	case backend.TextureFormatDepth24:
		return wgpuTextureFormatDepth24Plus
	case backend.TextureFormatDepth32F:
		return wgpuTextureFormatDepth32Float
	default:
		return wgpuTextureFormatRGBA8UNorm
	}
}

// wgpuBufferUsageFromBackend maps backend buffer usage to WGPUBufferUsage flags.
func wgpuBufferUsageFromBackend(u backend.BufferUsage) int {
	switch u {
	case backend.BufferUsageVertex:
		return wgpuBufferUsageVertex | wgpuBufferUsageCopyDst
	case backend.BufferUsageIndex:
		return wgpuBufferUsageIndex | wgpuBufferUsageCopyDst
	case backend.BufferUsageUniform:
		return wgpuBufferUsageUniform | wgpuBufferUsageCopyDst
	default:
		return wgpuBufferUsageVertex | wgpuBufferUsageCopyDst
	}
}
