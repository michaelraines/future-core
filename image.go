package futurerender

import (
	"fmt"
	goimage "image"
	"image/color"
	"image/draw"
	gomath "math"
	"os"
	"sync/atomic"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
	fmath "github.com/michaelraines/future-core/math"
)

// pixelSnapEnabled is the global pixel-snap setting.
// When enabled, DrawImage rounds transformed vertex positions to the nearest
// integer pixel, eliminating sub-pixel shimmer and seams between adjacent
// sprites. Individual DrawImageOptions.PixelSnap overrides this.
var pixelSnapEnabled atomic.Bool

// SetPixelSnapEnabled enables or disables global pixel snapping for DrawImage.
// When enabled, all DrawImage calls round vertex positions to integer pixels
// unless overridden by DrawImageOptions.PixelSnap.
func SetPixelSnapEnabled(enabled bool) {
	pixelSnapEnabled.Store(enabled)
}

// IsPixelSnapEnabled returns the current global pixel-snap setting.
func IsPixelSnapEnabled() bool {
	return pixelSnapEnabled.Load()
}

// pixelSnap rounds a float64 coordinate to the nearest integer pixel.
func pixelSnap(v float64) float64 {
	return gomath.Round(v)
}

// Image represents a renderable image (texture). It can be used as a
// render target or as a source for drawing operations.
//
// This type is the equivalent of ebiten.Image.
type Image struct {
	width, height int

	// bounds is the destination-rect this Image represents on its root
	// render target. Top-level images: {0, 0, width, height} (Min.X/Y
	// are zero). Sub-images: the rect passed to SubImage, intersected
	// with the parent's bounds — Min can be non-zero. The draw path
	// forwards this to the sprite pass as a GPU scissor so writes to a
	// sub-image are clipped to the sub-rect instead of bleeding onto
	// the rest of the root target. Mirrors ebiten's Image.Bounds()
	// semantics (see vendor/.../ebiten/v2/image.go:968-973).
	bounds goimage.Rectangle

	// disposed is true once disposeNow() has released the GPU resources
	// — at that point any further use is a no-op. Draw-path guards check
	// this flag, not pendingDispose: an image between Dispose() and the
	// end-of-frame drain still has live GPU resources and must continue
	// to accept draws, or the batcher silently loses commands that were
	// already queued against it (this was the "scene transition leak"
	// regression on WebGPU — see renderer.deferDispose docstring).
	disposed bool

	// pendingDispose is set synchronously by Dispose() to block double-
	// dispose while the image waits on the deferred-drain queue. It
	// intentionally does NOT gate the draw path — see `disposed` above.
	pendingDispose bool

	// GPU texture handle (nil for screen images or stub builds).
	texture   backend.Texture
	textureID uint32

	// renderTarget is the off-screen framebuffer for this image.
	// Non-nil when this image is used as a draw target.
	renderTarget backend.RenderTarget

	// Sub-image UV region within the parent texture.
	// Full image: u0=0, v0=0, u1=1, v1=1.
	parent         *Image
	u0, v0, u1, v1 float32

	// padded indicates the GPU texture has a 1px transparent border.
	// When true, pixel operations (Set, WritePixels, ReadPixels) must
	// offset coordinates by 1 to account for the padding.
	padded bool

	// atlased indicates this image is packed into a shared sprite atlas.
	// Atlased images do not support WritePixels, Set, or ReadPixels since
	// their pixel data is interleaved with other images in the atlas.
	atlased bool

	// atlasPageBackref, on atlased images AND their SubImage descendants,
	// points at the atlas page that owns the underlying GPU texture. On
	// atlas grow the page rescales its UVs in-place; tracking descendants
	// here lets `SubImage` enroll any derived glyph/sprite sub-image in
	// the same rescale list so cached SubImage references (e.g. the
	// DebugPrint per-rune cache, the text package's glyphImageCache)
	// don't end up sampling a doubled pixel region after grow and
	// producing white/garbled text.
	atlasPageBackref *atlasPage

	// aaBuffer is a lazily-allocated 2x-scale offscreen used to implement
	// DrawTrianglesOptions.AntiAlias. Drawn into at AA call sites, then
	// flushed (downsample-composited) back into img at the next sync
	// point. Mirrors Ebitengine's bigOffscreenImage. nil until the first
	// AA draw on this image.
	aaBuffer           *Image
	aaBufferRegion     goimage.Rectangle // 1x region this buffer covers
	aaBufferBlend      Blend             // blend mode currently accumulated in the buffer
	aaBufferDirty      bool              // true when buffer has pending draws not yet flushed
	aaBufferNeedsClear bool              // set by Clear(); consumed by drawTrianglesAA before first AA draw

	// cpuPixels is a CPU-side copy of this image's unpadded RGBA content,
	// populated when the image is constructed from CPU data
	// (NewImageFromImage, WritePixels). When cpuPixelsValid is true,
	// ReadPixels serves the cache synchronously without a GPU round-trip.
	// The cache is invalidated by any GPU-side write (DrawImage,
	// DrawTriangles, Fill, Clear, Set, DrawRectShader, DrawTrianglesShader,
	// AA flush) because the GPU texture then holds newer bytes than the
	// CPU cache. Rationale: WebGPU browser mode has no synchronous GPU
	// readback path — mapAsync is the only option, and awaiting it from
	// inside a RAF callback deadlocks the Go runtime (the JS event loop
	// can't tick until the callback returns, so the promise never
	// resolves). The cache fast-paths the common "read back an image I
	// just loaded from disk" pattern (AtlasPacker) so it doesn't need to
	// hit the GPU at all. Render-target readback still goes through the
	// async path — see ReadPixelsAsync for callers that need that.
	//
	// Format: len = width * height * 4 bytes, RGBA, row-major, no padding,
	// matching what ReadPixels(dst) would produce from a GPU readback.
	cpuPixels      []byte
	cpuPixelsValid bool
}

// aaBufferScale is the supersample factor for anti-aliased DrawTriangles.
// The AA buffer is allocated at Nx the 1x region so linear-downsampled
// bilinear averaging produces a smooth edge. Controlled by
// FUTURE_CORE_AA_SCALE (default "2").
var aaBufferScale = func() int {
	if os.Getenv("FUTURE_CORE_AA_SCALE") == "1" {
		return 1
	}
	return 2
}()

// NewImage creates a new blank image with the given dimensions.
// If the rendering backend is initialized, a GPU texture is allocated.
// The underlying GPU render target is labeled "image-rt-<id>-<w>x<h>"
// for diagnostic purposes; callers that want a more specific label
// (e.g. the sprite atlas, AA buffer) should use newImageLabeled.
func NewImage(width, height int) *Image {
	return newImageLabeled(width, height, "image-rt")
}

// newImageLabeled is NewImage with an explicit label prefix. Used by
// subsystems that create render-target-backed images for specific
// purposes — sprite atlas pages, AA supersample buffers — so the
// resulting label in WebGPU / Vulkan validation errors identifies the
// owning subsystem at a glance.
func newImageLabeled(width, height int, labelPrefix string) *Image {
	img := &Image{
		width:  width,
		height: height,
		u0:     0, v0: 0,
		u1: 1, v1: 1,
		bounds: goimage.Rect(0, 0, width, height),
	}

	// Allocate GPU render target if a device is available. The render
	// target's color texture is used as the image's texture so that
	// content drawn TO this image is visible when the image is sampled.
	if rend := getRenderer(); rend != nil && rend.device != nil {
		img.textureID = rend.allocTextureID()

		rt, rtErr := rend.device.NewRenderTarget(backend.RenderTargetDescriptor{
			Width:       width,
			Height:      height,
			ColorFormat: backend.TextureFormatRGBA8,
			Label:       fmt.Sprintf("%s-%d-%dx%d", labelPrefix, img.textureID, width, height),
		})
		if rtErr == nil {
			img.renderTarget = rt
			// Use the render target's color texture as the image texture
			// so draws into this image are visible when it's sampled.
			img.texture = rt.ColorTexture()
			if rend.registerTexture != nil {
				rend.registerTexture(img.textureID, img.texture)
			}
			if rend.registerRenderTarget != nil {
				rend.registerRenderTarget(img.textureID, rt)
			}
		}
	}

	// Track for context loss recovery.
	if tracker := getTracker(); tracker != nil {
		tracker.TrackImage(img, nil, true)
	}

	return img
}

// NewImageOptions specifies options for NewImageWithOptions.
type NewImageOptions struct {
	// Unmanaged indicates that the image is not managed by the engine.
	// Unmanaged images are not automatically disposed on context loss.
	Unmanaged bool
}

// NewImageWithOptions creates a new blank image with the given bounds and options.
// This matches Ebitengine's NewImageWithOptions.
func NewImageWithOptions(bounds goimage.Rectangle, opts *NewImageOptions) *Image {
	w := bounds.Dx()
	h := bounds.Dy()
	return NewImage(w, h)
}

// NewImageFromImage creates an Image from a Go image.Image.
// The pixel data is uploaded to the GPU immediately.
//
// The GPU texture is allocated with a 1px transparent border (padding) around
// the source content. This prevents bilinear filtering from bleeding in
// neighboring texel data at sprite edges and sub-image boundaries. The image's
// UV coordinates are adjusted to map to the padded content region.
func NewImageFromImage(src goimage.Image) *Image {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Convert to RGBA if needed.
	rgba, ok := src.(*goimage.RGBA)
	if !ok {
		rgba = goimage.NewRGBA(bounds)
		draw.Draw(rgba, bounds, src, bounds.Min, draw.Src)
	}

	// Allocate padded texture: 1px transparent border on all sides.
	padW, padH := w+2, h+2
	padded := make([]byte, padW*padH*4) // zero-initialized = transparent black
	for row := 0; row < h; row++ {
		srcOff := row * rgba.Stride
		dstOff := ((row + 1) * padW * 4) + 4 // offset by 1 row and 1 col
		copy(padded[dstOff:dstOff+w*4], rgba.Pix[srcOff:srcOff+w*4])
	}

	// UVs map to the content region within the padded texture.
	u0 := float32(1) / float32(padW)
	v0 := float32(1) / float32(padH)
	u1 := float32(w+1) / float32(padW)
	v1 := float32(h+1) / float32(padH)

	// Try to pack into a sprite atlas to share a GPU texture with other
	// small images, reducing texture-change batch breaks.
	if atlasEntryFits(w, h) {
		if atlased := newImageFromImageAtlased(padded, padW, padH, w, h); atlased != nil {
			// Track for context loss recovery.
			if tracker := getTracker(); tracker != nil {
				tracker.TrackImage(atlased, rgba.Pix, false)
			}
			return atlased
		}
	}

	img := &Image{
		width:  w,
		height: h,
		u0:     u0, v0: v0,
		u1: u1, v1: v1,
		padded: true,
		bounds: goimage.Rect(0, 0, w, h),
	}

	// Cache the unpadded CPU bytes so ReadPixels can serve them
	// synchronously without a GPU round-trip. rgba.Pix may have a stride
	// wider than w*4 when the source was a SubImage; copy row-by-row.
	contentBytes := make([]byte, w*h*4)
	for row := 0; row < h; row++ {
		srcOff := row * rgba.Stride
		dstOff := row * w * 4
		copy(contentBytes[dstOff:dstOff+w*4], rgba.Pix[srcOff:srcOff+w*4])
	}
	img.cpuPixels = contentBytes
	img.cpuPixelsValid = true

	if rend := getRenderer(); rend != nil && rend.device != nil {
		tex, err := rend.device.NewTexture(backend.TextureDescriptor{
			Width:  padW,
			Height: padH,
			Format: backend.TextureFormatRGBA8,
			Filter: backend.FilterNearest,
			WrapU:  backend.WrapClamp,
			WrapV:  backend.WrapClamp,
			Data:   padded,
			Label:  fmt.Sprintf("image-tex-%dx%d", w, h),
		})
		if err == nil {
			img.texture = tex
			img.textureID = rend.allocTextureID()
			if rend.registerTexture != nil {
				rend.registerTexture(img.textureID, tex)
			}
		}
	}

	// Track for context loss recovery, preserving original pixel data.
	if tracker := getTracker(); tracker != nil {
		tracker.TrackImage(img, rgba.Pix, false)
	}

	return img
}

// invalidateCPUPixels marks the CPU-side pixel cache stale so the next
// ReadPixels will go to the GPU. Called from every GPU write path
// (DrawImage, DrawTriangles, Fill, Clear, Set, DrawRectShader,
// DrawTrianglesShader, AA buffer flush). Cheap no-op when the image
// never had a cache in the first place.
func (img *Image) invalidateCPUPixels() {
	if img.cpuPixelsValid {
		img.cpuPixelsValid = false
	}
}

// Size returns the image dimensions.
func (img *Image) Size() (width, height int) {
	return img.width, img.height
}

// Bounds returns the image bounds as an image.Rectangle.
// This matches Ebitengine's Image.Bounds() signature: for top-level
// images, Min is (0, 0) and Max is (width, height); for SubImage
// results, Min is the rect passed to SubImage so draw coordinates can
// live in the same space as the root render target.
func (img *Image) Bounds() goimage.Rectangle {
	if img.bounds.Dx() == 0 && img.bounds.Dy() == 0 {
		// Safety net for any legacy Image constructed without
		// populating bounds directly (older tests, context-recovery
		// paths) — fall back to the size-based rect.
		return goimage.Rect(0, 0, img.width, img.height)
	}
	return img.bounds
}

// clipParams returns the scissor rect to apply when drawing TO this
// image, or (0, 0, 0, 0) meaning "no scissor — full render target".
// Sub-images (Bounds with a non-zero Min) get clipped to their sub-rect
// so draws don't bleed onto the rest of the root target; top-level
// images skip the scissor call entirely to avoid needless state changes.
func (img *Image) clipParams() (x, y, w, h int32) {
	b := img.Bounds()
	if b.Min.X == 0 && b.Min.Y == 0 {
		return 0, 0, 0, 0
	}
	return int32(b.Min.X), int32(b.Min.Y), int32(b.Dx()), int32(b.Dy())
}

// DrawImage draws src onto img with the given options.
//
// If either src or img has a pending anti-aliased triangle buffer (from a
// prior DrawTriangles call with AntiAlias=true), it is flushed to its
// parent before the draw — this ensures src's content is fully resolved
// before it's sampled, and any AA draws queued against img have taken
// effect before the new draw lands on top.
func (img *Image) DrawImage(src *Image, opts *DrawImageOptions) {
	src.flushAABufferIfNeeded()
	img.flushAABufferIfNeeded()
	img.drawImageRaw(src, opts)
}

// drawImageRaw is the hook-free variant of DrawImage. It skips the AA
// sync hooks so that flushAABuffer can call it during a flush without
// recursing. All other sites should prefer DrawImage.
func (img *Image) drawImageRaw(src *Image, opts *DrawImageOptions) {
	if img.disposed || src == nil || src.disposed {
		return
	}
	img.invalidateCPUPixels()
	rend := getRenderer()
	if rend == nil || rend.batcher == nil {
		return
	}

	var o DrawImageOptions
	if opts != nil {
		o = *opts
	}

	// Source dimensions and UV.
	srcW := float32(src.width)
	srcH := float32(src.height)
	u0, v0, u1, v1 := src.u0, src.v0, src.u1, src.v1

	// Apply GeoM to the four corners of the source rect.
	// Corners in source space: (0,0), (srcW,0), (srcW,srcH), (0,srcH).
	x0, y0 := o.GeoM.Apply(0, 0)
	x1, y1 := o.GeoM.Apply(float64(srcW), 0)
	x2, y2 := o.GeoM.Apply(float64(srcW), float64(srcH))
	x3, y3 := o.GeoM.Apply(0, float64(srcH))

	// Pixel snapping: round vertex positions to nearest integer to eliminate
	// sub-pixel shimmer and seams between adjacent sprites.
	if o.PixelSnap || pixelSnapEnabled.Load() {
		x0, y0 = pixelSnap(x0), pixelSnap(y0)
		x1, y1 = pixelSnap(x1), pixelSnap(y1)
		x2, y2 = pixelSnap(x2), pixelSnap(y2)
		x3, y3 = pixelSnap(x3), pixelSnap(y3)
	}

	// Color scale (default to opaque white). Passed through to the vertex
	// verbatim — matching Ebitengine's DrawImage, which lets the shader
	// handle the premultiplication + RGB-clamp. See the `min(clr.rgb,
	// clr.a)` line in spriteFragmentShader for the shader-side
	// correction that makes `ColorScale.ScaleAlpha(0.5)` produce a
	// correctly-premultiplied 50% fade instead of a blown-out SourceOver.
	cr, cg, cb, ca := o.ColorScale.rgbaOrDefault()

	// Determine texture ID: use source texture, or white texture for nil.
	texID := src.textureID
	if src.texture == nil {
		texID = rend.whiteTextureID
	}

	// Map public blend mode and filter to backend types.
	blend := blendToBackend(o.Blend)
	filter := filterToBackend(o.Filter)
	colorBody, colorTrans := colorMatrixToUniforms(o.ColorM)

	clipX, clipY, clipW, clipH := img.clipParams()
	rend.batcher.AddQuadDirect(
		batch.Vertex2D{PosX: float32(x0), PosY: float32(y0), TexU: u0, TexV: v0, R: cr, G: cg, B: cb, A: ca},
		batch.Vertex2D{PosX: float32(x1), PosY: float32(y1), TexU: u1, TexV: v0, R: cr, G: cg, B: cb, A: ca},
		batch.Vertex2D{PosX: float32(x2), PosY: float32(y2), TexU: u1, TexV: v1, R: cr, G: cg, B: cb, A: ca},
		batch.Vertex2D{PosX: float32(x3), PosY: float32(y3), TexU: u0, TexV: v1, R: cr, G: cg, B: cb, A: ca},
		batch.DrawCommand{
			TextureID:        texID,
			BlendMode:        blend,
			Filter:           filter,
			ShaderID:         0, // default sprite shader
			TargetID:         img.textureID,
			ColorBody:        colorBody,
			ColorTranslation: colorTrans,
			ClipX:            clipX, ClipY: clipY, ClipW: clipW, ClipH: clipH,
		},
	)
}

// DrawTriangles draws triangles with the given vertices, indices, and options.
// This is the low-level drawing primitive equivalent to ebiten.DrawTriangles.
//
// When opts.AntiAlias is true, the call is routed through drawTrianglesAA,
// which renders into a per-image 2x supersample buffer and defers the
// downsample-composite until the next sync point. Sub-images fall back to
// aliased rendering (they can't own a buffer — see drawTrianglesAA).
func (img *Image) DrawTriangles(vertices []Vertex, indices []uint16, src *Image, opts *DrawTrianglesOptions) {
	if img.disposed {
		return
	}
	if opts != nil && opts.AntiAlias && os.Getenv("FUTURE_CORE_NO_AA") == "" {
		img.drawTrianglesAA(vertices, indices, src, opts)
		return
	}
	src.flushAABufferIfNeeded()
	img.flushAABufferIfNeeded()
	img.invalidateCPUPixels()
	rend := getRenderer()
	if rend == nil || rend.batcher == nil {
		return
	}

	// Vertex.SrcX/SrcY are defined in source-image PIXEL coordinates
	// (matching Ebitengine's Vertex semantics), but the default sprite
	// shader samples texture(uTexture, vTexCoord) expecting normalized
	// UV in [0, 1]. Convert here using the src image's stored UV range,
	// which correctly accounts for the 1-pixel border that
	// NewImageFromImage adds around content. Without this conversion a
	// caller that computed sx/sy in atlas pixels (e.g. SpriteBatch,
	// ParticleEmitter) would see sprites rendered as a single texel
	// because pixel coordinates > 1 get clamped to the texture edge.
	var u0, v0, u1, v1 float32
	var srcW, srcH float32
	if src != nil {
		u0, v0, u1, v1 = src.u0, src.v0, src.u1, src.v1
		srcW = float32(src.width)
		srcH = float32(src.height)
	}

	// Write vertex data directly to the batcher's arena to avoid a
	// temporary make([]Vertex2D) allocation per call. In WASM, these
	// per-call allocations accumulate faster than GC can collect.
	//
	// Vertex colors (ColorR/G/B) arrive as STRAIGHT-alpha values, matching
	// Ebitengine's `DrawTrianglesOptions.ColorScaleMode ==
	// ColorScaleModeStraightAlpha` default (see ebitengine/image.go:479).
	// The default sprite shader samples `texture(uTex, uv) * vColor` and
	// BlendSourceOver uses (One, OneMinusSrcAlpha) — the PREMULTIPLIED-alpha
	// blend equation — so we must premultiply RGB by alpha here before
	// the vertex hits the shader. Without this, a caller passing (1, 1,
	// 1, 0.5) for "half-opaque white" sees SourceOver compute
	// `(1,1,1) + dst*0.5` — a blown-out white halo — instead of the
	// correct `(0.5,0.5,0.5) + dst*0.5`. Most callers construct colors
	// via `color.RGBA{...}` or `applyOpacity` which yields RGB unscaled
	// relative to A (the "invalid premultiplied"/"straight-ish" form);
	// those work correctly only with this premultiply step.
	batchVerts := rend.batcher.AllocVertices(len(vertices))
	for i, v := range vertices {
		texU, texV := v.SrcX, v.SrcY
		if src != nil && srcW > 0 && srcH > 0 {
			texU = u0 + (v.SrcX/srcW)*(u1-u0)
			texV = v0 + (v.SrcY/srcH)*(v1-v0)
		}
		batchVerts[i] = batch.Vertex2D{
			PosX: v.DstX,
			PosY: v.DstY,
			TexU: texU,
			TexV: texV,
			R:    v.ColorR * v.ColorA,
			G:    v.ColorG * v.ColorA,
			B:    v.ColorB * v.ColorA,
			A:    v.ColorA,
		}
	}
	batchIdx := rend.batcher.AllocIndices(len(indices))
	copy(batchIdx, indices)

	texID := rend.whiteTextureID
	blend := backend.BlendSourceOver
	filter := backend.FilterNearest
	fillRule := backend.FillRuleNonZero
	if src != nil {
		texID = src.textureID
	}
	if opts != nil {
		blend = blendToBackend(opts.Blend)
		filter = filterToBackend(opts.Filter)
		fillRule = fillRuleToBackend(opts.FillRule)
	}

	clipX, clipY, clipW, clipH := img.clipParams()
	rend.batcher.AddDirect(batch.DrawCommand{
		Vertices:  batchVerts,
		Indices:   batchIdx,
		TextureID: texID,
		BlendMode: blend,
		Filter:    filter,
		FillRule:  fillRule,
		ShaderID:  0,
		TargetID:  img.textureID,
		ColorBody: colorMatrixIdentityBody,
		ClipX:     clipX, ClipY: clipY, ClipW: clipW, ClipH: clipH,
	})
}

// Fill fills the entire image with the given color.
// The argument accepts any color.Color implementation (stdlib interface),
// matching Ebitengine's Image.Fill signature.
func (img *Image) Fill(clr color.Color) {
	if img.disposed {
		return
	}
	img.flushAABufferIfNeeded()
	img.invalidateCPUPixels()
	rend := getRenderer()
	if rend == nil || rend.batcher == nil {
		return
	}

	cr, cg, cb, ca := clr.RGBA()
	cR := float32(cr) / 0xffff
	cG := float32(cg) / 0xffff
	cB := float32(cb) / 0xffff
	cA := float32(ca) / 0xffff

	// Use the white texture and multiply by vertex color.
	texID := rend.whiteTextureID

	// Fill covers the image's full destination rect. For a sub-image
	// that means (bounds.Min..bounds.Max) on the root render target,
	// not (0..w, 0..h) — otherwise the quad would be positioned in the
	// wrong half of the root texture (clipped to nothing, or worse,
	// painting the root's top-left corner instead of the sub-region).
	bounds := img.Bounds()
	x0, y0 := float32(bounds.Min.X), float32(bounds.Min.Y)
	x1, y1 := float32(bounds.Max.X), float32(bounds.Max.Y)
	clipX, clipY, clipW, clipH := img.clipParams()
	rend.batcher.AddQuadDirect(
		batch.Vertex2D{PosX: x0, PosY: y0, TexU: 0, TexV: 0, R: cR, G: cG, B: cB, A: cA},
		batch.Vertex2D{PosX: x1, PosY: y0, TexU: 1, TexV: 0, R: cR, G: cG, B: cB, A: cA},
		batch.Vertex2D{PosX: x1, PosY: y1, TexU: 1, TexV: 1, R: cR, G: cG, B: cB, A: cA},
		batch.Vertex2D{PosX: x0, PosY: y1, TexU: 0, TexV: 1, R: cR, G: cG, B: cB, A: cA},
		batch.DrawCommand{
			TextureID: texID,
			BlendMode: backend.BlendSourceOver,
			ShaderID:  0,
			TargetID:  img.textureID,
			ColorBody: colorMatrixIdentityBody,
			ClipX:     clipX, ClipY: clipY, ClipW: clipW, ClipH: clipH,
		},
	)
}

// SubImage returns a sub-region of the image for sprite sheet support.
// The returned Image shares the parent's GPU texture with adjusted UVs.
// This matches Ebitengine's Image.SubImage signature (takes image.Rectangle).
//
// The returned Image's `Bounds()` is the passed rect intersected with
// the parent's bounds, so `Bounds().Min` is usually non-zero. Draws
// targeting a sub-image are clipped to that rect by the sprite pass —
// mirrors ebiten's `dstRegion` clipping, which is what orb-drop relies
// on to keep physics content out of the UI panel.
func (img *Image) SubImage(r goimage.Rectangle) *Image {
	w := float32(img.width)
	h := float32(img.height)
	rw := r.Dx()
	rh := r.Dy()

	// `r` is expressed in the parent's coordinate system — the same
	// space as `img.Bounds()` — mirroring ebiten's SubImage contract.
	// For a top-level image Bounds().Min is (0,0) so r maps directly;
	// for a nested SubImage the caller passes root-texture-absolute
	// coords. Clamp to the parent to stay inside valid sample space,
	// and convert to sub-local coords for the UV arithmetic below
	// (which expects offsets in [0, img.width/height]).
	absRect := r.Intersect(img.bounds)
	localMinX := r.Min.X - img.bounds.Min.X
	localMinY := r.Min.Y - img.bounds.Min.Y
	localMaxX := r.Max.X - img.bounds.Min.X
	localMaxY := r.Max.Y - img.bounds.Min.Y

	if w == 0 || h == 0 {
		return &Image{
			width:  rw,
			height: rh,
			bounds: absRect,
		}
	}

	// Map rect coordinates to UV space within this image's UV region.
	uRange := img.u1 - img.u0
	vRange := img.v1 - img.v0

	su0 := img.u0 + float32(localMinX)/w*uRange
	sv0 := img.v0 + float32(localMinY)/h*vRange
	su1 := img.u0 + float32(localMaxX)/w*uRange
	sv1 := img.v0 + float32(localMaxY)/h*vRange

	// Point to the root texture owner.
	parent := img
	if img.parent != nil {
		parent = img.parent
	}

	// Propagate img.textureID rather than parent.textureID. For atlased
	// images these now differ: the sub-image's textureID is the atlas's
	// shared alias (the one the renderer re-points on every grow), while
	// parent.textureID is the underlying atlas page's own ID — only used
	// for ownership bookkeeping inside disposeNow.
	sub := &Image{
		width:            rw,
		height:           rh,
		texture:          parent.texture,
		textureID:        img.textureID,
		parent:           parent,
		u0:               su0,
		v0:               sv0,
		u1:               su1,
		v1:               sv1,
		atlased:          img.atlased,
		atlasPageBackref: img.atlasPageBackref,
		bounds:           absRect,
	}
	// If this sub-image is rooted in a sprite-atlas page, enroll it in
	// the page's placed list so atlas.grow() rescales its UVs alongside
	// direct atlased images. Without this, long-lived cached sub-images
	// (e.g. util/debugprint.go's per-rune cache) retain pre-grow UVs,
	// end up sampling a 2× pixel region after every grow, and render
	// as white/garbled blocks. The `atlasPageBackref` may be nil if the
	// parent isn't atlased — non-atlased SubImage is a pure UV-range
	// calculation and needs no registration.
	if sub.atlasPageBackref != nil {
		sub.atlasPageBackref.registerDescendant(sub)
	}
	return sub
}

// Clear resets all pixels to transparent black (0, 0, 0, 0).
// This is equivalent to ebiten.Image.Clear.
//
// Implemented by marking the image's render target for clearing on its
// next BeginRenderPass (via LoadActionClear). This is a GPU-native clear
// with zero CPU-side data transfer — unlike texture.Upload(zeros) which
// would copy width×height×4 bytes through the Go→JS boundary per call.
//
// Note: using Fill(transparent) with BlendSourceOver would be a no-op
// (src*0 + dst*1 = dst), leaving the previous frame's content intact.
func (img *Image) Clear() {
	if img.disposed {
		return
	}
	img.flushAABufferIfNeeded()
	img.invalidateCPUPixels()
	// Mark this target for GPU-native clearing on its next
	// BeginRenderPass. The sprite pass checks this via
	// ConsumePendingClear and emits LoadActionClear with transparent
	// black — no CPU data transfer required.
	if rend := getRenderer(); rend != nil {
		rend.pendingClears.RequestOnce(img.textureID)
	}
	// Discard any pending AA buffer flush and mark the buffer for clearing
	// at the next AA draw. We can't use pendingClear here because its
	// timing relative to the sprite pass's batch processing is tricky —
	// instead, we set a flag that drawTrianglesAA checks before its first
	// draw each frame to enqueue a Clear on the buffer.
	if img.aaBuffer != nil {
		img.aaBufferDirty = false
		img.aaBufferNeedsClear = true
	}
}

// ReadPixels reads RGBA pixel data from the image into dst.
// dst must be at least 4*width*height bytes.
func (img *Image) ReadPixels(dst []byte) {
	if img.disposed || img.texture == nil || img.atlased {
		return
	}
	img.flushAABufferIfNeeded()

	// Fast path: serve from the CPU cache when it's still authoritative
	// (image was created from CPU data and no GPU writes have
	// invalidated the cache since). Avoids the GPU readback entirely —
	// critical on WebGPU browser mode where synchronous GPU readback
	// deadlocks (see cpuPixels field doc on Image).
	if img.cpuPixelsValid && img.cpuPixels != nil {
		n := len(img.cpuPixels)
		if n > len(dst) {
			n = len(dst)
		}
		copy(dst[:n], img.cpuPixels[:n])
		return
	}

	if img.padded {
		// Read the full padded texture, then extract the content region.
		padW := img.width + 2
		padH := img.height + 2
		full := make([]byte, padW*padH*4)
		img.texture.ReadPixels(full)
		for row := 0; row < img.height; row++ {
			srcOff := ((row + 1) * padW * 4) + 4
			dstOff := row * img.width * 4
			copy(dst[dstOff:dstOff+img.width*4], full[srcOff:srcOff+img.width*4])
		}
		return
	}
	img.texture.ReadPixels(dst)
}

// RenderTarget returns the backend render target for this image, or nil.
// This is used internally by the pipeline to bind off-screen FBOs.
func (img *Image) RenderTarget() backend.RenderTarget {
	return img.renderTarget
}

// Dispose releases the image's GPU resources.
// Sub-images do not release the parent's texture.
//
// Dispose does not synchronously invalidate the image. The GPU texture
// and render target remain live until renderer.disposeDeferred() drains
// the queue after device.EndFrame(), and the draw path continues to
// honor commands against this image until then. This matches
// ebiten.Image.Deallocate's soft-hint semantics — callers can Dispose()
// a canvas mid-Draw, continue compositing already-queued references,
// and the teardown happens at the frame boundary. See the `disposed`
// vs `pendingDispose` fields on Image for the rationale.
func (img *Image) Dispose() {
	if img.disposed || img.pendingDispose {
		return
	}

	// Untrack from context loss recovery.
	if tracker := getTracker(); tracker != nil {
		tracker.UntrackImage(img)
	}

	// Defer the actual GPU-resource teardown until AFTER the current
	// frame's command buffer has been submitted. Callers routinely
	// Dispose temporary canvases mid-Draw (see e.g. future's
	// comp/ui/vector/ops.go drawWithTransform, which allocates a temp
	// canvas, draws to it, composites back with a transform, and
	// `defer tmp.Dispose()`s on return). The batcher has already
	// recorded draw commands referencing this image's texture; destroying
	// the GPU texture synchronously would trigger the WebGPU validation
	// error "Destroyed texture used in a submit" on the next queue
	// submit. The engine drains the deferred list via
	// renderer.disposeDeferred() immediately AFTER device.EndFrame()
	// completes submit, so resources are released exactly one frame
	// later — still bounded memory, still deterministic.
	//
	// When no renderer is active (tests, teardown before/after engine
	// run), fall through to immediate disposal.
	if rend := getRenderer(); rend != nil {
		img.pendingDispose = true
		rend.deferDispose(img)
		return
	}
	img.disposeNow()
}

// disposeNow performs the actual GPU-resource teardown. Called
// immediately for images disposed outside an active frame, or via
// renderer.disposeDeferred() one frame after Dispose() for images
// disposed mid-frame. Safe to call once; subsequent calls are no-ops
// because texture/renderTarget/aaBuffer have been nilled out.
func (img *Image) disposeNow() {
	// Flip `disposed` here — NOT in Dispose(). Setting it in Dispose()
	// would cause the draw-path guards (drawImageRaw, Fill, Clear, etc.)
	// to drop commands queued against this image between Dispose() and
	// the end-of-frame drain, even though the GPU texture is still live.
	// See the Image struct docs for the `disposed` / `pendingDispose`
	// split.
	img.disposed = true
	img.pendingDispose = false

	// Dispose the AA buffer BEFORE tearing down this image's own GPU
	// resources. We deliberately skip flushing pending AA content: the
	// parent is being thrown away, so the content is worthless. Calling
	// flushAABuffer here would enqueue a draw against a soon-to-be-dead
	// target.
	if img.aaBuffer != nil {
		img.aaBuffer.disposeNow()
		img.aaBuffer = nil
		img.aaBufferDirty = false
	}

	if img.parent == nil {
		// Drop the registry entries for this image's texture and render
		// target BEFORE disposing the GPU resources, so that any late
		// resolveTexture / resolveRenderTarget call for this ID (e.g.
		// an app component that hasn't been unmounted yet and still
		// holds the old textureID) returns nil — the sprite pass
		// already skips nil bindings — instead of handing out a
		// destroyed GPU handle which WebGPU rejects at submit time
		// with "Destroyed texture used in a submit".
		if rend := getRenderer(); rend != nil && img.textureID != 0 {
			if rend.unregisterTexture != nil {
				rend.unregisterTexture(img.textureID)
			}
			if rend.unregisterRenderTarget != nil && img.renderTarget != nil {
				rend.unregisterRenderTarget(img.textureID)
			}
			rend.markDisposedID(img.textureID)
		}
		if img.renderTarget != nil {
			img.renderTarget.Dispose()
			img.renderTarget = nil
		}
		if img.texture != nil {
			img.texture.Dispose()
			img.texture = nil
		}
	}
}

// Set writes a single pixel at (x, y) with the given color.
// If the coordinate is outside the image bounds, Set is a no-op.
// This matches ebiten.Image.Set and satisfies the draw.Image interface.
func (img *Image) Set(x, y int, clr color.Color) {
	if img.disposed || img.texture == nil || img.atlased {
		return
	}
	if x < 0 || y < 0 || x >= img.width || y >= img.height {
		return
	}
	img.flushAABufferIfNeeded()
	r, g, b, a := clr.RGBA()
	pix := []byte{byte(r >> 8), byte(g >> 8), byte(b >> 8), byte(a >> 8)}
	tx, ty := x, y
	if img.padded {
		tx++
		ty++
	}
	img.texture.UploadRegion(pix, tx, ty, 1, 1, 0)
	// Keep the CPU cache in sync for single-pixel writes when the cache
	// was previously valid — the new byte is known, no need to
	// invalidate.
	if img.cpuPixelsValid && len(img.cpuPixels) >= (y*img.width+x+1)*4 {
		off := (y*img.width + x) * 4
		copy(img.cpuPixels[off:off+4], pix)
	} else {
		img.invalidateCPUPixels()
	}
}

// WritePixels uploads RGBA pixel data to the entire image.
// The data must be len(pix) == 4*width*height bytes in RGBA order.
// This matches Ebitengine's Image.WritePixels signature (single arg, full image).
func (img *Image) WritePixels(pix []byte) {
	if img.disposed || img.texture == nil || img.atlased {
		return
	}
	img.flushAABufferIfNeeded()
	if img.padded {
		// Upload into the content region of the padded texture.
		img.texture.UploadRegion(pix, 1, 1, img.width, img.height, 0)
	} else {
		img.texture.Upload(pix, 0)
	}
	// Refresh the CPU cache with the newly-written bytes so ReadPixels
	// stays on the fast path. Allocate the backing buffer if this is the
	// first write to an image that didn't start from CPU data.
	expected := img.width * img.height * 4
	if len(img.cpuPixels) != expected {
		img.cpuPixels = make([]byte, expected)
	}
	copy(img.cpuPixels, pix)
	img.cpuPixelsValid = true
}

// WritePixelsRegion uploads RGBA pixel data to a rectangular region of the image.
// The data must be len(pix) == 4*width*height bytes in RGBA order.
func (img *Image) WritePixelsRegion(pix []byte, x, y, width, height int) {
	if img.disposed || img.texture == nil || img.atlased {
		return
	}
	img.flushAABufferIfNeeded()
	tx, ty := x, y
	if img.padded {
		tx++
		ty++
	}
	img.texture.UploadRegion(pix, tx, ty, width, height, 0)
	// Update the CPU cache region in-place when the cache is live so
	// ReadPixels stays on the fast path after a partial write.
	if img.cpuPixelsValid && len(img.cpuPixels) == img.width*img.height*4 &&
		x >= 0 && y >= 0 && x+width <= img.width && y+height <= img.height {
		for row := 0; row < height; row++ {
			srcOff := row * width * 4
			dstOff := ((y+row)*img.width + x) * 4
			copy(img.cpuPixels[dstOff:dstOff+width*4], pix[srcOff:srcOff+width*4])
		}
	} else {
		img.invalidateCPUPixels()
	}
}

// DrawImageOptions holds options for DrawImage.
type DrawImageOptions struct {
	// GeoM is the geometry transformation matrix (2D affine transform).
	GeoM GeoM

	// ColorScale scales the RGBA color of each pixel.
	// A zero-valued ColorScale is treated as opaque white (1,1,1,1), matching
	// Ebitengine's behavior so that a default DrawImageOptions{} draws the
	// image unmodified.
	ColorScale ColorScale

	// ColorM is the color matrix transformation.
	ColorM fmath.ColorMatrix

	// Blend specifies the blend mode.
	Blend Blend

	// Filter specifies the texture filter.
	Filter Filter

	// PixelSnap rounds transformed vertex positions to the nearest integer
	// pixel. This eliminates sub-pixel shimmer and seams between adjacent
	// sprites in pixel-art or tile-based games. When false, the global
	// PixelSnapEnabled setting is checked as a fallback.
	PixelSnap bool
}

// DrawTrianglesOptions holds options for DrawTriangles.
type DrawTrianglesOptions struct {
	// Blend specifies the blend mode.
	Blend Blend

	// Filter specifies the texture filter.
	Filter Filter

	// FillRule specifies the fill rule for overlapping triangles.
	FillRule FillRule

	// AntiAlias indicates whether anti-aliasing should be applied to
	// triangle edges. This matches ebiten.DrawTrianglesOptions.AntiAlias.
	AntiAlias bool
}

// DrawRectShaderOptions holds options for DrawRectShader.
type DrawRectShaderOptions struct {
	// GeoM is the geometry transformation matrix.
	GeoM GeoM

	// ColorScale scales the RGBA color of each pixel.
	// A zero-valued ColorScale is treated as opaque white (1,1,1,1), matching
	// Ebitengine's behavior so that a default DrawRectShaderOptions{} draws the
	// image unmodified.
	ColorScale ColorScale

	// Blend specifies the blend mode.
	Blend Blend

	// Uniforms maps uniform names to values. Values can be float32, float64,
	// int, int32, or []float32. Slice length determines the GLSL type:
	// 1→float, 2→vec2, 4→vec4, 16→mat4.
	Uniforms map[string]any

	// Images are up to 4 source textures. Images[0] is bound as uTexture0, etc.
	Images [4]*Image
}

// DrawTrianglesShaderOptions holds options for DrawTrianglesShader.
type DrawTrianglesShaderOptions struct {
	// Blend specifies the blend mode.
	Blend Blend

	// FillRule specifies the fill rule for overlapping triangles.
	FillRule FillRule

	// AntiAlias indicates whether anti-aliasing should be applied to
	// triangle edges. This matches ebiten.DrawTrianglesShaderOptions.AntiAlias.
	AntiAlias bool

	// Uniforms maps uniform names to values.
	Uniforms map[string]any

	// Images are up to 4 source textures.
	Images [4]*Image
}

// DrawRectShader draws a rectangle of the given dimensions using a custom
// shader. This is the equivalent of ebiten.Image.DrawRectShader.
func (img *Image) DrawRectShader(width, height int, shader *Shader, opts *DrawRectShaderOptions) {
	if img.disposed || shader == nil || shader.disposed {
		return
	}
	img.flushAABufferIfNeeded()
	img.invalidateCPUPixels()
	if opts != nil {
		for _, src := range opts.Images {
			src.flushAABufferIfNeeded()
		}
	}
	rend := getRenderer()
	if rend == nil || rend.batcher == nil {
		return
	}

	var o DrawRectShaderOptions
	if opts != nil {
		o = *opts
	}

	// Apply uniforms to shader before draw, then snapshot for the batcher.
	shader.applyUniforms(o.Uniforms)

	w := float32(width)
	h := float32(height)

	// Apply GeoM to quad corners.
	x0, y0 := o.GeoM.Apply(0, 0)
	x1, y1 := o.GeoM.Apply(float64(w), 0)
	x2, y2 := o.GeoM.Apply(float64(w), float64(h))
	x3, y3 := o.GeoM.Apply(0, float64(h))

	cr, cg, cb, ca := o.ColorScale.rgbaOrDefault()
	blend := blendToBackend(o.Blend)

	// Determine texture ID from first source image, or white texture.
	texID := rend.whiteTextureID
	if o.Images[0] != nil && o.Images[0].texture != nil {
		texID = o.Images[0].textureID
	}

	// Collect extra texture IDs for multi-texture shader bindings.
	var extraTexIDs [3]uint32
	for i := 1; i < 4; i++ {
		if o.Images[i] != nil && o.Images[i].texture != nil {
			extraTexIDs[i-1] = o.Images[i].textureID
		}
	}

	// Copy user uniforms so per-draw state isn't overwritten by later
	// draws using the same shader (e.g., different lights). The sprite
	// pass re-applies these before each draw.
	var uniformsCopy map[string]any
	if len(o.Uniforms) > 0 {
		uniformsCopy = make(map[string]any, len(o.Uniforms))
		for k, v := range o.Uniforms {
			uniformsCopy[k] = v
		}
	}

	// Kage convention: when `kage:unit pixels` is declared (true for every
	// shader we ship today) the fragment's `srcPos` is expected to be in
	// pixel coordinates of the source image. DrawTrianglesShader callers
	// (e.g. lighting) already set TexU/TexV in pixel space, so DrawRectShader
	// must too — otherwise `imageSrc0At(srcPos)` samples in sub-pixel UV
	// space and the multiply_dither/bloom shaders return near-zero.
	clipX, clipY, clipW, clipH := img.clipParams()
	rend.batcher.AddQuadDirect(
		batch.Vertex2D{PosX: float32(x0), PosY: float32(y0), TexU: 0, TexV: 0, R: cr, G: cg, B: cb, A: ca},
		batch.Vertex2D{PosX: float32(x1), PosY: float32(y1), TexU: w, TexV: 0, R: cr, G: cg, B: cb, A: ca},
		batch.Vertex2D{PosX: float32(x2), PosY: float32(y2), TexU: w, TexV: h, R: cr, G: cg, B: cb, A: ca},
		batch.Vertex2D{PosX: float32(x3), PosY: float32(y3), TexU: 0, TexV: h, R: cr, G: cg, B: cb, A: ca},
		batch.DrawCommand{
			TextureID:       texID,
			BlendMode:       blend,
			ShaderID:        shader.id,
			TargetID:        img.textureID,
			ColorBody:       colorMatrixIdentityBody,
			ExtraTextureIDs: extraTexIDs,
			Uniforms:        uniformsCopy,
			ClipX:           clipX, ClipY: clipY, ClipW: clipW, ClipH: clipH,
		},
	)
}

// DrawTrianglesShader draws triangles using a custom shader. This is the
// equivalent of ebiten.Image.DrawTrianglesShader.
func (img *Image) DrawTrianglesShader(vertices []Vertex, indices []uint16, shader *Shader, opts *DrawTrianglesShaderOptions) {
	if img.disposed || shader == nil || shader.disposed {
		return
	}
	img.flushAABufferIfNeeded()
	img.invalidateCPUPixels()
	if opts != nil {
		for _, src := range opts.Images {
			src.flushAABufferIfNeeded()
		}
	}
	rend := getRenderer()
	if rend == nil || rend.batcher == nil {
		return
	}

	var o DrawTrianglesShaderOptions
	if opts != nil {
		o = *opts
	}

	// Apply uniforms, then snapshot for the batcher.
	shader.applyUniforms(o.Uniforms)

	batchVerts := make([]batch.Vertex2D, len(vertices))
	for i, v := range vertices {
		batchVerts[i] = batch.Vertex2D{
			PosX: v.DstX,
			PosY: v.DstY,
			TexU: v.SrcX,
			TexV: v.SrcY,
			R:    v.ColorR,
			G:    v.ColorG,
			B:    v.ColorB,
			A:    v.ColorA,
		}
	}

	texID := rend.whiteTextureID
	blend := blendToBackend(o.Blend)
	fillRule := fillRuleToBackend(o.FillRule)

	if o.Images[0] != nil && o.Images[0].texture != nil {
		texID = o.Images[0].textureID
	}

	// Collect extra texture IDs for multi-texture shader bindings.
	var extraTexIDs [3]uint32
	for i := 1; i < 4; i++ {
		if o.Images[i] != nil && o.Images[i].texture != nil {
			extraTexIDs[i-1] = o.Images[i].textureID
		}
	}

	// Copy user uniforms so per-draw state isn't overwritten.
	var dtUniformsCopy map[string]any
	if len(o.Uniforms) > 0 {
		dtUniformsCopy = make(map[string]any, len(o.Uniforms))
		for k, v := range o.Uniforms {
			dtUniformsCopy[k] = v
		}
	}

	clipX, clipY, clipW, clipH := img.clipParams()
	rend.batcher.Add(batch.DrawCommand{
		Vertices:        batchVerts,
		Indices:         indices,
		TextureID:       texID,
		BlendMode:       blend,
		FillRule:        fillRule,
		ShaderID:        shader.id,
		TargetID:        img.textureID,
		ColorBody:       colorMatrixIdentityBody,
		ExtraTextureIDs: extraTexIDs,
		Uniforms:        dtUniformsCopy,
		ClipX:           clipX, ClipY: clipY, ClipW: clipW, ClipH: clipH,
	})
}

// Vertex represents a vertex for DrawTriangles.
type Vertex struct {
	DstX, DstY                     float32
	SrcX, SrcY                     float32
	ColorR, ColorG, ColorB, ColorA float32
}

// GeoM represents a 2D affine transformation matrix.
// This provides an API compatible with ebiten.GeoM.
type GeoM struct {
	m fmath.Mat3
}

// NewGeoM creates an identity GeoM.
func NewGeoM() GeoM {
	return GeoM{m: fmath.Mat3Identity()}
}

// Translate adds a translation to the transformation.
func (g *GeoM) Translate(tx, ty float64) {
	g.m = fmath.Mat3Translate(tx, ty).Mul(g.mat3())
}

// Scale adds a scaling to the transformation.
func (g *GeoM) Scale(sx, sy float64) {
	g.m = fmath.Mat3Scale(sx, sy).Mul(g.mat3())
}

// Rotate adds a rotation (radians) to the transformation.
func (g *GeoM) Rotate(angle float64) {
	g.m = fmath.Mat3Rotate(angle).Mul(g.mat3())
}

// Skew adds a shear/skew to the transformation.
func (g *GeoM) Skew(sx, sy float64) {
	g.m = fmath.Mat3Shear(sx, sy).Mul(g.mat3())
}

// Concat concatenates another GeoM onto this one.
func (g *GeoM) Concat(other GeoM) {
	g.m = other.mat3().Mul(g.mat3())
}

// Reset resets the GeoM to identity.
func (g *GeoM) Reset() {
	g.m = fmath.Mat3Identity()
}

// Apply transforms a point by this GeoM.
// A zero-valued GeoM acts as the identity transform.
func (g *GeoM) Apply(x, y float64) (rx, ry float64) {
	m := g.mat3()
	v := m.MulVec2(fmath.NewVec2(x, y))
	return v.X, v.Y
}

// Element returns the element at row i, column j of the affine transform.
// i must be 0 or 1, j must be 0, 1, or 2.
// The matrix is:
//
//	| a b tx |   Element(0,0) Element(0,1) Element(0,2)
//	| c d ty |   Element(1,0) Element(1,1) Element(1,2)
//
// This matches Ebitengine's GeoM.Element.
func (g *GeoM) Element(i, j int) float64 {
	return g.mat3().At(i, j)
}

// SetElement sets the element at row i, column j of the affine transform.
// i must be 0 or 1, j must be 0, 1, or 2.
// This matches Ebitengine's GeoM.SetElement.
func (g *GeoM) SetElement(i, j int, v float64) {
	g.m = g.mat3().Set(i, j, v)
}

// Invert inverts the GeoM. If the matrix is not invertible, it becomes
// a zero-value identity.
func (g *GeoM) Invert() {
	inv, ok := g.mat3().Inverse()
	if ok {
		g.m = inv
	}
}

// Mat3 returns the underlying 3x3 matrix.
// A zero-valued GeoM returns the identity matrix.
func (g *GeoM) Mat3() fmath.Mat3 {
	return g.mat3()
}

// mat3 returns the underlying matrix, treating a zero-valued GeoM as identity.
// This ensures that the default DrawImageOptions{} draws without transformation.
func (g *GeoM) mat3() fmath.Mat3 {
	if g.m == (fmath.Mat3{}) {
		return fmath.Mat3Identity()
	}
	return g.m
}

// ColorScale represents a color scale applied to drawn images.
// This matches Ebitengine's ColorScale type.
// A zero-valued ColorScale is treated as opaque white (1,1,1,1).
type ColorScale struct {
	r, g, b, a float32
	set        bool
}

// Scale multiplies all color components.
func (cs *ColorScale) Scale(r, g, b, a float32) {
	if !cs.set {
		cs.r, cs.g, cs.b, cs.a = r, g, b, a
		cs.set = true
		return
	}
	cs.r *= r
	cs.g *= g
	cs.b *= b
	cs.a *= a
}

// ScaleAlpha multiplies the alpha component only.
func (cs *ColorScale) ScaleAlpha(a float32) {
	if !cs.set {
		cs.r, cs.g, cs.b, cs.a = 1, 1, 1, a
		cs.set = true
		return
	}
	cs.a *= a
}

// R returns the red component. Returns 1 if not set.
func (cs ColorScale) R() float32 {
	if !cs.set {
		return 1
	}
	return cs.r
}

// G returns the green component. Returns 1 if not set.
func (cs ColorScale) G() float32 {
	if !cs.set {
		return 1
	}
	return cs.g
}

// B returns the blue component. Returns 1 if not set.
func (cs ColorScale) B() float32 {
	if !cs.set {
		return 1
	}
	return cs.b
}

// A returns the alpha component. Returns 1 if not set.
func (cs ColorScale) A() float32 {
	if !cs.set {
		return 1
	}
	return cs.a
}

// Reset resets the ColorScale to the default (opaque white).
func (cs *ColorScale) Reset() {
	cs.r, cs.g, cs.b, cs.a = 0, 0, 0, 0
	cs.set = false
}

// rgbaOrDefault returns RGBA float32 values, defaulting to (1,1,1,1) if not set.
func (cs ColorScale) rgbaOrDefault() (r, g, b, a float32) {
	if !cs.set {
		return 1, 1, 1, 1
	}
	return cs.r, cs.g, cs.b, cs.a
}

// SetColor sets the ColorScale from an fmath.Color.
func (cs *ColorScale) SetColor(c fmath.Color) {
	cs.r = float32(c.R)
	cs.g = float32(c.G)
	cs.b = float32(c.B)
	cs.a = float32(c.A)
	cs.set = true
}

// BlendFactor represents a blend factor for the Blend struct.
type BlendFactor int

// BlendFactor constants matching Ebitengine.
const (
	BlendFactorZero                     BlendFactor = iota // 0
	BlendFactorOne                                         // 1
	BlendFactorSourceAlpha                                 // src alpha
	BlendFactorDestinationAlpha                            // dst alpha
	BlendFactorOneMinusSourceAlpha                         // 1 - src alpha
	BlendFactorOneMinusDestinationAlpha                    // 1 - dst alpha
	BlendFactorSourceColor                                 // src color (RGB)
	BlendFactorDestinationColor                            // dst color (RGB)
)

// BlendOperation represents a blend operation for the Blend struct.
type BlendOperation int

// BlendOperation constants matching Ebitengine.
const (
	BlendOperationAdd             BlendOperation = iota // src + dst
	BlendOperationSubtract                              // src - dst
	BlendOperationReverseSubtract                       // dst - src
	BlendOperationMin                                   // min(src, dst)
	BlendOperationMax                                   // max(src, dst)
)

// Blend specifies how colors are blended, matching Ebitengine's ebiten.Blend struct.
// A zero-valued Blend represents source-over (standard alpha) blending.
type Blend struct {
	// BlendFactorSourceRGB is the source RGB blend factor.
	BlendFactorSourceRGB BlendFactor
	// BlendFactorSourceAlpha is the source alpha blend factor.
	BlendFactorSourceAlpha BlendFactor
	// BlendFactorDestinationRGB is the destination RGB blend factor.
	BlendFactorDestinationRGB BlendFactor
	// BlendFactorDestinationAlpha is the destination alpha blend factor.
	BlendFactorDestinationAlpha BlendFactor
	// BlendOperationRGB is the RGB blend operation.
	BlendOperationRGB BlendOperation
	// BlendOperationAlpha is the alpha blend operation.
	BlendOperationAlpha BlendOperation
}

// Preset Blend values matching Ebitengine's variables.
var (
	// BlendSourceOver is standard alpha blending: src*srcA + dst*(1-srcA).
	BlendSourceOver = Blend{
		BlendFactorSourceRGB:        BlendFactorOne,
		BlendFactorSourceAlpha:      BlendFactorOne,
		BlendFactorDestinationRGB:   BlendFactorOneMinusSourceAlpha,
		BlendFactorDestinationAlpha: BlendFactorOneMinusSourceAlpha,
		BlendOperationRGB:           BlendOperationAdd,
		BlendOperationAlpha:         BlendOperationAdd,
	}

	// BlendLighter is additive blending: src + dst.
	BlendLighter = Blend{
		BlendFactorSourceRGB:        BlendFactorOne,
		BlendFactorSourceAlpha:      BlendFactorOne,
		BlendFactorDestinationRGB:   BlendFactorOne,
		BlendFactorDestinationAlpha: BlendFactorOne,
		BlendOperationRGB:           BlendOperationAdd,
		BlendOperationAlpha:         BlendOperationAdd,
	}

	// BlendMultiply is multiplicative blending: src * dst.
	BlendMultiply = Blend{
		BlendFactorSourceRGB:        BlendFactorDestinationColor,
		BlendFactorSourceAlpha:      BlendFactorDestinationAlpha,
		BlendFactorDestinationRGB:   BlendFactorZero,
		BlendFactorDestinationAlpha: BlendFactorZero,
		BlendOperationRGB:           BlendOperationAdd,
		BlendOperationAlpha:         BlendOperationAdd,
	}
)

// Filter specifies texture filtering.
type Filter int

// Filter constants.
const (
	FilterNearest Filter = iota
	FilterLinear
)

// FillRule specifies the fill rule for overlapping triangles.
type FillRule int

// FillRule constants.
const (
	FillRuleNonZero FillRule = iota
	FillRuleEvenOdd
)

// ColorFromRGBA creates a Color from float64 RGBA components in [0,1].
func ColorFromRGBA(r, g, b, a float64) fmath.Color {
	return fmath.Color{R: r, G: g, B: b, A: a}
}

// --- Internal helpers ---

// colorMatrixIdentityBody is the 4x4 identity body for the color matrix.
var colorMatrixIdentityBody = [16]float32{
	1, 0, 0, 0,
	0, 1, 0, 0,
	0, 0, 1, 0,
	0, 0, 0, 1,
}

// colorMatrixToUniforms converts a ColorMatrix to body (mat4) and translation
// (vec4) uniform values. A zero-valued ColorMatrix is treated as identity.
func colorMatrixToUniforms(cm fmath.ColorMatrix) (body [16]float32, translation [4]float32) {
	if cm == (fmath.ColorMatrix{}) || cm.IsIdentity() {
		return colorMatrixIdentityBody, [4]float32{}
	}
	// The ColorMatrix is row-major: rows [0..4], [5..9], [10..14], [15..19].
	// Columns 0-3 are the body, column 4 is translation.
	// GLSL mat4 is column-major, so we transpose the body.
	body = [16]float32{
		float32(cm[0]), float32(cm[5]), float32(cm[10]), float32(cm[15]), // col 0
		float32(cm[1]), float32(cm[6]), float32(cm[11]), float32(cm[16]), // col 1
		float32(cm[2]), float32(cm[7]), float32(cm[12]), float32(cm[17]), // col 2
		float32(cm[3]), float32(cm[8]), float32(cm[13]), float32(cm[18]), // col 3
	}
	translation = [4]float32{
		float32(cm[4]), float32(cm[9]), float32(cm[14]), float32(cm[19]),
	}
	return body, translation
}

// blendToBackend converts the public Blend struct to the internal
// backend.BlendMode struct by copying each factor and operation directly.
// A zero-valued Blend produces a BlendMode that has Enabled=true with
// src-over semantics, matching Ebitengine's behavior where an uninitialized
// Blend value blends as source-over.
func blendToBackend(b Blend) backend.BlendMode {
	if (b == Blend{}) {
		return backend.BlendSourceOver
	}
	return backend.BlendMode{
		Enabled:        true,
		SrcFactorRGB:   blendFactorToBackend(b.BlendFactorSourceRGB),
		SrcFactorAlpha: blendFactorToBackend(b.BlendFactorSourceAlpha),
		DstFactorRGB:   blendFactorToBackend(b.BlendFactorDestinationRGB),
		DstFactorAlpha: blendFactorToBackend(b.BlendFactorDestinationAlpha),
		OpRGB:          blendOperationToBackend(b.BlendOperationRGB),
		OpAlpha:        blendOperationToBackend(b.BlendOperationAlpha),
	}
}

// blendFactorToBackend maps the public BlendFactor enum (matching Ebitengine)
// to the backend equivalent. The two enums intentionally have different
// integer orderings so that a direct cast is incorrect.
func blendFactorToBackend(f BlendFactor) backend.BlendFactor {
	switch f {
	case BlendFactorZero:
		return backend.BlendFactorZero
	case BlendFactorOne:
		return backend.BlendFactorOne
	case BlendFactorSourceAlpha:
		return backend.BlendFactorSrcAlpha
	case BlendFactorOneMinusSourceAlpha:
		return backend.BlendFactorOneMinusSrcAlpha
	case BlendFactorDestinationAlpha:
		return backend.BlendFactorDstAlpha
	case BlendFactorOneMinusDestinationAlpha:
		return backend.BlendFactorOneMinusDstAlpha
	case BlendFactorSourceColor:
		return backend.BlendFactorSrcColor
	case BlendFactorDestinationColor:
		return backend.BlendFactorDstColor
	default:
		// Unknown factor (e.g. zero-value of a future extension) →
		// treat as opaque source (safe default that won't silently
		// darken or brighten the scene).
		return backend.BlendFactorOne
	}
}

// blendOperationToBackend maps the public BlendOperation enum to the backend
// equivalent. The two enums currently share the same ordering but this
// conversion makes the dependence explicit.
func blendOperationToBackend(op BlendOperation) backend.BlendOperation {
	switch op {
	case BlendOperationAdd:
		return backend.BlendOpAdd
	case BlendOperationSubtract:
		return backend.BlendOpSubtract
	case BlendOperationReverseSubtract:
		return backend.BlendOpReverseSubtract
	case BlendOperationMin:
		return backend.BlendOpMin
	case BlendOperationMax:
		return backend.BlendOpMax
	default:
		return backend.BlendOpAdd
	}
}

// filterToBackend maps a public Filter to a backend TextureFilter.
func filterToBackend(f Filter) backend.TextureFilter {
	switch f {
	case FilterLinear:
		return backend.FilterLinear
	default:
		return backend.FilterNearest
	}
}

// fillRuleToBackend maps a public FillRule to a backend FillRule.
func fillRuleToBackend(f FillRule) backend.FillRule {
	switch f {
	case FillRuleEvenOdd:
		return backend.FillRuleEvenOdd
	default:
		return backend.FillRuleNonZero
	}
}

// --- Anti-aliased DrawTriangles via 2x supersample buffer ---
//
// Ebitengine implements DrawTrianglesOptions.AntiAlias by drawing AA'd
// geometry into a per-Image 2x-scaled offscreen "big offscreen buffer",
// then downsample-compositing that buffer back into the parent image at
// the next sync point (any operation that reads or writes the parent's
// texture). The 2x2 box-average produced by the linear-filtered
// downsample creates a 1-pixel anti-aliased fringe on every triangle
// edge, with zero changes to the shader or vertex format.
//
// This file mirrors Ebitengine's `bigOffscreenImage` design. Key pieces:
//
//   - requiredAARegion: computes the 16px-granular bounding box of a
//     triangle set, so the AA buffer only covers the affected region.
//   - drawTrianglesAA: entry point called from DrawTriangles when opts.AntiAlias
//     is true. Lazily allocates img.aaBuffer sized to the region, rewrites
//     vertex positions into the 2x buffer's coordinate space, and forwards
//     to the non-AA DrawTriangles path on the buffer.
//   - flushAABuffer: downsample-composites the buffer back into img via
//     drawImageRaw at 0.5x with a linear filter. Does NOT dispose the buffer
//     — it's reused for later AA draws on the same image with matching
//     region and blend.
//   - flushAABufferIfNeeded: nil-safe, parent-aware trigger called from
//     every public Image method that touches texture content.

// roundDown16 returns the largest multiple of 16 that is <= v.
func roundDown16(v int) int {
	return v &^ 15
}

// roundUp16 returns the smallest multiple of 16 that is >= v.
func roundUp16(v int) int {
	return (v + 15) &^ 15
}

// requiredAARegion returns the 1x region of img covered by the given
// triangle vertices, expanded by 1 pixel on each side and rounded to 16px
// granularity, clamped to img's bounds. The 16px rounding reduces churn
// when successive AA draws have slightly different extents so the buffer
// doesn't get reallocated on every draw.
//
// Returns the zero rectangle when vertices is empty or the computed
// region has no area after clamping.
func requiredAARegion(vertices []Vertex, imgW, imgH int) goimage.Rectangle {
	if len(vertices) == 0 {
		return goimage.Rectangle{}
	}
	minX := vertices[0].DstX
	minY := vertices[0].DstY
	maxX := vertices[0].DstX
	maxY := vertices[0].DstY
	for _, v := range vertices[1:] {
		if v.DstX < minX {
			minX = v.DstX
		}
		if v.DstY < minY {
			minY = v.DstY
		}
		if v.DstX > maxX {
			maxX = v.DstX
		}
		if v.DstY > maxY {
			maxY = v.DstY
		}
	}
	r := goimage.Rect(
		roundDown16(int(gomath.Floor(float64(minX)))-1),
		roundDown16(int(gomath.Floor(float64(minY)))-1),
		roundUp16(int(gomath.Ceil(float64(maxX)))+1),
		roundUp16(int(gomath.Ceil(float64(maxY)))+1),
	)
	if r.Min.X < 0 {
		r.Min.X = 0
	}
	if r.Min.Y < 0 {
		r.Min.Y = 0
	}
	if r.Max.X > imgW {
		r.Max.X = imgW
	}
	if r.Max.Y > imgH {
		r.Max.Y = imgH
	}
	if r.Min.X >= r.Max.X || r.Min.Y >= r.Max.Y {
		return goimage.Rectangle{}
	}
	return r
}

// drawTrianglesAA is the entry point for anti-aliased DrawTriangles.
// Called by DrawTriangles when opts.AntiAlias is true.
//
// Sub-images share their parent's texture and cannot own their own AA
// buffer (matching Ebitengine). The fallback path calls DrawTriangles
// again with AntiAlias=false, producing aliased output but no panic.
func (img *Image) drawTrianglesAA(vertices []Vertex, indices []uint16, src *Image, opts *DrawTrianglesOptions) {
	if len(vertices) == 0 || len(indices) == 0 {
		return
	}
	if img.parent != nil {
		// Sub-image: fall back to aliased rendering.
		fallback := DrawTrianglesOptions{}
		if opts != nil {
			fallback = *opts
		}
		fallback.AntiAlias = false
		img.DrawTriangles(vertices, indices, src, &fallback)
		return
	}

	reqBlend := Blend{}
	if opts != nil {
		reqBlend = opts.Blend
	}

	// A different blend mode from what the buffer currently holds forces
	// a flush; the single downsample composite uses one blend mode.
	if img.aaBuffer != nil && img.aaBufferDirty && img.aaBufferBlend != reqBlend {
		img.flushAABuffer()
	}

	// Use the full image bounds as the AA region. This ensures ALL
	// AA draws to the same image share a single persistent buffer,
	// regardless of their individual vertex bounding boxes. Per-draw
	// region computation caused a new buffer allocation per AA call
	// (different shapes have different bboxes), creating ~200 unique
	// render targets per frame and destroying batch mergeability.
	//
	// Ebitengine's bigOffscreenBuffer uses the same approach: one
	// buffer per image, sized to the full image.
	region := goimage.Rect(0, 0, img.width, img.height)

	// Lazy-allocate the 2x buffer sized to the full image. Reused
	// across all AA draws and across frames — only disposed when the
	// parent image is disposed or when the blend mode changes.
	if img.aaBuffer == nil {
		w := region.Dx() * aaBufferScale
		h := region.Dy() * aaBufferScale
		buf := newImageLabeled(w, h, "aa-buffer")
		if buf == nil || buf.renderTarget == nil {
			if buf != nil {
				buf.Dispose()
			}
			fallback := DrawTrianglesOptions{}
			if opts != nil {
				fallback = *opts
			}
			fallback.AntiAlias = false
			img.DrawTriangles(vertices, indices, src, &fallback)
			return
		}
		img.aaBuffer = buf
		img.aaBufferRegion = region
		// Clear the newly allocated buffer so it starts from transparent
		// black rather than undefined content. The pendingClear is consumed
		// by the sprite pass's beginTargetPass on the buffer's first
		// render pass, emitting LoadActionClear.
		if rend := getRenderer(); rend != nil {
			rend.pendingClears.RequestOnce(img.aaBuffer.textureID)
		}
	}
	img.aaBufferBlend = reqBlend

	// If the parent was Clear()'d since the last AA draw, clear the
	// persistent buffer so stale content from the previous frame doesn't
	// accumulate. We use pendingClear (GPU-native LoadActionClear) which
	// is consumed by the sprite pass's beginTargetPass on the buffer's
	// first render pass this frame.
	if img.aaBufferNeedsClear {
		img.aaBufferNeedsClear = false
		if rend := getRenderer(); rend != nil {
			rend.pendingClears.RequestOnce(img.aaBuffer.textureID)
		}
	}

	// Rewrite vertex positions into the 2x buffer's coordinate space.
	// Translate the 1x region origin to (0,0) and scale by 2.
	// Texture coordinates and vertex colors pass through unchanged.
	scaled := make([]Vertex, len(vertices))
	for i, v := range vertices {
		scaled[i] = v
		scaled[i].DstX = (v.DstX - float32(region.Min.X)) * float32(aaBufferScale)
		scaled[i].DstY = (v.DstY - float32(region.Min.Y)) * float32(aaBufferScale)
	}

	// Forward to the non-AA path on the offscreen buffer. inner.AntiAlias
	// must be false or we'd recurse into drawTrianglesAA on the buffer.
	inner := DrawTrianglesOptions{}
	if opts != nil {
		inner = *opts
	}
	inner.AntiAlias = false
	img.aaBuffer.DrawTriangles(scaled, indices, src, &inner)
	img.aaBufferDirty = true
}

// flushAABuffer downsample-composites img.aaBuffer back into img at 0.5x
// with a linear filter, producing the anti-aliased result. The buffer is
// NOT disposed — it persists across all AA draws on this image, matching
// Ebitengine's bigOffscreenBuffer approach. After the composite, a
// pendingClear is registered so the next AA draw starts from a clean
// buffer. The buffer is only disposed when img.Dispose() is called.
//
// drawImageRaw is used so the flush doesn't recurse back into
// flushAABufferIfNeeded on the parent.
func (img *Image) flushAABuffer() {
	if img == nil || img.aaBuffer == nil || !img.aaBufferDirty {
		return
	}

	down := &DrawImageOptions{
		Filter: FilterLinear,
		Blend:  img.aaBufferBlend,
	}
	down.GeoM.Scale(1.0/float64(aaBufferScale), 1.0/float64(aaBufferScale))
	down.GeoM.Translate(float64(img.aaBufferRegion.Min.X), float64(img.aaBufferRegion.Min.Y))

	img.drawImageRaw(img.aaBuffer, down)

	img.aaBufferDirty = false

	// Clear the persistent buffer after compositing so subsequent AA
	// draws start from transparent black. Without this, the next flush
	// would re-composite stale content that was already applied above.
	if rend := getRenderer(); rend != nil {
		rend.pendingClears.Request(img.aaBuffer.textureID)
	}
}

// flushAABufferIfNeeded triggers a flush on img's AA buffer when the
// buffer has pending draws. Called at the top of every public Image
// method that reads or writes the image's texture, enforcing the
// Painter's-algorithm guarantee that img.texture is up to date before
// any operation that depends on its contents.
//
// Sub-images forward to their parent, because the parent owns the
// texture. nil receiver is safe so call sites can say
// `src.flushAABufferIfNeeded()` without guarding.
func (img *Image) flushAABufferIfNeeded() {
	if img == nil {
		return
	}
	if img.parent != nil {
		img.parent.flushAABufferIfNeeded()
		return
	}
	if img.aaBufferDirty {
		img.flushAABuffer()
	}
}
