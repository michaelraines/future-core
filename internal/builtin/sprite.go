// Package builtin holds shader source assets for the engine's
// internal sprite pipeline. Both forms are kept:
//
//   - GLSL: SpriteVertexGLSL + SpriteFragmentGLSL — what the
//     desktop and browser engine paths feed to dev.NewShader, where
//     the backend (or the futurecore translator) drives any further
//     compilation.
//   - SPIR-V: SpriteVertexSPIRV + SpriteFragmentSPIRV — produced from
//     the GLSL by cmd/precompile-builtin-spirv at build time on a
//     host with libshaderc. The Vulkan backend's NewShaderNative
//     SPIR-V path consumes these directly via vkCreateShaderModule —
//     this is what makes Vulkan run on Android, where libshaderc.so
//     is not available and bundling it would add ~5 MB per ABI.
//
// Run `go generate ./internal/builtin/...` (host-only — needs
// libshaderc) after editing either *.glsl file to refresh the *.spv
// blobs and the matching SPIR-V byte arrays in this file.
package builtin

//go:generate go run ../../cmd/precompile-builtin-spirv

import _ "embed"

//go:embed sprite.vert.glsl
var spriteVertexGLSL []byte

//go:embed sprite.frag.glsl
var spriteFragmentGLSL []byte

// SpriteVertexGLSL is the GLSL 330 source for the engine's built-in
// sprite vertex shader.
func SpriteVertexGLSL() string { return string(spriteVertexGLSL) }

// SpriteFragmentGLSL is the GLSL 330 source for the engine's built-in
// sprite fragment shader.
func SpriteFragmentGLSL() string { return string(spriteFragmentGLSL) }
