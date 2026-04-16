package futurerender

import (
	goimage "image"
	"image/color"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
	fmath "github.com/michaelraines/future-core/math"
)

// --- Mock device for testing GPU texture lifecycle ---

type mockTexture struct {
	w, h     int
	fmt      backend.TextureFormat
	disposed bool
}

func (t *mockTexture) Upload(_ []byte, _ int)                   {}
func (t *mockTexture) UploadRegion(_ []byte, _, _, _, _, _ int) {}
func (t *mockTexture) ReadPixels(dst []byte) {
	for i := range dst {
		dst[i] = 0xFF
	}
}
func (t *mockTexture) Width() int                    { return t.w }
func (t *mockTexture) Height() int                   { return t.h }
func (t *mockTexture) Format() backend.TextureFormat { return t.fmt }
func (t *mockTexture) Dispose()                      { t.disposed = true }

// mockRenderTarget implements backend.RenderTarget for testing.
type mockRenderTarget struct {
	colorTex *mockTexture
	w, h     int
	disposed bool
}

func (rt *mockRenderTarget) ColorTexture() backend.Texture { return rt.colorTex }
func (rt *mockRenderTarget) DepthTexture() backend.Texture { return nil }
func (rt *mockRenderTarget) Width() int                    { return rt.w }
func (rt *mockRenderTarget) Height() int                   { return rt.h }
func (rt *mockRenderTarget) Dispose()                      { rt.disposed = true }

type mockDevice struct {
	textures          []*mockTexture
	renderTargets     []*mockRenderTarget
	textureDescs      []backend.TextureDescriptor
	renderTargetDescs []backend.RenderTargetDescriptor
	readScreenFn      func([]byte) bool
}

func (d *mockDevice) Init(_ backend.DeviceConfig) error { return nil }
func (d *mockDevice) Dispose()                          {}
func (d *mockDevice) ReadScreen(pixels []byte) bool {
	if d.readScreenFn != nil {
		return d.readScreenFn(pixels)
	}
	return false
}
func (d *mockDevice) BeginFrame() {}
func (d *mockDevice) EndFrame()   {}
func (d *mockDevice) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	t := &mockTexture{w: desc.Width, h: desc.Height, fmt: desc.Format}
	d.textures = append(d.textures, t)
	d.textureDescs = append(d.textureDescs, desc)
	return t, nil
}
func (d *mockDevice) NewBuffer(_ backend.BufferDescriptor) (backend.Buffer, error) {
	return nil, nil
}
func (d *mockDevice) NewShader(_ backend.ShaderDescriptor) (backend.Shader, error) {
	return nil, nil
}
func (d *mockDevice) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	colorTex := &mockTexture{w: desc.Width, h: desc.Height}
	rt := &mockRenderTarget{colorTex: colorTex, w: desc.Width, h: desc.Height}
	d.renderTargets = append(d.renderTargets, rt)
	d.renderTargetDescs = append(d.renderTargetDescs, desc)
	return rt, nil
}
func (d *mockDevice) NewPipeline(_ backend.PipelineDescriptor) (backend.Pipeline, error) {
	return nil, nil
}
func (d *mockDevice) Capabilities() backend.DeviceCapabilities {
	return backend.DeviceCapabilities{MaxTextureSize: 4096}
}
func (d *mockDevice) Encoder() backend.CommandEncoder { return nil }

// withMockRenderer sets up a globalRenderer with a mock device and batcher,
// restoring the previous state on cleanup.
func withMockRenderer(t *testing.T) (dev *mockDevice, registered map[uint32]backend.Texture) {
	t.Helper()
	dev = &mockDevice{}
	registered = make(map[uint32]backend.Texture)
	rend := &renderer{
		device:  dev,
		batcher: batch.NewBatcher(1024, 1024),
		registerTexture: func(id uint32, tex backend.Texture) {
			registered[id] = tex
		},
		unregisterTexture: func(id uint32) {
			delete(registered, id)
		},
		registerRenderTarget:   func(_ uint32, _ backend.RenderTarget) {},
		unregisterRenderTarget: func(_ uint32) {},
		pendingClears:          newPendingClearTracker(),
	}
	old := getRenderer()
	setRenderer(rend)

	// Disable sprite atlasing so tests that inspect per-image textures
	// and UV coordinates work as expected.
	SetSpriteAtlasEnabled(false)

	t.Cleanup(func() {
		setRenderer(old)
		SetSpriteAtlasEnabled(true)
		ResetSpriteAtlas()
	})
	return dev, registered
}

// withBatchRenderer sets up a globalRenderer with a batcher but no device,
// restoring the previous state on cleanup.
func withBatchRenderer(t *testing.T, whiteTexID uint32) *batch.Batcher {
	t.Helper()
	b := batch.NewBatcher(1024, 1024)
	rend := &renderer{
		batcher:        b,
		whiteTextureID: whiteTexID,
	}
	old := getRenderer()
	setRenderer(rend)
	t.Cleanup(func() { setRenderer(old) })
	return b
}

func TestNewImageNoRenderer(t *testing.T) {
	old := getRenderer()
	setRenderer(nil)
	defer func() { setRenderer(old) }()

	img := NewImage(100, 200)
	require.NotNil(t, img, "NewImage returned nil")

	w, h := img.Size()
	require.Equal(t, 100, w)
	require.Equal(t, 200, h)
	require.Nil(t, img.texture, "texture should be nil without a renderer")
}

func TestNewImageWithDevice(t *testing.T) {
	dev, registered := withMockRenderer(t)

	img := NewImage(64, 128)
	require.NotNil(t, img.texture, "texture should be allocated with a mock device")
	require.NotEqual(t, uint32(0), img.textureID, "textureID should be non-zero")
	require.NotNil(t, registered[img.textureID], "texture should be registered")

	// The image texture comes from the render target's color texture.
	require.NotEmpty(t, dev.renderTargets, "render target should be created")
	rt := dev.renderTargets[len(dev.renderTargets)-1]
	require.Equal(t, 64, rt.w)
	require.Equal(t, 128, rt.h)
}

func TestNewImageFromImageWithDevice(t *testing.T) {
	dev, registered := withMockRenderer(t)

	src := goimage.NewRGBA(goimage.Rect(0, 0, 32, 32))
	src.Set(0, 0, color.RGBA{R: 255, A: 255})

	img := NewImageFromImage(src)
	require.NotNil(t, img.texture, "texture should be allocated")

	w, h := img.Size()
	require.Equal(t, 32, w)
	require.Equal(t, 32, h)
	require.NotEqual(t, uint32(0), img.textureID, "textureID should be non-zero")
	require.NotNil(t, registered[img.textureID], "texture should be registered")

	// GPU texture is padded: 32+2=34 in each dimension.
	mt := dev.textures[len(dev.textures)-1]
	require.Equal(t, 34, mt.w)
	require.Equal(t, 34, mt.h)

	// UVs should map to the content region within the padded texture.
	require.True(t, img.padded, "image should be marked as padded")
	require.InDelta(t, float32(1)/float32(34), img.u0, 1e-6)
	require.InDelta(t, float32(1)/float32(34), img.v0, 1e-6)
	require.InDelta(t, float32(33)/float32(34), img.u1, 1e-6)
	require.InDelta(t, float32(33)/float32(34), img.v1, 1e-6)
}

func TestNewImageFromImageNonRGBA(t *testing.T) {
	withMockRenderer(t)

	src := goimage.NewNRGBA(goimage.Rect(0, 0, 16, 16))
	src.Set(0, 0, color.NRGBA{R: 128, G: 64, B: 32, A: 200})

	img := NewImageFromImage(src)
	require.NotNil(t, img.texture, "texture should be allocated for non-RGBA source")

	w, h := img.Size()
	require.Equal(t, 16, w)
	require.Equal(t, 16, h)
}

func TestDisposeReleasesTexture(t *testing.T) {
	dev, _ := withMockRenderer(t)
	rend := getRenderer()

	img := NewImage(32, 32)
	require.NotNil(t, img.texture, "texture should be allocated")

	// The texture comes from the render target's color texture.
	require.NotEmpty(t, dev.renderTargets, "render target should be created")
	rt := dev.renderTargets[len(dev.renderTargets)-1]
	mt := rt.colorTex
	require.False(t, mt.disposed, "texture should not be disposed yet")

	// Dispose marks the image as disposed immediately but defers the
	// GPU-resource teardown until renderer.disposeDeferred() runs after
	// the frame's submit. This avoids "Destroyed texture used in a
	// submit" WebGPU validation errors from mid-frame disposals.
	img.Dispose()
	require.True(t, img.disposed, "image should be disposed")
	require.False(t, rt.disposed, "render target should remain live until the deferred drain runs")
	require.NotNil(t, img.texture, "texture reference should remain until the deferred drain runs")

	rend.disposeDeferred()
	require.True(t, rt.disposed, "render target should be disposed after the deferred drain")
	require.Nil(t, img.texture, "texture reference should be nil after the deferred drain")
}

func TestDisposeIdempotent(t *testing.T) {
	withMockRenderer(t)

	img := NewImage(8, 8)
	img.Dispose()
	img.Dispose() // should not panic or double-free
	require.True(t, img.disposed, "image should remain disposed")
}

func TestWritePixels(t *testing.T) {
	_, _ = withMockRenderer(t)

	img := NewImage(64, 64)
	require.NotNil(t, img.texture)

	pix := make([]byte, 64*64*4)
	// Verify WritePixels doesn't panic.
	img.WritePixels(pix)
}

func TestWritePixelsNoTexture(t *testing.T) {
	old := getRenderer()
	setRenderer(nil)
	defer func() { setRenderer(old) }()

	img := NewImage(32, 32)
	// Should not panic with nil texture.
	img.WritePixels(make([]byte, 32*32*4))
}

func TestWritePixelsDisposed(t *testing.T) {
	withMockRenderer(t)

	img := NewImage(32, 32)
	img.Dispose()
	// Should not panic on disposed image.
	img.WritePixels(make([]byte, 32*32*4))
}

func TestWritePixelsRegion(t *testing.T) {
	withMockRenderer(t)

	img := NewImage(64, 64)
	require.NotNil(t, img.texture)

	pix := make([]byte, 10*10*4)
	img.WritePixelsRegion(pix, 5, 5, 10, 10)
}

func TestAllocTextureIDMonotonic(t *testing.T) {
	withMockRenderer(t)

	id1 := getRenderer().allocTextureID()
	id2 := getRenderer().allocTextureID()
	id3 := getRenderer().allocTextureID()
	require.True(t, id1 < id2, "texture IDs should be monotonically increasing")
	require.True(t, id2 < id3, "texture IDs should be monotonically increasing")
}

func TestSubImageUVMapping(t *testing.T) {
	img := &Image{
		width: 256, height: 256,
		textureID: 42,
		u0:        0, v0: 0, u1: 1, v1: 1,
	}

	sub := img.SubImage(goimage.Rect(0, 0, 128, 128))
	require.Equal(t, 128, sub.width)
	require.Equal(t, 128, sub.height)
	require.Equal(t, uint32(42), sub.textureID)
	require.Equal(t, img, sub.parent, "sub-image should reference parent")
	require.InDelta(t, float32(0), sub.u0, 1e-6)
	require.InDelta(t, float32(0), sub.v0, 1e-6)
	require.InDelta(t, float32(0.5), sub.u1, 1e-6)
	require.InDelta(t, float32(0.5), sub.v1, 1e-6)

	sub2 := img.SubImage(goimage.Rect(128, 128, 256, 256))
	require.InDelta(t, float32(0.5), sub2.u0, 1e-6)
	require.InDelta(t, float32(0.5), sub2.v0, 1e-6)
	require.InDelta(t, float32(1.0), sub2.u1, 1e-6)
	require.InDelta(t, float32(1.0), sub2.v1, 1e-6)
}

func TestSubImageZeroSize(t *testing.T) {
	// Zero-width image should not cause division by zero.
	img := &Image{width: 0, height: 100}
	sub := img.SubImage(goimage.Rect(0, 0, 50, 50))
	require.Equal(t, 50, sub.width)
	require.Equal(t, 50, sub.height)
	require.Nil(t, sub.texture)

	// Zero-height image should not cause division by zero.
	img2 := &Image{width: 100, height: 0}
	sub2 := img2.SubImage(goimage.Rect(0, 0, 30, 30))
	require.Equal(t, 30, sub2.width)
	require.Equal(t, 30, sub2.height)

	// Both zero should not cause division by zero.
	img3 := &Image{width: 0, height: 0}
	sub3 := img3.SubImage(goimage.Rect(0, 0, 10, 20))
	require.Equal(t, 10, sub3.width)
	require.Equal(t, 20, sub3.height)
}

func TestSubImageOfSubImage(t *testing.T) {
	root := &Image{
		width: 256, height: 256,
		textureID: 1,
		u0:        0, v0: 0, u1: 1, v1: 1,
	}

	sub := root.SubImage(goimage.Rect(0, 0, 128, 128))
	subsub := sub.SubImage(goimage.Rect(0, 0, 64, 64))
	require.Equal(t, root, subsub.parent, "nested sub-image should reference root parent")
	require.InDelta(t, float32(0), subsub.u0, 1e-6)
	require.InDelta(t, float32(0), subsub.v0, 1e-6)
	require.InDelta(t, float32(0.25), subsub.u1, 1e-6)
	require.InDelta(t, float32(0.25), subsub.v1, 1e-6)
}

func TestDispose(t *testing.T) {
	img := NewImage(10, 10)
	img.Dispose()
	require.True(t, img.disposed, "image should be disposed")

	// DrawImage on disposed image should be a no-op.
	img.DrawImage(NewImage(5, 5), nil) // should not panic
}

func TestSubImageDisposeDoesNotReleaseParent(t *testing.T) {
	root := &Image{
		width: 64, height: 64,
		textureID: 1,
		u0:        0, v0: 0, u1: 1, v1: 1,
	}
	sub := root.SubImage(goimage.Rect(0, 0, 32, 32))
	sub.Dispose()
	require.False(t, root.disposed, "disposing sub-image should not dispose root")
}

func TestDrawImageSubmitsToBatcher(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{
		width: 320, height: 240,
		u0: 0, v0: 0, u1: 1, v1: 1,
	}
	src := &Image{
		width: 64, height: 64,
		textureID: 5,
		u0:        0, v0: 0, u1: 1, v1: 1,
	}

	opts := &DrawImageOptions{}
	opts.GeoM.Translate(100, 50)
	dst.DrawImage(src, opts)

	require.Equal(t, 1, b.CommandCount())

	batches := b.Flush()
	require.Equal(t, 1, len(batches))

	got := batches[0]
	require.Equal(t, 4, len(got.Vertices))
	require.Equal(t, 6, len(got.Indices))

	v0 := got.Vertices[0]
	require.InDelta(t, float32(100), v0.PosX, 1e-6)
	require.InDelta(t, float32(50), v0.PosY, 1e-6)

	v2 := got.Vertices[2]
	require.InDelta(t, float32(164), v2.PosX, 1e-6)
	require.InDelta(t, float32(114), v2.PosY, 1e-6)
}

func TestDrawImageColorScale(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 100, height: 100, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 32, height: 32, textureID: 2, u0: 0, v0: 0, u1: 1, v1: 1}

	opts := &DrawImageOptions{}
	opts.ColorScale.Scale(0.5, 0.5, 0.5, 0.5)
	dst.DrawImage(src, opts)

	batches := b.Flush()
	v := batches[0].Vertices[0]
	require.InDelta(t, float32(0.5), v.R, 1e-6)
	require.InDelta(t, float32(0.5), v.G, 1e-6)
	require.InDelta(t, float32(0.5), v.B, 1e-6)
	require.InDelta(t, float32(0.5), v.A, 1e-6)
}

func TestDrawImageDefaultColorIsWhite(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 100, height: 100, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 32, height: 32, textureID: 2, u0: 0, v0: 0, u1: 1, v1: 1}

	dst.DrawImage(src, nil) // nil opts -> default color

	batches := b.Flush()
	v := batches[0].Vertices[0]
	require.InDelta(t, float32(1), v.R, 1e-6)
	require.InDelta(t, float32(1), v.G, 1e-6)
	require.InDelta(t, float32(1), v.B, 1e-6)
	require.InDelta(t, float32(1), v.A, 1e-6)
}

func TestFillSubmitsToBatcher(t *testing.T) {
	b := withBatchRenderer(t, 99)

	img := &Image{width: 320, height: 240, u0: 0, v0: 0, u1: 1, v1: 1}
	img.Fill(color.NRGBA{R: 255, G: 0, B: 0, A: 255})

	batches := b.Flush()
	require.Equal(t, 1, len(batches))
	require.Equal(t, uint32(99), batches[0].TextureID)

	v := batches[0].Vertices[0]
	require.InDelta(t, float32(1), v.R, 1e-6)
	require.InDelta(t, float32(0), v.G, 1e-6)
}

func TestBlendToBackend(t *testing.T) {
	tests := []struct {
		pub  Blend
		want backend.BlendMode
	}{
		{BlendSourceOver, backend.BlendSourceOver},
		{BlendLighter, backend.BlendAdditive},
		{BlendMultiply, backend.BlendMultiplicative},
	}
	for _, tt := range tests {
		got := blendToBackend(tt.pub)
		require.Equal(t, tt.want, got)
	}
}

// --- New tests ---

func TestBounds(t *testing.T) {
	img := NewImage(320, 240)
	b := img.Bounds()
	require.Equal(t, 0, b.Min.X)
	require.Equal(t, 0, b.Min.Y)
	require.Equal(t, 320, b.Max.X)
	require.Equal(t, 240, b.Max.Y)
}

func TestNewGeoM(t *testing.T) {
	g := NewGeoM()
	x, y := g.Apply(10, 20)
	require.InDelta(t, 10.0, x, 1e-6)
	require.InDelta(t, 20.0, y, 1e-6)
}

func TestGeoMScale(t *testing.T) {
	g := NewGeoM()
	g.Scale(2, 3)
	x, y := g.Apply(10, 20)
	require.InDelta(t, 20.0, x, 1e-6)
	require.InDelta(t, 60.0, y, 1e-6)
}

func TestGeoMRotate(t *testing.T) {
	g := NewGeoM()
	g.Rotate(math.Pi / 2)
	x, y := g.Apply(1, 0)
	require.InDelta(t, 0.0, x, 1e-6)
	require.InDelta(t, 1.0, y, 1e-6)
}

func TestGeoMSkew(t *testing.T) {
	g := NewGeoM()
	g.Skew(1, 0)
	x, y := g.Apply(0, 5)
	require.InDelta(t, 5.0, x, 1e-6)
	require.InDelta(t, 5.0, y, 1e-6)
}

func TestGeoMConcat(t *testing.T) {
	g1 := NewGeoM()
	g1.Scale(2, 2)

	g2 := NewGeoM()
	g2.Translate(10, 20)

	g1.Concat(g2)
	x, y := g1.Apply(5, 5)
	require.InDelta(t, 20.0, x, 1e-6)
	require.InDelta(t, 30.0, y, 1e-6)
}

func TestGeoMReset(t *testing.T) {
	g := NewGeoM()
	g.Scale(5, 5)
	g.Reset()
	x, y := g.Apply(10, 20)
	require.InDelta(t, 10.0, x, 1e-6)
	require.InDelta(t, 20.0, y, 1e-6)
}

func TestGeoMMat3(t *testing.T) {
	g := NewGeoM()
	m := g.Mat3()
	identity := fmath.Mat3Identity()
	require.Equal(t, identity, m)
}

func TestGeoMElement(t *testing.T) {
	g := NewGeoM()
	g.Translate(10, 20)
	require.InDelta(t, 1.0, g.Element(0, 0), 1e-6)
	require.InDelta(t, 0.0, g.Element(0, 1), 1e-6)
	require.InDelta(t, 10.0, g.Element(0, 2), 1e-6)
	require.InDelta(t, 0.0, g.Element(1, 0), 1e-6)
	require.InDelta(t, 1.0, g.Element(1, 1), 1e-6)
	require.InDelta(t, 20.0, g.Element(1, 2), 1e-6)
}

func TestGeoMSetElement(t *testing.T) {
	g := NewGeoM()
	g.SetElement(0, 2, 50)
	g.SetElement(1, 2, 100)
	x, y := g.Apply(0, 0)
	require.InDelta(t, 50.0, x, 1e-6)
	require.InDelta(t, 100.0, y, 1e-6)
}

func TestGeoMInvert(t *testing.T) {
	g := NewGeoM()
	g.Translate(10, 20)
	g.Scale(2, 3)
	g.Invert()

	// Applying the inverted transform to the output should recover input.
	x, y := g.Apply(20, 60)
	require.InDelta(t, 0.0, x, 1e-6)
	require.InDelta(t, 0.0, y, 1e-6)
}

func TestNewImageWithOptions(t *testing.T) {
	img := NewImageWithOptions(goimage.Rect(0, 0, 64, 32), nil)
	require.NotNil(t, img)
	w, h := img.Size()
	require.Equal(t, 64, w)
	require.Equal(t, 32, h)
}

func TestNewImageWithOptionsUnmanaged(t *testing.T) {
	opts := &NewImageOptions{Unmanaged: true}
	img := NewImageWithOptions(goimage.Rect(0, 0, 16, 16), opts)
	require.NotNil(t, img)
	w, h := img.Size()
	require.Equal(t, 16, w)
	require.Equal(t, 16, h)
}

func TestColorFromRGBA(t *testing.T) {
	c := ColorFromRGBA(0.1, 0.2, 0.3, 0.4)
	require.InDelta(t, 0.1, c.R, 1e-6)
	require.InDelta(t, 0.2, c.G, 1e-6)
	require.InDelta(t, 0.3, c.B, 1e-6)
	require.InDelta(t, 0.4, c.A, 1e-6)
}

func TestDrawTrianglesSubmitsToBatcher(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 320, height: 240, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 64, height: 64, textureID: 7, u0: 0, v0: 0, u1: 1, v1: 1}

	verts := []Vertex{
		{DstX: 0, DstY: 0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 64, DstY: 0, SrcX: 1, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 64, DstY: 64, SrcX: 1, SrcY: 1, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	indices := []uint16{0, 1, 2}

	dst.DrawTriangles(verts, indices, src, nil)

	require.Equal(t, 1, b.CommandCount())

	batches := b.Flush()
	require.Equal(t, 1, len(batches))
	require.Equal(t, 3, len(batches[0].Vertices))
	require.Equal(t, 3, len(batches[0].Indices))
	require.Equal(t, uint32(7), batches[0].TextureID)
}

func TestDrawTrianglesWithOptions(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 320, height: 240, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 64, height: 64, textureID: 3, u0: 0, v0: 0, u1: 1, v1: 1}

	verts := []Vertex{
		{DstX: 0, DstY: 0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 0, ColorB: 0, ColorA: 1},
		{DstX: 10, DstY: 0, SrcX: 1, SrcY: 0, ColorR: 1, ColorG: 0, ColorB: 0, ColorA: 1},
		{DstX: 10, DstY: 10, SrcX: 1, SrcY: 1, ColorR: 1, ColorG: 0, ColorB: 0, ColorA: 1},
	}
	indices := []uint16{0, 1, 2}

	opts := &DrawTrianglesOptions{Blend: BlendLighter}
	dst.DrawTriangles(verts, indices, src, opts)

	batches := b.Flush()
	require.Equal(t, 1, len(batches))
	require.Equal(t, backend.BlendAdditive, batches[0].BlendMode)
}

func TestDrawTrianglesNilSrc(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 320, height: 240, u0: 0, v0: 0, u1: 1, v1: 1}
	verts := []Vertex{
		{DstX: 0, DstY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 10, DstY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 10, DstY: 10, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	indices := []uint16{0, 1, 2}

	dst.DrawTriangles(verts, indices, nil, nil)

	batches := b.Flush()
	require.Equal(t, 1, len(batches))
	// nil src uses the white texture so vertex colors pass through
	// unmodified (white * vertex_color = vertex_color).
	require.Equal(t, uint32(1), batches[0].TextureID)
}

func TestDrawTrianglesDisposedIsNoop(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 320, height: 240, disposed: true, u0: 0, v0: 0, u1: 1, v1: 1}
	verts := []Vertex{{DstX: 0, DstY: 0}}
	indices := []uint16{0}

	dst.DrawTriangles(verts, indices, nil, nil)
	require.Equal(t, 0, b.CommandCount())
}

func TestFillDisposed(t *testing.T) {
	b := withBatchRenderer(t, 1)

	img := &Image{width: 100, height: 100, disposed: true, u0: 0, v0: 0, u1: 1, v1: 1}
	img.Fill(color.NRGBA{R: 255, G: 0, B: 0, A: 255})
	require.Equal(t, 0, b.CommandCount())
}

func TestDrawImageNilSrc(t *testing.T) {
	withBatchRenderer(t, 1)

	dst := &Image{width: 100, height: 100, u0: 0, v0: 0, u1: 1, v1: 1}
	// Should not panic.
	dst.DrawImage(nil, nil)
}

func TestDrawImageDisposedSrc(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 100, height: 100, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 32, height: 32, disposed: true, u0: 0, v0: 0, u1: 1, v1: 1}
	dst.DrawImage(src, nil)
	require.Equal(t, 0, b.CommandCount())
}

func TestDrawImageNoRenderer(t *testing.T) {
	old := getRenderer()
	setRenderer(nil)
	defer func() { setRenderer(old) }()

	dst := &Image{width: 100, height: 100, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 32, height: 32, u0: 0, v0: 0, u1: 1, v1: 1}
	// Should not panic.
	dst.DrawImage(src, nil)
}

func TestFillNoRenderer(t *testing.T) {
	old := getRenderer()
	setRenderer(nil)
	defer func() { setRenderer(old) }()

	img := &Image{width: 100, height: 100, u0: 0, v0: 0, u1: 1, v1: 1}
	// Should not panic.
	img.Fill(color.NRGBA{R: 255, G: 0, B: 0, A: 255})
}

func TestDrawTrianglesNoRenderer(t *testing.T) {
	old := getRenderer()
	setRenderer(nil)
	defer func() { setRenderer(old) }()

	dst := &Image{width: 100, height: 100, u0: 0, v0: 0, u1: 1, v1: 1}
	verts := []Vertex{{DstX: 0, DstY: 0}}
	indices := []uint16{0}
	// Should not panic.
	dst.DrawTriangles(verts, indices, nil, nil)
}

func TestGeoMZeroValueActsAsIdentity(t *testing.T) {
	var g GeoM
	x, y := g.Apply(7, 13)
	require.InDelta(t, 7.0, x, 1e-6)
	require.InDelta(t, 13.0, y, 1e-6)
}

func TestColorScaleRGBAOrDefault(t *testing.T) {
	// Zero-valued ColorScale should default to white.
	var cs ColorScale
	r, g, b, a := cs.rgbaOrDefault()
	require.InDelta(t, float32(1), r, 1e-6)
	require.InDelta(t, float32(1), g, 1e-6)
	require.InDelta(t, float32(1), b, 1e-6)
	require.InDelta(t, float32(1), a, 1e-6)

	// Set color should be returned as-is.
	cs.Scale(0.2, 0.3, 0.4, 0.5)
	r2, g2, b2, a2 := cs.rgbaOrDefault()
	require.InDelta(t, float32(0.2), r2, 1e-6)
	require.InDelta(t, float32(0.3), g2, 1e-6)
	require.InDelta(t, float32(0.4), b2, 1e-6)
	require.InDelta(t, float32(0.5), a2, 1e-6)
}

func TestColorScaleMethods(t *testing.T) {
	var cs ColorScale
	// Default values should be 1.
	require.InDelta(t, float32(1), cs.R(), 1e-6)
	require.InDelta(t, float32(1), cs.G(), 1e-6)
	require.InDelta(t, float32(1), cs.B(), 1e-6)
	require.InDelta(t, float32(1), cs.A(), 1e-6)

	// After Scale, values should reflect.
	cs.Scale(0.5, 0.6, 0.7, 0.8)
	require.InDelta(t, float32(0.5), cs.R(), 1e-6)
	require.InDelta(t, float32(0.6), cs.G(), 1e-6)
	require.InDelta(t, float32(0.7), cs.B(), 1e-6)
	require.InDelta(t, float32(0.8), cs.A(), 1e-6)

	// Multiply again.
	cs.Scale(0.5, 0.5, 0.5, 0.5)
	require.InDelta(t, float32(0.25), cs.R(), 1e-6)
	require.InDelta(t, float32(0.3), cs.G(), 1e-6)
	require.InDelta(t, float32(0.35), cs.B(), 1e-6)
	require.InDelta(t, float32(0.4), cs.A(), 1e-6)
}

func TestColorScaleAlpha(t *testing.T) {
	var cs ColorScale
	cs.ScaleAlpha(0.5)
	require.InDelta(t, float32(1), cs.R(), 1e-6)
	require.InDelta(t, float32(0.5), cs.A(), 1e-6)

	cs.ScaleAlpha(0.5)
	require.InDelta(t, float32(0.25), cs.A(), 1e-6)
}

func TestColorScaleReset(t *testing.T) {
	var cs ColorScale
	cs.Scale(0.2, 0.3, 0.4, 0.5)
	cs.Reset()
	require.InDelta(t, float32(1), cs.R(), 1e-6)
	require.InDelta(t, float32(1), cs.A(), 1e-6)
}

func TestColorScaleSetColor(t *testing.T) {
	var cs ColorScale
	cs.SetColor(fmath.Color{R: 0.1, G: 0.2, B: 0.3, A: 0.4})
	require.InDelta(t, float32(0.1), cs.R(), 1e-6)
	require.InDelta(t, float32(0.2), cs.G(), 1e-6)
	require.InDelta(t, float32(0.3), cs.B(), 1e-6)
	require.InDelta(t, float32(0.4), cs.A(), 1e-6)
}

func TestBlendToBackendUnknown(t *testing.T) {
	// Zero-valued blend should default to SourceOver.
	got := blendToBackend(Blend{})
	require.Equal(t, backend.BlendSourceOver, got)
}

func TestFilterToBackend(t *testing.T) {
	tests := []struct {
		pub  Filter
		want backend.TextureFilter
	}{
		{FilterNearest, backend.FilterNearest},
		{FilterLinear, backend.FilterLinear},
	}
	for _, tt := range tests {
		got := filterToBackend(tt.pub)
		require.Equal(t, tt.want, got)
	}
}

func TestFilterToBackendUnknown(t *testing.T) {
	got := filterToBackend(Filter(999))
	require.Equal(t, backend.FilterNearest, got)
}

func TestDrawImageFilterPassedToBatcher(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 100, height: 100, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 32, height: 32, textureID: 2, u0: 0, v0: 0, u1: 1, v1: 1}

	opts := &DrawImageOptions{Filter: FilterLinear}
	dst.DrawImage(src, opts)

	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Equal(t, backend.FilterLinear, batches[0].Filter)
}

func TestDrawImageDefaultFilter(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 100, height: 100, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 32, height: 32, textureID: 2, u0: 0, v0: 0, u1: 1, v1: 1}

	dst.DrawImage(src, nil)

	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Equal(t, backend.FilterNearest, batches[0].Filter)
}

func TestDrawTrianglesFilterPassedToBatcher(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 320, height: 240, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 64, height: 64, textureID: 3, u0: 0, v0: 0, u1: 1, v1: 1}

	verts := []Vertex{
		{DstX: 0, DstY: 0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 10, DstY: 0, SrcX: 1, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 10, DstY: 10, SrcX: 1, SrcY: 1, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	indices := []uint16{0, 1, 2}

	opts := &DrawTrianglesOptions{Filter: FilterLinear}
	dst.DrawTriangles(verts, indices, src, opts)

	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Equal(t, backend.FilterLinear, batches[0].Filter)
}

func TestDrawTrianglesDefaultFilter(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 320, height: 240, u0: 0, v0: 0, u1: 1, v1: 1}
	verts := []Vertex{
		{DstX: 0, DstY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 10, DstY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 10, DstY: 10, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	indices := []uint16{0, 1, 2}

	dst.DrawTriangles(verts, indices, nil, nil)

	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Equal(t, backend.FilterNearest, batches[0].Filter)
}

func TestFillRuleToBackend(t *testing.T) {
	tests := []struct {
		pub  FillRule
		want backend.FillRule
	}{
		{FillRuleNonZero, backend.FillRuleNonZero},
		{FillRuleEvenOdd, backend.FillRuleEvenOdd},
	}
	for _, tt := range tests {
		got := fillRuleToBackend(tt.pub)
		require.Equal(t, tt.want, got)
	}
}

func TestFillRuleToBackendUnknown(t *testing.T) {
	got := fillRuleToBackend(FillRule(999))
	require.Equal(t, backend.FillRuleNonZero, got)
}

func TestDrawTrianglesFillRulePassedToBatcher(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 320, height: 240, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 64, height: 64, textureID: 3, u0: 0, v0: 0, u1: 1, v1: 1}

	verts := []Vertex{
		{DstX: 0, DstY: 0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 10, DstY: 0, SrcX: 1, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 10, DstY: 10, SrcX: 1, SrcY: 1, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	indices := []uint16{0, 1, 2}

	opts := &DrawTrianglesOptions{FillRule: FillRuleEvenOdd}
	dst.DrawTriangles(verts, indices, src, opts)

	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Equal(t, backend.FillRuleEvenOdd, batches[0].FillRule)
}

func TestDrawTrianglesDefaultFillRule(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 320, height: 240, u0: 0, v0: 0, u1: 1, v1: 1}
	verts := []Vertex{
		{DstX: 0, DstY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 10, DstY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 10, DstY: 10, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	indices := []uint16{0, 1, 2}

	dst.DrawTriangles(verts, indices, nil, nil)

	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Equal(t, backend.FillRuleNonZero, batches[0].FillRule)
}

func TestNewImageFromImageNoRenderer(t *testing.T) {
	old := getRenderer()
	setRenderer(nil)
	defer func() { setRenderer(old) }()

	src := goimage.NewRGBA(goimage.Rect(0, 0, 8, 8))
	img := NewImageFromImage(src)
	require.NotNil(t, img)
	w, h := img.Size()
	require.Equal(t, 8, w)
	require.Equal(t, 8, h)
	require.Nil(t, img.texture)
}

// --- Off-screen render target tests ---

func TestNewImageCreatesRenderTarget(t *testing.T) {
	dev, _ := withMockRenderer(t)

	img := NewImage(128, 64)
	require.NotNil(t, img.texture)
	require.NotNil(t, img.renderTarget)
	require.Len(t, dev.renderTargets, 1)
	require.Equal(t, 128, dev.renderTargets[0].w)
	require.Equal(t, 64, dev.renderTargets[0].h)
}

func TestDisposeReleasesRenderTarget(t *testing.T) {
	dev, _ := withMockRenderer(t)
	rend := getRenderer()

	img := NewImage(32, 32)
	require.NotNil(t, img.renderTarget)
	rt := dev.renderTargets[0]

	img.Dispose()
	// Dispose is deferred — render target stays live until the
	// renderer drains its deferred-dispose list after submit.
	require.False(t, rt.disposed)
	require.NotNil(t, img.renderTarget)

	rend.disposeDeferred()
	require.True(t, rt.disposed)
	require.Nil(t, img.renderTarget)
}

func TestImageClear(t *testing.T) {
	withMockRenderer(t)

	img := NewImage(4, 4)
	require.NotNil(t, img.texture)

	// Clear bypasses the batcher — it uploads zeros directly to the
	// texture via texture.Upload. Verify it doesn't panic.
	img.Clear()

	// Verify no batcher commands were produced (Clear is direct upload).
	rend := getRenderer()
	require.NotNil(t, rend)
	batches := rend.batcher.Flush()
	require.Len(t, batches, 0, "Clear should bypass the batcher (direct texture upload)")
}

func TestImageReadPixels(t *testing.T) {
	withMockRenderer(t)

	img := NewImage(4, 4)
	require.NotNil(t, img.texture)

	dst := make([]byte, 4*4*4)
	img.ReadPixels(dst)
	// Mock fills with 0xFF.
	require.Equal(t, byte(0xFF), dst[0])
}

func TestImageReadPixelsDisposed(t *testing.T) {
	withMockRenderer(t)

	img := NewImage(4, 4)
	img.Dispose()

	dst := make([]byte, 4*4*4)
	img.ReadPixels(dst)
	// Should be all zeros since no read happened.
	require.Equal(t, byte(0), dst[0])
}

func TestImageReadPixelsNoTexture(t *testing.T) {
	old := getRenderer()
	setRenderer(nil)
	defer func() { setRenderer(old) }()

	img := NewImage(4, 4)
	dst := make([]byte, 4*4*4)
	// Should not panic.
	img.ReadPixels(dst)
}

func TestImageRenderTarget(t *testing.T) {
	withMockRenderer(t)
	img := NewImage(64, 64)
	require.NotNil(t, img.RenderTarget())

	// Screen-like image has no render target.
	screen := &Image{width: 800, height: 600}
	require.Nil(t, screen.RenderTarget())
}

func TestDrawImageSetsTargetID(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 100, height: 100, textureID: 42, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 32, height: 32, textureID: 5, u0: 0, v0: 0, u1: 1, v1: 1}

	dst.DrawImage(src, nil)

	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Equal(t, uint32(42), batches[0].TargetID)
}

func TestFillSetsTargetID(t *testing.T) {
	b := withBatchRenderer(t, 1)

	img := &Image{width: 100, height: 100, textureID: 7, u0: 0, v0: 0, u1: 1, v1: 1}
	img.Fill(color.NRGBA{R: 255, G: 0, B: 0, A: 255})

	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Equal(t, uint32(7), batches[0].TargetID)
}

func TestScreenImageTargetIDZero(t *testing.T) {
	b := withBatchRenderer(t, 1)

	// Screen image has textureID 0.
	screen := &Image{width: 800, height: 600, textureID: 0, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 32, height: 32, textureID: 5, u0: 0, v0: 0, u1: 1, v1: 1}
	screen.DrawImage(src, nil)

	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Equal(t, uint32(0), batches[0].TargetID)
}

func TestDrawImageColorM(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 100, height: 100, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 32, height: 32, textureID: 2, u0: 0, v0: 0, u1: 1, v1: 1}

	opts := &DrawImageOptions{
		ColorM: fmath.ColorMatrixScale(0.5, 1.0, 0.5, 1.0),
	}
	dst.DrawImage(src, opts)

	batches := b.Flush()
	require.Len(t, batches, 1)
	// ColorBody should be a scaled identity.
	require.InDelta(t, float32(0.5), batches[0].ColorBody[0], 1e-6)  // R scale
	require.InDelta(t, float32(1.0), batches[0].ColorBody[5], 1e-6)  // G scale
	require.InDelta(t, float32(0.5), batches[0].ColorBody[10], 1e-6) // B scale
	require.InDelta(t, float32(1.0), batches[0].ColorBody[15], 1e-6) // A scale
}

func TestDrawImageDefaultColorM(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 100, height: 100, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 32, height: 32, textureID: 2, u0: 0, v0: 0, u1: 1, v1: 1}

	dst.DrawImage(src, nil)

	batches := b.Flush()
	require.Len(t, batches, 1)
	// Default ColorM should be identity.
	require.Equal(t, colorMatrixIdentityBody, batches[0].ColorBody)
	require.Equal(t, [4]float32{}, batches[0].ColorTranslation)
}

func TestColorMatrixToUniforms(t *testing.T) {
	// Identity
	body, trans := colorMatrixToUniforms(fmath.ColorMatrixIdentity())
	require.Equal(t, colorMatrixIdentityBody, body)
	require.Equal(t, [4]float32{}, trans)

	// Zero value treated as identity
	body, trans = colorMatrixToUniforms(fmath.ColorMatrix{})
	require.Equal(t, colorMatrixIdentityBody, body)
	require.Equal(t, [4]float32{}, trans)

	// Scale
	body, trans = colorMatrixToUniforms(fmath.ColorMatrixScale(2, 0.5, 1, 1))
	require.InDelta(t, float32(2), body[0], 1e-6)
	require.InDelta(t, float32(0.5), body[5], 1e-6)
	require.Equal(t, [4]float32{}, trans)

	// Translate
	body, trans = colorMatrixToUniforms(fmath.ColorMatrixTranslate(0.1, 0.2, 0.3, 0.4))
	require.Equal(t, colorMatrixIdentityBody, body)
	require.InDelta(t, float32(0.1), trans[0], 1e-6)
	require.InDelta(t, float32(0.2), trans[1], 1e-6)
	require.InDelta(t, float32(0.3), trans[2], 1e-6)
	require.InDelta(t, float32(0.4), trans[3], 1e-6)
}

func TestSetPixel(t *testing.T) {
	img := &Image{
		width: 10, height: 10,
		texture: &mockTexture{w: 10, h: 10},
		u0:      0, v0: 0, u1: 1, v1: 1,
	}
	// Should not panic — writes a single pixel via UploadRegion.
	img.Set(5, 5, color.NRGBA{R: 255, G: 128, B: 64, A: 255})
}

func TestSetPixelOutOfBounds(t *testing.T) {
	img := &Image{
		width: 10, height: 10,
		texture: &mockTexture{w: 10, h: 10},
		u0:      0, v0: 0, u1: 1, v1: 1,
	}
	// All out-of-bounds — should be no-ops, no panic.
	img.Set(-1, 0, color.White)
	img.Set(0, -1, color.White)
	img.Set(10, 0, color.White)
	img.Set(0, 10, color.White)
}

func TestSetPixelDisposed(t *testing.T) {
	img := &Image{
		width: 10, height: 10, disposed: true,
		texture: &mockTexture{w: 10, h: 10},
		u0:      0, v0: 0, u1: 1, v1: 1,
	}
	img.Set(0, 0, color.White) // no-op, no panic
}

func TestSetPixelNoTexture(t *testing.T) {
	img := &Image{width: 10, height: 10, u0: 0, v0: 0, u1: 1, v1: 1}
	img.Set(0, 0, color.White) // no-op, no panic
}

func TestDrawTrianglesAntiAliasField(t *testing.T) {
	opts := &DrawTrianglesOptions{AntiAlias: true}
	require.True(t, opts.AntiAlias)
}

func TestDrawTrianglesShaderAntiAliasField(t *testing.T) {
	opts := &DrawTrianglesShaderOptions{AntiAlias: true}
	require.True(t, opts.AntiAlias)
}

func TestDrawImageSubImage(t *testing.T) {
	b := withBatchRenderer(t, 1)

	parent := &Image{
		width: 256, height: 256,
		textureID: 10,
		texture:   &mockTexture{w: 256, h: 256},
		u0:        0, v0: 0, u1: 1, v1: 1,
	}
	sub := parent.SubImage(goimage.Rect(0, 0, 128, 128))

	dst := &Image{width: 320, height: 240, u0: 0, v0: 0, u1: 1, v1: 1}
	dst.DrawImage(sub, nil)

	batches := b.Flush()
	require.Equal(t, 1, len(batches))
	// The sub-image should use the parent's textureID.
	require.Equal(t, uint32(10), batches[0].TextureID)
	// UV coords should reflect the sub-image region.
	v0 := batches[0].Vertices[0]
	require.InDelta(t, float32(0), v0.TexU, 1e-6)
	require.InDelta(t, float32(0), v0.TexV, 1e-6)
	v2 := batches[0].Vertices[2]
	require.InDelta(t, float32(0.5), v2.TexU, 1e-6)
	require.InDelta(t, float32(0.5), v2.TexV, 1e-6)
}

// --- Pixel snap tests ---

func TestPixelSnapOption(t *testing.T) {
	b := withBatchRenderer(t, 1)

	dst := &Image{width: 320, height: 240, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 32, height: 32, textureID: 5, u0: 0, v0: 0, u1: 1, v1: 1}

	opts := &DrawImageOptions{PixelSnap: true}
	opts.GeoM.Translate(10.7, 20.3)
	dst.DrawImage(src, opts)

	batches := b.Flush()
	require.Len(t, batches, 1)

	v0 := batches[0].Vertices[0]
	require.InDelta(t, float32(11), v0.PosX, 1e-6, "should snap to nearest integer")
	require.InDelta(t, float32(20), v0.PosY, 1e-6, "should snap to nearest integer")

	v2 := batches[0].Vertices[2]
	require.InDelta(t, float32(43), v2.PosX, 1e-6, "should snap to nearest integer")
	require.InDelta(t, float32(52), v2.PosY, 1e-6, "should snap to nearest integer")
}

func TestPixelSnapGlobal(t *testing.T) {
	b := withBatchRenderer(t, 1)

	SetPixelSnapEnabled(true)
	t.Cleanup(func() { SetPixelSnapEnabled(false) })

	require.True(t, IsPixelSnapEnabled())

	dst := &Image{width: 320, height: 240, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 16, height: 16, textureID: 5, u0: 0, v0: 0, u1: 1, v1: 1}

	opts := &DrawImageOptions{}
	opts.GeoM.Translate(5.4, 3.6)
	dst.DrawImage(src, opts)

	batches := b.Flush()
	require.Len(t, batches, 1)

	v0 := batches[0].Vertices[0]
	require.InDelta(t, float32(5), v0.PosX, 1e-6, "global pixel snap should round down")
	require.InDelta(t, float32(4), v0.PosY, 1e-6, "global pixel snap should round up")
}

func TestPixelSnapDisabledByDefault(t *testing.T) {
	b := withBatchRenderer(t, 1)

	// Ensure global is off.
	SetPixelSnapEnabled(false)

	dst := &Image{width: 320, height: 240, u0: 0, v0: 0, u1: 1, v1: 1}
	src := &Image{width: 16, height: 16, textureID: 5, u0: 0, v0: 0, u1: 1, v1: 1}

	opts := &DrawImageOptions{}
	opts.GeoM.Translate(5.7, 3.3)
	dst.DrawImage(src, opts)

	batches := b.Flush()
	require.Len(t, batches, 1)

	v0 := batches[0].Vertices[0]
	require.InDelta(t, float32(5.7), v0.PosX, 1e-3, "should not snap when disabled")
	require.InDelta(t, float32(3.3), v0.PosY, 1e-3, "should not snap when disabled")
}

func TestPixelSnapFunc(t *testing.T) {
	tests := []struct {
		in, want float64
	}{
		{0.0, 0.0},
		{0.4, 0.0},
		{0.5, 1.0}, // math.Round rounds half away from zero: 0.5 → 1
		{0.6, 1.0},
		{1.5, 2.0}, // banker's rounding: 1.5 → 2
		{-0.3, 0.0},
		{-0.7, -1.0},
		{100.0, 100.0},
	}
	for _, tt := range tests {
		got := pixelSnap(tt.in)
		require.InDelta(t, tt.want, got, 1e-9)
	}
}

// --- Texture padding tests ---

func TestNewImageFromImagePadding(t *testing.T) {
	withMockRenderer(t)

	// Create a 4x4 red image.
	src := goimage.NewRGBA(goimage.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}

	img := NewImageFromImage(src)
	require.True(t, img.padded, "image should be padded")

	// UVs should map to content region (1/6 to 5/6 for 4+2=6 padded size).
	require.InDelta(t, float32(1)/float32(6), img.u0, 1e-6)
	require.InDelta(t, float32(1)/float32(6), img.v0, 1e-6)
	require.InDelta(t, float32(5)/float32(6), img.u1, 1e-6)
	require.InDelta(t, float32(5)/float32(6), img.v1, 1e-6)
}

func TestNewImageFromImageNonRGBAPadded(t *testing.T) {
	withMockRenderer(t)

	src := goimage.NewNRGBA(goimage.Rect(0, 0, 8, 8))
	img := NewImageFromImage(src)
	require.True(t, img.padded, "non-RGBA images should also be padded")
	require.InDelta(t, float32(1)/float32(10), img.u0, 1e-6)
}

func TestNewImageNotPadded(t *testing.T) {
	withMockRenderer(t)

	// NewImage (blank) should NOT be padded — it's a render target.
	img := NewImage(64, 64)
	require.False(t, img.padded, "blank images created as render targets should not be padded")
}

func TestSetPixelPaddedOffset(t *testing.T) {
	// Create a mock texture that records UploadRegion calls.
	var uploadX, uploadY int
	mt := &mockTexture{w: 12, h: 12}
	img := &Image{
		width: 10, height: 10,
		texture: mt,
		padded:  true,
		u0:      float32(1) / 12, v0: float32(1) / 12,
		u1: float32(11) / 12, v1: float32(11) / 12,
	}

	// Override UploadRegion via a tracking texture.
	tt := &trackingTexture{inner: mt}
	img.texture = tt
	img.Set(3, 4, color.White)
	uploadX, uploadY = tt.lastX, tt.lastY
	require.Equal(t, 4, uploadX, "Set should offset x by 1 for padded images")
	require.Equal(t, 5, uploadY, "Set should offset y by 1 for padded images")
}

// trackingTexture wraps a mock texture and records the last UploadRegion call.
type trackingTexture struct {
	inner         *mockTexture
	lastX, lastY  int
	lastW, lastH  int
	uploadRegionN int
	uploadFullN   int
}

func (t *trackingTexture) Upload(data []byte, mip int) { t.uploadFullN++; t.inner.Upload(data, mip) }
func (t *trackingTexture) UploadRegion(data []byte, x, y, w, h, mip int) {
	t.lastX, t.lastY = x, y
	t.lastW, t.lastH = w, h
	t.uploadRegionN++
}
func (t *trackingTexture) ReadPixels(dst []byte)         { t.inner.ReadPixels(dst) }
func (t *trackingTexture) Width() int                    { return t.inner.Width() }
func (t *trackingTexture) Height() int                   { return t.inner.Height() }
func (t *trackingTexture) Format() backend.TextureFormat { return t.inner.Format() }
func (t *trackingTexture) Dispose()                      { t.inner.Dispose() }

func TestWritePixelsPaddedUsesUploadRegion(t *testing.T) {
	tt := &trackingTexture{inner: &mockTexture{w: 12, h: 12}}
	img := &Image{
		width: 10, height: 10,
		texture: tt,
		padded:  true,
	}

	pix := make([]byte, 10*10*4)
	img.WritePixels(pix)
	require.Equal(t, 1, tt.uploadRegionN, "padded WritePixels should use UploadRegion")
	require.Equal(t, 1, tt.lastX, "should offset x by 1")
	require.Equal(t, 1, tt.lastY, "should offset y by 1")
	require.Equal(t, 10, tt.lastW)
	require.Equal(t, 10, tt.lastH)
}

func TestWritePixelsRegionPaddedOffset(t *testing.T) {
	tt := &trackingTexture{inner: &mockTexture{w: 12, h: 12}}
	img := &Image{
		width: 10, height: 10,
		texture: tt,
		padded:  true,
	}

	pix := make([]byte, 4*4*4)
	img.WritePixelsRegion(pix, 2, 3, 4, 4)
	require.Equal(t, 3, tt.lastX, "should offset x by 1")
	require.Equal(t, 4, tt.lastY, "should offset y by 1")
}

// --- Anti-aliased DrawTriangles (bigOffscreenBuffer port) ---

// aaTriangle returns a simple 3-vertex triangle covering a bbox of
// (minX, minY) to (maxX, maxY) in white, for use in AA tests.
func aaTriangle(minX, minY, maxX, maxY float32) (vertices []Vertex, indices []uint16) {
	vertices = []Vertex{
		{DstX: minX, DstY: minY, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: maxX, DstY: minY, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: maxX, DstY: maxY, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	indices = []uint16{0, 1, 2}
	return vertices, indices
}

func TestRequiredAARegion16PxGranularity(t *testing.T) {
	tests := []struct {
		name               string
		verts              []Vertex
		imgW, imgH         int
		wantMinX, wantMinY int
		wantMaxX, wantMaxY int
	}{
		{
			name: "small triangle rounds up to 16px",
			verts: []Vertex{
				{DstX: 10, DstY: 10},
				{DstX: 20, DstY: 10},
				{DstX: 15, DstY: 20},
			},
			imgW: 128, imgH: 128,
			// minX=10, padded=-1 → 9, rounded down to 0
			// minY=10, padded=-1 → 9, rounded down to 0
			// maxX=20, padded=+1 → 21, rounded up to 32
			// maxY=20, padded=+1 → 21, rounded up to 32
			wantMinX: 0, wantMinY: 0, wantMaxX: 32, wantMaxY: 32,
		},
		{
			name: "clamped to image bounds",
			verts: []Vertex{
				{DstX: 100, DstY: 100},
				{DstX: 200, DstY: 100},
				{DstX: 150, DstY: 200},
			},
			imgW: 128, imgH: 128,
			wantMinX: 96, wantMinY: 96, wantMaxX: 128, wantMaxY: 128,
		},
		{
			name: "tiny triangle still gets padded region",
			verts: []Vertex{
				{DstX: 50, DstY: 50},
				{DstX: 51, DstY: 50},
				{DstX: 50, DstY: 51},
			},
			imgW: 128, imgH: 128,
			// minX=50, padded=-1 → 49, rounded down to 48
			// maxX=51, padded=+1 → 52, rounded up to 64
			wantMinX: 48, wantMinY: 48, wantMaxX: 64, wantMaxY: 64,
		},
		{
			name: "min and max update as loop iterates",
			// First vertex seeds min/max. Subsequent vertices push the
			// bounds out in both directions, exercising each of the
			// four update branches in requiredAARegion's loop.
			verts: []Vertex{
				{DstX: 50, DstY: 50},
				{DstX: 70, DstY: 70},
				{DstX: 30, DstY: 30},
				{DstX: 20, DstY: 80},
			},
			imgW: 128, imgH: 128,
			// minX = 20, minY = 30, maxX = 70, maxY = 80
			// padded → down16 → (16,16); up16 → (80,96)
			wantMinX: 16, wantMinY: 16, wantMaxX: 80, wantMaxY: 96,
		},
		{
			name: "negative coordinates clamp to zero on Min",
			verts: []Vertex{
				{DstX: -20, DstY: -10},
				{DstX: 40, DstY: 40},
				{DstX: 40, DstY: -10},
			},
			imgW: 128, imgH: 128,
			wantMinX: 0, wantMinY: 0, wantMaxX: 48, wantMaxY: 48,
		},
		{
			name: "all-off-screen verts collapse to empty rect",
			// All vertices below the image → post-clamp Min >= Max → zero.
			verts: []Vertex{
				{DstX: -100, DstY: -100},
				{DstX: -50, DstY: -100},
				{DstX: -100, DstY: -50},
			},
			imgW: 128, imgH: 128,
			wantMinX: 0, wantMinY: 0, wantMaxX: 0, wantMaxY: 0,
		},
		{
			name:  "empty vertices returns zero rect",
			verts: nil,
			imgW:  64, imgH: 64,
			wantMinX: 0, wantMinY: 0, wantMaxX: 0, wantMaxY: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := requiredAARegion(tt.verts, tt.imgW, tt.imgH)
			require.Equal(t, tt.wantMinX, r.Min.X, "Min.X")
			require.Equal(t, tt.wantMinY, r.Min.Y, "Min.Y")
			require.Equal(t, tt.wantMaxX, r.Max.X, "Max.X")
			require.Equal(t, tt.wantMaxY, r.Max.Y, "Max.Y")
		})
	}
}

func TestDrawTrianglesAAEmptyVerticesNoop(t *testing.T) {
	withMockRenderer(t)
	img := NewImage(128, 128)

	// Empty vertices → return before allocating a buffer.
	img.DrawTriangles(nil, nil, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.Nil(t, img.aaBuffer, "empty vertices must not allocate a buffer")

	// Empty indices with non-empty vertices → also no-op.
	verts := []Vertex{
		{DstX: 0, DstY: 0},
		{DstX: 10, DstY: 0},
		{DstX: 10, DstY: 10},
	}
	img.DrawTriangles(verts, nil, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.Nil(t, img.aaBuffer, "empty indices must not allocate a buffer")
}

func TestDrawTrianglesAAAllocatesForOffScreenVertices(t *testing.T) {
	withMockRenderer(t)
	img := NewImage(128, 128)

	// With full-image AA region, even off-screen vertices allocate the
	// buffer (the region is always the full image). The vertices produce
	// invisible draws into the 2x buffer — not ideal for performance,
	// but correct, and the persistent buffer is reused across frames.
	verts := []Vertex{
		{DstX: -100, DstY: -100},
		{DstX: -50, DstY: -100},
		{DstX: -100, DstY: -50},
	}
	idx := []uint16{0, 1, 2}
	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.NotNil(t, img.aaBuffer, "buffer should be allocated even for off-screen vertices")
}

func TestDrawTrianglesAASubImageNilOpts(t *testing.T) {
	withMockRenderer(t)

	parent := NewImage(128, 128)
	sub := parent.SubImage(goimage.Rect(16, 16, 80, 80))

	// Exercise drawTrianglesAA directly with nil opts so the `if opts != nil`
	// clone guard in the sub-image fallback path is covered. (Public
	// DrawTriangles with nil opts takes the non-AA branch.)
	verts, idx := aaTriangle(20, 20, 40, 40)
	sub.drawTrianglesAA(verts, idx, nil, nil)
	require.Nil(t, sub.aaBuffer, "nil-opts sub-image AA must fall back aliased")
}

func TestDrawTrianglesAANilOptsAllocation(t *testing.T) {
	withMockRenderer(t)
	img := NewImage(128, 128)

	// Same nil-opts guard on the allocation path (non-sub-image). Public
	// DrawTriangles with nil opts takes the non-AA branch, so call the
	// internal method directly.
	verts, idx := aaTriangle(10, 10, 40, 40)
	img.drawTrianglesAA(verts, idx, nil, nil)
	require.NotNil(t, img.aaBuffer, "nil-opts AA call must still allocate buffer")
	require.True(t, img.aaBufferDirty)
}

func TestFlushAABufferCleanNoop(t *testing.T) {
	withMockRenderer(t)
	img := NewImage(128, 128)

	// flushAABuffer is a no-op when aaBuffer is nil.
	img.flushAABuffer()

	// And a no-op when the buffer exists but is not dirty.
	verts, idx := aaTriangle(10, 10, 40, 40)
	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})
	img.flushAABuffer() // first call flushes
	require.False(t, img.aaBufferDirty)
	img.flushAABuffer() // second call is a no-op
	require.False(t, img.aaBufferDirty)
}

func TestFlushAABufferIfNeededNil(t *testing.T) {
	// nil-safe: should not panic.
	var img *Image
	img.flushAABufferIfNeeded()
}

func TestRequiredAARegionRoundingHelpers(t *testing.T) {
	require.Equal(t, 0, roundDown16(0))
	require.Equal(t, 0, roundDown16(15))
	require.Equal(t, 16, roundDown16(16))
	require.Equal(t, 16, roundDown16(17))
	require.Equal(t, 48, roundDown16(63))

	require.Equal(t, 0, roundUp16(0))
	require.Equal(t, 16, roundUp16(1))
	require.Equal(t, 16, roundUp16(16))
	require.Equal(t, 32, roundUp16(17))
	require.Equal(t, 64, roundUp16(63))
}

func TestDrawTrianglesAAAllocatesBuffer(t *testing.T) {
	withMockRenderer(t)

	img := NewImage(128, 128)
	require.NotNil(t, img.texture, "img should have a texture")
	require.Nil(t, img.aaBuffer, "aaBuffer should start nil")

	verts, idx := aaTriangle(10, 10, 50, 50)
	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})

	require.NotNil(t, img.aaBuffer, "AA draw must lazily allocate aaBuffer")
	require.True(t, img.aaBufferDirty, "aaBuffer must be marked dirty")
	require.False(t, img.aaBufferRegion.Empty(), "aaBufferRegion must be non-empty")

	// The region covers the full parent image (not per-vertex bbox) so
	// that all AA draws to the same image share a single persistent buffer.
	require.Equal(t, 0, img.aaBufferRegion.Min.X)
	require.Equal(t, 0, img.aaBufferRegion.Min.Y)
	require.Equal(t, 128, img.aaBufferRegion.Max.X)
	require.Equal(t, 128, img.aaBufferRegion.Max.Y)

	// The aaBuffer is 2x the full image dimensions.
	bw, bh := img.aaBuffer.Size()
	require.Equal(t, 256, bw, "aaBuffer should be 2x image width")
	require.Equal(t, 256, bh, "aaBuffer should be 2x image height")
}

func TestDrawTrianglesAASubImageFallsBack(t *testing.T) {
	withMockRenderer(t)

	parent := NewImage(128, 128)
	sub := parent.SubImage(goimage.Rect(16, 16, 80, 80))
	require.NotNil(t, sub, "sub-image allocated")
	require.Nil(t, sub.aaBuffer, "sub-image should have no AA buffer initially")

	verts, idx := aaTriangle(20, 20, 40, 40)
	// Should not panic. Should not allocate a buffer on the sub-image.
	sub.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})

	require.Nil(t, sub.aaBuffer, "sub-image must NOT own an aaBuffer (fallback to aliased)")
	require.Nil(t, parent.aaBuffer, "parent should also not have one (fallback forwarded to sub)")
}

func TestAABufferFlushesOnNonAADraw(t *testing.T) {
	withMockRenderer(t)

	img := NewImage(128, 128)
	verts, idx := aaTriangle(10, 10, 50, 50)
	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.True(t, img.aaBufferDirty, "precondition: dirty after AA draw")

	// A non-AA draw on the same image must flush the dirty buffer first.
	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: false})
	require.False(t, img.aaBufferDirty, "non-AA draw should flush the AA buffer")
	// Buffer itself is reused, not disposed.
	require.NotNil(t, img.aaBuffer, "aaBuffer must be reused after flush, not disposed")
}

func TestAABufferFlushesOnDrawImageOfSelf(t *testing.T) {
	withMockRenderer(t)

	src := NewImage(128, 128)
	dst := NewImage(256, 256)
	verts, idx := aaTriangle(10, 10, 50, 50)
	src.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.True(t, src.aaBufferDirty)

	// Sampling src as a source of DrawImage must flush src's AA buffer
	// before the sample so the content is up to date.
	dst.DrawImage(src, nil)
	require.False(t, src.aaBufferDirty, "src's AA buffer should flush when sampled")
}

func TestAABufferFlushesOnFill(t *testing.T) {
	withMockRenderer(t)

	img := NewImage(128, 128)
	verts, idx := aaTriangle(10, 10, 50, 50)
	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.True(t, img.aaBufferDirty)

	img.Fill(color.RGBA{R: 255, A: 255})
	require.False(t, img.aaBufferDirty, "Fill should flush the AA buffer first")
}

func TestAABufferFlushesOnClear(t *testing.T) {
	withMockRenderer(t)

	img := NewImage(128, 128)
	verts, idx := aaTriangle(10, 10, 50, 50)
	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.True(t, img.aaBufferDirty)

	img.Clear()
	require.False(t, img.aaBufferDirty, "Clear should flush the AA buffer first")
}

func TestAABufferFlushesOnBlendChange(t *testing.T) {
	withMockRenderer(t)

	img := NewImage(128, 128)
	verts, idx := aaTriangle(10, 10, 50, 50)

	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{
		AntiAlias: true,
		Blend:     BlendSourceOver,
	})
	firstBuffer := img.aaBuffer
	require.NotNil(t, firstBuffer)
	require.True(t, img.aaBufferDirty)

	// Second AA draw with a DIFFERENT blend forces a flush between them.
	// The buffer should still exist (same region) but no longer be dirty
	// from the first draw, and then be re-dirtied by the second.
	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{
		AntiAlias: true,
		Blend:     BlendLighter,
	})
	require.True(t, img.aaBufferDirty, "second draw re-dirties the buffer")
	require.Equal(t, BlendLighter, img.aaBufferBlend, "buffer now holds the new blend")
}

func TestAABufferReusedAcrossDraws(t *testing.T) {
	withMockRenderer(t)

	img := NewImage(128, 128)
	verts1, idx1 := aaTriangle(10, 10, 50, 50)
	verts2, idx2 := aaTriangle(60, 60, 80, 80)

	// First AA draw allocates the buffer.
	img.DrawTriangles(verts1, idx1, nil, &DrawTrianglesOptions{AntiAlias: true})
	firstBuf := img.aaBuffer
	require.NotNil(t, firstBuf)
	// Buffer region is the full image, not per-vertex bbox.
	require.Equal(t, goimage.Rect(0, 0, 128, 128), img.aaBufferRegion)

	// Second AA draw with DIFFERENT vertices → same buffer (full-image region).
	img.DrawTriangles(verts2, idx2, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.Same(t, firstBuf, img.aaBuffer, "buffer must be reused across all AA draws")
}

func TestAABufferClearedOnCreation(t *testing.T) {
	withMockRenderer(t)
	rend := getRenderer()

	img := NewImage(128, 128)
	verts, idx := aaTriangle(10, 10, 50, 50)

	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.NotNil(t, img.aaBuffer, "AA buffer must be allocated")

	// The newly created AA buffer must have a pending clear registered
	// so its first render pass uses LoadActionClear instead of loading
	// undefined content.
	require.True(t, rend.pendingClears.Has(img.aaBuffer.textureID),
		"newly allocated AA buffer must have a pending clear")
}

func TestAABufferClearedAfterFlush(t *testing.T) {
	withMockRenderer(t)
	rend := getRenderer()

	img := NewImage(128, 128)
	verts, idx := aaTriangle(10, 10, 50, 50)

	// First AA draw → allocates buffer, marks dirty.
	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.True(t, img.aaBufferDirty)
	bufID := img.aaBuffer.textureID

	// Consume the creation clear so we can observe the post-flush clear.
	_ = rend.pendingClears.Consume(bufID)

	// Trigger a flush via a public sync point (DrawImage reads src's
	// AA buffer, which is our img).
	dst := NewImage(128, 128)
	dst.DrawImage(img, nil)

	require.False(t, img.aaBufferDirty, "flush must clear dirty flag")
	require.True(t, rend.pendingClears.Has(bufID),
		"flushed AA buffer must have a pending clear so the next AA draw starts clean")
}

func TestAABufferNeedsClearOnParentClear(t *testing.T) {
	withMockRenderer(t)
	rend := getRenderer()

	img := NewImage(128, 128)
	verts, idx := aaTriangle(10, 10, 50, 50)

	// Draw AA content and flush it.
	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.NotNil(t, img.aaBuffer)
	bufID := img.aaBuffer.textureID

	// Simulate end-of-frame: flush the buffer.
	dst := NewImage(128, 128)
	dst.DrawImage(img, nil)

	// Consume any pending clears.
	_ = rend.pendingClears.Consume(bufID)
	_ = rend.pendingClears.Consume(img.textureID)

	// Clear the parent. The AA buffer should be flagged for clearing.
	img.Clear()
	require.False(t, img.aaBufferDirty, "Clear must reset dirty flag")
	require.True(t, img.aaBufferNeedsClear, "Clear must set aaBufferNeedsClear")

	// Next AA draw should register pendingClear for the AA buffer.
	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.True(t, rend.pendingClears.Has(bufID),
		"aaBufferNeedsClear must register a pending clear on next AA draw")
	require.False(t, img.aaBufferNeedsClear, "flag must be consumed")
}

func TestAABufferDisposedOnImageDispose(t *testing.T) {
	withMockRenderer(t)
	rend := getRenderer()

	img := NewImage(128, 128)
	verts, idx := aaTriangle(10, 10, 50, 50)
	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.NotNil(t, img.aaBuffer)

	img.Dispose()
	// Image.Dispose() marks the image as disposed immediately but
	// defers the GPU-resource teardown to the end-of-frame drain
	// (renderer.disposeDeferred) so in-flight draw commands don't
	// reference destroyed textures. The image is unusable
	// (disposed=true) but aaBuffer stays live until the deferred
	// drain runs.
	require.True(t, img.disposed)
	require.NotNil(t, img.aaBuffer, "AA buffer must still be alive until the deferred drain runs")

	rend.disposeDeferred()
	require.Nil(t, img.aaBuffer, "disposeDeferred must release the AA buffer")
}

func TestClearDisposedIsNoop(t *testing.T) {
	withMockRenderer(t)
	rend := getRenderer()

	img := NewImage(128, 128)
	img.Dispose()

	// Clear on a disposed image must be a no-op — no pending clear
	// should be registered and no panic should occur.
	img.Clear()
	require.False(t, rend.pendingClears.Has(img.textureID))
}

func TestClearWithDirtyAABufferFlushesFirst(t *testing.T) {
	withMockRenderer(t)

	img := NewImage(128, 128)
	verts, idx := aaTriangle(10, 10, 50, 50)

	img.DrawTriangles(verts, idx, nil, &DrawTrianglesOptions{AntiAlias: true})
	require.True(t, img.aaBufferDirty)

	// Clear should flush the dirty AA buffer, then mark the parent for
	// clearing, then flag the AA buffer for clearing on next use.
	img.Clear()
	require.False(t, img.aaBufferDirty, "flush must clear dirty flag")
	require.True(t, img.aaBufferNeedsClear, "Clear must set aaBufferNeedsClear")
}

func TestReadPixelsPaddedImage(t *testing.T) {
	dev, _ := withMockRenderer(t)

	// Create a padded image (4x4 content → 6x6 padded texture).
	tex, err := dev.NewTexture(backend.TextureDescriptor{
		Width: 6, Height: 6,
		Format: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)

	img := &Image{
		width: 4, height: 4,
		padded:  true,
		texture: tex,
	}

	// ReadPixels on a padded image must read the 6x6 padded texture
	// and extract the 4x4 content region. The mock texture fills all
	// bytes with 0xFF, so all extracted pixels should be 0xFF.
	dst := make([]byte, 4*4*4)
	img.ReadPixels(dst)

	for i, b := range dst {
		require.Equal(t, byte(0xFF), b, "byte %d should be 0xFF after extraction", i)
	}
}
