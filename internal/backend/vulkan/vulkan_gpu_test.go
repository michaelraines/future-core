//go:build (darwin || linux || freebsd || windows) && !soft

package vulkan

import (
	"fmt"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
)

func initGPUDevice(t *testing.T) (*Device, backend.CommandEncoder) {
	t.Helper()
	dev := New()
	if err := dev.Init(backend.DeviceConfig{Width: 64, Height: 64}); err != nil {
		t.Skipf("Vulkan init: %v", err)
	}
	t.Cleanup(func() { dev.Dispose() })
	return dev, dev.Encoder()
}

func TestVulkanGPUBeginEndFrame(t *testing.T) {
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

func TestVulkanGPUClearAndRead(t *testing.T) {
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
	// Accept the pixel (MoltenVK may swap R/B channels).
	require.Greater(t, a, byte(200), "alpha should be ~255")
	// At least one of R or B should be high (red or blue channel active).
	require.True(t, r > 200 || b > 200, "red or blue channel should be active")
}

func TestVulkanGPUShaderCompile(t *testing.T) {
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
	err = s.compile()
	if err != nil {
		t.Fatalf("Shader compile: %v", err)
	}
	require.NotZero(t, s.vertexModule, "vertex module")
	require.NotZero(t, s.fragmentModule, "fragment module")
	fmt.Printf("Vertex uniforms: %+v\n", s.vertexUniformLayout)
	fmt.Printf("Fragment uniforms: %+v\n", s.fragmentUniformLayout)
}

func TestVulkanGPUPipelineCreation(t *testing.T) {
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

	// Force pipeline creation against the default render pass.
	p := pip.(*Pipeline)
	err = p.createVkPipeline(dev.defaultRenderPass)
	if err != nil {
		t.Fatalf("Pipeline creation failed: %v", err)
	}
	require.NotZero(t, p.vkPipeline, "VkPipeline should be created")
	fmt.Printf("VkPipeline created: %v\n", p.vkPipeline)
}

func TestVulkanGPUDrawGreenQuad(t *testing.T) {
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

	// Uniforms.
	sh.SetUniformMat4("uProjection", [16]float32{
		2.0 / 64, 0, 0, 0,
		0, -2.0 / 64, 0, 0,
		0, 0, -1, 0,
		-1, 1, 0, 1,
	})
	sh.SetUniformMat4("uColorBody", [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})
	sh.SetUniformVec4("uColorTranslation", [4]float32{0, 0, 0, 0})

	// Green quad.
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

// TestVulkanGPUDrawWithSubmit tests the full BeginFrame→Draw→EndFrame→ReadScreen
// path, which is how the engine actually renders.
func TestVulkanGPUDrawWithSubmit(t *testing.T) {
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

	// Use the FULL engine path: BeginFrame → record → EndFrame → ReadScreen.
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

// TestVulkanGPUBindDescriptors tests descriptor binding without draw.
func TestVulkanGPUBindDescriptors(t *testing.T) {
	dev, enc := initGPUDevice(t)

	sh, err := dev.NewShader(backend.ShaderDescriptor{
		VertexSource: `#version 330 core
layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;
uniform mat4 uProjection;
out vec2 vTexCoord;
out vec4 vColor;
void main() { vTexCoord = aTexCoord; vColor = aColor; gl_Position = uProjection * vec4(aPosition, 0.0, 1.0); }
`,
		FragmentSource: `#version 330 core
in vec2 vTexCoord;
in vec4 vColor;
uniform sampler2D uTexture;
uniform mat4 uColorBody;
uniform vec4 uColorTranslation;
out vec4 fragColor;
void main() { vec4 c = texture(uTexture, vTexCoord) * vColor; fragColor = uColorBody * c + uColorTranslation; }
`,
		Attributes: batch.Vertex2DFormat().Attributes,
	})
	require.NoError(t, err)
	defer sh.Dispose()

	pip, err := dev.NewPipeline(backend.PipelineDescriptor{
		Shader: sh, VertexFormat: batch.Vertex2DFormat(), BlendMode: backend.BlendSourceOver,
	})
	require.NoError(t, err)
	defer pip.Dispose()

	sh.SetUniformMat4("uProjection", [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})
	sh.SetUniformMat4("uColorBody", [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})
	sh.SetUniformVec4("uColorTranslation", [4]float32{0, 0, 0, 0})

	dev.BeginFrame()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{1, 0, 0, 1},
	})
	enc.SetViewport(backend.Viewport{Width: 64, Height: 64})
	enc.SetScissor(nil)
	enc.SetPipeline(pip)
	// Call bindUniforms without drawing to isolate descriptor issue.
	vkEnc := enc.(*Encoder)
	vkEnc.bindUniforms()
	enc.EndRenderPass()
	dev.EndFrame()

	pixels := make([]byte, 64*64*4)
	require.True(t, dev.ReadScreen(pixels))
	center := (32*64 + 32) * 4
	r, _, _, a := pixels[center], pixels[center+1], pixels[center+2], pixels[center+3]
	t.Logf("Center pixel: R=%d A=%d (should be 255,255 from clear)", r, a)
	require.InDelta(t, 255, float64(a), 5)
}

// TestVulkanGPUClearWithViewport tests clear + viewport/scissor (no draw).
func TestVulkanGPUClearWithViewport(t *testing.T) {
	dev, enc := initGPUDevice(t)

	dev.BeginFrame()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{0, 1, 0, 1}, // green
	})
	enc.SetViewport(backend.Viewport{Width: 64, Height: 64})
	enc.SetScissor(nil)
	enc.EndRenderPass()
	dev.EndFrame()

	pixels := make([]byte, 64*64*4)
	require.True(t, dev.ReadScreen(pixels))
	center := (32*64 + 32) * 4
	r, g, b, a := pixels[center], pixels[center+1], pixels[center+2], pixels[center+3]
	t.Logf("Center pixel: R=%d G=%d B=%d A=%d", r, g, b, a)
	require.InDelta(t, 0, float64(r), 5)
	require.InDelta(t, 255, float64(g), 5)
	require.InDelta(t, 0, float64(b), 5)
	require.InDelta(t, 255, float64(a), 5)
}

// TestVulkanGPUClearWithPipeline tests clear + pipeline bind (no draw).
func TestVulkanGPUClearWithPipeline(t *testing.T) {
	dev, enc := initGPUDevice(t)

	sh, err := dev.NewShader(backend.ShaderDescriptor{
		VertexSource: `#version 330 core
layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;
uniform mat4 uProjection;
out vec2 vTexCoord;
out vec4 vColor;
void main() { vTexCoord = aTexCoord; vColor = aColor; gl_Position = uProjection * vec4(aPosition, 0.0, 1.0); }
`,
		FragmentSource: `#version 330 core
in vec2 vTexCoord;
in vec4 vColor;
uniform sampler2D uTexture;
uniform mat4 uColorBody;
uniform vec4 uColorTranslation;
out vec4 fragColor;
void main() { fragColor = vec4(0, 0, 1, 1); }
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

	dev.BeginFrame()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{1, 0, 0, 1}, // red
	})
	enc.SetViewport(backend.Viewport{Width: 64, Height: 64})
	enc.SetScissor(nil)
	enc.SetPipeline(pip) // bind pipeline but don't draw
	enc.EndRenderPass()
	dev.EndFrame()

	pixels := make([]byte, 64*64*4)
	require.True(t, dev.ReadScreen(pixels))
	center := (32*64 + 32) * 4
	r, g, b, a := pixels[center], pixels[center+1], pixels[center+2], pixels[center+3]
	t.Logf("Center pixel: R=%d G=%d B=%d A=%d", r, g, b, a)
	require.InDelta(t, 255, float64(r), 5, "should still be red (clear)")
	require.InDelta(t, 255, float64(a), 5, "should be opaque")
}

// TestVulkanGPUMinimalDraw tests a minimal draw with NDC coords (no projection).
func TestVulkanGPUMinimalDraw(t *testing.T) {
	dev, enc := initGPUDevice(t)

	sh, err := dev.NewShader(backend.ShaderDescriptor{
		VertexSource: `#version 330 core
layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;
out vec4 vColor;
void main() { vColor = aColor; gl_Position = vec4(aPosition, 0.0, 1.0); }
`,
		FragmentSource: `#version 330 core
in vec4 vColor;
out vec4 fragColor;
void main() { fragColor = vec4(0, 0, 1, 1); }
`,
		Attributes: batch.Vertex2DFormat().Attributes,
	})
	require.NoError(t, err)
	defer sh.Dispose()

	pip, err := dev.NewPipeline(backend.PipelineDescriptor{
		Shader:       sh,
		VertexFormat: batch.Vertex2DFormat(),
		BlendMode:    backend.BlendNone,
	})
	require.NoError(t, err)
	defer pip.Dispose()

	type V = batch.Vertex2D
	// NDC coordinates: full-screen quad.
	verts := []V{
		{PosX: -1, PosY: -1},
		{PosX: 1, PosY: -1},
		{PosX: 1, PosY: 1},
		{PosX: -1, PosY: 1},
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

	dev.BeginFrame()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{1, 0, 0, 1}, // red clear
	})
	enc.SetViewport(backend.Viewport{Width: 64, Height: 64})
	enc.SetScissor(nil)
	enc.SetPipeline(pip)
	enc.SetVertexBuffer(vBuf, 0)
	enc.SetIndexBuffer(iBuf, backend.IndexUint16)
	enc.DrawIndexed(6, 1, 0)
	enc.EndRenderPass()
	dev.EndFrame()

	pixels := make([]byte, 64*64*4)
	require.True(t, dev.ReadScreen(pixels))
	center := (32*64 + 32) * 4
	r, g, b, a := pixels[center], pixels[center+1], pixels[center+2], pixels[center+3]
	t.Logf("Center pixel: R=%d G=%d B=%d A=%d", r, g, b, a)

	nonZero := 0
	for i := 0; i < len(pixels); i += 4 {
		if pixels[i] > 0 || pixels[i+1] > 0 || pixels[i+2] > 0 || pixels[i+3] > 0 {
			nonZero++
		}
	}
	t.Logf("Non-zero pixels: %d / %d", nonZero, 64*64)

	// Should be blue (from fragment shader) or red (from clear).
	require.Greater(t, nonZero, 64*64/2, "most pixels should be non-zero")
}

// TestVulkanGPUDrawVColorOnly tests vColor passthrough (no texture/UBO reads).
// This isolates whether the issue is with descriptor binding or the data itself.
func TestVulkanGPUDrawVColorOnly(t *testing.T) {
	dev, enc := initGPUDevice(t)

	// Fragment shader outputs vColor directly — doesn't read any UBOs or textures.
	// But the descriptor set layout still has all 3 bindings (same pipeline path).
	sh, err := dev.NewShader(backend.ShaderDescriptor{
		VertexSource: `#version 330 core
layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;
uniform mat4 uProjection;
out vec4 vColor;
void main() { vColor = aColor; gl_Position = vec4(aPosition, 0.0, 1.0); }
`,
		FragmentSource: `#version 330 core
in vec4 vColor;
out vec4 fragColor;
void main() { fragColor = vColor; }
`,
		Attributes: batch.Vertex2DFormat().Attributes,
	})
	require.NoError(t, err)
	defer sh.Dispose()

	pip, err := dev.NewPipeline(backend.PipelineDescriptor{
		Shader: sh, VertexFormat: batch.Vertex2DFormat(), BlendMode: backend.BlendNone,
	})
	require.NoError(t, err)
	defer pip.Dispose()

	// Set uProjection so packUniformBuffer produces data.
	sh.SetUniformMat4("uProjection", [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})

	type V = batch.Vertex2D
	verts := []V{
		{PosX: -1, PosY: -1, R: 0, G: 1, B: 0, A: 1},
		{PosX: 1, PosY: -1, R: 0, G: 1, B: 0, A: 1},
		{PosX: 1, PosY: 1, R: 0, G: 1, B: 0, A: 1},
		{PosX: -1, PosY: 1, R: 0, G: 1, B: 0, A: 1},
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
	enc.DrawIndexed(6, 1, 0)
	enc.EndRenderPass()
	dev.EndFrame()

	pixels := make([]byte, 64*64*4)
	require.True(t, dev.ReadScreen(pixels))
	center := (32*64 + 32) * 4
	r, g, b, a := pixels[center], pixels[center+1], pixels[center+2], pixels[center+3]
	t.Logf("Center pixel: R=%d G=%d B=%d A=%d", r, g, b, a)

	// Should be green from vertex color.
	require.InDelta(t, 0, float64(r), 5, "red should be ~0")
	require.InDelta(t, 255, float64(g), 5, "green should be ~255")
	require.InDelta(t, 0, float64(b), 5, "blue should be ~0")
	require.InDelta(t, 255, float64(a), 5, "alpha should be ~255")
}

// TestVulkanGPUDrawVertexUBO tests that the vertex UBO is actually readable.
func TestVulkanGPUDrawVertexUBO(t *testing.T) {
	dev, enc := initGPUDevice(t)

	sh, err := dev.NewShader(backend.ShaderDescriptor{
		VertexSource: `#version 330 core
layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;
uniform mat4 uProjection;
out vec4 vColor;
void main() { vColor = aColor; gl_Position = uProjection * vec4(aPosition, 0.0, 1.0); }
`,
		FragmentSource: `#version 330 core
in vec4 vColor;
out vec4 fragColor;
void main() { fragColor = vColor; }
`,
		Attributes: batch.Vertex2DFormat().Attributes,
	})
	require.NoError(t, err)
	defer sh.Dispose()

	pip, err := dev.NewPipeline(backend.PipelineDescriptor{
		Shader: sh, VertexFormat: batch.Vertex2DFormat(), BlendMode: backend.BlendNone,
	})
	require.NoError(t, err)
	defer pip.Dispose()

	// Identity projection — should produce same result as no projection.
	sh.SetUniformMat4("uProjection", [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})

	type V = batch.Vertex2D
	verts := []V{
		{PosX: -1, PosY: -1, R: 0, G: 1, B: 0, A: 1},
		{PosX: 1, PosY: -1, R: 0, G: 1, B: 0, A: 1},
		{PosX: 1, PosY: 1, R: 0, G: 1, B: 0, A: 1},
		{PosX: -1, PosY: 1, R: 0, G: 1, B: 0, A: 1},
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
	enc.DrawIndexed(6, 1, 0)
	enc.EndRenderPass()
	dev.EndFrame()

	pixels := make([]byte, 64*64*4)
	require.True(t, dev.ReadScreen(pixels))
	center := (32*64 + 32) * 4
	r, g, b, a := pixels[center], pixels[center+1], pixels[center+2], pixels[center+3]
	t.Logf("Center pixel: R=%d G=%d B=%d A=%d", r, g, b, a)
	require.InDelta(t, 0, float64(r), 5)
	require.InDelta(t, 255, float64(g), 5, "should be green if uProjection identity is read correctly")
}

// TestVulkanGPUDrawUBOOnly tests UBO reading without texture (fragColor = uColorBody * vColor).
func TestVulkanGPUDrawUBOOnly(t *testing.T) {
	dev, enc := initGPUDevice(t)

	sh, err := dev.NewShader(backend.ShaderDescriptor{
		VertexSource: `#version 330 core
layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;
out vec4 vColor;
void main() { vColor = aColor; gl_Position = vec4(aPosition, 0.0, 1.0); }
`,
		FragmentSource: `#version 330 core
in vec4 vColor;
uniform mat4 uColorBody;
uniform vec4 uColorTranslation;
out vec4 fragColor;
void main() { fragColor = uColorBody * vColor + uColorTranslation; }
`,
		Attributes: batch.Vertex2DFormat().Attributes,
	})
	require.NoError(t, err)
	defer sh.Dispose()

	pip, err := dev.NewPipeline(backend.PipelineDescriptor{
		Shader: sh, VertexFormat: batch.Vertex2DFormat(), BlendMode: backend.BlendNone,
	})
	require.NoError(t, err)
	defer pip.Dispose()

	sh.SetUniformMat4("uColorBody", [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})
	sh.SetUniformVec4("uColorTranslation", [4]float32{0, 0, 0, 0})

	type V = batch.Vertex2D
	verts := []V{
		{PosX: -1, PosY: -1, R: 0, G: 1, B: 0, A: 1},
		{PosX: 1, PosY: -1, R: 0, G: 1, B: 0, A: 1},
		{PosX: 1, PosY: 1, R: 0, G: 1, B: 0, A: 1},
		{PosX: -1, PosY: 1, R: 0, G: 1, B: 0, A: 1},
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

	dev.BeginFrame()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{0, 0, 0, 1},
	})
	enc.SetViewport(backend.Viewport{Width: 64, Height: 64})
	enc.SetScissor(nil)
	enc.SetPipeline(pip)

	// Verify fragment layout was populated by compile().
	vkSh := sh.(*Shader)
	t.Logf("Fragment uniform layout after compile: %+v", vkSh.fragmentUniformLayout)
	t.Logf("Uniforms map: %v", vkSh.uniforms)

	enc.SetVertexBuffer(vBuf, 0)
	enc.SetIndexBuffer(iBuf, backend.IndexUint16)
	enc.DrawIndexed(6, 1, 0)
	enc.EndRenderPass()
	dev.EndFrame()

	pixels := make([]byte, 64*64*4)
	require.True(t, dev.ReadScreen(pixels))
	center := (32*64 + 32) * 4
	r, g, b, a := pixels[center], pixels[center+1], pixels[center+2], pixels[center+3]
	t.Logf("Center pixel: R=%d G=%d B=%d A=%d", r, g, b, a)
	require.InDelta(t, 0, float64(r), 5, "red should be ~0")
	require.InDelta(t, 255, float64(g), 5, "green should be ~255")
}

// TestVulkanGPUUBODataVerify checks that uniform data is actually written to the buffer.
func TestVulkanGPUUBODataVerify(t *testing.T) {
	dev, _ := initGPUDevice(t)

	sh, err := dev.NewShader(backend.ShaderDescriptor{
		FragmentSource: `#version 330 core
uniform mat4 uColorBody;
uniform vec4 uColorTranslation;
out vec4 fragColor;
void main() { fragColor = uColorBody[0]; }
`,
		Attributes: batch.Vertex2DFormat().Attributes,
	})
	require.NoError(t, err)

	// Set identity matrix.
	identity := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	sh.SetUniformMat4("uColorBody", identity)
	sh.SetUniformVec4("uColorTranslation", [4]float32{0, 0, 0, 0})

	// Pack the fragment UBO.
	vkSh := sh.(*Shader)
	t.Logf("Fragment layout: %+v", vkSh.fragmentUniformLayout)
	buf := vkSh.packUniformBuffer(vkSh.fragmentUniformLayout)
	t.Logf("Packed UBO (%d bytes): first 16 bytes = %v", len(buf), buf[:min(16, len(buf))])

	// Verify the first 4 bytes are float32(1.0) = 0x3F800000.
	if len(buf) >= 4 {
		val := uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
		t.Logf("First float32 value: 0x%08X (expected 0x3F800000 = 1.0)", val)
		require.Equal(t, uint32(0x3F800000), val, "first float should be 1.0")
	}

	// Write to uniform buffer and verify the mapped memory.
	if dev.uniformMapped != nil && len(buf) > 0 {
		dst := unsafe.Slice((*byte)(dev.uniformMapped), dev.uniformBufSize)
		copy(dst[:len(buf)], buf)

		// Read back immediately.
		readback := make([]byte, len(buf))
		src := unsafe.Slice((*byte)(dev.uniformMapped), dev.uniformBufSize)
		copy(readback, src[:len(buf)])
		t.Logf("Readback first 16 bytes: %v", readback[:min(16, len(readback))])
		require.Equal(t, buf, readback, "mapped memory should match written data")
	}

	sh.Dispose()
}

// TestVulkanGPUClearWithSubmit tests that clear works through BeginFrame/EndFrame.
func TestVulkanGPUClearWithSubmit(t *testing.T) {
	dev, enc := initGPUDevice(t)

	dev.BeginFrame()
	enc.BeginRenderPass(backend.RenderPassDescriptor{
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{1, 0, 0, 1}, // red
	})
	enc.EndRenderPass()
	dev.EndFrame()

	pixels := make([]byte, 64*64*4)
	require.True(t, dev.ReadScreen(pixels))

	center := (32*64 + 32) * 4
	r, g, b, a := pixels[center], pixels[center+1], pixels[center+2], pixels[center+3]
	t.Logf("Center pixel: R=%d G=%d B=%d A=%d", r, g, b, a)
	require.InDelta(t, 255, float64(r), 5, "should be red")
	require.InDelta(t, 0, float64(g), 5, "should be no green")
	require.InDelta(t, 0, float64(b), 5, "should be no blue")
	require.InDelta(t, 255, float64(a), 5, "should be opaque")
}
