//go:build !js

package webgl

import "github.com/michaelraines/future-render/internal/backend"

// Shader implements backend.Shader for WebGL2.
// Wraps a soft.Shader and stores the GLSL ES 3.00 source that a real
// WebGL2 implementation would compile via gl.createProgram().
type Shader struct {
	backend.Shader        // delegates all Shader methods to inner
	vertexSource   string // GLSL ES 3.00 vertex shader source
	fragSource     string // GLSL ES 3.00 fragment shader source
}
