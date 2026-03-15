package futurerender

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/batch"
)

// --- Mock types for shader testing ---

type mockShader struct {
	disposed      bool
	floatUniforms map[string]float32
	vec2Uniforms  map[string][2]float32
	vec4Uniforms  map[string][4]float32
	mat4Uniforms  map[string][16]float32
	intUniforms   map[string]int32
}

func newMockShader() *mockShader {
	return &mockShader{
		floatUniforms: make(map[string]float32),
		vec2Uniforms:  make(map[string][2]float32),
		vec4Uniforms:  make(map[string][4]float32),
		mat4Uniforms:  make(map[string][16]float32),
		intUniforms:   make(map[string]int32),
	}
}

func (s *mockShader) SetUniformFloat(name string, v float32)    { s.floatUniforms[name] = v }
func (s *mockShader) SetUniformVec2(name string, v [2]float32)  { s.vec2Uniforms[name] = v }
func (s *mockShader) SetUniformVec4(name string, v [4]float32)  { s.vec4Uniforms[name] = v }
func (s *mockShader) SetUniformMat4(name string, v [16]float32) { s.mat4Uniforms[name] = v }
func (s *mockShader) SetUniformInt(name string, v int32)        { s.intUniforms[name] = v }
func (s *mockShader) SetUniformBlock(_ string, _ []byte)        {}
func (s *mockShader) Dispose()                                  { s.disposed = true }

type mockPipeline struct {
	disposed bool
}

func (p *mockPipeline) Dispose() { p.disposed = true }

type shaderMockDevice struct {
	mockDevice
	shaders   []*mockShader
	pipelines []*mockPipeline
}

func (d *shaderMockDevice) NewShader(_ backend.ShaderDescriptor) (backend.Shader, error) {
	s := newMockShader()
	d.shaders = append(d.shaders, s)
	return s, nil
}

func (d *shaderMockDevice) NewPipeline(_ backend.PipelineDescriptor) (backend.Pipeline, error) {
	p := &mockPipeline{}
	d.pipelines = append(d.pipelines, p)
	return p, nil
}

// withShaderRenderer sets up a globalRenderer with a device that supports
// shader creation.
func withShaderRenderer(t *testing.T) *shaderMockDevice {
	t.Helper()
	dev := &shaderMockDevice{}
	registeredShaders := make(map[uint32]*Shader)
	rend := &renderer{
		device:               dev,
		batcher:              batch.NewBatcher(1024, 1024),
		registerTexture:      func(_ uint32, _ backend.Texture) {},
		registerRenderTarget: func(_ uint32, _ backend.RenderTarget) {},
		registerShader: func(id uint32, shader *Shader) {
			registeredShaders[id] = shader
		},
	}
	old := getRenderer()
	setRenderer(rend)
	t.Cleanup(func() { setRenderer(old) })
	return dev
}

// --- Shader tests ---

func TestNewShaderFromGLSL(t *testing.T) {
	dev := withShaderRenderer(t)

	vertSrc := []byte("#version 330 core\nvoid main() { gl_Position = vec4(0); }")
	fragSrc := []byte("#version 330 core\nout vec4 c; void main() { c = vec4(1); }")

	shader, err := NewShaderFromGLSL(vertSrc, fragSrc)
	require.NoError(t, err)
	require.NotNil(t, shader)
	require.Greater(t, shader.ID(), uint32(0))
	require.Len(t, dev.shaders, 1)
	require.Len(t, dev.pipelines, 1)
}

func TestNewShaderKage(t *testing.T) {
	_ = withShaderRenderer(t)

	src := []byte(`
//go:build ignore

package main

var Time float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return imageSrc0At(srcPos)
}
`)
	shader, err := NewShader(src)
	require.NoError(t, err)
	require.NotNil(t, shader)
	require.Greater(t, shader.ID(), uint32(0))
	require.Len(t, shader.uniforms, 1)
	require.Equal(t, "Time", shader.uniforms[0].Name)
}

func TestNewShaderKageInvalid(t *testing.T) {
	_ = withShaderRenderer(t)

	_, err := NewShader([]byte("not valid kage"))
	require.Error(t, err)
}

func TestNewShaderFromGLSLNoRenderer(t *testing.T) {
	old := getRenderer()
	setRenderer(nil)
	defer func() { setRenderer(old) }()

	_, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "no rendering device")
}

func TestShaderDeallocate(t *testing.T) {
	dev := withShaderRenderer(t)

	shader, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
	require.NoError(t, err)

	shader.Deallocate()
	require.True(t, dev.shaders[0].disposed)
	require.True(t, dev.pipelines[0].disposed)

	// Double deallocate is safe.
	shader.Deallocate()
}

func TestShaderSetUniforms(t *testing.T) {
	dev := withShaderRenderer(t)

	shader, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
	require.NoError(t, err)

	shader.SetUniformFloat("time", 1.5)
	require.InDelta(t, 1.5, dev.shaders[0].floatUniforms["time"], 1e-6)

	shader.SetUniformVec2("offset", [2]float32{1.0, 2.0})
	require.Equal(t, [2]float32{1.0, 2.0}, dev.shaders[0].vec2Uniforms["offset"])

	shader.SetUniformVec4("color", [4]float32{1, 0, 0, 1})
	require.Equal(t, [4]float32{1, 0, 0, 1}, dev.shaders[0].vec4Uniforms["color"])

	shader.SetUniformMat4("proj", [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})
	require.Equal(t, [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}, dev.shaders[0].mat4Uniforms["proj"])
}

func TestShaderApplyUniforms(t *testing.T) {
	dev := withShaderRenderer(t)

	shader, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
	require.NoError(t, err)

	uniforms := map[string]any{
		"time":   float32(2.5),
		"scale":  float64(3.0),
		"count":  int(7),
		"offset": []float32{1.0, 2.0},
		"color":  []float32{1, 0, 0, 1},
	}
	shader.applyUniforms(uniforms)

	ms := dev.shaders[0]
	require.InDelta(t, 2.5, ms.floatUniforms["time"], 1e-6)
	require.InDelta(t, 3.0, ms.floatUniforms["scale"], 1e-6)
	require.Equal(t, int32(7), ms.intUniforms["count"])
	require.Equal(t, [2]float32{1.0, 2.0}, ms.vec2Uniforms["offset"])
	require.Equal(t, [4]float32{1, 0, 0, 1}, ms.vec4Uniforms["color"])
}

func TestShaderApplyUniformsNil(t *testing.T) {
	_ = withShaderRenderer(t)

	shader, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
	require.NoError(t, err)

	// Should not panic.
	shader.applyUniforms(nil)
}

func TestShaderSetUniformNilBackend(t *testing.T) {
	s := &Shader{}
	// Should not panic with nil backend.
	s.SetUniformFloat("x", 1)
	s.SetUniformVec2("x", [2]float32{})
	s.SetUniformVec4("x", [4]float32{})
	s.SetUniformMat4("x", [16]float32{})
}

func TestApplyFloatSliceUniform(t *testing.T) {
	ms := newMockShader()

	applyFloatSliceUniform(ms, "f1", []float32{1.0})
	require.InDelta(t, 1.0, ms.floatUniforms["f1"], 1e-6)

	applyFloatSliceUniform(ms, "v2", []float32{1.0, 2.0})
	require.Equal(t, [2]float32{1.0, 2.0}, ms.vec2Uniforms["v2"])

	applyFloatSliceUniform(ms, "v4", []float32{1, 2, 3, 4})
	require.Equal(t, [4]float32{1, 2, 3, 4}, ms.vec4Uniforms["v4"])

	mat := make([]float32, 16)
	mat[0] = 1
	applyFloatSliceUniform(ms, "m4", mat)
	require.InDelta(t, 1.0, ms.mat4Uniforms["m4"][0], 1e-6)

	// Unsupported length (3) should be ignored.
	applyFloatSliceUniform(ms, "bad", []float32{1, 2, 3})
}

func TestShaderUniqueIDs(t *testing.T) {
	_ = withShaderRenderer(t)

	s1, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
	require.NoError(t, err)
	s2, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
	require.NoError(t, err)

	require.NotEqual(t, s1.ID(), s2.ID())
}

// --- DrawRectShader tests ---

func TestDrawRectShaderBasic(t *testing.T) {
	_ = withShaderRenderer(t)

	shader, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
	require.NoError(t, err)

	img := &Image{width: 100, height: 100}
	img.DrawRectShader(50, 50, shader, nil)

	// Verify a draw command was batched with the shader's ID.
	batches := getRenderer().batcher.Flush()
	require.Len(t, batches, 1)
	require.Equal(t, shader.id, batches[0].ShaderID)
	require.Len(t, batches[0].Vertices, 4)
	require.Len(t, batches[0].Indices, 6)
}

func TestDrawRectShaderWithGeoM(t *testing.T) {
	_ = withShaderRenderer(t)

	shader, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
	require.NoError(t, err)

	img := &Image{width: 100, height: 100}
	opts := &DrawRectShaderOptions{}
	opts.GeoM.Translate(10, 20)
	img.DrawRectShader(50, 50, shader, opts)

	batches := getRenderer().batcher.Flush()
	require.Len(t, batches, 1)
	// First vertex should be translated.
	require.InDelta(t, 10.0, batches[0].Vertices[0].PosX, 1e-3)
	require.InDelta(t, 20.0, batches[0].Vertices[0].PosY, 1e-3)
}

func TestDrawRectShaderDisposed(t *testing.T) {
	_ = withShaderRenderer(t)

	shader, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
	require.NoError(t, err)

	// Disposed image.
	img := &Image{width: 100, height: 100, disposed: true}
	img.DrawRectShader(50, 50, shader, nil)
	batches := getRenderer().batcher.Flush()
	require.Empty(t, batches)

	// Disposed shader.
	img2 := &Image{width: 100, height: 100}
	shader.Deallocate()
	img2.DrawRectShader(50, 50, shader, nil)
	batches = getRenderer().batcher.Flush()
	require.Empty(t, batches)
}

func TestDrawRectShaderNilShader(t *testing.T) {
	_ = withShaderRenderer(t)
	img := &Image{width: 100, height: 100}
	img.DrawRectShader(50, 50, nil, nil)
	batches := getRenderer().batcher.Flush()
	require.Empty(t, batches)
}

// --- DrawTrianglesShader tests ---

func TestDrawTrianglesShaderBasic(t *testing.T) {
	_ = withShaderRenderer(t)

	shader, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
	require.NoError(t, err)

	img := &Image{width: 100, height: 100}
	verts := []Vertex{
		{DstX: 0, DstY: 0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 100, DstY: 0, SrcX: 1, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 50, DstY: 100, SrcX: 0.5, SrcY: 1, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	indices := []uint16{0, 1, 2}

	img.DrawTrianglesShader(verts, indices, shader, nil)

	batches := getRenderer().batcher.Flush()
	require.Len(t, batches, 1)
	require.Equal(t, shader.id, batches[0].ShaderID)
	require.Len(t, batches[0].Vertices, 3)
}

func TestDrawTrianglesShaderDisposed(t *testing.T) {
	_ = withShaderRenderer(t)

	img := &Image{width: 100, height: 100, disposed: true}
	shader, _ := NewShaderFromGLSL([]byte("v"), []byte("f"))
	img.DrawTrianglesShader(nil, nil, shader, nil)
	batches := getRenderer().batcher.Flush()
	require.Empty(t, batches)
}

// Ensure we don't break existing test helpers.
func TestShaderIDReserved(t *testing.T) {
	// Shader IDs should start above 0 (0 is reserved for default sprite shader).
	_ = withShaderRenderer(t)
	shader, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
	require.NoError(t, err)
	require.Greater(t, shader.ID(), uint32(0))
}

// Use time package to avoid unused import lint — this test validates
// that shader IDs monotonically increase across rapid creation.
func TestShaderIDMonotonic(t *testing.T) {
	_ = withShaderRenderer(t)
	start := time.Now()

	ids := make([]uint32, 0, 5)
	for range 5 {
		s, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
		require.NoError(t, err)
		ids = append(ids, s.ID())
	}

	for i := 1; i < len(ids); i++ {
		require.Greater(t, ids[i], ids[i-1])
	}

	_ = time.Since(start) // Use time import.
}
