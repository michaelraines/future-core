package futurerender

import (
	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
	"github.com/michaelraines/future-core/internal/builtin"
)

// createSpriteShader builds the engine's built-in sprite shader on
// the active device. Two paths converge here:
//
//   - SPIR-V via NewShaderNative when the device implements
//     NativeShaderDevice and prefers SPIR-V (Vulkan). The bytes are
//     pre-compiled at build time by cmd/precompile-builtin-spirv,
//     so the runtime never invokes shaderc — required on Android,
//     where libshaderc.so is not available and bundling it would
//     add ~5 MB per ABI.
//   - GLSL via Device.NewShader for every other path. The backend
//     (or the futurecore translator) handles any further compile
//     step. shaderc IS used here on Vulkan-capable desktop builds,
//     but those hosts always carry it.
//
// Callers don't need to know which path fired — both produce a
// backend.Shader configured for the engine's standard Vertex2D
// attribute layout.
func createSpriteShader(dev backend.Device) (backend.Shader, error) {
	if nsd, ok := dev.(backend.NativeShaderDevice); ok &&
		nsd.PreferredShaderLanguage() == backend.ShaderLanguageSPIRV {
		return nsd.NewShaderNative(backend.NativeShaderDescriptor{
			Language:   backend.ShaderLanguageSPIRV,
			Vertex:     builtin.SpriteVertexSPIRV(),
			Fragment:   builtin.SpriteFragmentSPIRV(),
			Uniforms:   builtin.SpriteUniformLayout(),
			Attributes: batch.Vertex2DFormat().Attributes,
		})
	}
	return dev.NewShader(backend.ShaderDescriptor{
		VertexSource:   builtin.SpriteVertexGLSL(),
		FragmentSource: builtin.SpriteFragmentGLSL(),
		Attributes:     batch.Vertex2DFormat().Attributes,
	})
}
