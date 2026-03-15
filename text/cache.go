package text

import (
	goimage "image"
	"image/color"
	"image/draw"

	"golang.org/x/image/math/fixed"
)

// glyphEntry holds cached information about a single rasterized glyph.
type glyphEntry struct {
	// advance is the horizontal advance in pixels.
	advance float64

	// atlasX, atlasY is the position of this glyph in the atlas.
	atlasX, atlasY int
	// width, height is the size of the glyph bitmap.
	width, height int

	// bearingX, bearingY are the offsets from the drawing position to the
	// top-left of the glyph bitmap.
	bearingX, bearingY float64

	// empty is true if the glyph has no visible pixels (e.g., space).
	empty bool
}

// glyphCache manages rasterized glyph bitmaps for a single Face.
type glyphCache struct {
	face    *Face
	entries map[rune]*glyphEntry

	// generation tracks the atlas generation at the time entries were cached.
	// When the atlas grows, its generation increments and all cached atlas
	// coordinates become stale. get() detects this mismatch and re-rasterizes.
	generation int
}

// newGlyphCache creates a glyph cache for the given face.
func newGlyphCache(f *Face) *glyphCache {
	return &glyphCache{
		face:    f,
		entries: make(map[rune]*glyphEntry),
	}
}

// get returns the glyph entry for the given rune, rasterizing it if needed.
// atlas must be provided for new glyphs that need atlas allocation.
// If the atlas has grown since the last call, all cached entries are discarded
// because their atlas coordinates are stale.
func (c *glyphCache) get(r rune, atlas *fontAtlas) *glyphEntry {
	if atlas != nil && atlas.generation != c.generation {
		// Atlas was rebuilt — all cached coordinates are invalid.
		clear(c.entries)
		c.generation = atlas.generation
	}

	if e, ok := c.entries[r]; ok {
		return e
	}

	e := c.rasterize(r, atlas)
	c.entries[r] = e
	return e
}

// rasterize creates a glyph bitmap and uploads it to the atlas.
func (c *glyphCache) rasterize(r rune, atlas *fontAtlas) *glyphEntry {
	f := c.face.face

	// Get glyph metrics.
	advance, ok := f.GlyphAdvance(r)
	if !ok {
		return &glyphEntry{advance: 0, empty: true}
	}

	bounds, _, ok := f.GlyphBounds(r)
	if !ok {
		return &glyphEntry{advance: fixedToFloat(advance), empty: true}
	}

	// Compute glyph bitmap size.
	w := fixedCeil(bounds.Max.X) - fixedFloor(bounds.Min.X)
	h := fixedCeil(bounds.Max.Y) - fixedFloor(bounds.Min.Y)

	if w <= 0 || h <= 0 {
		return &glyphEntry{advance: fixedToFloat(advance), empty: true}
	}

	// Rasterize: draw the glyph into an RGBA image (white on transparent).
	dst := goimage.NewRGBA(goimage.Rect(0, 0, w, h))

	// The dot position: glyph origin is at (-bounds.Min.X, -bounds.Min.Y).
	dot := fixed.Point26_6{
		X: -bounds.Min.X,
		Y: -bounds.Min.Y,
	}

	dr, mask, maskp, _, ok := f.Glyph(dot, r)
	if !ok || mask == nil {
		return &glyphEntry{advance: fixedToFloat(advance), empty: true}
	}

	// Draw the glyph mask as white pixels with the mask's alpha.
	src := goimage.NewUniform(color.White)
	draw.DrawMask(dst, dr.Sub(dr.Min), src, goimage.Point{}, mask, maskp, draw.Over)

	// Allocate space in the atlas and upload.
	entry := &glyphEntry{
		advance:  fixedToFloat(advance),
		width:    w,
		height:   h,
		bearingX: fixedToFloat(bounds.Min.X),
		bearingY: fixedToFloat(bounds.Min.Y),
	}

	if atlas != nil {
		ax, ay, ok := atlas.allocate(w, h)
		if ok {
			atlas.upload(dst.Pix, ax, ay, w, h)
			entry.atlasX = ax
			entry.atlasY = ay
		} else {
			entry.empty = true
		}
	}

	return entry
}

// fixedFloor returns floor of a fixed-point value as int.
func fixedFloor(v fixed.Int26_6) int {
	return int(v >> 6)
}

// fixedCeil returns ceil of a fixed-point value as int.
func fixedCeil(v fixed.Int26_6) int {
	return int((v + 63) >> 6)
}
