package builtin

import (
	_ "embed"

	"github.com/michaelraines/future-core/internal/backend"
)

//go:embed sprite.vert.spv
var spriteVertexSPIRV []byte

//go:embed sprite.frag.spv
var spriteFragmentSPIRV []byte

// SpriteVertexSPIRV is the precompiled SPIR-V bytecode for the
// engine's built-in sprite vertex shader. Generated from
// sprite.vert.glsl by cmd/precompile-builtin-spirv at build time on
// a host with libshaderc; consumed at runtime by the Vulkan backend's
// NewShaderNative SPIR-V path (skipping shaderc, so this works on
// Android where libshaderc is unavailable).
func SpriteVertexSPIRV() []byte { return spriteVertexSPIRV }

// SpriteFragmentSPIRV is the precompiled SPIR-V bytecode for the
// engine's built-in sprite fragment shader.
func SpriteFragmentSPIRV() []byte { return spriteFragmentSPIRV }

// SpriteUniformLayout is the std140 uniform layout that matches both
// stages of the sprite shader. Vertex stage holds uProjection (mat4);
// fragment stage holds uColorBody (mat4) and uColorTranslation (vec4)
// plus a sampler2D (uTexture). The sampler binds via the descriptor
// machinery, not the UBO, so it isn't listed here.
//
// Offsets are byte offsets into the shared per-draw uniform buffer.
// Order and sizes match what shadertranslate.ExtractUniformLayout
// computes from the GLSL source — kept in sync by hand because the
// SPIR-V path doesn't have the GLSL to derive the layout from at
// runtime. Bump the offsets here whenever the shader's uniform
// declarations change.
func SpriteUniformLayout() []backend.NativeUniformField {
	return []backend.NativeUniformField{
		{Name: "uProjection", Offset: 0, Size: 64},
		{Name: "uColorBody", Offset: 64, Size: 64},
		{Name: "uColorTranslation", Offset: 128, Size: 16},
	}
}
