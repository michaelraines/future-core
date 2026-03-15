package metal

import "github.com/michaelraines/future-render/internal/backend"

// MTLPixelFormat equivalents.
const (
	mtlPixelFormatRGBA8UNorm           = 70
	mtlPixelFormatBGRA8UNorm           = 80
	mtlPixelFormatRGB8UNorm            = 0 // Metal doesn't have RGB8; would use RGBA8
	mtlPixelFormatR8UNorm              = 10
	mtlPixelFormatRGBA16Float          = 115
	mtlPixelFormatRGBA32Float          = 125
	mtlPixelFormatDepth24UNormStencil8 = 255 // MTLPixelFormatDepth24Unorm_Stencil8
	mtlPixelFormatDepth32Float         = 252
)

// MTLTextureUsage equivalents.
const (
	mtlTextureUsageShaderRead   = 0x0001
	mtlTextureUsageShaderWrite  = 0x0002
	mtlTextureUsageRenderTarget = 0x0004
)

// MTLStorageMode equivalents.
const (
	mtlStorageModeShared  = 0
	mtlStorageModeManaged = 1
	mtlStorageModePrivate = 2
)

// mtlPixelFormatFromBackend maps backend texture formats to MTLPixelFormat.
func mtlPixelFormatFromBackend(f backend.TextureFormat) int {
	switch f {
	case backend.TextureFormatRGBA8:
		return mtlPixelFormatRGBA8UNorm
	case backend.TextureFormatRGB8:
		return mtlPixelFormatRGBA8UNorm // Metal uses RGBA8 (no native RGB8)
	case backend.TextureFormatR8:
		return mtlPixelFormatR8UNorm
	case backend.TextureFormatRGBA16F:
		return mtlPixelFormatRGBA16Float
	case backend.TextureFormatRGBA32F:
		return mtlPixelFormatRGBA32Float
	case backend.TextureFormatDepth24:
		return mtlPixelFormatDepth24UNormStencil8
	case backend.TextureFormatDepth32F:
		return mtlPixelFormatDepth32Float
	default:
		return mtlPixelFormatRGBA8UNorm
	}
}
