package futurerender

import (
	"sync/atomic"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/batch"
)

// renderer holds internal rendering state shared between the engine loop
// and public API types like Image. It is initialized by the platform engine.
type renderer struct {
	device  backend.Device
	batcher *batch.Batcher

	// Monotonic texture ID counter for batcher sort keys.
	nextTextureID atomic.Uint32

	// whiteTextureID is the texture ID for a 1x1 white texture,
	// used for untextured draws (Fill, etc.).
	whiteTextureID uint32

	// registerTexture is called when a new texture is created (e.g. by
	// NewImage) so the engine can track it for lookup during rendering.
	registerTexture func(id uint32, tex backend.Texture)

	// registerShader is called when a new shader is created so the
	// pipeline can look it up by ID during rendering.
	registerShader func(id uint32, shader *Shader)

	// registerRenderTarget is called when a new render target is created
	// so the engine can resolve target IDs during rendering.
	registerRenderTarget func(id uint32, rt backend.RenderTarget)
}

// globalRendererPtr is the active renderer, set atomically by the engine during init.
var globalRendererPtr atomic.Pointer[renderer]

// getRenderer returns the current renderer, or nil if not initialized.
func getRenderer() *renderer { return globalRendererPtr.Load() }

// setRenderer stores the renderer atomically.
func setRenderer(r *renderer) { globalRendererPtr.Store(r) }

func (r *renderer) allocTextureID() uint32 {
	return r.nextTextureID.Add(1)
}
