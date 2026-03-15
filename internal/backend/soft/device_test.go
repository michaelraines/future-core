package soft

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/backend"
)

func validConfig() backend.DeviceConfig {
	return backend.DeviceConfig{Width: 800, Height: 600}
}

func initDevice(t *testing.T) *Device {
	t.Helper()
	d := New()
	require.NoError(t, d.Init(validConfig()))
	return d
}

// --- Device tests ---

func TestDeviceInit(t *testing.T) {
	d := New()
	err := d.Init(validConfig())
	require.NoError(t, err)
	require.True(t, d.inited)
}

func TestDeviceInitInvalidDimensions(t *testing.T) {
	tests := []struct {
		name string
		cfg  backend.DeviceConfig
	}{
		{"zero width", backend.DeviceConfig{Width: 0, Height: 600}},
		{"zero height", backend.DeviceConfig{Width: 800, Height: 0}},
		{"negative width", backend.DeviceConfig{Width: -1, Height: 600}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := New()
			err := d.Init(tt.cfg)
			require.Error(t, err)
		})
	}
}

func TestDeviceDispose(t *testing.T) {
	d := initDevice(t)
	d.Dispose()
	require.False(t, d.inited)
}

func TestDeviceBeginEndFrame(t *testing.T) {
	d := initDevice(t)
	d.BeginFrame()
	d.EndFrame()
}

func TestDeviceCapabilities(t *testing.T) {
	d := initDevice(t)
	caps := d.Capabilities()
	require.Equal(t, 8192, caps.MaxTextureSize)
	require.Equal(t, 8, caps.MaxRenderTargets)
	require.True(t, caps.SupportsInstanced)
	require.False(t, caps.SupportsCompute)
	require.False(t, caps.SupportsMSAA)
	require.True(t, caps.SupportsFloat16)
}

func TestDeviceEncoder(t *testing.T) {
	d := initDevice(t)
	enc := d.Encoder()
	require.NotNil(t, enc)
}

// --- Texture tests ---

func TestNewTexture(t *testing.T) {
	d := initDevice(t)
	tex, err := d.NewTexture(backend.TextureDescriptor{
		Width:  64,
		Height: 64,
		Format: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)
	require.NotNil(t, tex)
	require.Equal(t, 64, tex.Width())
	require.Equal(t, 64, tex.Height())
	require.Equal(t, backend.TextureFormatRGBA8, tex.Format())
}

func TestNewTextureWithData(t *testing.T) {
	d := initDevice(t)
	data := make([]byte, 4*4*4) // 4x4 RGBA
	for i := range data {
		data[i] = 0xAB
	}
	tex, err := d.NewTexture(backend.TextureDescriptor{
		Width:  4,
		Height: 4,
		Format: backend.TextureFormatRGBA8,
		Data:   data,
	})
	require.NoError(t, err)
	st := tex.(*Texture)
	require.Equal(t, data, st.Pixels())
}

func TestNewTextureWithImage(t *testing.T) {
	d := initDevice(t)
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 1, color.RGBA{G: 255, A: 255})

	tex, err := d.NewTexture(backend.TextureDescriptor{
		Width:  2,
		Height: 2,
		Format: backend.TextureFormatRGBA8,
		Image:  img,
	})
	require.NoError(t, err)
	st := tex.(*Texture)
	require.Equal(t, img.Pix[:len(st.Pixels())], st.Pixels())
}

func TestNewTextureInvalidDimensions(t *testing.T) {
	d := initDevice(t)
	_, err := d.NewTexture(backend.TextureDescriptor{Width: 0, Height: 10})
	require.Error(t, err)
}

func TestTextureUpload(t *testing.T) {
	d := initDevice(t)
	tex, err := d.NewTexture(backend.TextureDescriptor{
		Width: 2, Height: 2, Format: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	tex.Upload(data, 0)
	dst := make([]byte, 16)
	tex.ReadPixels(dst)
	require.Equal(t, data, dst)
}

func TestTextureUploadRegion(t *testing.T) {
	d := initDevice(t)
	tex, err := d.NewTexture(backend.TextureDescriptor{
		Width: 4, Height: 4, Format: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)

	// Upload a 2x2 red region at offset (1,1)
	red := []byte{255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255}
	tex.UploadRegion(red, 1, 1, 2, 2, 0)

	st := tex.(*Texture)
	// Check pixel at (1,1): offset = (1*4+1)*4 = 20
	require.Equal(t, byte(255), st.Pixels()[20])
	require.Equal(t, byte(0), st.Pixels()[21])
	require.Equal(t, byte(0), st.Pixels()[22])
	require.Equal(t, byte(255), st.Pixels()[23])
}

func TestTextureDispose(t *testing.T) {
	d := initDevice(t)
	tex, err := d.NewTexture(backend.TextureDescriptor{
		Width: 4, Height: 4, Format: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)
	tex.Dispose()
	st := tex.(*Texture)
	require.True(t, st.disposed)
	require.Nil(t, st.Pixels())
}

// --- Buffer tests ---

func TestNewBuffer(t *testing.T) {
	d := initDevice(t)
	buf, err := d.NewBuffer(backend.BufferDescriptor{Size: 256})
	require.NoError(t, err)
	require.Equal(t, 256, buf.Size())
}

func TestNewBufferWithData(t *testing.T) {
	d := initDevice(t)
	data := []byte{1, 2, 3, 4}
	buf, err := d.NewBuffer(backend.BufferDescriptor{Data: data})
	require.NoError(t, err)
	sb := buf.(*Buffer)
	require.Equal(t, data, sb.Data())
}

func TestNewBufferInvalidSize(t *testing.T) {
	d := initDevice(t)
	_, err := d.NewBuffer(backend.BufferDescriptor{Size: 0})
	require.Error(t, err)
}

func TestBufferUpload(t *testing.T) {
	d := initDevice(t)
	buf, err := d.NewBuffer(backend.BufferDescriptor{Size: 4})
	require.NoError(t, err)
	buf.Upload([]byte{10, 20, 30, 40})
	sb := buf.(*Buffer)
	require.Equal(t, []byte{10, 20, 30, 40}, sb.Data())
}

func TestBufferUploadRegion(t *testing.T) {
	d := initDevice(t)
	buf, err := d.NewBuffer(backend.BufferDescriptor{Size: 8})
	require.NoError(t, err)
	buf.UploadRegion([]byte{99, 98}, 2)
	sb := buf.(*Buffer)
	require.Equal(t, byte(99), sb.Data()[2])
	require.Equal(t, byte(98), sb.Data()[3])
}

func TestBufferDispose(t *testing.T) {
	d := initDevice(t)
	buf, err := d.NewBuffer(backend.BufferDescriptor{Size: 16})
	require.NoError(t, err)
	buf.Dispose()
	sb := buf.(*Buffer)
	require.True(t, sb.disposed)
	require.Nil(t, sb.Data())
}

// --- Shader tests ---

func TestNewShader(t *testing.T) {
	d := initDevice(t)
	s, err := d.NewShader(backend.ShaderDescriptor{
		VertexSource:   "vertex",
		FragmentSource: "fragment",
	})
	require.NoError(t, err)
	require.NotNil(t, s)
}

func TestShaderUniforms(t *testing.T) {
	d := initDevice(t)
	s, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)

	ss := s.(*Shader)

	s.SetUniformFloat("f", 1.5)
	v, ok := ss.Uniform("f")
	require.True(t, ok)
	require.Equal(t, float32(1.5), v)

	s.SetUniformVec2("v2", [2]float32{1, 2})
	v, ok = ss.Uniform("v2")
	require.True(t, ok)
	require.Equal(t, [2]float32{1, 2}, v)

	s.SetUniformVec4("v4", [4]float32{1, 2, 3, 4})
	v, ok = ss.Uniform("v4")
	require.True(t, ok)
	require.Equal(t, [4]float32{1, 2, 3, 4}, v)

	s.SetUniformInt("i", 42)
	v, ok = ss.Uniform("i")
	require.True(t, ok)
	require.Equal(t, int32(42), v)

	mat := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	s.SetUniformMat4("m", mat)
	v, ok = ss.Uniform("m")
	require.True(t, ok)
	require.Equal(t, mat, v)

	blockData := []byte{10, 20, 30}
	s.SetUniformBlock("blk", blockData)
	v, ok = ss.Uniform("blk")
	require.True(t, ok)
	require.Equal(t, blockData, v)
}

func TestShaderDispose(t *testing.T) {
	d := initDevice(t)
	s, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)
	s.Dispose()
	ss := s.(*Shader)
	require.True(t, ss.disposed)
	require.Nil(t, ss.uniforms)
}

func TestShaderDisposeUniformsNoPanic(t *testing.T) {
	d := initDevice(t)
	s, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)

	// Dispose sets uniforms to nil.
	s.Dispose()

	// All uniform setters should return early without panicking.
	require.NotPanics(t, func() {
		s.SetUniformFloat("f", 1.0)
	})
	require.NotPanics(t, func() {
		s.SetUniformVec4("v", [4]float32{1, 2, 3, 4})
	})
	require.NotPanics(t, func() {
		s.SetUniformMat4("m", [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})
	})
	require.NotPanics(t, func() {
		s.SetUniformVec2("v2", [2]float32{1, 2})
	})
	require.NotPanics(t, func() {
		s.SetUniformInt("i", 42)
	})
	require.NotPanics(t, func() {
		s.SetUniformBlock("blk", []byte{1, 2, 3})
	})
}

// --- Pipeline tests ---

func TestNewPipeline(t *testing.T) {
	d := initDevice(t)
	s, err := d.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)

	p, err := d.NewPipeline(backend.PipelineDescriptor{
		Shader:    s,
		BlendMode: backend.BlendSourceOver,
		DepthTest: true,
		CullMode:  backend.CullBack,
	})
	require.NoError(t, err)
	require.NotNil(t, p)

	sp := p.(*Pipeline)
	require.Equal(t, backend.BlendSourceOver, sp.Desc().BlendMode)
	require.True(t, sp.Desc().DepthTest)
}

func TestPipelineDispose(t *testing.T) {
	d := initDevice(t)
	p, err := d.NewPipeline(backend.PipelineDescriptor{})
	require.NoError(t, err)
	p.Dispose()
	sp := p.(*Pipeline)
	require.True(t, sp.disposed)
}

// --- RenderTarget tests ---

func TestNewRenderTarget(t *testing.T) {
	d := initDevice(t)
	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width:       256,
		Height:      256,
		ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)
	require.Equal(t, 256, rt.Width())
	require.Equal(t, 256, rt.Height())
	require.NotNil(t, rt.ColorTexture())
	require.Nil(t, rt.DepthTexture())
}

func TestNewRenderTargetWithDepth(t *testing.T) {
	d := initDevice(t)
	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width:       128,
		Height:      128,
		ColorFormat: backend.TextureFormatRGBA8,
		HasDepth:    true,
		DepthFormat: backend.TextureFormatDepth24,
	})
	require.NoError(t, err)
	require.NotNil(t, rt.DepthTexture())
}

func TestNewRenderTargetInvalidDimensions(t *testing.T) {
	d := initDevice(t)
	_, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width: 0, Height: 128, ColorFormat: backend.TextureFormatRGBA8,
	})
	require.Error(t, err)
}

func TestRenderTargetDispose(t *testing.T) {
	d := initDevice(t)
	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width:       64,
		Height:      64,
		ColorFormat: backend.TextureFormatRGBA8,
		HasDepth:    true,
		DepthFormat: backend.TextureFormatDepth24,
	})
	require.NoError(t, err)
	rt.Dispose()
	srt := rt.(*RenderTarget)
	require.True(t, srt.disposed)
	require.True(t, srt.color.disposed)
}

// --- Encoder tests ---

func TestEncoderBeginEndRenderPass(t *testing.T) {
	d := initDevice(t)
	enc := d.Encoder().(*Encoder)

	require.False(t, enc.InPass())
	enc.BeginRenderPass(backend.RenderPassDescriptor{})
	require.True(t, enc.InPass())
	enc.EndRenderPass()
	require.False(t, enc.InPass())
}

func TestEncoderClearOnBegin(t *testing.T) {
	d := initDevice(t)
	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width:       4,
		Height:      4,
		ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)

	enc := d.Encoder().(*Encoder)
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		Target:     rt,
		ClearColor: [4]float32{1.0, 0.0, 0.0, 1.0},
		LoadAction: backend.LoadActionClear,
	})

	srt := rt.(*RenderTarget)
	// Check first pixel is red.
	require.Equal(t, byte(255), srt.color.Pixels()[0])
	require.Equal(t, byte(0), srt.color.Pixels()[1])
	require.Equal(t, byte(0), srt.color.Pixels()[2])
	require.Equal(t, byte(255), srt.color.Pixels()[3])
	enc.EndRenderPass()
}

func TestEncoderDrawCalls(t *testing.T) {
	d := initDevice(t)
	enc := d.Encoder().(*Encoder)

	enc.BeginRenderPass(backend.RenderPassDescriptor{})
	enc.Draw(6, 1, 0)
	enc.DrawIndexed(12, 1, 0)
	enc.EndRenderPass()

	draws := enc.Draws()
	require.Len(t, draws, 2)
	require.False(t, draws[0].Indexed)
	require.Equal(t, 6, draws[0].VertexCount)
	require.True(t, draws[1].Indexed)
	require.Equal(t, 12, draws[1].IndexCount)
}

func TestEncoderResetDraws(t *testing.T) {
	d := initDevice(t)
	enc := d.Encoder().(*Encoder)
	enc.Draw(3, 1, 0)
	require.Len(t, enc.Draws(), 1)
	enc.ResetDraws()
	require.Empty(t, enc.Draws())
}

func TestEncoderSetPipeline(t *testing.T) {
	d := initDevice(t)
	enc := d.Encoder().(*Encoder)
	p, err := d.NewPipeline(backend.PipelineDescriptor{})
	require.NoError(t, err)
	enc.SetPipeline(p)
	require.True(t, enc.pipelineBound)
}

func TestEncoderSetViewport(t *testing.T) {
	d := initDevice(t)
	enc := d.Encoder().(*Encoder)
	vp := backend.Viewport{X: 10, Y: 20, Width: 100, Height: 200}
	enc.SetViewport(vp)
	require.Equal(t, vp, enc.viewport)
}

func TestEncoderSetScissor(t *testing.T) {
	d := initDevice(t)
	enc := d.Encoder().(*Encoder)
	rect := &backend.ScissorRect{X: 5, Y: 5, Width: 50, Height: 50}
	enc.SetScissor(rect)
	require.Equal(t, rect, enc.scissor)
	enc.SetScissor(nil)
	require.Nil(t, enc.scissor)
}

func TestEncoderSetStencil(t *testing.T) {
	d := initDevice(t)
	enc := d.Encoder().(*Encoder)
	enc.SetStencil(true, backend.StencilDescriptor{})
	require.True(t, enc.stencil)
	enc.SetStencil(false, backend.StencilDescriptor{})
	require.False(t, enc.stencil)
}

func TestEncoderSetColorWrite(t *testing.T) {
	d := initDevice(t)
	enc := d.Encoder().(*Encoder)
	enc.SetColorWrite(false)
	require.False(t, enc.colorWrite)
	enc.SetColorWrite(true)
	require.True(t, enc.colorWrite)
}

func TestEncoderNoOpMethods(t *testing.T) {
	d := initDevice(t)
	enc := d.Encoder()

	buf, err := d.NewBuffer(backend.BufferDescriptor{Size: 64})
	require.NoError(t, err)
	tex, err := d.NewTexture(backend.TextureDescriptor{Width: 4, Height: 4, Format: backend.TextureFormatRGBA8})
	require.NoError(t, err)

	// These should not panic.
	enc.SetVertexBuffer(buf, 0)
	enc.SetIndexBuffer(buf, backend.IndexUint16)
	enc.SetTexture(tex, 0)
	enc.SetTextureFilter(0, backend.FilterLinear)
	enc.Flush()
}

// --- Registration test ---

func TestSoftRegistered(t *testing.T) {
	require.True(t, backend.IsRegistered("soft"))

	dev, err := backend.Create("soft")
	require.NoError(t, err)
	require.IsType(t, &Device{}, dev)
}

// --- bytesPerPixel tests ---

func TestBytesPerPixel(t *testing.T) {
	tests := []struct {
		format   backend.TextureFormat
		expected int
	}{
		{backend.TextureFormatRGBA8, 4},
		{backend.TextureFormatRGB8, 3},
		{backend.TextureFormatR8, 1},
		{backend.TextureFormatRGBA16F, 8},
		{backend.TextureFormatRGBA32F, 16},
		{backend.TextureFormatDepth24, 4},
		{backend.TextureFormatDepth32F, 4},
		{backend.TextureFormat(99), 4}, // unknown defaults to 4
	}
	for _, tt := range tests {
		require.Equal(t, tt.expected, bytesPerPixel(tt.format))
	}
}

// --- clampByte tests ---

func TestClampByte(t *testing.T) {
	require.Equal(t, byte(0), clampByte(-0.5))
	require.Equal(t, byte(0), clampByte(0))
	require.Equal(t, byte(128), clampByte(0.5))
	require.Equal(t, byte(255), clampByte(1.0))
	require.Equal(t, byte(255), clampByte(2.0))
}
