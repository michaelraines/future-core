package futurerender

import (
	"fmt"
	goimage "image"
	"image/draw"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/batch"
	fmath "github.com/michaelraines/future-render/math"
)

// Image represents a renderable image (texture). It can be used as a
// render target or as a source for drawing operations.
//
// This type is the equivalent of ebiten.Image.
type Image struct {
	width, height int
	disposed      bool

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
}

// NewImage creates a new blank image with the given dimensions.
// If the rendering backend is initialized, a GPU texture is allocated.
func NewImage(width, height int) *Image {
	img := &Image{
		width:  width,
		height: height,
		u0:     0, v0: 0,
		u1: 1, v1: 1,
	}

	// Allocate GPU texture and render target if a device is available.
	if rend := getRenderer(); rend != nil && rend.device != nil {
		tex, err := rend.device.NewTexture(backend.TextureDescriptor{
			Width:        width,
			Height:       height,
			Format:       backend.TextureFormatRGBA8,
			Filter:       backend.FilterNearest,
			WrapU:        backend.WrapClamp,
			WrapV:        backend.WrapClamp,
			RenderTarget: true,
		})
		if err == nil {
			img.texture = tex
			img.textureID = rend.allocTextureID()
			if rend.registerTexture != nil {
				rend.registerTexture(img.textureID, tex)
			}
		}

		// Create render target so this image can be drawn to.
		rt, rtErr := rend.device.NewRenderTarget(backend.RenderTargetDescriptor{
			Width:       width,
			Height:      height,
			ColorFormat: backend.TextureFormatRGBA8,
		})
		if rtErr == nil {
			img.renderTarget = rt
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

// NewImageFromImage creates an Image from a Go image.Image.
// The pixel data is uploaded to the GPU immediately.
func NewImageFromImage(src goimage.Image) *Image {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Convert to RGBA if needed.
	rgba, ok := src.(*goimage.RGBA)
	if !ok {
		rgba = goimage.NewRGBA(bounds)
		draw.Draw(rgba, bounds, src, bounds.Min, draw.Src)
	}

	img := &Image{
		width:  w,
		height: h,
		u0:     0, v0: 0,
		u1: 1, v1: 1,
	}

	if rend := getRenderer(); rend != nil && rend.device != nil {
		tex, err := rend.device.NewTexture(backend.TextureDescriptor{
			Width:  w,
			Height: h,
			Format: backend.TextureFormatRGBA8,
			Filter: backend.FilterNearest,
			WrapU:  backend.WrapClamp,
			WrapV:  backend.WrapClamp,
			Data:   rgba.Pix,
		})
		if err == nil {
			img.texture = tex
			img.textureID = rend.allocTextureID()
			if rend.registerTexture != nil {
				rend.registerTexture(img.textureID, tex)
			}
		}
	}

	// Track for context loss recovery, preserving pixel data.
	if tracker := getTracker(); tracker != nil {
		tracker.TrackImage(img, rgba.Pix, false)
	}

	return img
}

// Size returns the image dimensions.
func (img *Image) Size() (width, height int) {
	return img.width, img.height
}

// Bounds returns the image bounds as a Rect.
func (img *Image) Bounds() fmath.Rect {
	return fmath.NewRect(0, 0, float64(img.width), float64(img.height))
}

// DrawImage draws src onto img with the given options.
func (img *Image) DrawImage(src *Image, opts *DrawImageOptions) {
	if img.disposed || src == nil || src.disposed {
		return
	}
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

	// Color scale (default to opaque white).
	cr, cg, cb, ca := colorScaleOrDefault(o.ColorScale)

	// Determine texture ID: use source texture, or white texture for nil.
	texID := src.textureID
	if src.texture == nil {
		texID = rend.whiteTextureID
	}

	// Map public blend mode and filter to backend types.
	blend := blendToBackend(o.Blend)
	filter := filterToBackend(o.Filter)
	colorBody, colorTrans := colorMatrixToUniforms(o.ColorM)

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
		},
	)
}

// DrawTriangles draws triangles with the given vertices, indices, and options.
// This is the low-level drawing primitive equivalent to ebiten.DrawTriangles.
func (img *Image) DrawTriangles(vertices []Vertex, indices []uint16, src *Image, opts *DrawTrianglesOptions) {
	if img.disposed {
		return
	}
	rend := getRenderer()
	if rend == nil || rend.batcher == nil {
		return
	}

	// Convert public Vertex to batch Vertex2D.
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

	texID := uint32(0)
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

	rend.batcher.Add(batch.DrawCommand{
		Vertices:  batchVerts,
		Indices:   indices,
		TextureID: texID,
		BlendMode: blend,
		Filter:    filter,
		FillRule:  fillRule,
		ShaderID:  0,
		TargetID:  img.textureID,
		ColorBody: colorMatrixIdentityBody,
	})
}

// Fill fills the entire image with the given color.
func (img *Image) Fill(c fmath.Color) {
	if img.disposed {
		return
	}
	rend := getRenderer()
	if rend == nil || rend.batcher == nil {
		return
	}

	w := float32(img.width)
	h := float32(img.height)
	r := float32(c.R)
	g := float32(c.G)
	b := float32(c.B)
	a := float32(c.A)

	// Use the white texture and multiply by vertex color.
	texID := rend.whiteTextureID

	rend.batcher.AddQuadDirect(
		batch.Vertex2D{PosX: 0, PosY: 0, TexU: 0, TexV: 0, R: r, G: g, B: b, A: a},
		batch.Vertex2D{PosX: w, PosY: 0, TexU: 1, TexV: 0, R: r, G: g, B: b, A: a},
		batch.Vertex2D{PosX: w, PosY: h, TexU: 1, TexV: 1, R: r, G: g, B: b, A: a},
		batch.Vertex2D{PosX: 0, PosY: h, TexU: 0, TexV: 1, R: r, G: g, B: b, A: a},
		batch.DrawCommand{
			TextureID: texID,
			BlendMode: backend.BlendSourceOver,
			ShaderID:  0,
			TargetID:  img.textureID,
			ColorBody: colorMatrixIdentityBody,
		},
	)
}

// SubImage returns a sub-region of the image for sprite sheet support.
// The returned Image shares the parent's GPU texture with adjusted UVs.
func (img *Image) SubImage(r fmath.Rect) *Image {
	w := float32(img.width)
	h := float32(img.height)

	if w == 0 || h == 0 {
		return &Image{
			width:  int(r.Width()),
			height: int(r.Height()),
		}
	}

	// Map rect coordinates to UV space within this image's UV region.
	uRange := img.u1 - img.u0
	vRange := img.v1 - img.v0

	su0 := img.u0 + float32(r.Min.X)/w*uRange
	sv0 := img.v0 + float32(r.Min.Y)/h*vRange
	su1 := img.u0 + float32(r.Max.X)/w*uRange
	sv1 := img.v0 + float32(r.Max.Y)/h*vRange

	// Point to the root texture owner.
	parent := img
	if img.parent != nil {
		parent = img.parent
	}

	return &Image{
		width:     int(r.Width()),
		height:    int(r.Height()),
		texture:   parent.texture,
		textureID: parent.textureID,
		parent:    parent,
		u0:        su0,
		v0:        sv0,
		u1:        su1,
		v1:        sv1,
	}
}

// Clear resets all pixels to transparent black (0, 0, 0, 0).
// This is equivalent to ebiten.Image.Clear.
func (img *Image) Clear() {
	img.Fill(fmath.Color{R: 0, G: 0, B: 0, A: 0})
}

// ReadPixels reads RGBA pixel data from the image into dst.
// dst must be at least 4*width*height bytes.
func (img *Image) ReadPixels(dst []byte) {
	if img.disposed || img.texture == nil {
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
func (img *Image) Dispose() {
	if img.disposed {
		return
	}
	img.disposed = true

	// Untrack from context loss recovery.
	if tracker := getTracker(); tracker != nil {
		tracker.UntrackImage(img)
	}

	if img.parent == nil {
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

// WritePixels uploads RGBA pixel data to a rectangular region of the image.
// The data must be len(pix) == 4*width*height bytes in RGBA order.
func (img *Image) WritePixels(pix []byte, x, y, width, height int) {
	if img.disposed || img.texture == nil {
		return
	}
	img.texture.UploadRegion(pix, x, y, width, height, 0)
}

// DrawImageOptions holds options for DrawImage.
type DrawImageOptions struct {
	// GeoM is the geometry transformation matrix (2D affine transform).
	GeoM GeoM

	// ColorScale scales the RGBA color of each pixel.
	// A zero-valued ColorScale is treated as opaque white (1,1,1,1), matching
	// Ebitengine's behavior so that a default DrawImageOptions{} draws the
	// image unmodified. To make an image invisible, use ColorM or set the
	// alpha channel in vertex colors instead.
	ColorScale fmath.Color

	// ColorM is the color matrix transformation.
	ColorM fmath.ColorMatrix

	// Blend specifies the blend mode.
	Blend BlendMode

	// Filter specifies the texture filter.
	Filter Filter
}

// DrawTrianglesOptions holds options for DrawTriangles.
type DrawTrianglesOptions struct {
	// Blend specifies the blend mode.
	Blend BlendMode

	// Filter specifies the texture filter.
	Filter Filter

	// FillRule specifies the fill rule for overlapping triangles.
	FillRule FillRule
}

// DrawRectShaderOptions holds options for DrawRectShader.
type DrawRectShaderOptions struct {
	// GeoM is the geometry transformation matrix.
	GeoM GeoM

	// ColorScale scales the RGBA color of each pixel.
	// A zero-valued ColorScale is treated as opaque white (1,1,1,1), matching
	// Ebitengine's behavior so that a default DrawRectShaderOptions{} draws the
	// image unmodified. To make an image invisible, use ColorM or set the
	// alpha channel in vertex colors instead.
	ColorScale fmath.Color

	// Blend specifies the blend mode.
	Blend BlendMode

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
	Blend BlendMode

	// FillRule specifies the fill rule for overlapping triangles.
	FillRule FillRule

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
	rend := getRenderer()
	if rend == nil || rend.batcher == nil {
		return
	}

	var o DrawRectShaderOptions
	if opts != nil {
		o = *opts
	}

	// Apply uniforms to shader before draw.
	shader.applyUniforms(o.Uniforms)

	w := float32(width)
	h := float32(height)

	// Apply GeoM to quad corners.
	x0, y0 := o.GeoM.Apply(0, 0)
	x1, y1 := o.GeoM.Apply(float64(w), 0)
	x2, y2 := o.GeoM.Apply(float64(w), float64(h))
	x3, y3 := o.GeoM.Apply(0, float64(h))

	cr, cg, cb, ca := colorScaleOrDefault(o.ColorScale)
	blend := blendToBackend(o.Blend)

	// Determine texture ID from first source image, or white texture.
	texID := rend.whiteTextureID
	if o.Images[0] != nil && o.Images[0].texture != nil {
		texID = o.Images[0].textureID
	}

	// Bind additional textures via shader uniforms.
	for i := 0; i < 4; i++ {
		if o.Images[i] != nil && o.Images[i].texture != nil {
			shader.backend.SetUniformInt(fmt.Sprintf("uTexture%d", i), int32(i))
		}
	}

	rend.batcher.AddQuadDirect(
		batch.Vertex2D{PosX: float32(x0), PosY: float32(y0), TexU: 0, TexV: 0, R: cr, G: cg, B: cb, A: ca},
		batch.Vertex2D{PosX: float32(x1), PosY: float32(y1), TexU: 1, TexV: 0, R: cr, G: cg, B: cb, A: ca},
		batch.Vertex2D{PosX: float32(x2), PosY: float32(y2), TexU: 1, TexV: 1, R: cr, G: cg, B: cb, A: ca},
		batch.Vertex2D{PosX: float32(x3), PosY: float32(y3), TexU: 0, TexV: 1, R: cr, G: cg, B: cb, A: ca},
		batch.DrawCommand{
			TextureID: texID,
			BlendMode: blend,
			ShaderID:  shader.id,
			TargetID:  img.textureID,
			ColorBody: colorMatrixIdentityBody,
		},
	)
}

// DrawTrianglesShader draws triangles using a custom shader. This is the
// equivalent of ebiten.Image.DrawTrianglesShader.
func (img *Image) DrawTrianglesShader(vertices []Vertex, indices []uint16, shader *Shader, opts *DrawTrianglesShaderOptions) {
	if img.disposed || shader == nil || shader.disposed {
		return
	}
	rend := getRenderer()
	if rend == nil || rend.batcher == nil {
		return
	}

	var o DrawTrianglesShaderOptions
	if opts != nil {
		o = *opts
	}

	// Apply uniforms.
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

	rend.batcher.Add(batch.DrawCommand{
		Vertices:  batchVerts,
		Indices:   indices,
		TextureID: texID,
		BlendMode: blend,
		FillRule:  fillRule,
		ShaderID:  shader.id,
		TargetID:  img.textureID,
		ColorBody: colorMatrixIdentityBody,
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

// BlendMode specifies how colors are blended.
type BlendMode int

// BlendMode constants.
const (
	BlendSourceOver     BlendMode = iota // Standard alpha blending
	BlendAdditive                        // Additive blending
	BlendMultiplicative                  // Multiplicative blending
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

// colorScaleOrDefault returns RGBA components from a ColorScale, defaulting
// to opaque white if the color is zero-valued. This matches Ebitengine's
// convention: a zero-valued DrawImageOptions{} draws the image as-is.
// Transparent black (0,0,0,0) is indistinguishable from "not set".
func colorScaleOrDefault(c fmath.Color) (r, g, b, a float32) {
	if c.R == 0 && c.G == 0 && c.B == 0 && c.A == 0 {
		return 1, 1, 1, 1
	}
	return float32(c.R), float32(c.G), float32(c.B), float32(c.A)
}

// blendToBackend maps a public BlendMode to a backend BlendMode.
func blendToBackend(b BlendMode) backend.BlendMode {
	switch b {
	case BlendAdditive:
		return backend.BlendAdditive
	case BlendMultiplicative:
		return backend.BlendMultiplicative
	default:
		return backend.BlendSourceOver
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
