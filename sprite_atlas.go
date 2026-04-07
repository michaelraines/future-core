package futurerender

import (
	"sync"
)

// Sprite atlas constants. Small images (≤maxAtlasEntrySize on each axis)
// are packed into shared atlas textures to reduce texture-change batch breaks.
const (
	spriteAtlasInitialSize = 512
	spriteAtlasMaxSize     = 4096
	maxAtlasEntrySize      = 256 // images larger than this skip atlasing
)

// spriteAtlas manages a set of atlas textures for small sprites. When a small
// Image is created via NewImageFromImage, the atlas packs its padded pixel
// data into a shared GPU texture and returns an Image with adjusted UVs.
// All images sharing an atlas use the same textureID, allowing the batcher
// to merge their draw calls into a single batch.
type spriteAtlas struct {
	mu      sync.Mutex
	atlases []*atlasPage
}

// atlasPage is a single atlas texture with row-based packing.
type atlasPage struct {
	image     *Image
	textureID uint32
	size      int
	rows      []atlasPageRow
	// placed tracks all Images returned by tryPlace so that grow() can
	// rescale their UV coordinates when the atlas texture is resized.
	placed []*Image
	// pixels is a CPU-side shadow of the atlas texture data (RGBA).
	// Maintained on every UploadRegion so that grow() can copy old
	// content to a new texture without calling ReadPixels. This avoids
	// a GPU→CPU→GPU roundtrip and prevents a deadlock on WASM where
	// ReadPixels requires an async JS promise that cannot resolve
	// inside a synchronous Draw callback.
	pixels []byte
}

type atlasPageRow struct {
	y       int
	height  int
	cursorX int
}

var globalSpriteAtlas spriteAtlas

// spriteAtlasEnabled controls whether NewImageFromImage uses atlasing.
// Enabled by default.
var spriteAtlasEnabled = true

// SetSpriteAtlasEnabled enables or disables automatic sprite atlasing.
func SetSpriteAtlasEnabled(enabled bool) {
	globalSpriteAtlas.mu.Lock()
	defer globalSpriteAtlas.mu.Unlock()
	spriteAtlasEnabled = enabled
}

// tryAtlas attempts to pack a padded image into a sprite atlas. Returns an
// Image backed by an atlas page, or nil if atlasing is disabled/unavailable.
func (sa *spriteAtlas) tryAtlas(padded []byte, padW, padH, contentW, contentH int, rend *renderer) *Image {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if !spriteAtlasEnabled {
		return nil
	}
	if contentW > maxAtlasEntrySize || contentH > maxAtlasEntrySize {
		return nil
	}
	if rend == nil || rend.device == nil {
		return nil
	}

	// Try to place in an existing atlas page.
	for _, page := range sa.atlases {
		if img := page.tryPlace(padded, padW, padH, contentW, contentH, rend); img != nil {
			return img
		}
	}

	// Create a new atlas page.
	page := sa.newPage()
	if page == nil {
		return nil
	}
	return page.tryPlace(padded, padW, padH, contentW, contentH, rend)
}

func (sa *spriteAtlas) newPage() *atlasPage {
	size := spriteAtlasInitialSize
	img := NewImage(size, size)
	if img.texture == nil {
		return nil
	}
	page := &atlasPage{
		image:     img,
		textureID: img.textureID,
		size:      size,
		pixels:    make([]byte, size*size*4),
	}
	sa.atlases = append(sa.atlases, page)
	return page
}

// tryPlace attempts to allocate space for a padded image in this atlas page.
func (ap *atlasPage) tryPlace(padded []byte, padW, padH, contentW, contentH int, rend *renderer) *Image {
	x, y, ok := ap.allocate(padW, padH)
	if !ok {
		// Try growing the atlas.
		if !ap.grow(rend) {
			return nil
		}
		x, y, ok = ap.allocate(padW, padH)
		if !ok {
			return nil
		}
	}

	// Upload the padded pixel data at the allocated position.
	if ap.image.texture != nil {
		ap.image.texture.UploadRegion(padded, x, y, padW, padH, 0)
	}

	// Update the CPU-side shadow buffer.
	ap.copyToShadow(padded, x, y, padW, padH)

	// Compute UVs mapping to the content region within the padded area.
	atlasW := float32(ap.size)
	atlasH := float32(ap.size)
	u0 := float32(x+1) / atlasW
	v0 := float32(y+1) / atlasH
	u1 := float32(x+1+contentW) / atlasW
	v1 := float32(y+1+contentH) / atlasH

	img := &Image{
		width:     contentW,
		height:    contentH,
		texture:   ap.image.texture,
		textureID: ap.textureID,
		parent:    ap.image,
		u0:        u0,
		v0:        v0,
		u1:        u1,
		v1:        v1,
		atlased:   true,
	}
	ap.placed = append(ap.placed, img)
	return img
}

// allocate finds space using row-based packing.
func (ap *atlasPage) allocate(w, h int) (x, y int, ok bool) {
	if w <= 0 || h <= 0 || w > ap.size || h > ap.size {
		return 0, 0, false
	}

	// Try existing rows.
	for i := range ap.rows {
		row := &ap.rows[i]
		if row.cursorX+w <= ap.size && h <= row.height {
			x = row.cursorX
			y = row.y
			row.cursorX += w + 1 // 1px gap between entries
			return x, y, true
		}
	}

	// New row.
	nextY := 0
	if len(ap.rows) > 0 {
		last := ap.rows[len(ap.rows)-1]
		nextY = last.y + last.height + 1
	}
	if nextY+h > ap.size {
		return 0, 0, false
	}

	ap.rows = append(ap.rows, atlasPageRow{
		y:       nextY,
		height:  h,
		cursorX: w + 1,
	})
	return 0, nextY, true
}

// grow doubles the atlas page size up to spriteAtlasMaxSize.
func (ap *atlasPage) grow(rend *renderer) bool {
	newSize := ap.size * 2
	if newSize > spriteAtlasMaxSize {
		return false
	}

	// Create a new larger texture and copy old content via shadow buffer.
	newImg := NewImage(newSize, newSize)
	if newImg.texture == nil {
		return false
	}

	// Copy old atlas content to the new texture using the CPU shadow buffer.
	// This avoids ReadPixels which deadlocks on WASM (async promise inside
	// a synchronous Draw callback) and is faster on all platforms (no GPU
	// readback stall).
	oldSize := ap.size
	if ap.pixels != nil && newImg.texture != nil {
		newImg.texture.UploadRegion(ap.pixels, 0, 0, oldSize, oldSize, 0)
	}

	// Dispose old atlas texture.
	ap.image.Dispose()

	// Update the page to use the new texture. All existing Images that
	// reference this atlas page still hold the old texture/textureID.
	// We need to update the texture registration so the engine resolves
	// the old textureID to the new GPU texture.
	ap.image = newImg
	ap.size = newSize

	// Grow the shadow buffer: allocate new size, copy old content.
	newPixels := make([]byte, newSize*newSize*4)
	if ap.pixels != nil {
		for row := 0; row < oldSize; row++ {
			srcOff := row * oldSize * 4
			dstOff := row * newSize * 4
			copy(newPixels[dstOff:dstOff+oldSize*4], ap.pixels[srcOff:srcOff+oldSize*4])
		}
	}
	ap.pixels = newPixels

	// Rescale UV coordinates for all previously placed images. Their UVs
	// were computed relative to the old atlas size and must be adjusted
	// for the new (larger) texture.
	scale := float32(oldSize) / float32(newSize)
	for _, img := range ap.placed {
		img.u0 *= scale
		img.v0 *= scale
		img.u1 *= scale
		img.v1 *= scale
	}

	// Re-register the new texture under the original atlas textureID so
	// existing Images (which still hold ap.textureID) resolve correctly.
	if rend.registerTexture != nil {
		rend.registerTexture(ap.textureID, newImg.texture)
	}

	return true
}

// copyToShadow copies a rectangular region of pixel data into the CPU shadow buffer.
func (ap *atlasPage) copyToShadow(data []byte, x, y, w, h int) {
	if ap.pixels == nil || len(data) < w*h*4 {
		return
	}
	stride := ap.size * 4
	for row := 0; row < h; row++ {
		srcOff := row * w * 4
		dstOff := (y+row)*stride + x*4
		if dstOff+w*4 > len(ap.pixels) {
			break
		}
		copy(ap.pixels[dstOff:dstOff+w*4], data[srcOff:srcOff+w*4])
	}
}

// atlasEntryFits returns true if a source image is small enough to atlas.
func atlasEntryFits(w, h int) bool {
	return w > 0 && h > 0 && w <= maxAtlasEntrySize && h <= maxAtlasEntrySize
}

// newImageFromImageAtlased attempts to place the image into a sprite atlas.
// Returns nil if the image is too large or atlasing is disabled.
func newImageFromImageAtlased(padded []byte, padW, padH, contentW, contentH int) *Image {
	rend := getRenderer()
	if rend == nil {
		return nil
	}
	img := globalSpriteAtlas.tryAtlas(padded, padW, padH, contentW, contentH, rend)
	if img == nil {
		return nil
	}
	// Register the atlas texture under the atlased image's textureID
	// is not needed — the image shares the atlas page's textureID.
	return img
}

// ResetSpriteAtlas disposes all atlas pages. Intended for testing and
// context-loss recovery.
func ResetSpriteAtlas() {
	globalSpriteAtlas.mu.Lock()
	defer globalSpriteAtlas.mu.Unlock()
	for _, page := range globalSpriteAtlas.atlases {
		if page.image != nil {
			page.image.Dispose()
		}
	}
	globalSpriteAtlas.atlases = nil
}

// SpriteAtlasStats returns diagnostic information about the sprite atlas.
func SpriteAtlasStats() (pageCount, totalPixels int) {
	globalSpriteAtlas.mu.Lock()
	defer globalSpriteAtlas.mu.Unlock()
	pageCount = len(globalSpriteAtlas.atlases)
	for _, page := range globalSpriteAtlas.atlases {
		totalPixels += page.size * page.size
	}
	return pageCount, totalPixels
}
