package builtin

import _ "embed"

//go:embed sprite.vert.msl
var spriteVertexMSL []byte

//go:embed sprite.frag.msl
var spriteFragmentMSL []byte

// SpriteVertexMSL is the hand-written MSL source for the engine's
// built-in sprite vertex shader. Consumed by the Metal backend's
// NewShaderNative MSL path; bypasses the GLSL→MSL translator (which
// doesn't recognize std140 UBO blocks and silently emits broken
// output for sprite.vert.glsl). See sprite.vert.msl for the rationale.
func SpriteVertexMSL() []byte { return spriteVertexMSL }

// SpriteFragmentMSL is the hand-written MSL source for the engine's
// built-in sprite fragment shader.
func SpriteFragmentMSL() []byte { return spriteFragmentMSL }
