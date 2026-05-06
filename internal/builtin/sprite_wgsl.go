package builtin

import _ "embed"

//go:embed sprite.vert.wgsl
var spriteVertexWGSL []byte

//go:embed sprite.frag.wgsl
var spriteFragmentWGSL []byte

// SpriteVertexWGSL is the hand-written WGSL source for the engine's
// built-in sprite vertex shader. Consumed by the WebGPU backend's
// NewShaderNative WGSL path; bypasses the GLSL→WGSL translator (which
// doesn't recognize std140 UBO blocks and silently emits broken
// output for sprite.vert.glsl). See sprite.vert.wgsl for the rationale.
func SpriteVertexWGSL() []byte { return spriteVertexWGSL }

// SpriteFragmentWGSL is the hand-written WGSL source for the engine's
// built-in sprite fragment shader.
func SpriteFragmentWGSL() []byte { return spriteFragmentWGSL }
