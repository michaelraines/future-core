package futurerender

import (
	"sync/atomic"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
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

	// pendingClears tracks render targets that need GPU-native clearing on
	// their next BeginRenderPass. Supports multiple clear requests per
	// target within a single frame (the AA buffer is flushed and
	// re-entered many times per frame).
	pendingClears pendingClearTracker

	// registerRenderTarget is called when a new render target is created
	// so the engine can resolve target IDs during rendering.
	registerRenderTarget func(id uint32, rt backend.RenderTarget)

	// deferredDispose holds images whose GPU resources should be released
	// AFTER the current frame's sprite-pass Flush+Execute has consumed
	// any command-buffer references to them. Used by drawTrianglesAA when
	// an aaBuffer needs to be replaced mid-frame: the batcher still holds
	// draw commands targeting the old buffer, so we can't synchronously
	// Dispose it — doing so leaves the sprite pass with a nil render
	// target when it begins the pass. The engine drains this list by
	// calling disposeDeferred() immediately after renderPipeline.Execute.
	deferredDispose []*Image
}

// disposeDeferred drains the pending-disposal list. The engine calls this
// once per frame, AFTER the sprite pass's Execute has flushed and
// submitted its command buffer.
func (r *renderer) disposeDeferred() {
	if r == nil {
		return
	}
	for _, img := range r.deferredDispose {
		if img != nil {
			img.Dispose()
		}
	}
	r.deferredDispose = r.deferredDispose[:0]
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

// pendingClearTracker tracks render targets that need GPU-native clearing.
// Each Request call increments a per-target counter; each Consume call
// decrements and returns true while the count is positive. This supports
// multiple clear requests per target within a single frame — the AA
// buffer is flushed and re-entered many times per frame, and each
// re-entry needs its own clear.
type pendingClearTracker struct {
	counts map[uint32]int
}

// newPendingClearTracker creates an initialized tracker.
func newPendingClearTracker() pendingClearTracker {
	return pendingClearTracker{counts: make(map[uint32]int)}
}

// Request registers a clear request for the given target. Multiple
// requests accumulate — each will be consumed by a separate
// BeginRenderPass. Used by flushAABuffer where each flush cycle
// needs its own independent clear.
func (t *pendingClearTracker) Request(targetID uint32) {
	if t.counts == nil {
		t.counts = make(map[uint32]int)
	}
	t.counts[targetID]++
}

// RequestOnce ensures at least one clear is pending for the target,
// but does not accumulate. Multiple calls are idempotent. Used by
// Image.Clear() where the caller wants the target cleared on its
// next render pass regardless of how many times Clear() is called.
func (t *pendingClearTracker) RequestOnce(targetID uint32) {
	if t.counts == nil {
		t.counts = make(map[uint32]int)
	}
	if t.counts[targetID] == 0 {
		t.counts[targetID] = 1
	}
}

// Consume returns true and decrements the counter if the target has
// pending clears. Returns false when no clears remain.
func (t *pendingClearTracker) Consume(targetID uint32) bool {
	if t.counts == nil || t.counts[targetID] <= 0 {
		return false
	}
	t.counts[targetID]--
	if t.counts[targetID] == 0 {
		delete(t.counts, targetID)
	}
	return true
}

// Has returns true if the target has any pending clear requests.
func (t *pendingClearTracker) Has(targetID uint32) bool {
	return t.counts != nil && t.counts[targetID] > 0
}
