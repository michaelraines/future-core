package softdelegate

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/backend/soft"
)

// wrapperPipeline implements PipelineUnwrapper.
type wrapperPipeline struct {
	backend.Pipeline
}

func (w *wrapperPipeline) InnerPipeline() backend.Pipeline { return w.Pipeline }

// wrapperBuffer implements BufferUnwrapper.
type wrapperBuffer struct {
	backend.Buffer
}

func (w *wrapperBuffer) InnerBuffer() backend.Buffer { return w.Buffer }

// wrapperTexture implements TextureUnwrapper.
type wrapperTexture struct {
	backend.Texture
}

func (w *wrapperTexture) InnerTexture() backend.Texture { return w.Texture }

// wrapperRenderTarget implements RenderTargetUnwrapper.
type wrapperRenderTarget struct {
	backend.RenderTarget
}

func (w *wrapperRenderTarget) InnerRenderTarget() backend.RenderTarget { return w.RenderTarget }

func newTestEncoder(t *testing.T) (*Encoder, *soft.Device) {
	t.Helper()
	dev := soft.New()
	require.NoError(t, dev.Init(backend.DeviceConfig{Width: 64, Height: 64}))
	t.Cleanup(func() { dev.Dispose() })
	return &Encoder{Inner: dev.Encoder()}, dev
}

func TestEncoderBeginEndRenderPass(t *testing.T) {
	enc, dev := newTestEncoder(t)
	rt, err := dev.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 64, Height: 64, ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)
	defer rt.Dispose()

	// Direct render target (no wrapper).
	enc.BeginRenderPass(backend.RenderPassDescriptor{Target: rt})
	enc.EndRenderPass()

	// Wrapped render target.
	wrapped := &wrapperRenderTarget{RenderTarget: rt}
	enc.BeginRenderPass(backend.RenderPassDescriptor{Target: wrapped})
	enc.EndRenderPass()
}

func TestEncoderSetPipelineUnwrap(t *testing.T) {
	enc, dev := newTestEncoder(t)
	sh, err := dev.NewShader(backend.ShaderDescriptor{
		VertexSource: "v", FragmentSource: "f",
	})
	require.NoError(t, err)
	defer sh.Dispose()

	pip, err := dev.NewPipeline(backend.PipelineDescriptor{Shader: sh})
	require.NoError(t, err)
	defer pip.Dispose()

	// Direct pipeline.
	enc.SetPipeline(pip)

	// Wrapped pipeline.
	wrapped := &wrapperPipeline{Pipeline: pip}
	enc.SetPipeline(wrapped)
}

func TestEncoderSetVertexBufferUnwrap(t *testing.T) {
	enc, dev := newTestEncoder(t)
	buf, err := dev.NewBuffer(backend.BufferDescriptor{
		Size: 64, Usage: backend.BufferUsageVertex,
	})
	require.NoError(t, err)
	defer buf.Dispose()

	enc.SetVertexBuffer(buf, 0)
	enc.SetVertexBuffer(&wrapperBuffer{Buffer: buf}, 0)
}

func TestEncoderSetIndexBufferUnwrap(t *testing.T) {
	enc, dev := newTestEncoder(t)
	buf, err := dev.NewBuffer(backend.BufferDescriptor{
		Size: 64, Usage: backend.BufferUsageIndex,
	})
	require.NoError(t, err)
	defer buf.Dispose()

	enc.SetIndexBuffer(buf, backend.IndexUint16)
	enc.SetIndexBuffer(&wrapperBuffer{Buffer: buf}, backend.IndexUint16)
}

func TestEncoderSetTextureUnwrap(t *testing.T) {
	enc, dev := newTestEncoder(t)
	tex, err := dev.NewTexture(backend.TextureDescriptor{
		Width: 8, Height: 8, Format: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)
	defer tex.Dispose()

	enc.SetTexture(tex, 0)
	enc.SetTexture(&wrapperTexture{Texture: tex}, 0)
}

func TestEncoderPassthroughMethods(t *testing.T) {
	enc, _ := newTestEncoder(t)

	// These methods are pure passthrough — just verify they don't panic.
	enc.SetTextureFilter(0, backend.FilterLinear)
	enc.SetStencil(false, backend.StencilDescriptor{})
	enc.SetColorWrite(true)
	enc.SetViewport(backend.Viewport{X: 0, Y: 0, Width: 64, Height: 64})
	enc.SetScissor(nil)
	enc.Draw(3, 1, 0)
	enc.DrawIndexed(6, 1, 0)
	enc.Flush()
}
