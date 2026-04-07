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
