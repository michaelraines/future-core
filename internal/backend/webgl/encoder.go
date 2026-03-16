//go:build !js

package webgl

import "github.com/michaelraines/future-render/internal/backend/softdelegate"

// Encoder implements backend.CommandEncoder for WebGL2.
// Delegates all commands to the soft rasterizer via the embedded
// softdelegate.Encoder. A real implementation would translate these to
// WebGL2 gl.bindBuffer, gl.drawElements, etc. calls via syscall/js.
type Encoder struct {
	softdelegate.Encoder
}
