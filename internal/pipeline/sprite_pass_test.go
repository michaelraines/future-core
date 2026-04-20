package pipeline

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
)

// --- Mock implementations for testing ---

type mockBuffer struct {
	size     int
	uploaded []byte
	disposed bool
}

func (b *mockBuffer) Upload(data []byte)           { b.uploaded = data }
func (b *mockBuffer) UploadRegion(_ []byte, _ int) {}
func (b *mockBuffer) Size() int                    { return b.size }
func (b *mockBuffer) Dispose()                     { b.disposed = true }

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
func (s *mockShader) SetUniformVec3(name string, v [3]float32)  { s.uniforms[name] = v }
func (s *mockShader) SetUniformVec4(name string, v [4]float32)  { s.uniforms[name] = v }
func (s *mockShader) SetUniformMat4(name string, v [16]float32) { s.uniforms[name] = v }
func (s *mockShader) SetUniformInt(name string, v int32)        { s.uniforms[name] = v }
func (s *mockShader) SetUniformBlock(_ string, _ []byte)        {}
func (s *mockShader) PackCurrentUniforms() []byte               { return nil }
func (s *mockShader) Dispose()                                  {}

// mockPipeline carries a distinguishing tag so tests can verify the
// sequence of pipelines bound across a drawStenciled call (write →
// color → default) rather than merely asserting "some pipeline was
// bound three times." The mockDevice's NewPipeline stamps tags based
// on pipeline configuration (StencilEnable, ColorWriteDisabled, Func).
type mockPipeline struct {
	tag string
}

func (p *mockPipeline) Dispose() {}

type mockDevice struct {
	// supportsStencil flips the device capability flag so tests can
	// exercise the sprite pass's stencil routing without any real GPU.
	supportsStencil bool
}

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
func (d *mockDevice) NewPipeline(desc backend.PipelineDescriptor) (backend.Pipeline, error) {
	tag := "default"
	switch {
	case desc.StencilEnable && desc.ColorWriteDisabled && desc.Stencil.TwoSided:
		tag = "writeNonZero"
	case desc.StencilEnable && desc.ColorWriteDisabled:
		tag = "writeEvenOdd"
	case desc.StencilEnable && desc.Stencil.Func == backend.CompareNotEqual:
		tag = "colorPass"
	}
	return &mockPipeline{tag: tag}, nil
}
func (d *mockDevice) Capabilities() backend.DeviceCapabilities {
	return backend.DeviceCapabilities{
		MaxTextureSize:  4096,
		SupportsStencil: d.supportsStencil,
	}
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
func (e *mockEncoder) EndRenderPass() { e.record("EndRenderPass") }
func (e *mockEncoder) SetPipeline(p backend.Pipeline) {
	tag := ""
	if mp, ok := p.(*mockPipeline); ok {
		tag = mp.tag
	}
	e.record("SetPipeline", tag)
}
func (e *mockEncoder) SetVertexBuffer(_ backend.Buffer, slot int) { e.record("SetVertexBuffer", slot) }
func (e *mockEncoder) SetIndexBuffer(_ backend.Buffer, _ backend.IndexFormat) {
	e.record("SetIndexBuffer")
}
func (e *mockEncoder) SetTexture(_ backend.Texture, slot int) { e.record("SetTexture", slot) }
func (e *mockEncoder) SetTextureFilter(slot int, f backend.TextureFilter) {
	e.record("SetTextureFilter", slot, f)
}
func (e *mockEncoder) SetStencilReference(ref uint32) {
	e.record("SetStencilReference", ref)
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
func (e *mockEncoder) SetBlendMode(mode backend.BlendMode) {
	e.record("SetBlendMode", mode)
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

	// NonZero against a device that doesn't advertise stencil support:
	// falls through to a single plain DrawIndexed. No SetStencilReference
	// calls should fire because stencilEligible rejects on
	// !Capabilities().SupportsStencil.
	draws := enc.callsByMethod("DrawIndexed")
	require.Len(t, draws, 1)
	require.Equal(t, 3, draws[0].Args[0]) // indexCount
	require.Empty(t, enc.callsByMethod("SetStencilReference"))
}

func TestSpritePassExecuteEvenOddFallback(t *testing.T) {
	// When the device reports SupportsStencil=false (default mock state),
	// EvenOdd batches fall through to a plain DrawIndexed — same behavior
	// as NonZero on a non-stencil backend. Pre-stencil-rewrite this test
	// verified three SetStencil calls; that encoder method no longer
	// exists and the sprite pass no longer issues inline stencil commands.
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

	draws := enc.callsByMethod("DrawIndexed")
	require.Len(t, draws, 1)
	require.Empty(t, enc.callsByMethod("SetStencilReference"))
}

func TestSpritePassExecuteEvenOddStenciled(t *testing.T) {
	// With SupportsStencil=true and ScreenHasStencil=true, an EvenOdd
	// batch takes the two-pass stencil path: write pipe → ref → draw,
	// color pipe → ref → draw, restore default pipe. Each DrawIndexed
	// is preceded by a SetStencilReference(0) and bracketed by color
	// writes off/on.
	b := batch.NewBatcher(1024, 1024)
	dev := &mockDevice{supportsStencil: true}
	vbuf, err := dev.NewBuffer(backend.BufferDescriptor{Size: 1024})
	require.NoError(t, err)
	shader := &mockShader{uniforms: make(map[string]interface{})}
	sp, err := NewSpritePass(SpritePassConfig{
		Device:      dev,
		Batcher:     b,
		Pipeline:    &mockPipeline{tag: "default"},
		Shader:      shader,
		MaxVertices: 1024,
		MaxIndices:  1024,
	})
	require.NoError(t, err)
	defer sp.Dispose()
	_ = vbuf

	sp.ResolveTexture = func(_ uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		BlendMode: backend.BlendSourceOver,
		FillRule:  backend.FillRuleEvenOdd,
	})

	enc := &mockEncoder{}
	ctx := NewPassContext(800, 600)
	ctx.ScreenHasStencil = true
	sp.Execute(enc, ctx)

	draws := enc.callsByMethod("DrawIndexed")
	require.Len(t, draws, 2, "EvenOdd stenciled path emits stencil-write + color draws")
	refs := enc.callsByMethod("SetStencilReference")
	require.Len(t, refs, 2)
	require.Equal(t, uint32(0), refs[0].Args[0].(uint32))
	require.Equal(t, uint32(0), refs[1].Args[0].(uint32))
	// Color-write toggling lives on the pipelines (ColorWriteDisabled),
	// not on encoder state — no SetColorWrite calls should fire for
	// the stencil dance.
	require.Empty(t, enc.callsByMethod("SetColorWrite"))

	// Verify the stencil dance's pipeline identity via tagged
	// mockPipelines. The preamble binds the default pipeline one or
	// more times (bindDefaultShader plus a potential blend-mode-change
	// re-bind when the batch's BlendSourceOver differs from the
	// zero-value lastBlendMode). The last three SetPipeline calls
	// must be writeEvenOdd → colorPass → default in that exact order,
	// proving drawStenciled swapped to the write pipeline, the color
	// pipeline, and restored the default in turn.
	sets := enc.callsByMethod("SetPipeline")
	require.GreaterOrEqual(t, len(sets), 3)
	tail := sets[len(sets)-3:]
	require.Equal(t, "writeEvenOdd", tail[0].Args[0].(string))
	require.Equal(t, "colorPass", tail[1].Args[0].(string))
	require.Equal(t, "default", tail[2].Args[0].(string))
}

// stencilFailingDevice returns a non-nil pipeline for the first
// pipelineFailAfter pipeline creations and errors after that. Used to
// exercise ensureStencilPipelines's partial-build cleanup.
type stencilFailingDevice struct {
	mockDevice
	pipelineCount     *int
	pipelineFailAfter int
}

func (d *stencilFailingDevice) Capabilities() backend.DeviceCapabilities {
	return backend.DeviceCapabilities{
		MaxTextureSize:  4096,
		SupportsStencil: d.supportsStencil,
	}
}

func (d *stencilFailingDevice) NewPipeline(_ backend.PipelineDescriptor) (backend.Pipeline, error) {
	*d.pipelineCount++
	if *d.pipelineCount > d.pipelineFailAfter {
		return nil, errMockFail
	}
	return &mockPipeline{}, nil
}

func TestSpritePassEnsureStencilPipelinesFailureSafeDispose(t *testing.T) {
	// If NewPipeline fails mid-build (e.g. the third create for the
	// color pass), ensureStencilPipelines must dispose the already-
	// built write pipelines and leave the SpritePass fields nil so a
	// later Dispose() doesn't double-free. Regression guard for
	// `stencilPipesBuilt` remaining false after a partial build.
	b := batch.NewBatcher(1024, 1024)
	count := 0
	dev := &stencilFailingDevice{
		mockDevice:        mockDevice{supportsStencil: true},
		pipelineCount:     &count,
		pipelineFailAfter: 2, // succeed for writeNZ + writeEO, fail for colorPipe
	}
	sp, err := NewSpritePass(SpritePassConfig{
		Device:      dev,
		Batcher:     b,
		Pipeline:    &mockPipeline{},
		Shader:      &mockShader{uniforms: make(map[string]interface{})},
		MaxVertices: 1024,
		MaxIndices:  1024,
	})
	require.NoError(t, err)
	sp.ResolveTexture = func(_ uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		BlendMode: backend.BlendSourceOver,
		FillRule:  backend.FillRuleEvenOdd,
	})

	enc := &mockEncoder{}
	ctx := NewPassContext(800, 600)
	ctx.ScreenHasStencil = true
	sp.Execute(enc, ctx)

	// Third NewPipeline failure forced the fallback to plain DrawIndexed.
	require.Len(t, enc.callsByMethod("DrawIndexed"), 1)
	require.Empty(t, enc.callsByMethod("SetStencilReference"))
	// Dispose must not panic even though two stencil pipelines were
	// transiently created and then released inside ensureStencilPipelines.
	require.NotPanics(t, func() { sp.Dispose() })
}

func TestSpritePassStencilThenRegularBatchInteraction(t *testing.T) {
	// A stencil fill-rule batch followed by a regular (non-fill-rule)
	// batch with the same default shader must not corrupt state for
	// the second batch: drawStenciled restores sp.pipeline and sets
	// lastBlendMode = BlendSourceOver, so the sp.lastBlendMode guard
	// in Execute fires correctly when the second batch uses a
	// different blend mode.
	b := batch.NewBatcher(1024, 1024)
	dev := &mockDevice{supportsStencil: true}
	sp, err := NewSpritePass(SpritePassConfig{
		Device:      dev,
		Batcher:     b,
		Pipeline:    &mockPipeline{},
		Shader:      &mockShader{uniforms: make(map[string]interface{})},
		MaxVertices: 1024,
		MaxIndices:  1024,
	})
	require.NoError(t, err)
	defer sp.Dispose()
	sp.ResolveTexture = func(_ uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	// First: a stencil-routed EvenOdd batch.
	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		BlendMode: backend.BlendSourceOver,
		FillRule:  backend.FillRuleEvenOdd,
	})
	// Second: a regular batch with a DIFFERENT blend — the Execute
	// loop's "same default shader, different blend" guard depends on
	// sp.lastBlendMode being current after drawStenciled.
	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		BlendMode: backend.BlendAdditive,
	})

	enc := &mockEncoder{}
	ctx := NewPassContext(800, 600)
	ctx.ScreenHasStencil = true
	sp.Execute(enc, ctx)

	// Stencil batch: 2 DrawIndexed; regular batch: 1 DrawIndexed. Three total.
	require.Len(t, enc.callsByMethod("DrawIndexed"), 3)
	// The regular batch's blend differs from the stencil pipelines'
	// baked BlendSourceOver, so a SetBlendMode(Additive) must have
	// fired before its draw.
	blendCalls := enc.callsByMethod("SetBlendMode")
	require.NotEmpty(t, blendCalls, "expected SetBlendMode for the additive batch")
}

func TestSpritePassBackToBackStencilBatches(t *testing.T) {
	// Two consecutive fill-rule batches in the same render pass must
	// each run the write/color pipeline pair. The color pass's
	// DPPass=Zero op self-clears the stencil buffer as it draws so the
	// second batch starts with stencil=0 without an explicit clear —
	// this test guards the sequencing by verifying four DrawIndexed
	// calls and four SetStencilReference calls alternate correctly.
	b := batch.NewBatcher(1024, 1024)
	dev := &mockDevice{supportsStencil: true}
	sp, err := NewSpritePass(SpritePassConfig{
		Device:      dev,
		Batcher:     b,
		Pipeline:    &mockPipeline{},
		Shader:      &mockShader{uniforms: make(map[string]interface{})},
		MaxVertices: 1024,
		MaxIndices:  1024,
	})
	require.NoError(t, err)
	defer sp.Dispose()
	sp.ResolveTexture = func(_ uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	// Different TextureIDs prevent the batcher from merging these into
	// one — we want two distinct fill-rule batches to exercise
	// drawStenciled being called back-to-back.
	for i := range 2 {
		b.Add(batch.DrawCommand{
			Vertices:  []batch.Vertex2D{{}, {}, {}},
			Indices:   []uint16{0, 1, 2},
			TextureID: uint32(1 + i),
			BlendMode: backend.BlendSourceOver,
			FillRule:  backend.FillRuleEvenOdd,
		})
	}

	enc := &mockEncoder{}
	ctx := NewPassContext(800, 600)
	ctx.ScreenHasStencil = true
	sp.Execute(enc, ctx)

	require.Len(t, enc.callsByMethod("DrawIndexed"), 4,
		"each fill-rule batch emits stencil-write + color draws")
	refs := enc.callsByMethod("SetStencilReference")
	require.Len(t, refs, 4)
	for i, c := range refs {
		require.Equal(t, uint32(0), c.Args[0].(uint32),
			"SetStencilReference(%d) should be 0", i)
	}
}

func TestSpritePassStencilFallbackNonSourceOver(t *testing.T) {
	// A fill-rule batch with a blend other than SourceOver must skip
	// the stencil path — the lazily-built stencil pipelines all bake
	// BlendSourceOver, so routing a BlendAdditive batch through them
	// would silently swap the blend. Expected behavior: fall back to
	// a single plain DrawIndexed with no SetStencilReference calls.
	b := batch.NewBatcher(1024, 1024)
	dev := &mockDevice{supportsStencil: true}
	sp, err := NewSpritePass(SpritePassConfig{
		Device:      dev,
		Batcher:     b,
		Pipeline:    &mockPipeline{},
		Shader:      &mockShader{uniforms: make(map[string]interface{})},
		MaxVertices: 1024,
		MaxIndices:  1024,
	})
	require.NoError(t, err)
	defer sp.Dispose()
	sp.ResolveTexture = func(_ uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		BlendMode: backend.BlendAdditive,
		FillRule:  backend.FillRuleEvenOdd,
	})

	enc := &mockEncoder{}
	ctx := NewPassContext(800, 600)
	ctx.ScreenHasStencil = true
	sp.Execute(enc, ctx)

	require.Len(t, enc.callsByMethod("DrawIndexed"), 1)
	require.Empty(t, enc.callsByMethod("SetStencilReference"))
}

func TestSpritePassStencilFallbackCustomShader(t *testing.T) {
	// A fill-rule batch bound to a custom ShaderID falls back to a
	// plain DrawIndexed — the sprite pass only builds stencil
	// pipelines against the default shader, so custom-shader fill
	// rules can't be routed through stencil without more pipeline
	// variants.
	b := batch.NewBatcher(1024, 1024)
	dev := &mockDevice{supportsStencil: true}
	sp, err := NewSpritePass(SpritePassConfig{
		Device:      dev,
		Batcher:     b,
		Pipeline:    &mockPipeline{},
		Shader:      &mockShader{uniforms: make(map[string]interface{})},
		MaxVertices: 1024,
		MaxIndices:  1024,
	})
	require.NoError(t, err)
	defer sp.Dispose()
	sp.ResolveTexture = func(_ uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }
	sp.ResolveShader = func(id uint32) *ShaderInfo {
		if id == 7 {
			return &ShaderInfo{
				Shader:   &mockShader{uniforms: make(map[string]interface{})},
				Pipeline: &mockPipeline{},
			}
		}
		return nil
	}

	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		BlendMode: backend.BlendSourceOver,
		ShaderID:  7,
		FillRule:  backend.FillRuleEvenOdd,
	})

	enc := &mockEncoder{}
	ctx := NewPassContext(800, 600)
	ctx.ScreenHasStencil = true
	sp.Execute(enc, ctx)

	require.Len(t, enc.callsByMethod("DrawIndexed"), 1)
	require.Empty(t, enc.callsByMethod("SetStencilReference"))
}

func TestSpritePassExecuteNonZeroStenciled(t *testing.T) {
	// NonZero batches also route through the stencil path when the
	// device + target support it. Difference from EvenOdd is the write
	// pipeline (Incr/Decr-wrap two-sided vs Invert); the emitted command
	// shape is identical from the encoder's perspective.
	b := batch.NewBatcher(1024, 1024)
	dev := &mockDevice{supportsStencil: true}
	sp, err := NewSpritePass(SpritePassConfig{
		Device:      dev,
		Batcher:     b,
		Pipeline:    &mockPipeline{},
		Shader:      &mockShader{uniforms: make(map[string]interface{})},
		MaxVertices: 1024,
		MaxIndices:  1024,
	})
	require.NoError(t, err)
	defer sp.Dispose()

	sp.ResolveTexture = func(_ uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		BlendMode: backend.BlendSourceOver,
		FillRule:  backend.FillRuleNonZero,
	})

	enc := &mockEncoder{}
	ctx := NewPassContext(800, 600)
	ctx.ScreenHasStencil = true
	sp.Execute(enc, ctx)

	// Explicit FillRuleNonZero routes through the stencil pipeline pair
	// (stencil-write + color). The zero-value FillRuleNone would bypass
	// this; only callers that opt in (DrawTrianglesOptions.FillRule set
	// explicitly, or future-core's vector library's FillPath) hit this
	// path.
	require.Len(t, enc.callsByMethod("DrawIndexed"), 2)
	require.Len(t, enc.callsByMethod("SetStencilReference"), 2)
}

func TestSpritePassMixedFillRules(t *testing.T) {
	// Both explicit FillRuleNonZero and FillRuleEvenOdd route through
	// the stencil pipeline pair (stencil-write + color = 2 DrawIndexed
	// each). Four total across the two batches.
	b := batch.NewBatcher(1024, 1024)
	dev := &mockDevice{supportsStencil: true}
	sp, err := NewSpritePass(SpritePassConfig{
		Device:      dev,
		Batcher:     b,
		Pipeline:    &mockPipeline{},
		Shader:      &mockShader{uniforms: make(map[string]interface{})},
		MaxVertices: 1024,
		MaxIndices:  1024,
	})
	require.NoError(t, err)
	defer sp.Dispose()
	sp.ResolveTexture = func(_ uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		FillRule:  backend.FillRuleNonZero,
	})
	b.Add(batch.DrawCommand{
		Vertices:  []batch.Vertex2D{{}, {}, {}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		FillRule:  backend.FillRuleEvenOdd,
	})

	enc := &mockEncoder{}
	ctx := NewPassContext(800, 600)
	ctx.ScreenHasStencil = true
	sp.Execute(enc, ctx)

	require.Len(t, enc.callsByMethod("DrawIndexed"), 4)
}

// TestSpritePassMultipleFillRuleBatchesCompositeIndependently guards the
// "overlapping transparent ellipses render fully opaque" regression.
// Two independent FillRuleNonZero draws at overlapping coordinates must
// each get their own stencil-write + color-pass pair — four DrawIndexed
// calls total, paired with four SetStencilReference(0) calls. If the
// batcher were to merge them (or the sprite pass were to collapse them),
// the color pass's DPPass=Zero would cause the first shape to consume
// the stencil in the overlap region and the second shape to silently
// drop at those pixels.
func TestSpritePassMultipleFillRuleBatchesCompositeIndependently(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	dev := &mockDevice{supportsStencil: true}
	sp, err := NewSpritePass(SpritePassConfig{
		Device:      dev,
		Batcher:     b,
		Pipeline:    &mockPipeline{},
		Shader:      &mockShader{uniforms: make(map[string]interface{})},
		MaxVertices: 1024,
		MaxIndices:  1024,
	})
	require.NoError(t, err)
	defer sp.Dispose()
	sp.ResolveTexture = func(_ uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	// Two identical-state NonZero draws, same texture, same blend. These
	// must stay separate — the batcher's merge predicate rejects any
	// non-None FillRule combination (see batch.go canMerge).
	for range 2 {
		b.Add(batch.DrawCommand{
			Vertices:  []batch.Vertex2D{{}, {}, {}},
			Indices:   []uint16{0, 1, 2},
			TextureID: 1,
			BlendMode: backend.BlendSourceOver,
			FillRule:  backend.FillRuleNonZero,
		})
	}

	enc := &mockEncoder{}
	ctx := NewPassContext(800, 600)
	ctx.ScreenHasStencil = true
	sp.Execute(enc, ctx)

	// Two independent stencil passes: write + color per batch = 4 draws,
	// 4 SetStencilReference calls.
	require.Len(t, enc.callsByMethod("DrawIndexed"), 4)
	require.Len(t, enc.callsByMethod("SetStencilReference"), 4)
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

	// Should have 2 BeginRenderPass calls (offscreen first so content is
	// ready when the screen pass samples it, then screen).
	begins := enc.callsByMethod("BeginRenderPass")
	require.Len(t, begins, 2)
	// First pass targets the mock RT (offscreen renders before screen).
	require.Equal(t, backend.RenderTarget(mockRT), begins[0].Args[0])
	// Second pass targets nil (screen, TargetID 0 sorts last).
	require.Nil(t, begins[1].Args[0])

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
	w, h       int
	hasStencil bool
}

func (rt *mockRenderTarget) ColorTexture() backend.Texture { return &mockTexture{w: rt.w, h: rt.h} }
func (rt *mockRenderTarget) DepthTexture() backend.Texture { return nil }
func (rt *mockRenderTarget) Width() int                    { return rt.w }
func (rt *mockRenderTarget) Height() int                   { return rt.h }
func (rt *mockRenderTarget) HasStencil() bool              { return rt.hasStencil }
func (rt *mockRenderTarget) Dispose()                      {}

func TestSpritePassTargetDims(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)

	// Screen target with no resolver: derives from sp.Projection.
	// sp.Projection = ortho(800, 600) → [0]=2/800, [5]=-2/600.
	sp.Projection = [16]float32{
		2.0 / 800, 0, 0, 0,
		0, -2.0 / 600, 0, 0,
		0, 0, -1, 0,
		-1, 1, 0, 1,
	}
	w, h := sp.targetDims(0)
	require.InDelta(t, 800.0, w, 1e-3)
	require.InDelta(t, 600.0, h, 1e-3)

	// Offscreen target with resolver: returns RT dimensions.
	sp.ResolveRenderTarget = func(id uint32) backend.RenderTarget {
		if id == 7 {
			return &mockRenderTarget{w: 320, h: 240}
		}
		return nil
	}
	w, h = sp.targetDims(7)
	require.InDelta(t, 320.0, w, 1e-3)
	require.InDelta(t, 240.0, h, 1e-3)

	// Unknown offscreen target: falls back to projection-derived screen dims.
	w, h = sp.targetDims(99)
	require.InDelta(t, 800.0, w, 1e-3)
	require.InDelta(t, 600.0, h, 1e-3)

	// No resolver and zero projection: returns zero.
	sp.ResolveRenderTarget = nil
	sp.Projection = [16]float32{}
	w, h = sp.targetDims(0)
	require.Equal(t, float32(0), w)
	require.Equal(t, float32(0), h)
}

func TestSpritePassBindKageImageUniforms(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	sp.Projection = [16]float32{
		2.0 / 1024, 0, 0, 0,
		0, -2.0 / 768, 0, 0,
		0, 0, -1, 0,
		-1, 1, 0, 1,
	}
	sp.ResolveTexture = func(id uint32) backend.Texture {
		switch id {
		case 1:
			return &mockTexture{w: 64, h: 32}
		case 2:
			return &mockTexture{w: 128, h: 256}
		}
		return nil
	}
	sp.ResolveRenderTarget = func(id uint32) backend.RenderTarget {
		if id == 5 {
			return &mockRenderTarget{w: 400, h: 300}
		}
		return nil
	}

	shader := &mockShader{uniforms: make(map[string]interface{})}
	batchObj := &batch.Batch{
		TextureID:       1,
		ExtraTextureIDs: [3]uint32{2, 0, 0},
	}

	// Offscreen target — uImageDstSize comes from RT, primary tex from
	// ResolveTexture(1), extra slot 1 from ResolveTexture(2).
	sp.bindKageImageUniforms(shader, 5, batchObj)
	require.Equal(t, [2]float32{0, 0}, shader.uniforms["uImageDstOrigin"])
	require.Equal(t, [2]float32{400, 300}, shader.uniforms["uImageDstSize"])
	require.Equal(t, [2]float32{0, 0}, shader.uniforms["uImageSrc0Origin"])
	require.Equal(t, [2]float32{64, 32}, shader.uniforms["uImageSrc0Size"])
	require.Equal(t, [2]float32{0, 0}, shader.uniforms["uImageSrc1Origin"])
	require.Equal(t, [2]float32{128, 256}, shader.uniforms["uImageSrc1Size"])
	// Unbound extra slots get zero size to keep the uniform layout
	// fully populated (Kage shaders that only use src0 still declare
	// src1/2/3 via the translator).
	require.Equal(t, [2]float32{0, 0}, shader.uniforms["uImageSrc2Size"])
	require.Equal(t, [2]float32{0, 0}, shader.uniforms["uImageSrc3Size"])

	// Screen target — uImageDstSize derives from sp.Projection.
	shader2 := &mockShader{uniforms: make(map[string]interface{})}
	sp.bindKageImageUniforms(shader2, 0, batchObj)
	require.Equal(t, [2]float32{1024, 768}, shader2.uniforms["uImageDstSize"])

	// Nil ResolveTexture is tolerated — only dst uniforms set.
	sp.ResolveTexture = nil
	shader3 := &mockShader{uniforms: make(map[string]interface{})}
	sp.bindKageImageUniforms(shader3, 5, batchObj)
	require.Equal(t, [2]float32{400, 300}, shader3.uniforms["uImageDstSize"])
	_, hasSrc0 := shader3.uniforms["uImageSrc0Size"]
	require.False(t, hasSrc0, "src uniforms must not be set when ResolveTexture is nil")
}

func TestSpritePassProjectionForTargetFallbacks(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	screen := [16]float32{1, 0, 0, 0, 0, 2, 0, 0, 0, 0, 3, 0, 0, 0, 0, 4}
	sp.Projection = screen

	// Screen target always returns sp.Projection.
	require.Equal(t, screen, sp.projectionForTarget(0))

	// Offscreen target with no resolver falls back to screen projection.
	require.Equal(t, screen, sp.projectionForTarget(5))

	// Offscreen target whose resolver returns nil also falls back.
	sp.ResolveRenderTarget = func(uint32) backend.RenderTarget { return nil }
	require.Equal(t, screen, sp.projectionForTarget(7))

	// Offscreen target with real RT: returns per-RT ortho.
	sp.ResolveRenderTarget = func(uint32) backend.RenderTarget {
		return &mockRenderTarget{w: 200, h: 100}
	}
	got := sp.projectionForTarget(9)
	require.InDelta(t, 2.0/200, got[0], 1e-6)
	require.InDelta(t, -2.0/100, got[5], 1e-6)
}

func TestSpritePassConsumePendingClear(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	sp.ResolveRenderTarget = func(uint32) backend.RenderTarget {
		return &mockRenderTarget{w: 128, h: 128}
	}
	sp.ResolveTexture = func(uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	var cleared uint32
	sp.ConsumePendingClear = func(id uint32) bool {
		if cleared != id {
			cleared = id
			return true
		}
		return false
	}

	b.Add(batch.DrawCommand{
		Vertices: []batch.Vertex2D{{}, {}, {}},
		Indices:  []uint16{0, 1, 2}, TextureID: 1, TargetID: 4,
	})

	enc := &mockEncoder{}
	sp.Execute(enc, NewPassContext(800, 600))

	begins := enc.callsByMethod("BeginRenderPass")
	require.Len(t, begins, 1)
	require.Equal(t, backend.LoadActionClear, begins[0].Args[1])
	require.Equal(t, uint32(4), cleared)
}

func TestSpritePassCustomShaderBatch(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)

	customShader := &mockShader{uniforms: make(map[string]interface{})}
	customPipe := &mockPipeline{}
	sp.ResolveShader = func(id uint32) *ShaderInfo {
		if id == 99 {
			return &ShaderInfo{Shader: customShader, Pipeline: customPipe}
		}
		return nil
	}
	sp.ResolveTexture = func(id uint32) backend.Texture {
		switch id {
		case 1, 2:
			return &mockTexture{w: 32, h: 32}
		}
		return nil
	}

	applied := false
	sp.ApplyUniforms = func(_ backend.Shader, u map[string]any) {
		applied = true
		require.Equal(t, float32(3.14), u["k"])
	}

	// Custom shader batch with extra texture + per-draw uniforms + non-default blend.
	b.Add(batch.DrawCommand{
		Vertices:        []batch.Vertex2D{{}, {}, {}},
		Indices:         []uint16{0, 1, 2},
		TextureID:       1,
		ExtraTextureIDs: [3]uint32{2, 0, 0},
		ShaderID:        99,
		BlendMode:       backend.BlendAdditive,
		Uniforms:        map[string]any{"k": float32(3.14)},
	})

	enc := &mockEncoder{}
	sp.Execute(enc, NewPassContext(800, 600))

	require.True(t, applied, "ApplyUniforms should be called for custom-shader batch")
	// Both primary (slot 0) and extra (slot 1) textures must be bound.
	texCalls := enc.callsByMethod("SetTexture")
	require.GreaterOrEqual(t, len(texCalls), 2)
	// Extra texture should bind at slot 1.
	slots := make(map[int]bool)
	for _, c := range texCalls {
		slots[c.Args[0].(int)] = true
	}
	require.True(t, slots[0], "primary texture must bind at slot 0")
	require.True(t, slots[1], "extra texture must bind at slot 1")
}

func TestSpritePassUnknownShaderFallsBackToDefault(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	sp.ResolveTexture = func(uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }
	// Resolver returns nil for every ID — the batch's non-zero ShaderID
	// should fall through to the default pipeline, not crash.
	sp.ResolveShader = func(uint32) *ShaderInfo { return nil }

	b.Add(batch.DrawCommand{
		Vertices: []batch.Vertex2D{{}, {}, {}},
		Indices:  []uint16{0, 1, 2}, TextureID: 1, ShaderID: 42,
		BlendMode: backend.BlendAdditive,
	})
	enc := &mockEncoder{}
	sp.Execute(enc, NewPassContext(800, 600))

	draws := enc.callsByMethod("DrawIndexed")
	require.Len(t, draws, 1)
}

func TestSpritePassFilterChange(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)
	sp.ResolveTexture = func(uint32) backend.Texture { return &mockTexture{w: 1, h: 1} }

	b.Add(batch.DrawCommand{
		Vertices: []batch.Vertex2D{{}, {}, {}},
		Indices:  []uint16{0, 1, 2}, TextureID: 1, Filter: backend.FilterNearest,
	})
	b.Add(batch.DrawCommand{
		Vertices: []batch.Vertex2D{{}, {}, {}},
		Indices:  []uint16{0, 1, 2}, TextureID: 1, Filter: backend.FilterLinear,
	})

	enc := &mockEncoder{}
	sp.Execute(enc, NewPassContext(800, 600))

	// Filter changes once (Nearest → Linear). The first batch may or may
	// not emit SetTextureFilter depending on the default, so only require
	// that a change was observed.
	filterCalls := enc.callsByMethod("SetTextureFilter")
	require.NotEmpty(t, filterCalls)
}

// --- Dynamic buffer growth ---
//
// The sprite pass starts with fixed-size vertex/index GPU buffers and
// grows them when a frame's accumulated geometry exceeds capacity.
// Without this, WebGPU rejects oversize uploads with "Write range
// does not fit in Buffer size" and the whole frame renders empty —
// see particle-garden, which routinely pushes >65k vertices.

func TestSpritePassGrowVertexBufferNoOpWhenUnderCapacity(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)

	oldBuf := sp.vertexBuf
	require.NoError(t, sp.growVertexBufferIfNeeded(sp.vertexBufVerts))
	require.Same(t, oldBuf, sp.vertexBuf, "buffer should not be replaced when cap is sufficient")
	require.NoError(t, sp.growVertexBufferIfNeeded(sp.vertexBufVerts-10))
	require.Same(t, oldBuf, sp.vertexBuf)
}

func TestSpritePassGrowVertexBufferDoublesCapacity(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	// newTestSpritePass starts the vertex buffer at 1024 capacity.
	sp := newTestSpritePass(t, b)

	oldBuf := sp.vertexBuf.(*mockBuffer)
	require.NoError(t, sp.growVertexBufferIfNeeded(1025))

	require.True(t, oldBuf.disposed, "old vertex buffer should be disposed on grow")
	require.NotSame(t, backend.Buffer(oldBuf), sp.vertexBuf)
	require.Equal(t, 2048, sp.vertexBufVerts, "should double when 2×current >= needed")
	require.Equal(t, 2048*batch.Vertex2DSize, sp.vertexBuf.(*mockBuffer).size)
}

func TestSpritePassGrowVertexBufferSnapsToNeededWhenDoubleIsSmaller(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	// newTestSpritePass starts the vertex buffer at 1024 capacity.
	sp := newTestSpritePass(t, b)

	// needed > 2×cap → use needed exactly, don't leave us still short.
	require.NoError(t, sp.growVertexBufferIfNeeded(5000))
	require.Equal(t, 5000, sp.vertexBufVerts)
}

func TestSpritePassGrowIndexBufferDoublesCapacity(t *testing.T) {
	b := batch.NewBatcher(1024, 1024)
	sp := newTestSpritePass(t, b)

	oldBuf := sp.indexBuf.(*mockBuffer)
	require.NoError(t, sp.growIndexBufferIfNeeded(1025))

	require.True(t, oldBuf.disposed)
	require.NotSame(t, backend.Buffer(oldBuf), sp.indexBuf)
	require.Equal(t, 2048, sp.indexBufIndices)
	require.Equal(t, 2048*4, sp.indexBuf.(*mockBuffer).size)
}

func TestSpritePassGrowVertexBufferAllocFailureLeavesOldBufferIntact(t *testing.T) {
	// failingDevice fails on the Nth NewBuffer call. The initial
	// SpritePass allocation uses calls 1 and 2 (vertex + index), so we
	// fail on call 3 — the grow attempt.
	callCount := 0
	dev := &failingDevice{failOn: 3, callCount: &callCount}
	sp, err := NewSpritePass(SpritePassConfig{
		Device:      dev,
		Batcher:     batch.NewBatcher(1024, 1024),
		Pipeline:    &mockPipeline{},
		Shader:      &mockShader{uniforms: make(map[string]interface{})},
		MaxVertices: 1024,
		MaxIndices:  1024,
	})
	require.NoError(t, err)
	defer sp.Dispose()

	oldBuf := sp.vertexBuf.(*mockBuffer)
	require.Error(t, sp.growVertexBufferIfNeeded(2048),
		"grow must surface the backend's allocation failure")
	require.Same(t, backend.Buffer(oldBuf), sp.vertexBuf,
		"old buffer must stay bound when allocation fails so the pass can still render")
	require.False(t, oldBuf.disposed,
		"old buffer must NOT be disposed when allocation fails — we still need it")
	require.Equal(t, 1024, sp.vertexBufVerts, "capacity stays unchanged on failure")
}

func TestSpritePassGrowWithNilDeviceIsNoOp(t *testing.T) {
	// Some construction paths (tests, certain soft fallbacks) may not
	// wire a device. The grow path must tolerate that rather than
	// panicking — it just won't grow.
	sp := &SpritePass{
		vertexBufVerts:  100,
		indexBufIndices: 100,
	}
	require.NoError(t, sp.growVertexBufferIfNeeded(1000))
	require.NoError(t, sp.growIndexBufferIfNeeded(1000))
	require.Equal(t, 100, sp.vertexBufVerts, "capacity unchanged when device is nil")
	require.Equal(t, 100, sp.indexBufIndices)
}

func TestSpritePassExecuteGrowsBuffersOnOverflow(t *testing.T) {
	// Starting capacity: 4 vertices, 6 indices. Three quads would need
	// 12 verts / 18 indices — forces both buffers to grow during Execute.
	dev := &mockDevice{}
	b := batch.NewBatcher(1024, 1024)
	sp, err := NewSpritePass(SpritePassConfig{
		Device:      dev,
		Batcher:     b,
		Pipeline:    &mockPipeline{},
		Shader:      &mockShader{uniforms: make(map[string]interface{})},
		MaxVertices: 4,
		MaxIndices:  6,
	})
	require.NoError(t, err)
	defer sp.Dispose()

	origVertexBuf := sp.vertexBuf
	origIndexBuf := sp.indexBuf

	// Three quads.
	for range 3 {
		b.Add(batch.DrawCommand{
			Vertices: []batch.Vertex2D{{}, {}, {}, {}},
			Indices:  []uint16{0, 1, 2, 0, 2, 3},
		})
	}

	enc := &mockEncoder{}
	require.NotPanics(t, func() {
		sp.Execute(enc, NewPassContext(800, 600))
	})

	require.NotSame(t, origVertexBuf, sp.vertexBuf, "Execute must grow the vertex buffer when overflow is detected")
	require.NotSame(t, origIndexBuf, sp.indexBuf, "Execute must grow the index buffer when overflow is detected")
	require.GreaterOrEqual(t, sp.vertexBufVerts, 12)
	require.GreaterOrEqual(t, sp.indexBufIndices, 18)
}

func TestIndexSliceToBytesU32(t *testing.T) {
	require.Nil(t, indexSliceToBytesU32(nil))
	require.Nil(t, indexSliceToBytesU32([]uint32{}))
	got := indexSliceToBytesU32([]uint32{1, 2})
	require.Len(t, got, 8)
}

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
