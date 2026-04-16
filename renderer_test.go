package futurerender

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
)

func TestPendingClearTrackerRequest(t *testing.T) {
	tr := newPendingClearTracker()

	require.False(t, tr.Consume(1), "empty tracker must return false")

	tr.Request(1)
	require.True(t, tr.Has(1))
	require.True(t, tr.Consume(1))
	require.False(t, tr.Consume(1), "single request consumed")
}

func TestPendingClearTrackerRequestAccumulates(t *testing.T) {
	tr := newPendingClearTracker()

	// Simulate the AA buffer scenario: 3 flush cycles each request a clear.
	tr.Request(10)
	tr.Request(10)
	tr.Request(10)

	// Each consume should succeed independently.
	require.True(t, tr.Consume(10), "first consume")
	require.True(t, tr.Consume(10), "second consume")
	require.True(t, tr.Consume(10), "third consume")
	require.False(t, tr.Consume(10), "all consumed")
	require.False(t, tr.Has(10), "cleaned up after full consumption")
}

func TestPendingClearTrackerRequestOnceIdempotent(t *testing.T) {
	tr := newPendingClearTracker()

	// Simulate Clear() being called multiple times per frame
	// (imageClearWrapper + Frame.Draw both call ClearImage).
	tr.RequestOnce(20)
	tr.RequestOnce(20)
	tr.RequestOnce(20)

	// Only one clear should be pending regardless of call count.
	require.True(t, tr.Consume(20), "first consume")
	require.False(t, tr.Consume(20), "RequestOnce is idempotent")
}

func TestPendingClearTrackerRequestOnceDoesNotReduceExisting(t *testing.T) {
	tr := newPendingClearTracker()

	// AA buffer accumulates 3 flushes via Request.
	tr.Request(30)
	tr.Request(30)
	tr.Request(30)

	// Then Clear() calls RequestOnce — should NOT reduce from 3 to 1.
	tr.RequestOnce(30)

	require.True(t, tr.Consume(30))
	require.True(t, tr.Consume(30))
	require.True(t, tr.Consume(30))
	require.False(t, tr.Consume(30))
}

func TestPendingClearTrackerMixedTargets(t *testing.T) {
	tr := newPendingClearTracker()

	// Canvas gets idempotent clear, AA buffer gets accumulated clears.
	tr.RequestOnce(100) // canvas
	tr.Request(200)     // AA buffer flush 1
	tr.Request(200)     // AA buffer flush 2

	// Canvas: one clear.
	require.True(t, tr.Consume(100))
	require.False(t, tr.Consume(100))

	// AA buffer: two clears.
	require.True(t, tr.Consume(200))
	require.True(t, tr.Consume(200))
	require.False(t, tr.Consume(200))
}

// TestAABufferMultiFlushClearLifecycle simulates the exact scenario that
// caused invisible text: multiple AA flush cycles on the same canvas
// within a single frame. Each flush must clear the AA buffer independently.
func TestAABufferMultiFlushClearLifecycle(t *testing.T) {
	withMockRenderer(t)

	canvas := NewImage(128, 128)
	require.NotNil(t, canvas)

	// Simulate frame lifecycle: Clear → AA draws → text → AA draws → text → composite.
	// This is what the component system's Frame does each frame.

	// 1. ClearImage (called twice by imageClearWrapper + Frame.Draw).
	canvas.Clear()
	canvas.Clear()

	rend := getRenderer()

	// Canvas should have exactly 1 pending clear (RequestOnce is idempotent).
	require.True(t, rend.pendingClears.Has(canvas.textureID))
	require.True(t, rend.pendingClears.Consume(canvas.textureID))
	require.False(t, rend.pendingClears.Consume(canvas.textureID),
		"double Clear must not produce double clear")

	// Re-request for the rest of the test.
	rend.pendingClears.RequestOnce(canvas.textureID)

	// 2. First AA draw cycle: panel → flush → text.
	verts, idx := aaTriangle(10, 10, 50, 50)
	canvas.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.NotNil(t, canvas.aaBuffer)
	require.True(t, canvas.aaBufferDirty)

	// Trigger AA flush via a DrawImage (simulates text rendering).
	glyph := NewImage(8, 10)
	canvas.DrawImage(glyph, nil)
	require.False(t, canvas.aaBufferDirty, "flush should clear dirty flag")

	// AA buffer should have a pending clear from the flush.
	require.True(t, rend.pendingClears.Has(canvas.aaBuffer.textureID),
		"post-flush clear must be registered")

	// 3. Second AA draw cycle.
	canvas.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.True(t, canvas.aaBufferDirty)

	// Trigger second flush.
	canvas.DrawImage(glyph, nil)

	// AA buffer should still have pending clears (2 total from 2 flushes,
	// minus whatever was consumed — but since we're in the Draw callback
	// and the sprite pass hasn't run yet, both should be pending).
	aaID := canvas.aaBuffer.textureID
	require.True(t, rend.pendingClears.Consume(aaID), "flush 1 clear")
	require.True(t, rend.pendingClears.Consume(aaID), "flush 2 clear")
	// The aaBufferNeedsClear from Clear() adds one more via RequestOnce,
	// but it doesn't stack beyond what Request already accumulated.
}

// --- Registry cleanup + stale-ID warning ---
//
// These tests pin the dispose/resolve contract that replaced the
// silent "destroyed texture used in a submit" crash:
//   1. disposeNow calls rend.unregisterTexture / unregisterRenderTarget
//      so subsequent resolveTexture lookups return nil rather than a
//      stale/destroyed GPU handle.
//   2. The same disposal records the ID in rend.disposedIDs so that
//      the FIRST subsequent resolve emits a one-shot diagnostic
//      warning, which turns "app silently breaks" into an actionable
//      log line pointing at the lifecycle bug.

func TestDisposeNowUnregistersTextureAndRenderTarget(t *testing.T) {
	_, registered := withMockRenderer(t)
	rend := getRenderer()
	// withMockRenderer leaves unregisterRenderTarget as a no-op (the
	// RT map isn't tracked by the helper) — override with a real one
	// for this test.
	rtMap := make(map[uint32]struct{})
	rend.registerRenderTarget = func(id uint32, _ backend.RenderTarget) {
		rtMap[id] = struct{}{}
	}
	rend.unregisterRenderTarget = func(id uint32) {
		delete(rtMap, id)
	}

	img := NewImage(16, 16)
	require.NotZero(t, img.textureID)
	require.Contains(t, registered, img.textureID, "texture must be registered on creation")
	require.Contains(t, rtMap, img.textureID, "render target must be registered on creation")

	img.Dispose()
	// Dispose is deferred — drain the queue to trigger disposeNow.
	rend.disposeDeferred()

	require.NotContains(t, registered, img.textureID, "unregisterTexture must remove the ID on dispose")
	require.NotContains(t, rtMap, img.textureID, "unregisterRenderTarget must remove the ID on dispose")
}

func TestDisposeNowRecordsDisposedID(t *testing.T) {
	withMockRenderer(t)
	rend := getRenderer()

	img := NewImage(8, 8)
	id := img.textureID
	require.NotZero(t, id)

	img.Dispose()
	rend.disposeDeferred()

	_, recorded := rend.disposedIDs[id]
	require.True(t, recorded, "disposeNow must add textureID to disposedIDs")
}

func TestMarkDisposedIDNilRendererIsNoOp(t *testing.T) {
	var r *renderer
	require.NotPanics(t, func() { r.markDisposedID(1) })
}

func TestMarkDisposedIDZeroIDIsNoOp(t *testing.T) {
	// Texture ID 0 is the default "no texture" sentinel — never a real
	// allocation. Storing it in disposedIDs would produce spurious
	// warnings when code resolves untextured draws.
	r := &renderer{}
	r.markDisposedID(0)
	require.Empty(t, r.disposedIDs)
}

func TestWarnStaleIDOnceEmitsExactlyOnce(t *testing.T) {
	r := &renderer{}
	r.markDisposedID(42)

	// Capture stderr so we can verify the warning text without
	// spamming test output. This also asserts the warning fires
	// exactly once for a given stale ID.
	origStderr := os.Stderr
	readEnd, writeEnd, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = writeEnd
	t.Cleanup(func() { os.Stderr = origStderr })

	require.True(t, r.warnStaleIDOnce(42, "texture"), "stale ID must be detected")
	require.False(t, r.warnStaleIDOnce(42, "texture"), "second call on same ID must be quiet")

	require.NoError(t, writeEnd.Close())
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(readEnd)
	require.Contains(t, buf.String(), "texture with id 42 was resolved after it was disposed")
	require.Contains(t, buf.String(), "stale reference")
}

func TestWarnStaleIDOnceReturnsFalseForUnknownID(t *testing.T) {
	r := &renderer{}
	require.False(t, r.warnStaleIDOnce(99, "texture"),
		"resolve miss for a never-registered ID is NOT a stale-dispose signal")
}

func TestWarnStaleIDOnceNilRendererIsNoOp(t *testing.T) {
	var r *renderer
	require.False(t, r.warnStaleIDOnce(1, "texture"))
}

func TestNewImageLabeledUsesProvidedPrefix(t *testing.T) {
	dev, _ := withMockRenderer(t)

	// mockDevice.NewRenderTarget drops the Label; we only care that
	// the code path uses newImageLabeled correctly from the call
	// sites. Inspect the last descriptor via the mock.
	_ = newImageLabeled(8, 8, "aa-buffer")

	require.NotEmpty(t, dev.renderTargetDescs, "mock device should have recorded the RT descriptor")
	last := dev.renderTargetDescs[len(dev.renderTargetDescs)-1]
	require.Contains(t, last.Label, "aa-buffer-")
	require.Contains(t, last.Label, "-8x8")
}
