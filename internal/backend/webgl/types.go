package webgl

import "github.com/michaelraines/future-render/internal/backend"

// WebGL2 constant equivalents. These mirror the real WebGL2 GLenum values
// that a syscall/js implementation would use.
const (
	glTexture2D = 0x0DE1
	glRGBA      = 0x1908
	glRGB       = 0x1907
	glRed       = 0x1903 // WebGL2 RED format
	glRGBA16F   = 0x881A
	glRGBA32F   = 0x8814
	glDepth24   = 0x81A6 // DEPTH_COMPONENT24
	glDepth32F  = 0x8CAC // DEPTH_COMPONENT32F

	glArrayBuffer        = 0x8892
	glElementArrayBuffer = 0x8893
	glUniformBuffer      = 0x8A11
)

// glFormatFromTextureFormat maps backend texture formats to WebGL2 internal format constants.
func glFormatFromTextureFormat(f backend.TextureFormat) int {
	switch f {
	case backend.TextureFormatRGBA8:
		return glRGBA
	case backend.TextureFormatRGB8:
		return glRGB
	case backend.TextureFormatR8:
		return glRed
	case backend.TextureFormatRGBA16F:
		return glRGBA16F
	case backend.TextureFormatRGBA32F:
		return glRGBA32F
	case backend.TextureFormatDepth24:
		return glDepth24
	case backend.TextureFormatDepth32F:
		return glDepth32F
	default:
		return glRGBA
	}
}

// glUsageFromBufferUsage maps backend buffer usage to WebGL2 buffer target constants.
func glUsageFromBufferUsage(u backend.BufferUsage) int {
	switch u {
	case backend.BufferUsageVertex:
		return glArrayBuffer
	case backend.BufferUsageIndex:
		return glElementArrayBuffer
	case backend.BufferUsageUniform:
		return glUniformBuffer
	default:
		return glArrayBuffer
	}
}

// translateGLSLES performs a lightweight GLSL 330 → GLSL ES 3.00 translation.
// In a real implementation this would rewrite version directives, add precision
// qualifiers, and adjust in/out keywords. Currently a pass-through since the
// soft rasterizer doesn't execute shaders.
func translateGLSLES(source string) string {
	return source
}
