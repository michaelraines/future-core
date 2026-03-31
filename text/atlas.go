package text

import (
	"image"

	futurerender "github.com/michaelraines/future-core"
)

const (
	defaultAtlasSize = 512
	maxAtlasSize     = 4096
)

// fontAtlas manages a texture atlas for glyph bitmaps.
// It uses row-based packing: each row has a fixed height determined by
// the tallest glyph placed in it.
type fontAtlas struct {
	image *futurerender.Image
	size  int // current atlas dimension (square)

	// generation increments on every grow(), allowing caches to detect
	// that their stored atlas coordinates are stale.
	generation int

	// Row packing state.
	rows []atlasRow
}

// atlasRow represents a horizontal row in the atlas.
type atlasRow struct {
	y       int // top-left y position in the atlas
	height  int // row height (tallest glyph in this row)
	cursorX int // next free x position in this row
}

// newFontAtlas creates a new font atlas with the default initial size.
func newFontAtlas() *fontAtlas {
	return &fontAtlas{
		size: defaultAtlasSize,
	}
}

// ensureImage lazily creates the atlas texture.
func (a *fontAtlas) ensureImage() {
	if a.image != nil {
		return
	}
	a.image = futurerender.NewImage(a.size, a.size)
}

// allocate finds space for a glyph of the given dimensions.
// Returns the top-left position and true if successful.
func (a *fontAtlas) allocate(w, h int) (x, y int, ok bool) {
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}

	a.ensureImage()

	// Try to fit in an existing row.
	for i := range a.rows {
		row := &a.rows[i]
		if row.cursorX+w <= a.size && h <= row.height {
			x = row.cursorX
			y = row.y
			row.cursorX += w + 1 // 1px padding to avoid bleeding
			return x, y, true
		}
	}

	// Start a new row.
	nextY := 0
	if len(a.rows) > 0 {
		last := a.rows[len(a.rows)-1]
		nextY = last.y + last.height + 1 // 1px padding between rows
	}

	if nextY+h > a.size {
		// Atlas is full — grow it.
		if !a.grow() {
			return 0, 0, false
		}
		return a.allocate(w, h) // retry after growth
	}

	a.rows = append(a.rows, atlasRow{
		y:       nextY,
		height:  h,
		cursorX: w + 1,
	})
	return 0, nextY, true
}

// grow doubles the atlas size up to maxAtlasSize, copying existing content.
func (a *fontAtlas) grow() bool {
	newSize := a.size * 2
	if newSize > maxAtlasSize {
		return false
	}
	a.size = newSize
	// Create new larger image. Old glyph data needs to be re-uploaded
	// by the cache, but for simplicity we just allocate a new image.
	// The glyph cache will be rebuilt on next draw.
	if a.image != nil {
		a.image.Dispose()
	}
	a.image = futurerender.NewImage(newSize, newSize)
	a.rows = a.rows[:0]
	a.generation++
	return true
}

// upload writes glyph pixel data to the atlas at the given position.
func (a *fontAtlas) upload(pix []byte, x, y, w, h int) {
	if a.image == nil {
		return
	}
	a.image.WritePixelsRegion(pix, x, y, w, h)
}

// subImage returns a sub-image of the atlas for the given glyph region.
func (a *fontAtlas) subImage(x, y, w, h int) *futurerender.Image {
	if a.image == nil {
		return nil
	}
	return a.image.SubImage(image.Rect(x, y, x+w, y+h))
}
