package vulkan

import "github.com/michaelraines/future-render/internal/backend"

// Vulkan API version constants.
const (
	vkAPIVersion1_0 = uint32(1<<22 | 0<<12)
	vkAPIVersion1_1 = uint32(1<<22 | 1<<12)
	vkAPIVersion1_2 = uint32(1<<22 | 2<<12)
	vkAPIVersion1_3 = uint32(1<<22 | 3<<12)
)

// VkFormat equivalents for texture formats.
const (
	vkFormatR8UNorm            = 9
	vkFormatR8G8B8UNorm        = 23
	vkFormatR8G8B8A8UNorm      = 37
	vkFormatR16G16B16A16SFloat = 97
	vkFormatR32G32B32A32SFloat = 109
	vkFormatD24UNormS8UInt     = 129
	vkFormatD32SFloat          = 126
)

// VkImageUsageFlags equivalents.
const (
	vkImageUsageSampled     = 0x00000004
	vkImageUsageTransferDst = 0x00000002
	vkImageUsageTransferSrc = 0x00000001
	vkImageUsageColorAttach = 0x00000010
	vkImageUsageDepthAttach = 0x00000020
)

// VkBufferUsageFlags equivalents.
const (
	vkBufferUsageVertexBuf   = 0x00000080
	vkBufferUsageIndexBuf    = 0x00000040
	vkBufferUsageUniformBuf  = 0x00000010
	vkBufferUsageTransferDst = 0x00000002
)

// vkFormatFromTextureFormat maps backend texture formats to VkFormat values.
func vkFormatFromTextureFormat(f backend.TextureFormat) int {
	switch f {
	case backend.TextureFormatRGBA8:
		return vkFormatR8G8B8A8UNorm
	case backend.TextureFormatRGB8:
		return vkFormatR8G8B8UNorm
	case backend.TextureFormatR8:
		return vkFormatR8UNorm
	case backend.TextureFormatRGBA16F:
		return vkFormatR16G16B16A16SFloat
	case backend.TextureFormatRGBA32F:
		return vkFormatR32G32B32A32SFloat
	case backend.TextureFormatDepth24:
		return vkFormatD24UNormS8UInt
	case backend.TextureFormatDepth32F:
		return vkFormatD32SFloat
	default:
		return vkFormatR8G8B8A8UNorm
	}
}

// vkBufferUsageFromBackend maps backend buffer usage to VkBufferUsageFlags.
func vkBufferUsageFromBackend(u backend.BufferUsage) int {
	switch u {
	case backend.BufferUsageVertex:
		return vkBufferUsageVertexBuf | vkBufferUsageTransferDst
	case backend.BufferUsageIndex:
		return vkBufferUsageIndexBuf | vkBufferUsageTransferDst
	case backend.BufferUsageUniform:
		return vkBufferUsageUniformBuf | vkBufferUsageTransferDst
	default:
		return vkBufferUsageVertexBuf | vkBufferUsageTransferDst
	}
}
