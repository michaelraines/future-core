//go:build !wgpunative

package webgpu

import "github.com/michaelraines/future-render/internal/backend"

// Texture implements backend.Texture for WebGPU.
// Models a GPUTexture object.
type Texture struct {
	backend.Texture     // delegates all Texture methods to inner
	format          int // WGPUTextureFormat
	usage           int // WGPUTextureUsage
}

// InnerTexture returns the wrapped soft texture for encoder unwrapping.
func (t *Texture) InnerTexture() backend.Texture { return t.Texture }
