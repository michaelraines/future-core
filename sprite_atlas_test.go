package futurerender

import (
	goimage "image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSpriteAtlasBasic(t *testing.T) {
	_, registered := withMockRenderer(t)
	// Re-enable atlas for this test.
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	src := goimage.NewRGBA(goimage.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			src.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}

	img := NewImageFromImage(src)
	require.NotNil(t, img)
	require.NotNil(t, img.texture)
	require.True(t, img.atlased, "small image should be atlased")

	w, h := img.Size()
	require.Equal(t, 16, w)
	require.Equal(t, 16, h)

	// Atlas page should be registered.
	require.NotEqual(t, uint32(0), img.textureID)
	require.NotNil(t, registered[img.textureID])
}

func TestSpriteAtlasSharedTextureID(t *testing.T) {
	withMockRenderer(t)
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	src1 := goimage.NewRGBA(goimage.Rect(0, 0, 8, 8))
	src2 := goimage.NewRGBA(goimage.Rect(0, 0, 8, 8))

	img1 := NewImageFromImage(src1)
	img2 := NewImageFromImage(src2)

	require.True(t, img1.atlased)
	require.True(t, img2.atlased)

	// Both images should share the same atlas textureID.
	require.Equal(t, img1.textureID, img2.textureID)
}

func TestSpriteAtlasUVsAreDistinct(t *testing.T) {
	withMockRenderer(t)
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	src1 := goimage.NewRGBA(goimage.Rect(0, 0, 8, 8))
	src2 := goimage.NewRGBA(goimage.Rect(0, 0, 8, 8))

	img1 := NewImageFromImage(src1)
	img2 := NewImageFromImage(src2)

	// UVs should differ since they're placed in different atlas positions.
	require.NotEqual(t, img1.u0, img2.u0, "atlas UVs should differ between images")
}

func TestSpriteAtlasLargeImageNotAtlased(t *testing.T) {
	withMockRenderer(t)
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	// Create an image larger than maxAtlasEntrySize.
	src := goimage.NewRGBA(goimage.Rect(0, 0, 300, 300))
	img := NewImageFromImage(src)

	require.NotNil(t, img)
	require.False(t, img.atlased, "large image should not be atlased")
	require.True(t, img.padded, "large image should be padded normally")
}

func TestSpriteAtlasDisabled(t *testing.T) {
	withMockRenderer(t)
	// Atlas is already disabled by withMockRenderer.

	src := goimage.NewRGBA(goimage.Rect(0, 0, 16, 16))
	img := NewImageFromImage(src)

	require.NotNil(t, img)
	require.False(t, img.atlased, "image should not be atlased when disabled")
	require.True(t, img.padded, "image should be padded normally")
}

func TestSpriteAtlasStats(t *testing.T) {
	withMockRenderer(t)
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	pages, pixels := SpriteAtlasStats()
	require.Equal(t, 0, pages)
	require.Equal(t, 0, pixels)

	src := goimage.NewRGBA(goimage.Rect(0, 0, 8, 8))
	NewImageFromImage(src)

	pages, pixels = SpriteAtlasStats()
	require.Equal(t, 1, pages)
	require.Equal(t, spriteAtlasInitialSize*spriteAtlasInitialSize, pixels)
}

func TestSpriteAtlasReset(t *testing.T) {
	withMockRenderer(t)
	SetSpriteAtlasEnabled(true)

	src := goimage.NewRGBA(goimage.Rect(0, 0, 8, 8))
	NewImageFromImage(src)

	pages, _ := SpriteAtlasStats()
	require.Equal(t, 1, pages)

	ResetSpriteAtlas()
	pages, _ = SpriteAtlasStats()
	require.Equal(t, 0, pages)
}

func TestSpriteAtlasDisposeDoesNotFreeAtlas(t *testing.T) {
	withMockRenderer(t)
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	src := goimage.NewRGBA(goimage.Rect(0, 0, 8, 8))
	img := NewImageFromImage(src)
	require.True(t, img.atlased)

	// Disposing an atlased image should not destroy the atlas texture.
	tex := img.texture
	img.Dispose()
	require.True(t, img.pendingDispose)

	getRenderer().disposeDeferred()
	require.True(t, img.disposed)
	// The parent image (atlas page) should still be intact.
	require.NotNil(t, tex)
}

func TestSpriteAtlasWritePixelsNoop(t *testing.T) {
	withMockRenderer(t)
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	src := goimage.NewRGBA(goimage.Rect(0, 0, 4, 4))
	img := NewImageFromImage(src)
	require.True(t, img.atlased)

	// WritePixels should be a no-op on atlased images.
	pix := make([]byte, 4*4*4)
	img.WritePixels(pix) // should not panic

	// Set should be a no-op.
	img.Set(0, 0, color.RGBA{R: 255, A: 255}) // should not panic

	// ReadPixels should be a no-op.
	dst := make([]byte, 4*4*4)
	img.ReadPixels(dst) // should not panic
}

func TestSpriteAtlasSubImage(t *testing.T) {
	withMockRenderer(t)
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	src := goimage.NewRGBA(goimage.Rect(0, 0, 16, 16))
	img := NewImageFromImage(src)
	require.True(t, img.atlased)

	// SubImage should work on atlased images, producing a sub-region
	// with further adjusted UVs.
	sub := img.SubImage(goimage.Rect(4, 4, 12, 12))
	require.NotNil(t, sub)
	w, h := sub.Size()
	require.Equal(t, 8, w)
	require.Equal(t, 8, h)

	// Sub-image should share the same atlas texture.
	require.Equal(t, img.textureID, sub.textureID)
}

func TestSpriteAtlasGrows(t *testing.T) {
	withMockRenderer(t)
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	// Create enough large sprites to force the atlas to grow.
	// Each padded 200x200 image is 202x202, and the initial atlas is 512x512.
	// After ~4 images the 512 atlas fills up and must grow.
	for i := 0; i < 6; i++ {
		src := goimage.NewRGBA(goimage.Rect(0, 0, 200, 200))
		img := NewImageFromImage(src)
		require.True(t, img.atlased)
		_ = img
	}

	pages, totalPixels := SpriteAtlasStats()
	require.Equal(t, 1, pages, "atlas should grow instead of creating new pages")
	require.Greater(t, totalPixels, spriteAtlasInitialSize*spriteAtlasInitialSize,
		"atlas should have grown beyond initial size")
}

// TestSpriteAtlasGrowPreservesTextureRegistration is the regression test
// for the scene-selector thumbnail bug. Growing the atlas replaces the
// GPU texture while keeping the shared textureID stable, so every
// atlased sub-image (thumbnails etc.) must continue to resolve through
// the deferred drain that releases the old GPU resources.
func TestSpriteAtlasGrowPreservesTextureRegistration(t *testing.T) {
	_, registered := withMockRenderer(t)
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	rend := getRenderer()

	// Capture a sub-image placed BEFORE the grow.
	small := goimage.NewRGBA(goimage.Rect(0, 0, 8, 8))
	early := NewImageFromImage(small)
	require.True(t, early.atlased)
	sharedID := early.textureID

	// Force a grow by placing enough 200x200 images to overflow the
	// initial 512x512 page (same pattern as TestSpriteAtlasGrows).
	for i := 0; i < 6; i++ {
		src := goimage.NewRGBA(goimage.Rect(0, 0, 200, 200))
		img := NewImageFromImage(src)
		require.True(t, img.atlased)
	}

	// Drain the deferred queue — this is where the regression bit:
	// disposeDeferred used to unregister sharedID after grow() had
	// already re-registered it against the new GPU texture, leaving
	// sub-images pointing at a now-stale slot.
	rend.disposeDeferred()

	require.NotNil(t, registered[sharedID],
		"sub-images placed before grow must still resolve to a live GPU texture after the drain")
	_, stale := rend.disposedIDs[sharedID]
	require.False(t, stale,
		"the shared atlas textureID must not be flagged as stale after a grow")

	// No registry entry may point at a GPU texture that got destroyed in
	// this drain — even one in a slot that nothing currently queries —
	// because any later SetTexture call that happens to resolve to it
	// would rebuild a bind group around the destroyed handle, which on
	// WebGPU surfaces as "Destroyed texture used in a submit" for every
	// frame afterwards. This is the second-grow regression.
	for id, tex := range registered {
		mt, ok := tex.(*mockTexture)
		require.True(t, ok, "registered[%d] should be *mockTexture", id)
		require.False(t, mt.disposed,
			"registered[%d] still points at a destroyed GPU texture after drain", id)
	}
}

// TestSpriteAtlasMultipleGrowsLeaveNoStaleRegistrations is the explicit
// repro for the "Destroyed texture [sprite-atlas-page-grown-N] used in a
// submit" WebGPU warning that appears on scene transitions in the
// browser build. A SECOND grow disposes the FIRST-grown page's GPU
// texture; that texture must not be reachable via any live registry
// entry afterwards, or the next frame's SetTexture would resolve to it.
func TestSpriteAtlasMultipleGrowsLeaveNoStaleRegistrations(t *testing.T) {
	_, registered := withMockRenderer(t)
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	rend := getRenderer()

	// Enough 200x200 images to force two grows (512 → 1024 → 2048).
	for i := 0; i < 24; i++ {
		src := goimage.NewRGBA(goimage.Rect(0, 0, 200, 200))
		NewImageFromImage(src)
	}
	rend.disposeDeferred()

	for id, tex := range registered {
		mt, ok := tex.(*mockTexture)
		require.True(t, ok)
		require.False(t, mt.disposed,
			"registered[%d] points at a destroyed GPU texture after two grows", id)
	}
}

// TestSpriteAtlasGrowRescalesSubImageUVs catches the white-text-blocks
// regression: long-lived cached SubImage references (DebugPrint per-rune
// cache, font.glyphImageCache) kept pre-grow UVs across an atlas grow and
// ended up sampling a doubled pixel region, producing garbled characters
// in the HUD. SubImage must register derived images with the atlas so
// grow rescales them too.
func TestSpriteAtlasGrowRescalesSubImageUVs(t *testing.T) {
	withMockRenderer(t)
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	// Place an atlased source image and carve a SubImage out of it
	// BEFORE the atlas grows — this simulates the DebugPrint glyph-cache
	// flow where glyph sub-images are computed once and reused for
	// frames afterwards.
	src := goimage.NewRGBA(goimage.Rect(0, 0, 64, 64))
	atlased := NewImageFromImage(src)
	require.True(t, atlased.atlased)
	glyph := atlased.SubImage(goimage.Rect(4, 4, 12, 20))
	u0Before, u1Before := glyph.u0, glyph.u1
	v0Before, v1Before := glyph.v0, glyph.v1

	// Force a grow by placing enough 200x200 images to overflow the
	// initial 512x512 page.
	for i := 0; i < 6; i++ {
		big := goimage.NewRGBA(goimage.Rect(0, 0, 200, 200))
		NewImageFromImage(big)
	}

	// After grow, the atlas is 1024x1024 so every placed UV (including
	// this glyph) must have been halved. If SubImage didn't register
	// the glyph with the atlas, its UVs would still be the pre-grow
	// values and it would sample a doubled pixel region on the bigger
	// atlas.
	require.InDelta(t, float32(u0Before)/2, glyph.u0, 1e-6)
	require.InDelta(t, float32(u1Before)/2, glyph.u1, 1e-6)
	require.InDelta(t, float32(v0Before)/2, glyph.v0, 1e-6)
	require.InDelta(t, float32(v1Before)/2, glyph.v1, 1e-6)
}

// TestSpriteAtlasGrowRescalesSubImageUVsAcrossMultipleGrows simulates
// the DebugPrint glyph-cache flow across TWO consecutive atlas grows —
// DebugPrint is the HUD path that was showing garbled white blocks
// after scene transitions because cached SubImage references kept
// pre-grow UVs.
func TestSpriteAtlasGrowRescalesSubImageUVsAcrossMultipleGrows(t *testing.T) {
	withMockRenderer(t)
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	src := goimage.NewRGBA(goimage.Rect(0, 0, 64, 64))
	atlased := NewImageFromImage(src)
	require.True(t, atlased.atlased)
	glyph := atlased.SubImage(goimage.Rect(4, 4, 12, 20))
	u0Before, v0Before := glyph.u0, glyph.v0
	u1Before, v1Before := glyph.u1, glyph.v1

	// First grow (512→1024).
	for i := 0; i < 6; i++ {
		big := goimage.NewRGBA(goimage.Rect(0, 0, 200, 200))
		NewImageFromImage(big)
	}
	require.InDelta(t, u0Before*0.5, glyph.u0, 1e-6)

	// Second grow (1024→2048) — need enough additional 200x200 images
	// to overflow the 1024 atlas. 1024/202 ≈ 5 per row, 5 rows = 25,
	// so ~20 more on top of the 6 already placed triggers it.
	for i := 0; i < 24; i++ {
		big := goimage.NewRGBA(goimage.Rect(0, 0, 200, 200))
		NewImageFromImage(big)
	}

	// After two grows, UVs must be quartered (both × 0.5 chained).
	require.InDelta(t, u0Before*0.25, glyph.u0, 1e-6)
	require.InDelta(t, v0Before*0.25, glyph.v0, 1e-6)
	require.InDelta(t, u1Before*0.25, glyph.u1, 1e-6)
	require.InDelta(t, v1Before*0.25, glyph.v1, 1e-6)
}

// TestSpriteAtlasGrowRescalesInFlightBatchVertices catches the real
// root cause of the DebugPrint HUD garble-after-tour bug: when a mid-
// frame atlas grow happens between DrawImage(glyph) and frame Flush,
// the batcher already holds vertex data baked from the PRE-grow UVs.
// Rescaling the Image struct's u0..u1 alone doesn't retroactively
// update those vertices, so the queued quad samples a doubled pixel
// region on the bigger texture at submit time — which is what the HUD
// looked like.
func TestSpriteAtlasGrowRescalesInFlightBatchVertices(t *testing.T) {
	_, _ = withMockRenderer(t)
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	rend := getRenderer()

	src := goimage.NewRGBA(goimage.Rect(0, 0, 64, 64))
	atlased := NewImageFromImage(src)
	require.True(t, atlased.atlased)
	glyph := atlased.SubImage(goimage.Rect(4, 4, 12, 20))

	// Capture the quad's queued UVs BEFORE the grow. DrawImage onto a
	// throwaway destination is all it takes to push a quad into the
	// batcher arena.
	dst := NewImage(32, 32)
	dst.DrawImage(glyph, nil)

	cmdsBefore := rend.batcher.Flush()
	require.Equal(t, 1, len(cmdsBefore))
	queued := cmdsBefore[0].Vertices
	u0Queued := queued[0].TexU
	v0Queued := queued[0].TexV

	// Queue the SAME quad again so we have something live for the grow
	// to rescale.
	dst.DrawImage(glyph, nil)

	// Force a grow.
	for i := 0; i < 6; i++ {
		big := goimage.NewRGBA(goimage.Rect(0, 0, 200, 200))
		NewImageFromImage(big)
	}

	cmdsAfter := rend.batcher.Flush()
	require.Equal(t, 1, len(cmdsAfter))
	rescaled := cmdsAfter[0].Vertices
	require.InDelta(t, u0Queued*0.5, rescaled[0].TexU, 1e-6,
		"queued quad's TexU must be halved by the mid-frame grow")
	require.InDelta(t, v0Queued*0.5, rescaled[0].TexV, 1e-6,
		"queued quad's TexV must be halved by the mid-frame grow")
}

func TestSpriteAtlasGrowRescalesUVs(t *testing.T) {
	withMockRenderer(t)
	SetSpriteAtlasEnabled(true)
	defer ResetSpriteAtlas()

	// Place images that fill the initial 512x512 atlas. Each 200x200 image
	// becomes a 202x202 padded entry. Two fit per row (202+1+202=405 ≤ 512),
	// two rows (202+1+202=405 ≤ 512), so 4 images fill the atlas.
	var preGrowImages []*Image
	for i := 0; i < 4; i++ {
		src := goimage.NewRGBA(goimage.Rect(0, 0, 200, 200))
		img := NewImageFromImage(src)
		require.True(t, img.atlased)
		preGrowImages = append(preGrowImages, img)
	}

	// Record the UV coordinates before growth.
	type uvs struct{ u0, v0, u1, v1 float32 }
	preUVs := make([]uvs, len(preGrowImages))
	for i, img := range preGrowImages {
		preUVs[i] = uvs{img.u0, img.v0, img.u1, img.v1}
	}

	// Verify atlas hasn't grown yet.
	_, totalPixels := SpriteAtlasStats()
	require.Equal(t, spriteAtlasInitialSize*spriteAtlasInitialSize, totalPixels)

	// Add one more image to trigger growth from 512 to 1024.
	src := goimage.NewRGBA(goimage.Rect(0, 0, 200, 200))
	growImg := NewImageFromImage(src)
	require.True(t, growImg.atlased)

	// Verify atlas has grown.
	_, totalPixels = SpriteAtlasStats()
	require.Greater(t, totalPixels, spriteAtlasInitialSize*spriteAtlasInitialSize)

	// Pre-growth images should have their UVs scaled by oldSize/newSize = 0.5.
	for i, img := range preGrowImages {
		require.InDelta(t, preUVs[i].u0*0.5, img.u0, 1e-6, "u0 not rescaled for image %d", i)
		require.InDelta(t, preUVs[i].v0*0.5, img.v0, 1e-6, "v0 not rescaled for image %d", i)
		require.InDelta(t, preUVs[i].u1*0.5, img.u1, 1e-6, "u1 not rescaled for image %d", i)
		require.InDelta(t, preUVs[i].v1*0.5, img.v1, 1e-6, "v1 not rescaled for image %d", i)
	}

	// The post-growth image's UVs should be relative to the new atlas size.
	// They should NOT need rescaling since they were computed after growth.
	require.Greater(t, growImg.u1, float32(0))
	require.Greater(t, growImg.v1, float32(0))
}

func TestAtlasEntryFits(t *testing.T) {
	require.True(t, atlasEntryFits(1, 1))
	require.True(t, atlasEntryFits(256, 256))
	require.False(t, atlasEntryFits(257, 1))
	require.False(t, atlasEntryFits(1, 257))
	require.False(t, atlasEntryFits(0, 10))
	require.False(t, atlasEntryFits(10, 0))
}

func TestSetSpriteAtlasEnabled(t *testing.T) {
	withMockRenderer(t)

	SetSpriteAtlasEnabled(true)
	defer func() {
		SetSpriteAtlasEnabled(true)
		ResetSpriteAtlas()
	}()

	src := goimage.NewRGBA(goimage.Rect(0, 0, 8, 8))
	img := NewImageFromImage(src)
	require.True(t, img.atlased)

	SetSpriteAtlasEnabled(false)
	ResetSpriteAtlas()

	img2 := NewImageFromImage(src)
	require.False(t, img2.atlased)
}
