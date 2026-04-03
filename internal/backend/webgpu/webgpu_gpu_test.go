//go:build (darwin || linux || freebsd || windows) && !soft

package webgpu

import (
	"fmt"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
	"github.com/michaelraines/future-core/internal/wgpu"
)

func initGPUDevice(t *testing.T) (*Device, backend.CommandEncoder) {
	t.Helper()
	dev := New()
	if err := dev.Init(backend.DeviceConfig{Width: 64, Height: 64}); err != nil {
		t.Skipf("WebGPU init: %v", err)
	}
	t.Cleanup(func() { dev.Dispose() })
	enc := dev.Encoder()
	return dev, enc
}

func TestWebGPUGPUBeginEndFrame(t *testing.T) {
	dev, enc := initGPUDevice(t)
	dev.BeginFrame()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{0, 1, 0, 1},
	})
	enc.EndRenderPass()
	dev.EndFrame()

	buf := make([]byte, 64*64*4)
	ok := dev.ReadScreen(buf)
	require.True(t, ok)
	fmt.Printf("BeginEnd frame pixel: R=%d G=%d B=%d A=%d\n", buf[0], buf[1], buf[2], buf[3])
}

func TestWebGPUGPUClearAndRead(t *testing.T) {
	dev, enc := initGPUDevice(t)

	enc.BeginRenderPass(backend.RenderPassDescriptor{
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{1, 0, 0, 1},
	})
	enc.EndRenderPass()

	buf := make([]byte, 64*64*4)
	ok := dev.ReadScreen(buf)
	require.True(t, ok)

	r, g, b, a := buf[0], buf[1], buf[2], buf[3]
	fmt.Printf("First pixel: R=%d G=%d B=%d A=%d\n", r, g, b, a)
	require.Greater(t, a, byte(200), "alpha should be ~255")
	// At least one of R or B should be high (format may swap channels).
	require.True(t, r > 200 || b > 200, "red or blue channel should be active")
}

func TestWebGPUGPUShaderCompile(t *testing.T) {
	dev, _ := initGPUDevice(t)

	sh, err := dev.NewShader(backend.ShaderDescriptor{
		VertexSource: `#version 330 core
layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;
uniform mat4 uProjection;
out vec2 vTexCoord;
out vec4 vColor;
void main() {
    vTexCoord = aTexCoord;
    vColor = aColor;
    gl_Position = uProjection * vec4(aPosition, 0.0, 1.0);
}
`,
		FragmentSource: `#version 330 core
in vec2 vTexCoord;
in vec4 vColor;
uniform sampler2D uTexture;
uniform mat4 uColorBody;
uniform vec4 uColorTranslation;
out vec4 fragColor;
void main() {
    vec4 c = texture(uTexture, vTexCoord) * vColor;
    fragColor = uColorBody * c + uColorTranslation;
}
`,
		Attributes: batch.Vertex2DFormat().Attributes,
	})
	require.NoError(t, err)
	defer sh.Dispose()

	s := sh.(*Shader)
	s.compile()
	require.NotZero(t, s.vertexModule, "vertex module")
	require.NotZero(t, s.fragmentModule, "fragment module")
	fmt.Printf("Vertex uniforms: %+v\n", s.vertexUniformLayout)
	fmt.Printf("Fragment uniforms: %+v\n", s.fragmentUniformLayout)
}

func TestWebGPUGPUPipelineCreation(t *testing.T) {
	dev, _ := initGPUDevice(t)

	sh, err := dev.NewShader(backend.ShaderDescriptor{
		VertexSource: `#version 330 core
layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;
uniform mat4 uProjection;
out vec2 vTexCoord;
out vec4 vColor;
void main() {
    vTexCoord = aTexCoord;
    vColor = aColor;
    gl_Position = uProjection * vec4(aPosition, 0.0, 1.0);
}
`,
		FragmentSource: `#version 330 core
in vec2 vTexCoord;
in vec4 vColor;
uniform sampler2D uTexture;
uniform mat4 uColorBody;
uniform vec4 uColorTranslation;
out vec4 fragColor;
void main() {
    vec4 c = texture(uTexture, vTexCoord) * vColor;
    fragColor = uColorBody * c + uColorTranslation;
}
`,
		Attributes: batch.Vertex2DFormat().Attributes,
	})
	require.NoError(t, err)
	defer sh.Dispose()

	pip, err := dev.NewPipeline(backend.PipelineDescriptor{
		Shader:       sh,
		VertexFormat: batch.Vertex2DFormat(),
		BlendMode:    backend.BlendSourceOver,
	})
	require.NoError(t, err)
	defer pip.Dispose()

	p := pip.(*Pipeline)
	p.ensurePipelineForFormat(wgpu.TextureFormatRGBA8Unorm)
	require.NotZero(t, p.handle, "WGPURenderPipeline should be created")
	fmt.Printf("WGPURenderPipeline created: %v\n", p.handle)
}

func TestWebGPUGPUDrawGreenQuad(t *testing.T) {
	dev, enc := initGPUDevice(t)

	sh, err := dev.NewShader(backend.ShaderDescriptor{
		VertexSource: `#version 330 core
layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;
uniform mat4 uProjection;
out vec2 vTexCoord;
out vec4 vColor;
void main() {
    vTexCoord = aTexCoord;
    vColor = aColor;
    gl_Position = uProjection * vec4(aPosition, 0.0, 1.0);
}
`,
		FragmentSource: `#version 330 core
in vec2 vTexCoord;
in vec4 vColor;
uniform sampler2D uTexture;
uniform mat4 uColorBody;
uniform vec4 uColorTranslation;
out vec4 fragColor;
void main() {
    vec4 c = texture(uTexture, vTexCoord) * vColor;
    fragColor = uColorBody * c + uColorTranslation;
}
`,
		Attributes: batch.Vertex2DFormat().Attributes,
	})
	require.NoError(t, err)
	defer sh.Dispose()

	pip, err := dev.NewPipeline(backend.PipelineDescriptor{
		Shader:       sh,
		VertexFormat: batch.Vertex2DFormat(),
		BlendMode:    backend.BlendSourceOver,
	})
	require.NoError(t, err)
	defer pip.Dispose()

	// 1x1 white texture.
	tex, err := dev.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1, Format: backend.TextureFormatRGBA8,
		Data: []byte{255, 255, 255, 255},
	})
	require.NoError(t, err)
	defer tex.Dispose()

	// Orthographic projection for 64x64 viewport.
	sh.SetUniformMat4("uProjection", [16]float32{
		2.0 / 64, 0, 0, 0,
		0, -2.0 / 64, 0, 0,
		0, 0, -1, 0,
		-1, 1, 0, 1,
	})
	sh.SetUniformMat4("uColorBody", [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})
	sh.SetUniformVec4("uColorTranslation", [4]float32{0, 0, 0, 0})

	// Green quad covering the full 64x64 viewport.
	type V = batch.Vertex2D
	verts := []V{
		{PosX: 0, PosY: 0, TexU: 0, TexV: 0, R: 0, G: 1, B: 0, A: 1},
		{PosX: 64, PosY: 0, TexU: 1, TexV: 0, R: 0, G: 1, B: 0, A: 1},
		{PosX: 64, PosY: 64, TexU: 1, TexV: 1, R: 0, G: 1, B: 0, A: 1},
		{PosX: 0, PosY: 64, TexU: 0, TexV: 1, R: 0, G: 1, B: 0, A: 1},
	}
	indices := []uint16{0, 1, 2, 0, 2, 3}
	vBytes := unsafe.Slice((*byte)(unsafe.Pointer(&verts[0])), len(verts)*int(unsafe.Sizeof(V{})))
	iBytes := unsafe.Slice((*byte)(unsafe.Pointer(&indices[0])), len(indices)*2)

	vBuf, err := dev.NewBuffer(backend.BufferDescriptor{Data: vBytes})
	require.NoError(t, err)
	defer vBuf.Dispose()
	iBuf, err := dev.NewBuffer(backend.BufferDescriptor{Data: iBytes})
	require.NoError(t, err)
	defer iBuf.Dispose()

	enc.BeginRenderPass(backend.RenderPassDescriptor{
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{0, 0, 0, 1},
	})
	enc.SetPipeline(pip)
	enc.SetVertexBuffer(vBuf, 0)
	enc.SetIndexBuffer(iBuf, backend.IndexUint16)
	enc.SetTexture(tex, 0)
	enc.DrawIndexed(6, 1, 0)
	enc.EndRenderPass()

	pixels := make([]byte, 64*64*4)
	require.True(t, dev.ReadScreen(pixels))
	center := (32*64 + 32) * 4
	r, g, b, a := pixels[center], pixels[center+1], pixels[center+2], pixels[center+3]
	fmt.Printf("Center pixel: R=%d G=%d B=%d A=%d\n", r, g, b, a)

	// Check non-black (rendering happened).
	nonBlack := false
	for i := 0; i < len(pixels); i += 4 {
		if pixels[i] > 0 || pixels[i+1] > 0 || pixels[i+2] > 0 {
			nonBlack = true
			break
		}
	}
	if !nonBlack {
		t.Fatal("All pixels are black — draw command had no effect")
	}
	t.Logf("Rendering produced non-black pixels")
}

// TestWebGPUGPUDrawWithSubmit tests the full BeginFrame->Draw->EndFrame->ReadScreen
// path, which is how the engine actually renders.
func TestWebGPUGPUDrawWithSubmit(t *testing.T) {
	dev, enc := initGPUDevice(t)

	sh, err := dev.NewShader(backend.ShaderDescriptor{
		VertexSource: `#version 330 core
layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;
uniform mat4 uProjection;
out vec2 vTexCoord;
out vec4 vColor;
void main() {
    vTexCoord = aTexCoord;
    vColor = aColor;
    gl_Position = uProjection * vec4(aPosition, 0.0, 1.0);
}
`,
		FragmentSource: `#version 330 core
in vec2 vTexCoord;
in vec4 vColor;
uniform sampler2D uTexture;
uniform mat4 uColorBody;
uniform vec4 uColorTranslation;
out vec4 fragColor;
void main() {
    vec4 c = texture(uTexture, vTexCoord) * vColor;
    fragColor = uColorBody * c + uColorTranslation;
}
`,
		Attributes: batch.Vertex2DFormat().Attributes,
	})
	require.NoError(t, err)
	defer sh.Dispose()

	pip, err := dev.NewPipeline(backend.PipelineDescriptor{
		Shader:       sh,
		VertexFormat: batch.Vertex2DFormat(),
		BlendMode:    backend.BlendSourceOver,
	})
	require.NoError(t, err)
	defer pip.Dispose()

	tex, err := dev.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1, Format: backend.TextureFormatRGBA8,
		Data: []byte{255, 255, 255, 255},
	})
	require.NoError(t, err)
	defer tex.Dispose()

	// Identity projection — NDC passthrough.
	sh.SetUniformMat4("uProjection", [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})
	sh.SetUniformMat4("uColorBody", [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})
	sh.SetUniformVec4("uColorTranslation", [4]float32{0, 0, 0, 0})

	type V = batch.Vertex2D
	// NDC full-screen quad with green vertex color + white texture.
	verts := []V{
		{PosX: -1, PosY: -1, TexU: 0, TexV: 0, R: 0, G: 1, B: 0, A: 1},
		{PosX: 1, PosY: -1, TexU: 1, TexV: 0, R: 0, G: 1, B: 0, A: 1},
		{PosX: 1, PosY: 1, TexU: 1, TexV: 1, R: 0, G: 1, B: 0, A: 1},
		{PosX: -1, PosY: 1, TexU: 0, TexV: 1, R: 0, G: 1, B: 0, A: 1},
	}
	indices := []uint16{0, 1, 2, 0, 2, 3}
	vBytes := unsafe.Slice((*byte)(unsafe.Pointer(&verts[0])), len(verts)*int(unsafe.Sizeof(V{})))
	iBytes := unsafe.Slice((*byte)(unsafe.Pointer(&indices[0])), len(indices)*2)

	vBuf, err := dev.NewBuffer(backend.BufferDescriptor{Data: vBytes})
	require.NoError(t, err)
	defer vBuf.Dispose()
	iBuf, err := dev.NewBuffer(backend.BufferDescriptor{Data: iBytes})
	require.NoError(t, err)
	defer iBuf.Dispose()

	// Use the FULL engine path: BeginFrame -> record -> EndFrame -> ReadScreen.
	dev.BeginFrame()

	enc.BeginRenderPass(backend.RenderPassDescriptor{
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{0, 0, 0, 1},
	})
	enc.SetViewport(backend.Viewport{Width: 64, Height: 64})
	enc.SetScissor(nil)
	enc.SetPipeline(pip)
	enc.SetVertexBuffer(vBuf, 0)
	enc.SetIndexBuffer(iBuf, backend.IndexUint16)
	enc.SetTexture(tex, 0)
	enc.DrawIndexed(6, 1, 0)
	enc.EndRenderPass()

	dev.EndFrame()

	pixels := make([]byte, 64*64*4)
	require.True(t, dev.ReadScreen(pixels))

	// Count non-zero pixels.
	nonZero := 0
	for i := 0; i < len(pixels); i += 4 {
		if pixels[i] > 0 || pixels[i+1] > 0 || pixels[i+2] > 0 || pixels[i+3] > 0 {
			nonZero++
		}
	}

	center := (32*64 + 32) * 4
	r, g, b, a := pixels[center], pixels[center+1], pixels[center+2], pixels[center+3]
	t.Logf("Center pixel: R=%d G=%d B=%d A=%d", r, g, b, a)
	t.Logf("Non-zero pixels: %d / %d", nonZero, 64*64)

	// The quad should be green (0, 255, 0, 255) or close.
	require.Greater(t, nonZero, 64*64/2, "at least half the pixels should be non-zero")
	require.InDelta(t, 0, float64(r), 5, "red channel should be ~0")
	require.InDelta(t, 255, float64(g), 5, "green channel should be ~255")
	require.InDelta(t, 0, float64(b), 5, "blue channel should be ~0")
	require.InDelta(t, 255, float64(a), 5, "alpha should be ~255")
}
