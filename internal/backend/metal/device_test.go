package metal

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/backend/conformance"
)

func newTestDevice(t *testing.T) (*Device, backend.CommandEncoder) {
	t.Helper()
	dev := New()
	require.NoError(t, dev.Init(backend.DeviceConfig{
		Width:  conformance.SceneSize,
		Height: conformance.SceneSize,
	}))
	t.Cleanup(func() { dev.Dispose() })
	enc := dev.Encoder()
	return dev, enc
}

// TestConformanceMetal runs the full conformance suite against the Metal backend.
func TestConformanceMetal(t *testing.T) {
	dev, enc := newTestDevice(t)
	conformance.RunAll(t, dev, enc)
}

func TestDeviceInit(t *testing.T) {
	dev := New()
	require.NoError(t, dev.Init(backend.DeviceConfig{Width: 100, Height: 100}))
	dev.Dispose()
}

func TestDeviceInitInvalidDimensions(t *testing.T) {
	dev := New()
	require.Error(t, dev.Init(backend.DeviceConfig{Width: 0, Height: 100}))
	require.Error(t, dev.Init(backend.DeviceConfig{Width: 100, Height: -1}))
}

func TestDeviceBeginEndFrame(t *testing.T) {
	dev, _ := newTestDevice(t)
	dev.BeginFrame()
	dev.EndFrame()
}

func TestDeviceCapabilities(t *testing.T) {
	dev, _ := newTestDevice(t)
	caps := dev.Capabilities()
	require.Equal(t, 16384, caps.MaxTextureSize)
	require.Equal(t, 8, caps.MaxRenderTargets)
	require.True(t, caps.SupportsInstanced)
	require.True(t, caps.SupportsCompute)
	require.True(t, caps.SupportsMSAA)
	require.Equal(t, 8, caps.MaxMSAASamples)
	require.True(t, caps.SupportsFloat16)
}

func TestNewTexture(t *testing.T) {
	dev, _ := newTestDevice(t)
	tex, err := dev.NewTexture(backend.TextureDescriptor{
		Width: 16, Height: 16,
		Format: backend.TextureFormatRGBA8,
		Data:   make([]byte, 16*16*4),
	})
	require.NoError(t, err)
	require.Equal(t, 16, tex.Width())
	require.Equal(t, 16, tex.Height())
	tex.Dispose()
}

func TestNewTextureInvalidDimensions(t *testing.T) {
	dev, _ := newTestDevice(t)
	_, err := dev.NewTexture(backend.TextureDescriptor{Width: 0, Height: 10})
	require.Error(t, err)
}

func TestNewBuffer(t *testing.T) {
	dev, _ := newTestDevice(t)
	buf, err := dev.NewBuffer(backend.BufferDescriptor{Data: []byte{1, 2, 3, 4}})
	require.NoError(t, err)
	require.Equal(t, 4, buf.Size())
	buf.Dispose()
}

func TestNewBufferInvalidSize(t *testing.T) {
	dev, _ := newTestDevice(t)
	_, err := dev.NewBuffer(backend.BufferDescriptor{Size: 0})
	require.Error(t, err)
}

func TestNewShader(t *testing.T) {
	dev, _ := newTestDevice(t)
	shader, err := dev.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)
	shader.SetUniformFloat("test", 1.0)
	shader.SetUniformVec2("v2", [2]float32{1, 2})
	shader.SetUniformVec4("v4", [4]float32{1, 2, 3, 4})
	shader.SetUniformMat4("m4", [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})
	shader.SetUniformInt("i", 42)
	shader.SetUniformBlock("block", []byte{1, 2, 3, 4})
	shader.Dispose()
}

func TestNewRenderTarget(t *testing.T) {
	dev, _ := newTestDevice(t)
	rt, err := dev.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 32, Height: 32,
		ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)
	require.Equal(t, 32, rt.Width())
	require.Equal(t, 32, rt.Height())
	require.NotNil(t, rt.ColorTexture())
	rt.Dispose()
}

func TestNewRenderTargetInvalidDimensions(t *testing.T) {
	dev, _ := newTestDevice(t)
	_, err := dev.NewRenderTarget(backend.RenderTargetDescriptor{Width: 0, Height: 32})
	require.Error(t, err)
}

func TestNewRenderTargetWithDepth(t *testing.T) {
	dev, _ := newTestDevice(t)
	rt, err := dev.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 32, Height: 32,
		ColorFormat: backend.TextureFormatRGBA8,
		HasDepth:    true,
		DepthFormat: backend.TextureFormatDepth32F,
	})
	require.NoError(t, err)
	require.NotNil(t, rt.DepthTexture())
	rt.Dispose()
}

func TestNewPipeline(t *testing.T) {
	dev, _ := newTestDevice(t)
	shader, err := dev.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)
	pipeline, err := dev.NewPipeline(backend.PipelineDescriptor{
		Shader:    shader,
		BlendMode: backend.BlendSourceOver,
	})
	require.NoError(t, err)
	pipeline.Dispose()
	shader.Dispose()
}

func TestTextureUploadAndRead(t *testing.T) {
	dev, _ := newTestDevice(t)
	data := []byte{255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 255, 255, 0, 255}
	tex, err := dev.NewTexture(backend.TextureDescriptor{
		Width: 2, Height: 2,
		Format: backend.TextureFormatRGBA8,
		Data:   data,
	})
	require.NoError(t, err)
	dst := make([]byte, 16)
	tex.ReadPixels(dst)
	require.Equal(t, data, dst)
	tex.Upload(make([]byte, 16), 0)
	tex.UploadRegion([]byte{1, 2, 3, 4}, 0, 0, 1, 1, 0)
	tex.Dispose()
}

func TestBufferUploadAndRegion(t *testing.T) {
	dev, _ := newTestDevice(t)
	buf, err := dev.NewBuffer(backend.BufferDescriptor{Data: []byte{1, 2, 3, 4}})
	require.NoError(t, err)
	buf.Upload([]byte{5, 6, 7, 8})
	buf.UploadRegion([]byte{9, 10}, 2)
	require.Equal(t, 4, buf.Size())
	buf.Dispose()
}

func TestEncoderFullPass(t *testing.T) {
	dev, enc := newTestDevice(t)
	rt, err := dev.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 32, Height: 32, ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)
	defer rt.Dispose()

	enc.BeginRenderPass(backend.RenderPassDescriptor{
		Target: rt, LoadAction: backend.LoadActionClear, ClearColor: [4]float32{0, 0, 0, 1},
	})
	enc.SetTextureFilter(0, backend.FilterLinear)
	enc.SetStencil(false, backend.StencilDescriptor{})
	enc.SetColorWrite(true)
	enc.SetViewport(backend.Viewport{X: 0, Y: 0, Width: 32, Height: 32})
	enc.SetScissor(nil)
	enc.EndRenderPass()
	enc.Flush()
}

func TestFeatureSetConstants(t *testing.T) {
	require.Less(t, FeatureSetMacFamily1v1, FeatureSetMacFamily1v2)
	require.Less(t, FeatureSetMacFamily1v2, FeatureSetMacFamily2v1)
}
