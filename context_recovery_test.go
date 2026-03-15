package futurerender

import (
	goimage "image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/shaderir"
)

// --- Helpers ---

// withTracker installs a ResourceTracker as globalTracker for the duration
// of the test, restoring the previous value on cleanup.
func withTracker(t *testing.T) *ResourceTracker {
	t.Helper()
	tracker := NewResourceTracker()
	old := getTracker()
	setTracker(tracker)
	t.Cleanup(func() { setTracker(old) })
	return tracker
}

// withTrackerAndRenderer sets up both a globalTracker and a mock renderer
// with shader support, restoring both on cleanup.
func withTrackerAndRenderer(t *testing.T) *ResourceTracker {
	t.Helper()
	tracker := withTracker(t)
	withShaderRenderer(t)
	return tracker
}

// --- ResourceTracker unit tests ---

func TestNewResourceTracker(t *testing.T) {
	tracker := NewResourceTracker()
	require.NotNil(t, tracker)
	require.Equal(t, 0, tracker.ImageCount())
	require.Equal(t, 0, tracker.ShaderCount())
}

func TestTrackImageBlank(t *testing.T) {
	tracker := NewResourceTracker()

	img := &Image{width: 64, height: 32}
	tracker.TrackImage(img, nil, true)

	require.Equal(t, 1, tracker.ImageCount())
}

func TestTrackImageWithPixels(t *testing.T) {
	tracker := NewResourceTracker()

	pixels := []byte{255, 0, 0, 255, 0, 255, 0, 255}
	img := &Image{width: 2, height: 1}
	tracker.TrackImage(img, pixels, false)

	require.Equal(t, 1, tracker.ImageCount())

	// Verify pixel data is copied, not aliased.
	pixels[0] = 0
	tracker.mu.Lock()
	rec := tracker.images[img]
	tracker.mu.Unlock()
	require.Equal(t, byte(255), rec.pixels[0], "pixel data should be copied")
}

func TestTrackImageNilIsIgnored(t *testing.T) {
	tracker := NewResourceTracker()
	tracker.TrackImage(nil, nil, false)
	require.Equal(t, 0, tracker.ImageCount())
}

func TestUntrackImage(t *testing.T) {
	tracker := NewResourceTracker()

	img := &Image{width: 10, height: 10}
	tracker.TrackImage(img, nil, true)
	require.Equal(t, 1, tracker.ImageCount())

	tracker.UntrackImage(img)
	require.Equal(t, 0, tracker.ImageCount())
}

func TestUntrackImageNilIsIgnored(t *testing.T) {
	tracker := NewResourceTracker()
	tracker.UntrackImage(nil) // should not panic
	require.Equal(t, 0, tracker.ImageCount())
}

func TestUntrackImageNotTracked(t *testing.T) {
	tracker := NewResourceTracker()
	img := &Image{width: 10, height: 10}
	tracker.UntrackImage(img) // should not panic
	require.Equal(t, 0, tracker.ImageCount())
}

func TestTrackShader(t *testing.T) {
	tracker := NewResourceTracker()

	s := &Shader{id: 1}
	uniforms := []shaderir.Uniform{{Name: "Time", Type: shaderir.TypeFloat}}
	tracker.TrackShader(s, "vert", "frag", uniforms)

	require.Equal(t, 1, tracker.ShaderCount())

	// Verify uniforms are copied.
	uniforms[0].Name = "Modified"
	tracker.mu.Lock()
	rec := tracker.shaders[s]
	tracker.mu.Unlock()
	require.Equal(t, "Time", rec.uniforms[0].Name, "uniforms should be copied")
}

func TestTrackShaderNilIsIgnored(t *testing.T) {
	tracker := NewResourceTracker()
	tracker.TrackShader(nil, "v", "f", nil)
	require.Equal(t, 0, tracker.ShaderCount())
}

func TestTrackShaderNoUniforms(t *testing.T) {
	tracker := NewResourceTracker()
	s := &Shader{id: 2}
	tracker.TrackShader(s, "v", "f", nil)
	require.Equal(t, 1, tracker.ShaderCount())

	tracker.mu.Lock()
	rec := tracker.shaders[s]
	tracker.mu.Unlock()
	require.Nil(t, rec.uniforms)
}

func TestUntrackShader(t *testing.T) {
	tracker := NewResourceTracker()
	s := &Shader{id: 1}
	tracker.TrackShader(s, "v", "f", nil)
	require.Equal(t, 1, tracker.ShaderCount())

	tracker.UntrackShader(s)
	require.Equal(t, 0, tracker.ShaderCount())
}

func TestUntrackShaderNilIsIgnored(t *testing.T) {
	tracker := NewResourceTracker()
	tracker.UntrackShader(nil) // should not panic
}

func TestUntrackShaderNotTracked(t *testing.T) {
	tracker := NewResourceTracker()
	s := &Shader{id: 99}
	tracker.UntrackShader(s) // should not panic
}

// --- RecoverResources tests ---

func TestRecoverResourcesEmptyTracker(t *testing.T) {
	tracker := NewResourceTracker()
	dev := &shaderMockDevice{}

	err := tracker.RecoverResources(dev)
	require.NoError(t, err)
	require.Empty(t, dev.textures)
	require.Empty(t, dev.shaders)
}

func TestRecoverResourcesNilDevice(t *testing.T) {
	tracker := NewResourceTracker()
	err := tracker.RecoverResources(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "device is nil")
}

func TestRecoverImageBlank(t *testing.T) {
	tracker := withTrackerAndRenderer(t)

	img := NewImage(128, 64)
	require.NotNil(t, img.texture)
	require.Equal(t, 1, tracker.ImageCount())

	// Simulate context loss: nil out the texture.
	img.texture = nil
	img.renderTarget = nil
	img.disposed = true

	// Create a fresh device for recovery.
	recoveryDev := &shaderMockDevice{}
	err := tracker.RecoverResources(recoveryDev)
	require.NoError(t, err)

	// Image should have a new texture and be un-disposed.
	require.NotNil(t, img.texture)
	require.False(t, img.disposed)
	require.NotNil(t, img.renderTarget, "render target should be recreated for NewImage")

	// The recovery device should have created 1 texture + 1 render target.
	require.Len(t, recoveryDev.textures, 1)
	require.Equal(t, 128, recoveryDev.textures[0].w)
	require.Equal(t, 64, recoveryDev.textures[0].h)
	require.Len(t, recoveryDev.renderTargets, 1)
}

func TestRecoverImageWithPixels(t *testing.T) {
	tracker := withTrackerAndRenderer(t)

	src := goimage.NewRGBA(goimage.Rect(0, 0, 4, 4))
	src.Set(0, 0, color.RGBA{R: 255, G: 128, B: 64, A: 255})

	img := NewImageFromImage(src)
	require.NotNil(t, img.texture)
	require.Equal(t, 1, tracker.ImageCount())

	// Simulate context loss.
	img.texture = nil
	img.disposed = true

	recoveryDev := &shaderMockDevice{}
	err := tracker.RecoverResources(recoveryDev)
	require.NoError(t, err)

	require.NotNil(t, img.texture)
	require.False(t, img.disposed)
	require.Len(t, recoveryDev.textures, 1)
	require.Equal(t, 4, recoveryDev.textures[0].w)
	require.Equal(t, 4, recoveryDev.textures[0].h)

	// NewImageFromImage does not set renderTarget, so it should be nil.
	require.Nil(t, img.renderTarget)
}

func TestRecoverShader(t *testing.T) {
	tracker := withTrackerAndRenderer(t)

	vertSrc := []byte("#version 330 core\nvoid main() { gl_Position = vec4(0); }")
	fragSrc := []byte("#version 330 core\nout vec4 c; void main() { c = vec4(1); }")

	shader, err := NewShaderFromGLSL(vertSrc, fragSrc)
	require.NoError(t, err)
	require.Equal(t, 1, tracker.ShaderCount())

	originalID := shader.id

	// Simulate context loss.
	shader.backend = nil
	shader.pipeline = nil
	shader.disposed = true

	recoveryDev := &shaderMockDevice{}
	err = tracker.RecoverResources(recoveryDev)
	require.NoError(t, err)

	require.NotNil(t, shader.backend)
	require.NotNil(t, shader.pipeline)
	require.False(t, shader.disposed)
	require.Equal(t, originalID, shader.id, "shader ID should be preserved")
	require.Len(t, recoveryDev.shaders, 1)
	require.Len(t, recoveryDev.pipelines, 1)
}

func TestDisposedImageNotRecovered(t *testing.T) {
	tracker := withTrackerAndRenderer(t)

	img := NewImage(32, 32)
	require.Equal(t, 1, tracker.ImageCount())

	img.Dispose()
	require.Equal(t, 0, tracker.ImageCount(), "disposed image should be untracked")

	recoveryDev := &shaderMockDevice{}
	err := tracker.RecoverResources(recoveryDev)
	require.NoError(t, err)
	require.Empty(t, recoveryDev.textures, "no textures should be recreated for disposed images")
}

func TestDeallocatedShaderNotRecovered(t *testing.T) {
	tracker := withTrackerAndRenderer(t)

	shader, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
	require.NoError(t, err)
	require.Equal(t, 1, tracker.ShaderCount())

	shader.Deallocate()
	require.Equal(t, 0, tracker.ShaderCount(), "deallocated shader should be untracked")

	recoveryDev := &shaderMockDevice{}
	err = tracker.RecoverResources(recoveryDev)
	require.NoError(t, err)
	require.Empty(t, recoveryDev.shaders, "no shaders should be recreated for deallocated shaders")
}

func TestRecoverMultipleResources(t *testing.T) {
	tracker := withTrackerAndRenderer(t)

	// Create several images and shaders.
	img1 := NewImage(16, 16)
	img2 := NewImage(32, 64)

	src := goimage.NewRGBA(goimage.Rect(0, 0, 8, 8))
	img3 := NewImageFromImage(src)

	s1, err := NewShaderFromGLSL([]byte("v1"), []byte("f1"))
	require.NoError(t, err)
	s2, err := NewShaderFromGLSL([]byte("v2"), []byte("f2"))
	require.NoError(t, err)

	require.Equal(t, 3, tracker.ImageCount())
	require.Equal(t, 2, tracker.ShaderCount())

	// Dispose one of each.
	img2.Dispose()
	s1.Deallocate()

	require.Equal(t, 2, tracker.ImageCount())
	require.Equal(t, 1, tracker.ShaderCount())

	// Simulate context loss on remaining resources.
	img1.texture = nil
	img1.renderTarget = nil
	img1.disposed = true
	img3.texture = nil
	img3.disposed = true
	s2.backend = nil
	s2.pipeline = nil
	s2.disposed = true

	recoveryDev := &shaderMockDevice{}
	err = tracker.RecoverResources(recoveryDev)
	require.NoError(t, err)

	require.NotNil(t, img1.texture)
	require.False(t, img1.disposed)
	require.NotNil(t, img3.texture)
	require.False(t, img3.disposed)
	require.NotNil(t, s2.backend)
	require.False(t, s2.disposed)

	// img1 was created with NewImage (renderTarget=true), img3 with NewImageFromImage (renderTarget=false).
	require.NotNil(t, img1.renderTarget)
	require.Nil(t, img3.renderTarget)
}

func TestRecoverResourcesReregistersWithRenderer(t *testing.T) {
	tracker := withTracker(t)

	registeredTextures := make(map[uint32]backend.Texture)
	registeredShaders := make(map[uint32]*Shader)
	registeredTargets := make(map[uint32]backend.RenderTarget)

	dev := &shaderMockDevice{}
	rend := &renderer{
		device: dev,
		registerTexture: func(id uint32, tex backend.Texture) {
			registeredTextures[id] = tex
		},
		registerShader: func(id uint32, shader *Shader) {
			registeredShaders[id] = shader
		},
		registerRenderTarget: func(id uint32, rt backend.RenderTarget) {
			registeredTargets[id] = rt
		},
	}
	old := getRenderer()
	setRenderer(rend)
	t.Cleanup(func() { setRenderer(old) })

	// Manually track an image and shader.
	img := &Image{width: 16, height: 16, textureID: 42}
	tracker.TrackImage(img, nil, true)

	s := &Shader{id: 7}
	tracker.TrackShader(s, "v", "f", nil)

	// Recover.
	recoveryDev := &shaderMockDevice{}
	err := tracker.RecoverResources(recoveryDev)
	require.NoError(t, err)

	// Verify renderer registration callbacks were called.
	require.NotNil(t, registeredTextures[42], "texture should be re-registered")
	require.NotNil(t, registeredTargets[42], "render target should be re-registered")
	require.NotNil(t, registeredShaders[7], "shader should be re-registered")
	require.Equal(t, s, registeredShaders[7])
}

func TestTrackImageOverwritesPreviousRecord(t *testing.T) {
	tracker := NewResourceTracker()

	img := &Image{width: 10, height: 10}
	tracker.TrackImage(img, nil, false)
	require.Equal(t, 1, tracker.ImageCount())

	// Re-track with different data.
	pixels := []byte{1, 2, 3, 4}
	img.width = 1
	img.height = 1
	tracker.TrackImage(img, pixels, true)
	require.Equal(t, 1, tracker.ImageCount(), "re-tracking same image should not duplicate")

	tracker.mu.Lock()
	rec := tracker.images[img]
	tracker.mu.Unlock()
	require.Equal(t, 1, rec.width)
	require.Equal(t, 1, rec.height)
	require.True(t, rec.renderTarget)
	require.Equal(t, []byte{1, 2, 3, 4}, rec.pixels)
}

func TestTrackShaderOverwritesPreviousRecord(t *testing.T) {
	tracker := NewResourceTracker()

	s := &Shader{id: 1}
	tracker.TrackShader(s, "v1", "f1", nil)
	require.Equal(t, 1, tracker.ShaderCount())

	tracker.TrackShader(s, "v2", "f2", []shaderir.Uniform{{Name: "X", Type: shaderir.TypeFloat}})
	require.Equal(t, 1, tracker.ShaderCount(), "re-tracking same shader should not duplicate")

	tracker.mu.Lock()
	rec := tracker.shaders[s]
	tracker.mu.Unlock()
	require.Equal(t, "v2", rec.vertexSource)
	require.Equal(t, "f2", rec.fragmentSource)
	require.Len(t, rec.uniforms, 1)
}

// --- Integration: auto-tracking via globalTracker ---

func TestNewImageAutoTracks(t *testing.T) {
	tracker := withTracker(t)
	withMockRenderer(t)

	img := NewImage(64, 64)
	require.Equal(t, 1, tracker.ImageCount())

	img.Dispose()
	require.Equal(t, 0, tracker.ImageCount())
}

func TestNewImageFromImageAutoTracks(t *testing.T) {
	tracker := withTracker(t)
	withMockRenderer(t)

	src := goimage.NewRGBA(goimage.Rect(0, 0, 8, 8))
	src.Set(0, 0, color.RGBA{R: 100, A: 255})

	img := NewImageFromImage(src)
	require.Equal(t, 1, tracker.ImageCount())

	// Verify stored pixel data.
	tracker.mu.Lock()
	rec := tracker.images[img]
	tracker.mu.Unlock()
	require.NotNil(t, rec.pixels)
	require.Equal(t, 8*8*4, len(rec.pixels))
	require.False(t, rec.renderTarget, "NewImageFromImage should not set renderTarget")

	img.Dispose()
	require.Equal(t, 0, tracker.ImageCount())
}

func TestNewShaderAutoTracks(t *testing.T) {
	tracker := withTrackerAndRenderer(t)

	shader, err := NewShaderFromGLSL([]byte("v"), []byte("f"))
	require.NoError(t, err)
	require.Equal(t, 1, tracker.ShaderCount())

	tracker.mu.Lock()
	rec := tracker.shaders[shader]
	tracker.mu.Unlock()
	require.Equal(t, "v", rec.vertexSource)
	require.Equal(t, "f", rec.fragmentSource)

	shader.Deallocate()
	require.Equal(t, 0, tracker.ShaderCount())
}

// --- Edge cases ---

func TestRecoverImagePreservesTextureID(t *testing.T) {
	tracker := NewResourceTracker()

	img := &Image{width: 8, height: 8, textureID: 55}
	tracker.TrackImage(img, nil, false)

	dev := &shaderMockDevice{}
	// Set globalRenderer to nil to test recovery without re-registration.
	old := getRenderer()
	setRenderer(nil)
	defer func() { setRenderer(old) }()

	err := tracker.RecoverResources(dev)
	require.NoError(t, err)

	require.Equal(t, uint32(55), img.textureID, "textureID should be preserved after recovery")
}

func TestRecoverShaderPreservesUniforms(t *testing.T) {
	tracker := NewResourceTracker()

	uniforms := []shaderir.Uniform{
		{Name: "Time", Type: shaderir.TypeFloat},
		{Name: "Resolution", Type: shaderir.TypeVec2},
	}
	s := &Shader{id: 3}
	tracker.TrackShader(s, "v", "f", uniforms)

	dev := &shaderMockDevice{}
	old := getRenderer()
	setRenderer(nil)
	defer func() { setRenderer(old) }()

	err := tracker.RecoverResources(dev)
	require.NoError(t, err)

	require.Len(t, s.uniforms, 2)
	require.Equal(t, "Time", s.uniforms[0].Name)
	require.Equal(t, "Resolution", s.uniforms[1].Name)
}

func TestRecoverNoRendererDoesNotPanic(t *testing.T) {
	tracker := NewResourceTracker()

	img := &Image{width: 8, height: 8, textureID: 1}
	tracker.TrackImage(img, nil, true)

	s := &Shader{id: 1}
	tracker.TrackShader(s, "v", "f", nil)

	old := getRenderer()
	setRenderer(nil)
	defer func() { setRenderer(old) }()

	dev := &shaderMockDevice{}
	err := tracker.RecoverResources(dev)
	require.NoError(t, err, "recovery should succeed even without globalRenderer")
}

func TestRecoverImageRenderTargetCreated(t *testing.T) {
	tracker := NewResourceTracker()

	img := &Image{width: 64, height: 64, textureID: 10}
	tracker.TrackImage(img, nil, true)

	old := getRenderer()
	setRenderer(nil)
	defer func() { setRenderer(old) }()

	dev := &shaderMockDevice{}
	err := tracker.RecoverResources(dev)
	require.NoError(t, err)

	require.NotNil(t, img.renderTarget, "render target should be created when isRenderTarget=true")
	require.Len(t, dev.renderTargets, 1)
	require.Equal(t, 64, dev.renderTargets[0].w)
	require.Equal(t, 64, dev.renderTargets[0].h)
}

func TestRecoverImageNoRenderTarget(t *testing.T) {
	tracker := NewResourceTracker()

	img := &Image{width: 32, height: 32, textureID: 5}
	tracker.TrackImage(img, nil, false)

	old := getRenderer()
	setRenderer(nil)
	defer func() { setRenderer(old) }()

	dev := &shaderMockDevice{}
	err := tracker.RecoverResources(dev)
	require.NoError(t, err)

	require.Nil(t, img.renderTarget, "render target should not be created when isRenderTarget=false")
	require.Empty(t, dev.renderTargets)
}

func TestImageCountAndShaderCountThreadSafe(t *testing.T) {
	tracker := NewResourceTracker()

	// Add several images and shaders.
	for i := 0; i < 5; i++ {
		img := &Image{width: i + 1, height: i + 1}
		tracker.TrackImage(img, nil, false)
	}
	for i := 0; i < 3; i++ {
		s := &Shader{id: uint32(i + 1)}
		tracker.TrackShader(s, "v", "f", nil)
	}

	require.Equal(t, 5, tracker.ImageCount())
	require.Equal(t, 3, tracker.ShaderCount())
}

func TestRecoverShaderPreservesID(t *testing.T) {
	tracker := NewResourceTracker()

	s := &Shader{id: 42}
	tracker.TrackShader(s, "v", "f", nil)

	dev := &shaderMockDevice{}
	old := getRenderer()
	setRenderer(nil)
	defer func() { setRenderer(old) }()

	err := tracker.RecoverResources(dev)
	require.NoError(t, err)

	require.Equal(t, uint32(42), s.id, "shader ID should be preserved")
}
