package soft

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
)

func TestStencilCompareAllFuncs(t *testing.T) {
	tests := []struct {
		name     string
		fn       backend.CompareFunc
		ref, buf uint8
		wantPass bool
	}{
		{"never-equal", backend.CompareNever, 5, 5, false},
		{"less-true", backend.CompareLess, 3, 5, true},
		{"less-false", backend.CompareLess, 5, 5, false},
		{"less-equal-true", backend.CompareLessEqual, 5, 5, true},
		{"less-equal-false", backend.CompareLessEqual, 6, 5, false},
		{"equal-true", backend.CompareEqual, 5, 5, true},
		{"equal-false", backend.CompareEqual, 5, 4, false},
		{"greater-equal-true", backend.CompareGreaterEqual, 5, 5, true},
		{"greater-equal-false", backend.CompareGreaterEqual, 4, 5, false},
		{"greater-true", backend.CompareGreater, 5, 4, true},
		{"greater-false", backend.CompareGreater, 5, 5, false},
		{"not-equal-true", backend.CompareNotEqual, 5, 4, true},
		{"not-equal-false", backend.CompareNotEqual, 5, 5, false},
		{"always", backend.CompareAlways, 0, 0xFF, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stencilCompare(tt.fn, tt.ref, tt.buf)
			require.Equal(t, tt.wantPass, got)
		})
	}
}

func TestStencilApplyOpAllOps(t *testing.T) {
	tests := []struct {
		name    string
		op      backend.StencilOp
		ref     uint8
		current uint8
		want    uint8
	}{
		{"keep", backend.StencilKeep, 0xAA, 0x42, 0x42},
		{"zero", backend.StencilZero, 0xAA, 0x42, 0x00},
		{"replace", backend.StencilReplace, 0xAA, 0x42, 0xAA},
		{"incr-clamp-below-max", backend.StencilIncr, 0, 0x10, 0x11},
		{"incr-clamp-at-max", backend.StencilIncr, 0, 0xFF, 0xFF},
		{"decr-clamp-above-zero", backend.StencilDecr, 0, 0x10, 0x0F},
		{"decr-clamp-at-zero", backend.StencilDecr, 0, 0, 0},
		{"invert", backend.StencilInvert, 0, 0x0F, 0xF0},
		{"incr-wrap-rollover", backend.StencilIncrWrap, 0, 0xFF, 0x00},
		{"incr-wrap-below-max", backend.StencilIncrWrap, 0, 0x42, 0x43},
		{"decr-wrap-underflow", backend.StencilDecrWrap, 0, 0x00, 0xFF},
		{"decr-wrap-above-zero", backend.StencilDecrWrap, 0, 0x42, 0x41},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stencilApplyOp(tt.op, tt.ref, tt.current)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRenderTargetHasStencil(t *testing.T) {
	d := New()
	require.NoError(t, d.Init(backend.DeviceConfig{Width: 32, Height: 32}))
	defer d.Dispose()

	withStencil, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 16, Height: 16, ColorFormat: backend.TextureFormatRGBA8,
		HasStencil: true,
	})
	require.NoError(t, err)
	defer withStencil.Dispose()
	require.True(t, withStencil.HasStencil())

	withoutStencil, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 16, Height: 16, ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)
	defer withoutStencil.Dispose()
	require.False(t, withoutStencil.HasStencil())
}

// TestRasterizerStencilTestAndOp verifies the stencil test + op path in
// the rasterizer by running a write pipeline (StencilInvert on DPPass)
// then a color pipeline (NotEqual ref=0, StencilZero on DPPass) and
// checking that pixels outside any triangle stay black while pixels
// inside exactly one triangle get the yellow fill color.
func TestRasterizerStencilTestAndOp(t *testing.T) {
	d := New()
	require.NoError(t, d.Init(backend.DeviceConfig{Width: 32, Height: 32}))
	defer d.Dispose()

	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 32, Height: 32, ColorFormat: backend.TextureFormatRGBA8,
		HasStencil: true,
	})
	require.NoError(t, err)
	defer rt.Dispose()

	shader, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)
	defer shader.Dispose()

	writePipe, err := d.NewPipeline(backend.PipelineDescriptor{
		Shader:        shader,
		Primitive:     backend.PrimitiveTriangles,
		StencilEnable: true,
		Stencil: backend.StencilDescriptor{
			Func:      backend.CompareAlways,
			Mask:      0xFF,
			WriteMask: 0xFF,
			Front: backend.StencilFaceOps{
				SFail:  backend.StencilKeep,
				DPFail: backend.StencilKeep,
				DPPass: backend.StencilInvert,
			},
		},
		ColorWriteDisabled: true,
	})
	require.NoError(t, err)
	defer writePipe.Dispose()

	colorPipe, err := d.NewPipeline(backend.PipelineDescriptor{
		Shader:        shader,
		Primitive:     backend.PrimitiveTriangles,
		StencilEnable: true,
		Stencil: backend.StencilDescriptor{
			Func:      backend.CompareNotEqual,
			Mask:      0xFF,
			WriteMask: 0xFF,
			Front: backend.StencilFaceOps{
				SFail:  backend.StencilKeep,
				DPFail: backend.StencilKeep,
				DPPass: backend.StencilZero,
			},
		},
	})
	require.NoError(t, err)
	defer colorPipe.Dispose()

	whiteTex, err := d.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1, Format: backend.TextureFormatRGBA8,
		Data: []byte{255, 255, 255, 255},
	})
	require.NoError(t, err)
	defer whiteTex.Dispose()

	// Single triangle covering roughly the center quarter of the RT.
	// NDC coords; vertex colors yellow.
	verts := packVerts(
		vtx2{-0.4, -0.4, 0, 0, 1, 1, 0, 1},
		vtx2{0.4, -0.4, 0, 0, 1, 1, 0, 1},
		vtx2{0.0, 0.4, 0, 0, 1, 1, 0, 1},
	)
	indices := packIdx16(0, 1, 2)

	vbuf, err := d.NewBuffer(backend.BufferDescriptor{Data: verts})
	require.NoError(t, err)
	defer vbuf.Dispose()
	ibuf, err := d.NewBuffer(backend.BufferDescriptor{Data: indices})
	require.NoError(t, err)
	defer ibuf.Dispose()

	enc := d.Encoder()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		Target:             rt,
		LoadAction:         backend.LoadActionClear,
		ClearColor:         [4]float32{0, 0, 0, 1},
		StencilLoadAction:  backend.LoadActionClear,
		StencilStoreAction: backend.StoreActionStore,
	})
	enc.SetVertexBuffer(vbuf, 0)
	enc.SetIndexBuffer(ibuf, backend.IndexUint16)
	enc.SetTexture(whiteTex, 0)
	enc.SetViewport(backend.Viewport{X: 0, Y: 0, Width: 32, Height: 32})

	// Stencil-write pass: triangle inverts stencil (0 → 0xFF) under its
	// footprint; color writes disabled via pipeline's ColorWriteDisabled
	// flag (no runtime SetColorWrite needed).
	enc.SetPipeline(writePipe)
	enc.SetStencilReference(0)
	enc.DrawIndexed(3, 1, 0)

	// Color pass: NotEqual 0 selects inside-triangle pixels; Zero-on-pass
	// clears the stencil as we draw.
	enc.SetPipeline(colorPipe)
	enc.SetStencilReference(0)
	enc.DrawIndexed(3, 1, 0)
	enc.EndRenderPass()

	// Readback and verify: at least one pixel inside the triangle is
	// yellow, at least one corner pixel is still black.
	pixels := make([]byte, 32*32*4)
	rt.ColorTexture().ReadPixels(pixels)

	// Corner (0, 0) is outside the triangle → black.
	require.Equal(t, byte(0), pixels[0])
	require.Equal(t, byte(0), pixels[1])
	require.Equal(t, byte(0), pixels[2])
	require.Equal(t, byte(255), pixels[3])

	// Center-ish pixel (16, 16) is inside → yellow (R=255, G=255, B=0).
	centerOff := (16*32 + 16) * 4
	require.Equal(t, byte(255), pixels[centerOff])
	require.Equal(t, byte(255), pixels[centerOff+1])
	require.Equal(t, byte(0), pixels[centerOff+2])
}

// TestRasterizerStencilReferenceThreadsToCompare verifies that the
// value passed to SetStencilReference actually reaches the rasterizer's
// stencil compare — a regression guard against any future refactor
// that stops copying e.stencilRef into r.stencilRef in buildRasterizer.
// Fills the stencil buffer with 5 via StencilReplace, then draws the
// same triangle twice with CompareEqual: once with ref=5 (must fill
// yellow) and once with ref=7 (must produce no pixels).
func TestRasterizerStencilReferenceThreadsToCompare(t *testing.T) {
	d := New()
	require.NoError(t, d.Init(backend.DeviceConfig{Width: 16, Height: 16}))
	defer d.Dispose()

	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 16, Height: 16, ColorFormat: backend.TextureFormatRGBA8,
		HasStencil: true,
	})
	require.NoError(t, err)
	defer rt.Dispose()

	shader, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)
	defer shader.Dispose()

	// Pass 1 seed: stencil=5 everywhere the triangle covers, no color.
	seedPipe, err := d.NewPipeline(backend.PipelineDescriptor{
		Shader:        shader,
		Primitive:     backend.PrimitiveTriangles,
		StencilEnable: true,
		Stencil: backend.StencilDescriptor{
			Func:      backend.CompareAlways,
			Mask:      0xFF,
			WriteMask: 0xFF,
			Front: backend.StencilFaceOps{
				SFail:  backend.StencilKeep,
				DPFail: backend.StencilKeep,
				DPPass: backend.StencilReplace,
			},
		},
		ColorWriteDisabled: true,
	})
	require.NoError(t, err)
	defer seedPipe.Dispose()

	// Pass 2 test: only draw where stencil == ref.
	compareEqualPipe, err := d.NewPipeline(backend.PipelineDescriptor{
		Shader:        shader,
		Primitive:     backend.PrimitiveTriangles,
		StencilEnable: true,
		Stencil: backend.StencilDescriptor{
			Func:      backend.CompareEqual,
			Mask:      0xFF,
			WriteMask: 0xFF,
			Front: backend.StencilFaceOps{
				SFail:  backend.StencilKeep,
				DPFail: backend.StencilKeep,
				DPPass: backend.StencilKeep,
			},
		},
	})
	require.NoError(t, err)
	defer compareEqualPipe.Dispose()

	whiteTex, err := d.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1, Format: backend.TextureFormatRGBA8,
		Data: []byte{255, 255, 255, 255},
	})
	require.NoError(t, err)
	defer whiteTex.Dispose()

	verts := packVerts(
		vtx2{-0.6, -0.6, 0, 0, 1, 1, 0, 1},
		vtx2{0.6, -0.6, 0, 0, 1, 1, 0, 1},
		vtx2{0.0, 0.6, 0, 0, 1, 1, 0, 1},
	)
	indices := packIdx16(0, 1, 2)
	vbuf, err := d.NewBuffer(backend.BufferDescriptor{Data: verts})
	require.NoError(t, err)
	defer vbuf.Dispose()
	ibuf, err := d.NewBuffer(backend.BufferDescriptor{Data: indices})
	require.NoError(t, err)
	defer ibuf.Dispose()

	run := func(compareRef uint32) []byte {
		enc := d.Encoder()
		enc.BeginRenderPass(backend.RenderPassDescriptor{
			Target:             rt,
			LoadAction:         backend.LoadActionClear,
			ClearColor:         [4]float32{0, 0, 0, 1},
			StencilLoadAction:  backend.LoadActionClear,
			StencilStoreAction: backend.StoreActionStore,
		})
		enc.SetVertexBuffer(vbuf, 0)
		enc.SetIndexBuffer(ibuf, backend.IndexUint16)
		enc.SetTexture(whiteTex, 0)
		enc.SetViewport(backend.Viewport{X: 0, Y: 0, Width: 16, Height: 16})

		// Seed: reference=5 feeds StencilReplace, writing 5 under the triangle.
		enc.SetPipeline(seedPipe)
		enc.SetStencilReference(5)
		enc.DrawIndexed(3, 1, 0)

		// Test: reference=compareRef, compare func = Equal.
		enc.SetPipeline(compareEqualPipe)
		enc.SetStencilReference(compareRef)
		enc.DrawIndexed(3, 1, 0)
		enc.EndRenderPass()

		px := make([]byte, 16*16*4)
		rt.ColorTexture().ReadPixels(px)
		return px
	}

	// ref=5 matches the seeded stencil — center pixel should be yellow.
	matchingPixels := run(5)
	centerOff := (8*16 + 8) * 4
	require.Equal(t, byte(255), matchingPixels[centerOff], "ref=5 should match seeded stencil")
	require.Equal(t, byte(255), matchingPixels[centerOff+1])
	require.Equal(t, byte(0), matchingPixels[centerOff+2])

	// ref=7 mismatches — center pixel should be unfilled (cleared black).
	mismatchPixels := run(7)
	require.Equal(t, byte(0), mismatchPixels[centerOff], "ref=7 should not match")
	require.Equal(t, byte(0), mismatchPixels[centerOff+1])
}

// --- small helpers to avoid pulling from the conformance package ---

type vtx2 struct {
	px, py, tu, tv, r, g, b, a float32
}

func packVerts(vs ...vtx2) []byte {
	out := make([]byte, len(vs)*32)
	for i, v := range vs {
		off := i * 32
		putF32(out[off:], v.px)
		putF32(out[off+4:], v.py)
		putF32(out[off+8:], v.tu)
		putF32(out[off+12:], v.tv)
		putF32(out[off+16:], v.r)
		putF32(out[off+20:], v.g)
		putF32(out[off+24:], v.b)
		putF32(out[off+28:], v.a)
	}
	return out
}

func packIdx16(idx ...uint16) []byte {
	out := make([]byte, len(idx)*2)
	for i, x := range idx {
		binary.LittleEndian.PutUint16(out[i*2:], x)
	}
	return out
}

func putF32(dst []byte, f float32) {
	binary.LittleEndian.PutUint32(dst, math.Float32bits(f))
}
