package pipeline

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/batch"
)

// --- Mock implementations for testing ---

type mockBuffer struct {
	size     int
	uploaded []byte
}

func (b *mockBuffer) Upload(data []byte)           { b.uploaded = data }
func (b *mockBuffer) UploadRegion(_ []byte, _ int) {}
func (b *mockBuffer) Size() int                    { return b.size }
func (b *mockBuffer) Dispose()                     {}

type mockTexture struct {
	w, h int
}

func (t *mockTexture) Upload(_ []byte, _ int)                   {}
func (t *mockTexture) UploadRegion(_ []byte, _, _, _, _, _ int) {}
func (t *mockTexture) ReadPixels(_ []byte)                      {}
func (t *mockTexture) Width() int                               { return t.w }
func (t *mockTexture) Height() int                              { return t.h }
func (t *mockTexture) Format() backend.TextureFormat            { return backend.TextureFormatRGBA8 }
func (t *mockTexture) Dispose()                                 {}

type mockShader struct {
	uniforms map[string]interface{}
}

func (s *mockShader) SetUniformFloat(name string, v float32)    { s.uniforms[name] = v }
func (s *mockShader) SetUniformVec2(name string, v [2]float32)  { s.uniforms[name] = v }
func (s *mockShader) SetUniformVec4(name string, v [4]float32)  { s.uniforms[name] = v }
func (s *mockShader) SetUniformMat4(name string, v [16]float32) { s.uniforms[name] = v }
func (s *mockShader) SetUniformInt(name string, v int32)        { s.uniforms[name] = v }
func (s *mockShader) SetUniformBlock(_ string, _ []byte)        {}
func (s *mockShader) Dispose()                                  {}

type mockPipeline struct{}

func (p *mockPipeline) Dispose() {}

type mockDevice struct{}

func (d *mockDevice) Init(_ backend.DeviceConfig) error { return nil }
func (d *mockDevice) Dispose()                          {}
func (d *mockDevice) ReadScreen(_ []byte) bool          { return false }
func (d *mockDevice) BeginFrame()                       {}
func (d *mockDevice) EndFrame()                         {}
func (d *mockDevice) NewTexture(_ backend.TextureDescriptor) (backend.Texture, error) {
	return &mockTexture{}, nil
}
func (d *mockDevice) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	return &mockBuffer{size: desc.Size}, nil
}
func (d *mockDevice) NewShader(_ backend.ShaderDescriptor) (backend.Shader, error) {
	return &mockShader{uniforms: make(map[string]interface{})}, nil
}
func (d *mockDevice) NewRenderTarget(_ backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	return nil, nil
}
func (d *mockDevice) NewPipeline(_ backend.PipelineDescriptor) (backend.Pipeline, error) {
	return &mockPipeline{}, nil
}
func (d *mockDevice) Capabilities() backend.DeviceCapabilities {
	return backend.DeviceCapabilities{MaxTextureSize: 4096}
}
func (d *mockDevice) Encoder() backend.CommandEncoder { return nil }

// failingDevice fails on the Nth NewBuffer call.
type failingDevice struct {
	failOn    int
	callCount *int
}

func (d *failingDevice) Init(_ backend.DeviceConfig) error { return nil }
func (d *failingDevice) Dispose()                          {}
func (d *failingDevice) ReadScreen(_ []byte) bool          { return false }
func (d *failingDevice) BeginFrame()                       {}
func (d *failingDevice) EndFrame()                         {}
func (d *failingDevice) NewTexture(_ backend.TextureDescriptor) (backend.Texture, error) {
	return &mockTexture{}, nil
}
func (d *failingDevice) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	*d.callCount++
	if *d.callCount >= d.failOn {
		return nil, errMockFail
	}
	return &mockBuffer{size: desc.Size}, nil
}
func (d *failingDevice) NewShader(_ backend.ShaderDescriptor) (backend.Shader, error) {
	return &mockShader{uniforms: make(map[string]interface{})}, nil
}
func (d *failingDevice) NewRenderTarget(_ backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	return nil, nil
}
func (d *failingDevice) NewPipeline(_ backend.PipelineDescriptor) (backend.Pipeline, error) {
	return &mockPipeline{}, nil
}
func (d *failingDevice) Capabilities() backend.DeviceCapabilities {
	return backend.DeviceCapabilities{MaxTextureSize: 4096}
}
func (d *failingDevice) Encoder() backend.CommandEncoder { return nil }

var errMockFail = fmt.Errorf("mock failure")

// encoderCall records a method call on the mock encoder.
type encoderCall struct {
	Method string
	Args   []interface{}
}

type mockEncoder struct {
	calls []encoderCall
}

func (e *mockEncoder) record(method string, args ...interface{}) {
	e.calls = append(e.calls, encoderCall{Method: method, Args: args})
}

func (e *mockEncoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	e.record("BeginRenderPass", desc.Target, desc.LoadAction)
}
func (e *mockEncoder) EndRenderPass()                             { e.record("EndRenderPass") }
func (e *mockEncoder) SetPipeline(_ backend.Pipeline)             { e.record("SetPipeline") }
func (e *mockEncoder) SetVertexBuffer(_ backend.Buffer, slot int) { e.record("SetVertexBuffer", slot) }
func (e *mockEncoder) SetIndexBuffer(_ backend.Buffer, _ backend.IndexFormat) {
	e.record("SetIndexBuffer")
}
func (e *mockEncoder) SetTexture(_ backend.Texture, slot int) { e.record("SetTexture", slot) }
func (e *mockEncoder) SetTextureFilter(slot int, f backend.TextureFilter) {
	e.record("SetTextureFilter", slot, f)
}
func (e *mockEncoder) SetStencil(enabled bool, desc backend.StencilDescriptor) {
	e.record("SetStencil", enabled, desc)
}
func (e *mockEncoder) SetColorWrite(enabled bool)        { e.record("SetColorWrite", enabled) }
func (e *mockEncoder) SetViewport(_ backend.Viewport)    {}
func (e *mockEncoder) SetScissor(_ *backend.ScissorRect) {}
func (e *mockEncoder) Draw(vertexCount, instanceCount, firstVertex int) {
	e.record("Draw", vertexCount, instanceCount, firstVertex)
}
func (e *mockEncoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	e.record("DrawIndexed", indexCount, instanceCount, firstIndex)
}
func (e *mockEncoder) Flush() { e.record("Flush") }

// callsByMethod returns all calls with the given method name.
func (e *mockEncoder) callsByMethod(method string) []encoderCall {
	var result []encoderCall
	for _, c := range e.calls {
		if c.Method == method {
			result = append(result, c)
		}
	}
	return result
}

// --- Tests ---

func newTestSpritePass(t *testing.T, batcher *batch.Batcher) *SpritePass {
	t.Helper()
	dev := &mockDevice{}
	sp, err := NewSpritePass(SpritePassConfig{
		Device:      dev,
		Batcher:     batcher,
		Pipeline:    &mockPipeline{},
		Shader:      &mockShader{uniforms: make(map[string]interface{})},
		MaxVertices: 1024,
		MaxIndices:  1024,
	})
	require.NoError(t, err)
	t.Cleanup(sp.Dispose)
	return sp
}

func TestSpritePassName(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	require.Equal(t, "sprite", sp.Name())
}

func TestSpritePassExecuteEmpty(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	enc := &mockEncoder{}

	sp.Execute(enc, NewPassContext(800, 600))

	// No batches → no draw calls, but screen is still cleared.
	require.Empty(t, enc.callsByMethod("DrawIndexed"))
	begins := enc.callsByMethod("BeginRenderPass")
	require.Len(t, begins, 1)
	require.Nil(t, begins[0].Args[0]) // screen target (nil)
	ends := enc.callsByMethod("EndRenderPass")
	require.Len(t, ends, 1)
}

func TestSpritePassScreenClearDisabled(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	sp.ResolveTexture = func(_ uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		TargetID:  0,
	})

	enc := &mockEncoder{}
	ctx := NewPassContext(800, 600)
	ctx.ScreenClearEnabled = false
	sp.Execute(enc, ctx)

	// Screen target should use LoadActionLoad, not Clear.
	begins := enc.callsByMethod("BeginRenderPass")
	require.Len(t, begins, 1)
	require.Equal(t, backend.LoadActionLoad, begins[0].Args[1])
}

func TestSpritePassScreenClearEnabled(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	sp.ResolveTexture = func(_ uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		TargetID:  0,
	})

	enc := &mockEncoder{}
	ctx := NewPassContext(800, 600)
	// ScreenClearEnabled defaults to true via NewPassContext.
	sp.Execute(enc, ctx)

	begins := enc.callsByMethod("BeginRenderPass")
	require.Len(t, begins, 1)
	require.Equal(t, backend.LoadActionClear, begins[0].Args[1])
}

func TestSpritePassEmptyNoClearWhenDisabled(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	enc := &mockEncoder{}

	ctx := NewPassContext(800, 600)
	ctx.ScreenClearEnabled = false
	sp.Execute(enc, ctx)

	// Even with no batches, a render pass is emitted for the screen target,
	// but with LoadActionLoad (preserving previous content).
	begins := enc.callsByMethod("BeginRenderPass")
	require.Len(t, begins, 1)
	require.Equal(t, backend.LoadActionLoad, begins[0].Args[1])
}

func TestSpritePassExecuteNonZero(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)

	tex := &mockTexture{w: 32, h: 32}
	sp.ResolveTexture = func(id uint32) backend.Texture {
		if id == 1 {
			return tex
		}
		return nil
	}

	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{PosX: 0, PosY: 0}, {PosX: 10, PosY: 0}, {PosX: 10, PosY: 10}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		Filter:    backend.FilterNearest,
		FillRule:  backend.FillRuleNonZero,
	})

	enc := &mockEncoder{}
	sp.Execute(enc, NewPassContext(800, 600))

	// NonZero: single DrawIndexed, no stencil calls.
	draws := enc.callsByMethod("DrawIndexed")
	require.Len(t, draws, 1)
	require.Equal(t, 3, draws[0].Args[0]) // indexCount

	stencils := enc.callsByMethod("SetStencil")
	require.Empty(t, stencils)
}

func TestSpritePassExecuteEvenOdd(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)

	tex := &mockTexture{w: 32, h: 32}
	sp.ResolveTexture = func(id uint32) backend.Texture {
		if id == 1 {
			return tex
		}
		return nil
	}

	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{PosX: 0, PosY: 0}, {PosX: 10, PosY: 0}, {PosX: 10, PosY: 10}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		Filter:    backend.FilterNearest,
		FillRule:  backend.FillRuleEvenOdd,
	})

	enc := &mockEncoder{}
	sp.Execute(enc, NewPassContext(800, 600))

	// EvenOdd: two DrawIndexed calls (stencil pass + color pass).
	draws := enc.callsByMethod("DrawIndexed")
	require.Len(t, draws, 2)

	// Should have stencil calls: enable, enable (pass 2), disable.
	stencils := enc.callsByMethod("SetStencil")
	require.Len(t, stencils, 3)

	// First: enable stencil with INVERT
	require.True(t, stencils[0].Args[0].(bool))
	desc0 := stencils[0].Args[1].(backend.StencilDescriptor)
	require.Equal(t, backend.CompareAlways, desc0.Func)
	require.Equal(t, backend.StencilInvert, desc0.DPPass)

	// Second: enable stencil with NOTEQUAL
	require.True(t, stencils[1].Args[0].(bool))
	desc1 := stencils[1].Args[1].(backend.StencilDescriptor)
	require.Equal(t, backend.CompareNotEqual, desc1.Func)

	// Third: disable stencil
	require.False(t, stencils[2].Args[0].(bool))

	// Color write: disabled then re-enabled.
	colorWrites := enc.callsByMethod("SetColorWrite")
	require.Len(t, colorWrites, 2)
	require.False(t, colorWrites[0].Args[0].(bool))
	require.True(t, colorWrites[1].Args[0].(bool))
}

func TestSpritePassMixedFillRules(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	sp.ResolveTexture = func(_ uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	// Add one NonZero and one EvenOdd batch.
	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		FillRule:  backend.FillRuleNonZero,
	})
	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		FillRule:  backend.FillRuleEvenOdd,
	})

	enc := &mockEncoder{}
	sp.Execute(enc, NewPassContext(800, 600))

	// NonZero: 1 draw, EvenOdd: 2 draws = 3 total.
	draws := enc.callsByMethod("DrawIndexed")
	require.Len(t, draws, 3)
}

func TestSpritePassDispose(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	// Dispose should not panic.
	sp.Dispose()
}

func TestSpritePassTextureResolution(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)

	resolved := false
	sp.ResolveTexture = func(id uint32) backend.Texture {
		if id == 42 {
			resolved = true
			return &mockTexture{w: 64, h: 64}
		}
		return nil
	}

	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 42,
	})

	enc := &mockEncoder{}
	sp.Execute(enc, NewPassContext(800, 600))
	require.True(t, resolved)

	texCalls := enc.callsByMethod("SetTexture")
	require.Len(t, texCalls, 1)
}

func TestSpritePassNilVertexSlice(t *testing.T) {
	require.Nil(t, vertexSliceToBytes(nil))
	require.Nil(t, vertexSliceToBytes([]batch.Vertex2D{}))
}

func TestSpritePassNilIndexSlice(t *testing.T) {
	require.Nil(t, indexSliceToBytes(nil))
	require.Nil(t, indexSliceToBytes([]uint16{}))
}

func TestSpritePassNewError(t *testing.T) {
	// Test that error in index buffer creation cleans up vertex buffer.
	callCount := 0
	failDevice := &failingDevice{failOn: 2, callCount: &callCount}

	_, err := NewSpritePass(SpritePassConfig{
		Device:      failDevice,
		Batcher:     batch.NewBatcher(1024, 1024),
		Pipeline:    &mockPipeline{},
		Shader:      &mockShader{uniforms: make(map[string]interface{})},
		MaxVertices: 1024,
		MaxIndices:  1024,
	})
	require.Error(t, err)
}

func TestSpritePassNoResolver(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	// ResolveTexture is nil — should not panic.

	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
	})

	enc := &mockEncoder{}
	sp.Execute(enc, NewPassContext(800, 600))

	draws := enc.callsByMethod("DrawIndexed")
	require.Len(t, draws, 1)

	texCalls := enc.callsByMethod("SetTexture")
	require.Empty(t, texCalls)
}

func TestSpritePassRenderTargetSwitch(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	sp.ResolveTexture = func(_ uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	mockRT := &mockRenderTarget{w: 256, h: 256}
	sp.ResolveRenderTarget = func(id uint32) backend.RenderTarget {
		if id == 10 {
			return mockRT
		}
		return nil
	}

	// Draw to offscreen target (ID 10), then to screen (ID 0).
	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		TargetID:  10,
	})
	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		TargetID:  0,
	})

	enc := &mockEncoder{}
	sp.Execute(enc, NewPassContext(800, 600))

	// Should have 2 BeginRenderPass calls (screen first due to sort, then offscreen).
	begins := enc.callsByMethod("BeginRenderPass")
	require.Len(t, begins, 2)
	// First pass targets nil (screen, TargetID 0 sorts first).
	require.Nil(t, begins[0].Args[0])
	// Second pass targets the mock RT.
	require.Equal(t, backend.RenderTarget(mockRT), begins[1].Args[0])

	// Should have 2 EndRenderPass calls.
	ends := enc.callsByMethod("EndRenderPass")
	require.Len(t, ends, 2)

	// Should have 2 DrawIndexed calls.
	draws := enc.callsByMethod("DrawIndexed")
	require.Len(t, draws, 2)
}

func TestSpritePassSingleTargetOnlyOnePass(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	sp.ResolveTexture = func(_ uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	// All draws to screen.
	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		TargetID:  0,
	})
	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 2,
		TargetID:  0,
	})

	enc := &mockEncoder{}
	sp.Execute(enc, NewPassContext(800, 600))

	// Only 1 render pass.
	begins := enc.callsByMethod("BeginRenderPass")
	require.Len(t, begins, 1)
	ends := enc.callsByMethod("EndRenderPass")
	require.Len(t, ends, 1)
}

// mockRenderTarget implements backend.RenderTarget for testing.
type mockRenderTarget struct {
	w, h int
}

func (rt *mockRenderTarget) ColorTexture() backend.Texture { return &mockTexture{w: rt.w, h: rt.h} }
func (rt *mockRenderTarget) DepthTexture() backend.Texture { return nil }
func (rt *mockRenderTarget) Width() int                    { return rt.w }
func (rt *mockRenderTarget) Height() int                   { return rt.h }
func (rt *mockRenderTarget) Dispose()                      {}

// --- Pipeline struct tests ---

type dummyPass struct {
	name     string
	executed bool
}

func (p *dummyPass) Name() string                                     { return p.name }
func (p *dummyPass) Execute(_ backend.CommandEncoder, _ *PassContext) { p.executed = true }

func TestPipelineNew(t *testing.T) {
	p := New()
	require.NotNil(t, p)
	require.Empty(t, p.Passes())
}

func TestPipelineAddPass(t *testing.T) {
	p := New()
	p.AddPass(&dummyPass{name: "a"})
	p.AddPass(&dummyPass{name: "b"})
	require.Len(t, p.Passes(), 2)
	require.Equal(t, "a", p.Passes()[0].Name())
	require.Equal(t, "b", p.Passes()[1].Name())
}

func TestPipelineInsertPass(t *testing.T) {
	p := New()
	p.AddPass(&dummyPass{name: "a"})
	p.AddPass(&dummyPass{name: "c"})
	p.InsertPass(1, &dummyPass{name: "b"})
	names := make([]string, len(p.Passes()))
	for i, pass := range p.Passes() {
		names[i] = pass.Name()
	}
	require.Equal(t, []string{"a", "b", "c"}, names)
}

func TestPipelineRemovePass(t *testing.T) {
	p := New()
	p.AddPass(&dummyPass{name: "a"})
	p.AddPass(&dummyPass{name: "b"})
	p.AddPass(&dummyPass{name: "c"})

	p.RemovePass("b")
	require.Len(t, p.Passes(), 2)
	require.Equal(t, "a", p.Passes()[0].Name())
	require.Equal(t, "c", p.Passes()[1].Name())
}

func TestPipelineRemovePassNotFound(t *testing.T) {
	p := New()
	p.AddPass(&dummyPass{name: "a"})
	p.RemovePass("nonexistent")
	require.Len(t, p.Passes(), 1)
}

func TestPipelineExecute(t *testing.T) {
	p := New()
	a := &dummyPass{name: "a"}
	b := &dummyPass{name: "b"}
	p.AddPass(a)
	p.AddPass(b)

	enc := &mockEncoder{}
	p.Execute(enc, NewPassContext(800, 600))

	require.True(t, a.executed)
	require.True(t, b.executed)
}

func TestNewPassContext(t *testing.T) {
	ctx := NewPassContext(1920, 1080)
	require.Equal(t, 1920, ctx.FramebufferWidth)
	require.Equal(t, 1080, ctx.FramebufferHeight)
	require.True(t, ctx.ScreenClearEnabled)
	require.NotNil(t, ctx.Resources)
}
