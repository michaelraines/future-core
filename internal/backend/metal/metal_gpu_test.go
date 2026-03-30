//go:build darwin && !soft

package metal

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
		t.Skipf("Metal init: %v", err)
	}
	t.Cleanup(func() { dev.Dispose() })
	return dev, dev.Encoder()
}

func TestMetalClearAndReadScreen(t *testing.T) {
	dev, enc := initGPUDevice(t)

	enc.BeginRenderPass(backend.RenderPassDescriptor{
		LoadAction: backend.LoadActionClear,
		ClearColor: [4]float32{1, 0, 0, 1},
	})
	enc.EndRenderPass()

	buf := make([]byte, 64*64*4)
	require.True(t, dev.ReadScreen(buf))
	require.Equal(t, byte(255), buf[0], "red")
	require.Equal(t, byte(0), buf[1], "green")
	require.Equal(t, byte(0), buf[2], "blue")
	require.Equal(t, byte(255), buf[3], "alpha")
}

func TestMetalShaderCompile(t *testing.T) {
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
	require.NoError(t, s.compile())
	require.NotZero(t, s.vertexFn, "vertex function compiled")
	require.NotZero(t, s.fragmentFn, "fragment function compiled")

	fmt.Printf("Vertex uniforms: %+v\n", s.vertexUniformLayout)
	fmt.Printf("Fragment uniforms: %+v\n", s.fragmentUniformLayout)
}

func TestMetalDrawGreenQuad(t *testing.T) {
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

	// Set uniforms: ortho projection, identity color matrix.
	sh.SetUniformMat4("uProjection", [16]float32{
		2.0 / 64, 0, 0, 0,
		0, -2.0 / 64, 0, 0,
		0, 0, -1, 0,
		-1, 1, 0, 1,
	})
	sh.SetUniformMat4("uColorBody", [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})
	sh.SetUniformVec4("uColorTranslation", [4]float32{0, 0, 0, 0})

	// Green quad covering full 64×64.
	type V = batch.Vertex2D
	verts := []V{
		{PosX: 0, PosY: 0, TexU: 0, TexV: 0, R: 0, G: 1, B: 0, A: 1},
		{PosX: 64, PosY: 0, TexU: 1, TexV: 0, R: 0, G: 1, B: 0, A: 1},
		{PosX: 64, PosY: 64, TexU: 1, TexV: 1, R: 0, G: 1, B: 0, A: 1},
		{PosX: 0, PosY: 64, TexU: 0, TexV: 1, R: 0, G: 1, B: 0, A: 1},
	}
	indices := []uint16{0, 1, 2, 0, 2, 3}

	// Convert to bytes.
	vBytes := unsafe.Slice((*byte)(unsafe.Pointer(&verts[0])), len(verts)*int(unsafe.Sizeof(V{})))
	iBytes := unsafe.Slice((*byte)(unsafe.Pointer(&indices[0])), len(indices)*2)

	vBuf, err := dev.NewBuffer(backend.BufferDescriptor{Data: vBytes})
	require.NoError(t, err)
	defer vBuf.Dispose()

	iBuf, err := dev.NewBuffer(backend.BufferDescriptor{Data: iBytes})
	require.NoError(t, err)
	defer iBuf.Dispose()

	// Render.
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

	// Read center pixel.
	pixels := make([]byte, 64*64*4)
	require.True(t, dev.ReadScreen(pixels))
	center := (32*64 + 32) * 4
	r, g, b, a := pixels[center], pixels[center+1], pixels[center+2], pixels[center+3]
	fmt.Printf("Center pixel: R=%d G=%d B=%d A=%d\n", r, g, b, a)

	require.Less(t, r, byte(10), "red ~0")
	require.Greater(t, g, byte(200), "green ~255")
	require.Less(t, b, byte(10), "blue ~0")
	require.Greater(t, a, byte(200), "alpha ~255")
}
