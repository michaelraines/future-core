package soft

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/backend"
)

// --- Vertex unpacking ---

func makeVertexBytes(verts ...vertex2D) []byte {
	data := make([]byte, len(verts)*32)
	for i, v := range verts {
		off := i * 32
		binary.LittleEndian.PutUint32(data[off:], math.Float32bits(v.px))
		binary.LittleEndian.PutUint32(data[off+4:], math.Float32bits(v.py))
		binary.LittleEndian.PutUint32(data[off+8:], math.Float32bits(v.tu))
		binary.LittleEndian.PutUint32(data[off+12:], math.Float32bits(v.tv))
		binary.LittleEndian.PutUint32(data[off+16:], math.Float32bits(v.r))
		binary.LittleEndian.PutUint32(data[off+20:], math.Float32bits(v.g))
		binary.LittleEndian.PutUint32(data[off+24:], math.Float32bits(v.b))
		binary.LittleEndian.PutUint32(data[off+28:], math.Float32bits(v.a))
	}
	return data
}

func makeIndexBytes(indices ...uint16) []byte {
	data := make([]byte, len(indices)*2)
	for i, idx := range indices {
		binary.LittleEndian.PutUint16(data[i*2:], idx)
	}
	return data
}

func TestUnpackVertices(t *testing.T) {
	v := vertex2D{px: 1, py: 2, tu: 0.5, tv: 0.5, r: 1, g: 0, b: 0, a: 1}
	data := makeVertexBytes(v)
	result := unpackVertices(data)
	require.Len(t, result, 1)
	require.InDelta(t, 1.0, float64(result[0].px), 1e-6)
	require.InDelta(t, 2.0, float64(result[0].py), 1e-6)
	require.InDelta(t, 0.5, float64(result[0].tu), 1e-6)
	require.InDelta(t, 1.0, float64(result[0].r), 1e-6)
}

func TestUnpackIndicesU16(t *testing.T) {
	data := makeIndexBytes(0, 1, 2, 3, 4, 5)
	result := unpackIndicesU16(data)
	require.Equal(t, []uint16{0, 1, 2, 3, 4, 5}, result)
}

// --- Vertex transformation ---

func TestTransformVertexIdentity(t *testing.T) {
	v := vertex2D{px: 0.5, py: -0.5}
	proj := identityMatrix()
	x, y, z, w := transformVertex(v, proj)
	require.InDelta(t, 0.5, float64(x), 1e-6)
	require.InDelta(t, -0.5, float64(y), 1e-6)
	require.InDelta(t, 0.0, float64(z), 1e-6)
	require.InDelta(t, 1.0, float64(w), 1e-6)
}

func TestTransformVertexOrtho(t *testing.T) {
	// Simple 2D ortho: maps [0, 100] x [0, 100] → [-1, 1] x [-1, 1]
	proj := ortho2D(0, 100, 0, 100)
	v := vertex2D{px: 50, py: 50}
	x, y, _, w := transformVertex(v, proj)
	require.InDelta(t, 1.0, float64(w), 1e-6)
	require.InDelta(t, 0.0, float64(x/w), 1e-4)
	require.InDelta(t, 0.0, float64(y/w), 1e-4)
}

// --- NDC to screen ---

func TestNdcToScreen(t *testing.T) {
	vp := viewportRect{x: 0, y: 0, w: 800, h: 600}
	sx, sy := ndcToScreen(0, 0, vp)
	require.InDelta(t, 400, float64(sx), 1e-3)
	require.InDelta(t, 300, float64(sy), 1e-3)

	sx, sy = ndcToScreen(-1, -1, vp)
	require.InDelta(t, 0, float64(sx), 1e-3)
	require.InDelta(t, 0, float64(sy), 1e-3)
}

// --- Edge function ---

func TestEdgeFunc(t *testing.T) {
	// CCW triangle: (0,0), (1,0), (0,1) → positive area
	area := edgeFunc(0, 0, 1, 0, 0, 1)
	require.InDelta(t, 1.0, float64(area), 1e-6)
}

// --- Blend functions ---

func TestBlendNone(t *testing.T) {
	r, g, b, a := blendNone(1, 0, 0, 1, 0, 1, 0, 1)
	require.InDelta(t, 1.0, float64(r), 1e-6)
	require.InDelta(t, 0.0, float64(g), 1e-6)
	require.InDelta(t, 0.0, float64(b), 1e-6)
	require.InDelta(t, 1.0, float64(a), 1e-6)
}

func TestBlendSourceOver(t *testing.T) {
	// 50% red over green
	r, g, b, a := blendSourceOver(1, 0, 0, 0.5, 0, 1, 0, 1)
	require.InDelta(t, 1.0, float64(a), 1e-3)
	require.Greater(t, float64(r), 0.4) // red present
	require.Greater(t, float64(g), 0.3) // green shows through
	require.InDelta(t, 0.0, float64(b), 1e-3)
}

func TestBlendSourceOverZeroAlpha(t *testing.T) {
	r, g, b, a := blendSourceOver(1, 0, 0, 0, 0, 0, 0, 0)
	require.InDelta(t, 0, float64(r), 1e-6)
	require.InDelta(t, 0, float64(g), 1e-6)
	require.InDelta(t, 0, float64(b), 1e-6)
	require.InDelta(t, 0, float64(a), 1e-6)
}

func TestBlendAdditive(t *testing.T) {
	r, g, b, a := blendAdditive(1, 0, 0, 1, 0, 0.5, 0, 0.5)
	require.InDelta(t, 1.0, float64(r), 1e-3)
	require.InDelta(t, 0.5, float64(g), 1e-3)
	require.InDelta(t, 0.0, float64(b), 1e-3)
	require.Greater(t, float64(a), 0.5)
}

func TestBlendMultiplicative(t *testing.T) {
	r, g, b, a := blendMultiplicative(0.5, 1, 0, 1, 1, 1, 1, 1)
	require.InDelta(t, 0.5, float64(r), 1e-3)
	require.InDelta(t, 1.0, float64(g), 1e-3)
	require.InDelta(t, 0.0, float64(b), 1e-3)
	require.InDelta(t, 1.0, float64(a), 1e-3)
}

func TestBlendPremultiplied(t *testing.T) {
	r, g, b, a := blendPremultiplied(0.5, 0, 0, 0.5, 0, 0.5, 0, 1)
	require.Greater(t, float64(r), 0.4)
	require.Greater(t, float64(g), 0.2)
	require.InDelta(t, 0.0, float64(b), 1e-3)
	require.InDelta(t, 1.0, float64(a), 1e-3)
}

// --- Texture sampling ---

func TestSampleNearest(t *testing.T) {
	// 2x2 RGBA texture: red, green, blue, white
	pixels := []byte{
		255, 0, 0, 255, 0, 255, 0, 255,
		0, 0, 255, 255, 255, 255, 255, 255,
	}
	r, g, b, a := sampleNearest(pixels, 2, 2, 4, 0, 0)
	require.InDelta(t, 1.0, float64(r), 1e-2)
	require.InDelta(t, 0.0, float64(g), 1e-2)
	require.InDelta(t, 0.0, float64(b), 1e-2)
	require.InDelta(t, 1.0, float64(a), 1e-2)

	r2, g2, _, _ := sampleNearest(pixels, 2, 2, 4, 1, 0)
	require.InDelta(t, 0.0, float64(r2), 1e-2)
	require.InDelta(t, 1.0, float64(g2), 1e-2)
}

func TestEdgeFuncFMA(t *testing.T) {
	// CCW triangle: (0,0), (1,0), (0,1) → positive area
	area := edgeFuncFMA(0, 0, 1, 0, 0, 1)
	require.InDelta(t, 1.0, area, 1e-12)

	// CW triangle → negative area
	area = edgeFuncFMA(0, 0, 0, 1, 1, 0)
	require.InDelta(t, -1.0, area, 1e-12)

	// Degenerate (collinear) → zero
	area = edgeFuncFMA(0, 0, 1, 1, 2, 2)
	require.InDelta(t, 0.0, area, 1e-12)
}

func TestMin3(t *testing.T) {
	require.InDelta(t, 1.0, min3(1, 2, 3), 1e-12)
	require.InDelta(t, 1.0, min3(3, 1, 2), 1e-12)
	require.InDelta(t, 1.0, min3(2, 3, 1), 1e-12)
}

func TestMax3(t *testing.T) {
	require.InDelta(t, 3.0, max3(1, 2, 3), 1e-12)
	require.InDelta(t, 3.0, max3(3, 1, 2), 1e-12)
	require.InDelta(t, 3.0, max3(2, 3, 1), 1e-12)
}

func TestSampleNearestOutOfBounds(t *testing.T) {
	r, g, b, a := sampleNearest([]byte{}, 0, 0, 4, 0, 0)
	require.InDelta(t, 0, float64(r), 1e-6)
	require.InDelta(t, 0, float64(g), 1e-6)
	require.InDelta(t, 0, float64(b), 1e-6)
	require.InDelta(t, 0, float64(a), 1e-6)
}

func TestSampleLinear(t *testing.T) {
	// 2x2: red/green on top, blue/white on bottom
	pixels := []byte{
		255, 0, 0, 255, 0, 255, 0, 255,
		0, 0, 255, 255, 255, 255, 255, 255,
	}
	// Center (0.5, 0.5) should be average of all 4 pixels
	cr, _, _, ca := sampleLinear(pixels, 2, 2, 4, 0.5, 0.5)
	require.InDelta(t, 1.0, float64(ca), 1e-2) // all alpha=1
	// Average of red channels: (255 + 0 + 0 + 255) / 4 / 255 ≈ 0.5
	require.InDelta(t, 0.5, float64(cr), 0.1)
}

func TestSampleLinearEdge(t *testing.T) {
	pixels := []byte{255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 255, 255, 255, 255}
	// Corner (0,0) should be the top-left pixel
	cr, _, _, _ := sampleLinear(pixels, 2, 2, 4, 0, 0)
	require.InDelta(t, 1.0, float64(cr), 1e-2)
}

// --- Color matrix ---

func TestApplyColorMatrixIdentity(t *testing.T) {
	r, g, b, a := applyColorMatrix(0.5, 0.3, 0.8, 1.0, identityMatrix(), [4]float32{})
	require.InDelta(t, 0.5, float64(r), 1e-6)
	require.InDelta(t, 0.3, float64(g), 1e-6)
	require.InDelta(t, 0.8, float64(b), 1e-6)
	require.InDelta(t, 1.0, float64(a), 1e-6)
}

func TestApplyColorMatrixTranslation(t *testing.T) {
	trans := [4]float32{0.1, 0.2, 0, 0}
	r, g, _, _ := applyColorMatrix(0.5, 0.3, 0.8, 1.0, identityMatrix(), trans)
	require.InDelta(t, 0.6, float64(r), 1e-6)
	require.InDelta(t, 0.5, float64(g), 1e-6)
}

func TestApplyColorMatrixWithClamping(t *testing.T) {
	trans := [4]float32{1.0, 0, 0, 0}
	r, _, _, _ := applyColorMatrix(0.8, 0, 0, 1, identityMatrix(), trans)
	require.InDelta(t, 1.0, float64(r), 1e-6) // clamped to 1.0
}

// --- Helper functions ---

func TestMin3f(t *testing.T) {
	require.InDelta(t, 1.0, float64(min3f(1, 2, 3)), 1e-6)
	require.InDelta(t, 1.0, float64(min3f(3, 1, 2)), 1e-6)
	require.InDelta(t, 1.0, float64(min3f(2, 3, 1)), 1e-6)
}

func TestMax3f(t *testing.T) {
	require.InDelta(t, 3.0, float64(max3f(1, 2, 3)), 1e-6)
	require.InDelta(t, 3.0, float64(max3f(3, 1, 2)), 1e-6)
	require.InDelta(t, 3.0, float64(max3f(2, 3, 1)), 1e-6)
}

func TestClampf(t *testing.T) {
	require.InDelta(t, 0, float64(clampf(-1)), 1e-6)
	require.InDelta(t, 0.5, float64(clampf(0.5)), 1e-6)
	require.InDelta(t, 1.0, float64(clampf(2.0)), 1e-6)
}

func TestFloatToByte(t *testing.T) {
	require.Equal(t, byte(0), floatToByte(-0.5))
	require.Equal(t, byte(0), floatToByte(0))
	require.Equal(t, byte(128), floatToByte(0.5))
	require.Equal(t, byte(255), floatToByte(1.0))
	require.Equal(t, byte(255), floatToByte(2.0))
}

func TestIsIdentityMatrix(t *testing.T) {
	require.True(t, isIdentityMatrix(identityMatrix()))
	m := identityMatrix()
	m[5] = 2
	require.False(t, isIdentityMatrix(m))
}

// --- Integration: rasterize a triangle into a render target ---

func TestRasterizeTriangleIndexed(t *testing.T) {
	d := initDevice(t)

	// Create 8x8 render target.
	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 8, Height: 8, ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)

	// Create a white 1x1 texture.
	tex, err := d.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1, Format: backend.TextureFormatRGBA8,
		Data: []byte{255, 255, 255, 255},
	})
	require.NoError(t, err)

	// Create shader with identity projection.
	shader, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)

	// Create pipeline.
	pipeline, err := d.NewPipeline(backend.PipelineDescriptor{
		Shader:    shader,
		BlendMode: backend.BlendNone,
	})
	require.NoError(t, err)

	// Triangle covering the entire NDC space: (-1,-1), (3,-1), (-1,3)
	// This ensures every pixel gets covered.
	verts := makeVertexBytes(
		vertex2D{px: -1, py: -1, tu: 0, tv: 0, r: 1, g: 0, b: 0, a: 1},
		vertex2D{px: 3, py: -1, tu: 1, tv: 0, r: 1, g: 0, b: 0, a: 1},
		vertex2D{px: -1, py: 3, tu: 0, tv: 1, r: 1, g: 0, b: 0, a: 1},
	)
	indices := makeIndexBytes(0, 1, 2)

	vbuf, err := d.NewBuffer(backend.BufferDescriptor{Data: verts})
	require.NoError(t, err)
	ibuf, err := d.NewBuffer(backend.BufferDescriptor{Data: indices})
	require.NoError(t, err)

	enc := d.Encoder()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		Target:     rt,
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{0, 0, 0, 0},
	})
	enc.SetPipeline(pipeline)
	enc.SetVertexBuffer(vbuf, 0)
	enc.SetIndexBuffer(ibuf, backend.IndexUint16)
	enc.SetTexture(tex, 0)
	enc.DrawIndexed(3, 1, 0)
	enc.EndRenderPass()

	// Read pixels and verify red was written.
	srt := rt.(*RenderTarget)
	pixels := srt.color.Pixels()
	require.NotEmpty(t, pixels)

	// Check center pixel (4,4): should be red.
	idx := (4*8 + 4) * 4
	require.Equal(t, byte(255), pixels[idx], "red channel")
	require.Equal(t, byte(0), pixels[idx+1], "green channel")
	require.Equal(t, byte(0), pixels[idx+2], "blue channel")
	require.Equal(t, byte(255), pixels[idx+3], "alpha channel")
}

func TestRasterizeTriangleNonIndexed(t *testing.T) {
	d := initDevice(t)

	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 8, Height: 8, ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)

	tex, err := d.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1, Format: backend.TextureFormatRGBA8,
		Data: []byte{255, 255, 255, 255},
	})
	require.NoError(t, err)

	shader, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)

	pipeline, err := d.NewPipeline(backend.PipelineDescriptor{
		Shader:    shader,
		BlendMode: backend.BlendNone,
	})
	require.NoError(t, err)

	verts := makeVertexBytes(
		vertex2D{px: -1, py: -1, tu: 0, tv: 0, r: 0, g: 1, b: 0, a: 1},
		vertex2D{px: 3, py: -1, tu: 1, tv: 0, r: 0, g: 1, b: 0, a: 1},
		vertex2D{px: -1, py: 3, tu: 0, tv: 1, r: 0, g: 1, b: 0, a: 1},
	)
	vbuf, err := d.NewBuffer(backend.BufferDescriptor{Data: verts})
	require.NoError(t, err)

	enc := d.Encoder()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		Target:     rt,
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{0, 0, 0, 0},
	})
	enc.SetPipeline(pipeline)
	enc.SetVertexBuffer(vbuf, 0)
	enc.SetTexture(tex, 0)
	enc.Draw(3, 1, 0)
	enc.EndRenderPass()

	srt := rt.(*RenderTarget)
	pixels := srt.color.Pixels()
	idx := (4*8 + 4) * 4
	require.Equal(t, byte(0), pixels[idx], "red channel")
	require.Equal(t, byte(255), pixels[idx+1], "green channel")
}

func TestRasterizeWithOrthoProjection(t *testing.T) {
	d := initDevice(t)

	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 16, Height: 16, ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)

	tex, err := d.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1, Format: backend.TextureFormatRGBA8,
		Data: []byte{255, 255, 255, 255},
	})
	require.NoError(t, err)

	shader, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)
	// Set ortho projection: maps [0,16] x [0,16] → NDC
	shader.SetUniformMat4("uProjection", ortho2D(0, 16, 0, 16))

	pipeline, err := d.NewPipeline(backend.PipelineDescriptor{
		Shader:    shader,
		BlendMode: backend.BlendNone,
	})
	require.NoError(t, err)

	// Triangle covering left half of 16x16 screen.
	verts := makeVertexBytes(
		vertex2D{px: 0, py: 0, r: 0, g: 0, b: 1, a: 1},
		vertex2D{px: 8, py: 0, r: 0, g: 0, b: 1, a: 1},
		vertex2D{px: 0, py: 16, r: 0, g: 0, b: 1, a: 1},
	)
	indices := makeIndexBytes(0, 1, 2)
	vbuf, err := d.NewBuffer(backend.BufferDescriptor{Data: verts})
	require.NoError(t, err)
	ibuf, err := d.NewBuffer(backend.BufferDescriptor{Data: indices})
	require.NoError(t, err)

	enc := d.Encoder()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		Target:     rt,
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{0, 0, 0, 0},
	})
	enc.SetPipeline(pipeline)
	enc.SetVertexBuffer(vbuf, 0)
	enc.SetIndexBuffer(ibuf, backend.IndexUint16)
	enc.SetTexture(tex, 0)
	enc.DrawIndexed(3, 1, 0)
	enc.EndRenderPass()

	srt := rt.(*RenderTarget)
	pixels := srt.color.Pixels()

	// Pixel at (2, 8) should be blue (inside triangle).
	idx := (8*16 + 2) * 4
	require.Equal(t, byte(0), pixels[idx])
	require.Equal(t, byte(0), pixels[idx+1])
	require.Equal(t, byte(255), pixels[idx+2])

	// Pixel at (14, 8) should be black (outside triangle).
	idx2 := (8*16 + 14) * 4
	require.Equal(t, byte(0), pixels[idx2])
	require.Equal(t, byte(0), pixels[idx2+1])
	require.Equal(t, byte(0), pixels[idx2+2])
}

func TestRasterizeWithScissor(t *testing.T) {
	d := initDevice(t)

	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 16, Height: 16, ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)

	tex, err := d.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1, Format: backend.TextureFormatRGBA8,
		Data: []byte{255, 255, 255, 255},
	})
	require.NoError(t, err)

	shader, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)

	pipeline, err := d.NewPipeline(backend.PipelineDescriptor{
		Shader:    shader,
		BlendMode: backend.BlendNone,
	})
	require.NoError(t, err)

	// Full-screen triangle.
	verts := makeVertexBytes(
		vertex2D{px: -1, py: -1, r: 1, g: 1, b: 1, a: 1},
		vertex2D{px: 3, py: -1, r: 1, g: 1, b: 1, a: 1},
		vertex2D{px: -1, py: 3, r: 1, g: 1, b: 1, a: 1},
	)
	indices := makeIndexBytes(0, 1, 2)
	vbuf, err := d.NewBuffer(backend.BufferDescriptor{Data: verts})
	require.NoError(t, err)
	ibuf, err := d.NewBuffer(backend.BufferDescriptor{Data: indices})
	require.NoError(t, err)

	enc := d.Encoder()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		Target:     rt,
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{0, 0, 0, 0},
	})
	enc.SetPipeline(pipeline)
	enc.SetVertexBuffer(vbuf, 0)
	enc.SetIndexBuffer(ibuf, backend.IndexUint16)
	enc.SetTexture(tex, 0)
	// Scissor to top-left 4x4 region.
	enc.SetScissor(&backend.ScissorRect{X: 0, Y: 0, Width: 4, Height: 4})
	enc.DrawIndexed(3, 1, 0)
	enc.EndRenderPass()

	srt := rt.(*RenderTarget)
	pixels := srt.color.Pixels()

	// Pixel at (2,2) should be white (inside scissor).
	idx := (2*16 + 2) * 4
	require.Equal(t, byte(255), pixels[idx])

	// Pixel at (10,10) should be black (outside scissor).
	idx2 := (10*16 + 10) * 4
	require.Equal(t, byte(0), pixels[idx2])
}

func TestRasterizeColorWriteDisabled(t *testing.T) {
	d := initDevice(t)

	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 8, Height: 8, ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)

	tex, err := d.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1, Format: backend.TextureFormatRGBA8,
		Data: []byte{255, 255, 255, 255},
	})
	require.NoError(t, err)

	shader, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)

	pipeline, err := d.NewPipeline(backend.PipelineDescriptor{
		Shader:    shader,
		BlendMode: backend.BlendNone,
	})
	require.NoError(t, err)

	verts := makeVertexBytes(
		vertex2D{px: -1, py: -1, r: 1, g: 0, b: 0, a: 1},
		vertex2D{px: 3, py: -1, r: 1, g: 0, b: 0, a: 1},
		vertex2D{px: -1, py: 3, r: 1, g: 0, b: 0, a: 1},
	)
	indices := makeIndexBytes(0, 1, 2)
	vbuf, err := d.NewBuffer(backend.BufferDescriptor{Data: verts})
	require.NoError(t, err)
	ibuf, err := d.NewBuffer(backend.BufferDescriptor{Data: indices})
	require.NoError(t, err)

	enc := d.Encoder()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		Target:     rt,
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{0, 0, 0, 0},
	})
	enc.SetPipeline(pipeline)
	enc.SetVertexBuffer(vbuf, 0)
	enc.SetIndexBuffer(ibuf, backend.IndexUint16)
	enc.SetTexture(tex, 0)
	enc.SetColorWrite(false)
	enc.DrawIndexed(3, 1, 0)
	enc.EndRenderPass()

	srt := rt.(*RenderTarget)
	pixels := srt.color.Pixels()
	// Everything should remain black since color write was disabled.
	for i := 0; i+3 < len(pixels); i += 4 {
		require.Equal(t, byte(0), pixels[i], "pixel %d should be 0", i)
	}
}

func TestRasterizeNoRenderTarget(t *testing.T) {
	d := initDevice(t)
	enc := d.Encoder().(*Encoder)

	// BeginRenderPass with nil target — draw should not panic.
	enc.BeginRenderPass(backend.RenderPassDescriptor{Target: nil})

	shader, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)
	pipeline, err := d.NewPipeline(backend.PipelineDescriptor{Shader: shader})
	require.NoError(t, err)

	verts := makeVertexBytes(
		vertex2D{px: -1, py: -1, r: 1, g: 0, b: 0, a: 1},
		vertex2D{px: 1, py: -1, r: 1, g: 0, b: 0, a: 1},
		vertex2D{px: 0, py: 1, r: 1, g: 0, b: 0, a: 1},
	)
	indices := makeIndexBytes(0, 1, 2)
	vbuf, err := d.NewBuffer(backend.BufferDescriptor{Data: verts})
	require.NoError(t, err)
	ibuf, err := d.NewBuffer(backend.BufferDescriptor{Data: indices})
	require.NoError(t, err)

	enc.SetPipeline(pipeline)
	enc.SetVertexBuffer(vbuf, 0)
	enc.SetIndexBuffer(ibuf, backend.IndexUint16)
	enc.DrawIndexed(3, 1, 0) // should no-op without panic
	enc.EndRenderPass()
}

func TestRasterizeNoTexture(t *testing.T) {
	d := initDevice(t)

	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 8, Height: 8, ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)

	shader, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)
	pipeline, err := d.NewPipeline(backend.PipelineDescriptor{
		Shader:    shader,
		BlendMode: backend.BlendNone,
	})
	require.NoError(t, err)

	// Full-screen triangle with vertex color, no texture bound.
	verts := makeVertexBytes(
		vertex2D{px: -1, py: -1, r: 0, g: 1, b: 0, a: 1},
		vertex2D{px: 3, py: -1, r: 0, g: 1, b: 0, a: 1},
		vertex2D{px: -1, py: 3, r: 0, g: 1, b: 0, a: 1},
	)
	indices := makeIndexBytes(0, 1, 2)
	vbuf, err := d.NewBuffer(backend.BufferDescriptor{Data: verts})
	require.NoError(t, err)
	ibuf, err := d.NewBuffer(backend.BufferDescriptor{Data: indices})
	require.NoError(t, err)

	enc := d.Encoder()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		Target:     rt,
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{0, 0, 0, 0},
	})
	enc.SetPipeline(pipeline)
	enc.SetVertexBuffer(vbuf, 0)
	enc.SetIndexBuffer(ibuf, backend.IndexUint16)
	// No SetTexture call — should use white fallback.
	enc.DrawIndexed(3, 1, 0)
	enc.EndRenderPass()

	srt := rt.(*RenderTarget)
	pixels := srt.color.Pixels()
	idx := (4*8 + 4) * 4
	// Should be green (vertex color × white texture).
	require.Equal(t, byte(0), pixels[idx])
	require.Equal(t, byte(255), pixels[idx+1])
	require.Equal(t, byte(0), pixels[idx+2])
}

func TestRasterizeBlendSourceOver(t *testing.T) {
	d := initDevice(t)

	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 8, Height: 8, ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)

	tex, err := d.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1, Format: backend.TextureFormatRGBA8,
		Data: []byte{255, 255, 255, 255},
	})
	require.NoError(t, err)

	shader, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)

	pipeline, err := d.NewPipeline(backend.PipelineDescriptor{
		Shader:    shader,
		BlendMode: backend.BlendSourceOver,
	})
	require.NoError(t, err)

	// Draw red background first.
	bgVerts := makeVertexBytes(
		vertex2D{px: -1, py: -1, r: 1, g: 0, b: 0, a: 1},
		vertex2D{px: 3, py: -1, r: 1, g: 0, b: 0, a: 1},
		vertex2D{px: -1, py: 3, r: 1, g: 0, b: 0, a: 1},
	)
	bgIdx := makeIndexBytes(0, 1, 2)
	bgVbuf, err := d.NewBuffer(backend.BufferDescriptor{Data: bgVerts})
	require.NoError(t, err)
	bgIbuf, err := d.NewBuffer(backend.BufferDescriptor{Data: bgIdx})
	require.NoError(t, err)

	enc := d.Encoder()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		Target:     rt,
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{0, 0, 0, 0},
	})
	enc.SetPipeline(pipeline)
	enc.SetVertexBuffer(bgVbuf, 0)
	enc.SetIndexBuffer(bgIbuf, backend.IndexUint16)
	enc.SetTexture(tex, 0)
	enc.DrawIndexed(3, 1, 0) // red background

	// Draw semi-transparent green on top.
	fgVerts := makeVertexBytes(
		vertex2D{px: -1, py: -1, r: 0, g: 1, b: 0, a: 0.5},
		vertex2D{px: 3, py: -1, r: 0, g: 1, b: 0, a: 0.5},
		vertex2D{px: -1, py: 3, r: 0, g: 1, b: 0, a: 0.5},
	)
	fgIdx := makeIndexBytes(0, 1, 2)
	fgVbuf, err := d.NewBuffer(backend.BufferDescriptor{Data: fgVerts})
	require.NoError(t, err)
	fgIbuf, err := d.NewBuffer(backend.BufferDescriptor{Data: fgIdx})
	require.NoError(t, err)

	enc.SetVertexBuffer(fgVbuf, 0)
	enc.SetIndexBuffer(fgIbuf, backend.IndexUint16)
	enc.DrawIndexed(3, 1, 0)
	enc.EndRenderPass()

	srt := rt.(*RenderTarget)
	pixels := srt.color.Pixels()
	idx := (4*8 + 4) * 4

	// Should be a blend of red and green.
	r := pixels[idx]
	g := pixels[idx+1]
	require.Greater(t, r, byte(100), "red should show through")
	require.Greater(t, g, byte(100), "green should be present")
}

// --- uint32 index support ---

func makeIndexBytesU32(indices ...uint32) []byte {
	data := make([]byte, len(indices)*4)
	for i, idx := range indices {
		binary.LittleEndian.PutUint32(data[i*4:], idx)
	}
	return data
}

func TestUnpackIndicesU32(t *testing.T) {
	data := makeIndexBytesU32(0, 1, 2, 100, 200, 300)
	result := unpackIndicesU32(data)
	require.Equal(t, []uint32{0, 1, 2, 100, 200, 300}, result)
}

func TestRasterizeTriangleIndexedUint32(t *testing.T) {
	d := initDevice(t)

	// Create 8x8 render target.
	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 8, Height: 8, ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)

	// Create a white 1x1 texture.
	tex, err := d.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1, Format: backend.TextureFormatRGBA8,
		Data: []byte{255, 255, 255, 255},
	})
	require.NoError(t, err)

	// Create shader with identity projection.
	shader, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)

	// Create pipeline.
	pipeline, err := d.NewPipeline(backend.PipelineDescriptor{
		Shader:    shader,
		BlendMode: backend.BlendNone,
	})
	require.NoError(t, err)

	// Full-screen triangle.
	verts := makeVertexBytes(
		vertex2D{px: -1, py: -1, tu: 0, tv: 0, r: 0, g: 0, b: 1, a: 1},
		vertex2D{px: 3, py: -1, tu: 1, tv: 0, r: 0, g: 0, b: 1, a: 1},
		vertex2D{px: -1, py: 3, tu: 0, tv: 1, r: 0, g: 0, b: 1, a: 1},
	)
	// Use uint32 indices.
	indices := makeIndexBytesU32(0, 1, 2)

	vbuf, err := d.NewBuffer(backend.BufferDescriptor{Data: verts})
	require.NoError(t, err)
	ibuf, err := d.NewBuffer(backend.BufferDescriptor{Data: indices})
	require.NoError(t, err)

	enc := d.Encoder()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		Target:     rt,
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{0, 0, 0, 0},
	})
	enc.SetPipeline(pipeline)
	enc.SetVertexBuffer(vbuf, 0)
	enc.SetIndexBuffer(ibuf, backend.IndexUint32)
	enc.SetTexture(tex, 0)
	enc.DrawIndexed(3, 1, 0)
	enc.EndRenderPass()

	// Read pixels and verify blue was written.
	srt := rt.(*RenderTarget)
	pixels := srt.color.Pixels()
	require.NotEmpty(t, pixels)

	// Check center pixel (4,4): should be blue.
	idx := (4*8 + 4) * 4
	require.Equal(t, byte(0), pixels[idx], "red channel")
	require.Equal(t, byte(0), pixels[idx+1], "green channel")
	require.Equal(t, byte(255), pixels[idx+2], "blue channel")
	require.Equal(t, byte(255), pixels[idx+3], "alpha channel")
}

// --- Encoder state tests ---

func TestEncoderResolveBlendFunc(t *testing.T) {
	d := initDevice(t)
	enc := d.Encoder().(*Encoder)

	// No pipeline → source over.
	require.NotNil(t, enc.resolveBlendFunc())

	modes := []backend.BlendMode{
		backend.BlendNone,
		backend.BlendSourceOver,
		backend.BlendAdditive,
		backend.BlendMultiplicative,
		backend.BlendPremultiplied,
	}
	for _, mode := range modes {
		p, err := d.NewPipeline(backend.PipelineDescriptor{BlendMode: mode})
		require.NoError(t, err)
		enc.SetPipeline(p)
		require.NotNil(t, enc.resolveBlendFunc())
	}
}

func TestEncoderProjectionMatrix(t *testing.T) {
	d := initDevice(t)
	enc := d.Encoder().(*Encoder)

	// No shader → identity.
	proj := enc.projectionMatrix()
	require.Equal(t, identityMatrix(), proj)

	// Shader without uProjection → identity.
	s, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)
	p, err := d.NewPipeline(backend.PipelineDescriptor{Shader: s})
	require.NoError(t, err)
	enc.SetPipeline(p)
	proj = enc.projectionMatrix()
	require.Equal(t, identityMatrix(), proj)

	// Shader with uProjection.
	custom := [16]float32{2, 0, 0, 0, 0, 2, 0, 0, 0, 0, 1, 0, -1, -1, 0, 1}
	s.SetUniformMat4("uProjection", custom)
	proj = enc.projectionMatrix()
	require.Equal(t, custom, proj)
}

func TestEncoderTextureSampler(t *testing.T) {
	d := initDevice(t)
	enc := d.Encoder().(*Encoder)

	// No texture → white.
	sampler := enc.textureSampler()
	r, g, b, a := sampler(0.5, 0.5)
	require.InDelta(t, 1, float64(r), 1e-6)
	require.InDelta(t, 1, float64(g), 1e-6)
	require.InDelta(t, 1, float64(b), 1e-6)
	require.InDelta(t, 1, float64(a), 1e-6)
}

// --- Helpers ---

// ortho2D creates a simple 2D orthographic projection matrix (column-major).
func ortho2D(left, right, bottom, top float32) [16]float32 {
	w := right - left
	h := top - bottom
	return [16]float32{
		2 / w, 0, 0, 0,
		0, 2 / h, 0, 0,
		0, 0, -1, 0,
		-(right + left) / w, -(top + bottom) / h, 0, 1,
	}
}
