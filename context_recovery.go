package futurerender

import (
	"fmt"
	"sync"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/batch"
	"github.com/michaelraines/future-render/internal/shaderir"
)

// ResourceTracker records GPU resource creation commands so that all
// resources can be recreated after a context loss event (e.g. on mobile
// or web platforms). This implements a Godot-inspired command replay
// strategy: each resource registers a creation record on construction
// and deregisters on Dispose/Deallocate. RecoverResources replays all
// active records against a new Device to restore GPU state.
type ResourceTracker struct {
	mu      sync.Mutex
	images  map[*Image]*imageRecord
	shaders map[*Shader]*shaderRecord
}

// NewResourceTracker creates a new, empty ResourceTracker.
func NewResourceTracker() *ResourceTracker {
	return &ResourceTracker{
		images:  make(map[*Image]*imageRecord),
		shaders: make(map[*Shader]*shaderRecord),
	}
}

// imageRecord stores the creation parameters for an Image so it can be
// recreated after context loss.
type imageRecord struct {
	width, height int
	pixels        []byte // nil for blank images (NewImage)
	renderTarget  bool
}

// shaderRecord stores the creation parameters for a Shader so it can be
// recreated after context loss.
type shaderRecord struct {
	vertexSource   string
	fragmentSource string
	uniforms       []shaderir.Uniform
}

// TrackImage registers an image for context loss recovery. The record
// stores the image dimensions, optional pixel data, and whether the
// image has a render target. This is called automatically by NewImage
// and NewImageFromImage when a tracker is active.
func (rt *ResourceTracker) TrackImage(img *Image, pixels []byte, isRenderTarget bool) {
	if img == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Copy pixel data to avoid aliasing the caller's slice.
	var pixCopy []byte
	if len(pixels) > 0 {
		pixCopy = make([]byte, len(pixels))
		copy(pixCopy, pixels)
	}

	rt.images[img] = &imageRecord{
		width:        img.width,
		height:       img.height,
		pixels:       pixCopy,
		renderTarget: isRenderTarget,
	}
}

// UntrackImage removes an image from the tracker. This is called
// automatically by Image.Dispose when a tracker is active.
func (rt *ResourceTracker) UntrackImage(img *Image) {
	if img == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.images, img)
}

// TrackShader registers a shader for context loss recovery. The record
// stores the GLSL vertex and fragment source and the uniform metadata.
func (rt *ResourceTracker) TrackShader(s *Shader, vertSrc, fragSrc string, uniforms []shaderir.Uniform) {
	if s == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Copy uniforms to avoid aliasing.
	var uniformsCopy []shaderir.Uniform
	if len(uniforms) > 0 {
		uniformsCopy = make([]shaderir.Uniform, len(uniforms))
		copy(uniformsCopy, uniforms)
	}

	rt.shaders[s] = &shaderRecord{
		vertexSource:   vertSrc,
		fragmentSource: fragSrc,
		uniforms:       uniformsCopy,
	}
}

// UntrackShader removes a shader from the tracker. This is called
// automatically by Shader.Deallocate when a tracker is active.
func (rt *ResourceTracker) UntrackShader(s *Shader) {
	if s == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.shaders, s)
}

// ImageCount returns the number of tracked images.
func (rt *ResourceTracker) ImageCount() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return len(rt.images)
}

// ShaderCount returns the number of tracked shaders.
func (rt *ResourceTracker) ShaderCount() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return len(rt.shaders)
}

// RecoverResources replays all tracked creation commands against the
// given device, recreating GPU textures, render targets, and shaders.
// Each resource's internal handles are updated in place so that
// existing pointers held by game code remain valid.
//
// The renderer's registration callbacks are invoked for each recreated
// resource so the pipeline's lookup tables stay in sync.
func (rt *ResourceTracker) RecoverResources(device backend.Device) error {
	if device == nil {
		return fmt.Errorf("context recovery: device is nil")
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Recover images.
	for img, rec := range rt.images {
		if err := rt.recoverImage(device, img, rec); err != nil {
			return fmt.Errorf("context recovery: image %dx%d: %w", rec.width, rec.height, err)
		}
	}

	// Recover shaders.
	for s, rec := range rt.shaders {
		if err := rt.recoverShader(device, s, rec); err != nil {
			return fmt.Errorf("context recovery: shader: %w", err)
		}
	}

	return nil
}

// recoverImage recreates a single image's GPU resources from its record.
func (rt *ResourceTracker) recoverImage(device backend.Device, img *Image, rec *imageRecord) error {
	desc := backend.TextureDescriptor{
		Width:        rec.width,
		Height:       rec.height,
		Format:       backend.TextureFormatRGBA8,
		Filter:       backend.FilterNearest,
		WrapU:        backend.WrapClamp,
		WrapV:        backend.WrapClamp,
		RenderTarget: rec.renderTarget,
		Data:         rec.pixels,
	}

	tex, err := device.NewTexture(desc)
	if err != nil {
		return fmt.Errorf("create texture: %w", err)
	}

	img.texture = tex
	img.disposed = false

	// Re-register texture with renderer.
	if globalRenderer != nil && globalRenderer.registerTexture != nil {
		globalRenderer.registerTexture(img.textureID, tex)
	}

	// Recreate render target if needed.
	if rec.renderTarget {
		rtDesc := backend.RenderTargetDescriptor{
			Width:       rec.width,
			Height:      rec.height,
			ColorFormat: backend.TextureFormatRGBA8,
		}
		renderTarget, rtErr := device.NewRenderTarget(rtDesc)
		if rtErr != nil {
			return fmt.Errorf("create render target: %w", rtErr)
		}
		img.renderTarget = renderTarget

		if globalRenderer != nil && globalRenderer.registerRenderTarget != nil {
			globalRenderer.registerRenderTarget(img.textureID, renderTarget)
		}
	}

	return nil
}

// recoverShader recreates a single shader's GPU resources from its record.
func (rt *ResourceTracker) recoverShader(device backend.Device, s *Shader, rec *shaderRecord) error {
	sh, err := device.NewShader(backend.ShaderDescriptor{
		VertexSource:   rec.vertexSource,
		FragmentSource: rec.fragmentSource,
		Attributes:     batch.Vertex2DFormat().Attributes,
	})
	if err != nil {
		return fmt.Errorf("compile shader: %w", err)
	}

	pip, err := device.NewPipeline(backend.PipelineDescriptor{
		Shader:       sh,
		VertexFormat: batch.Vertex2DFormat(),
		BlendMode:    backend.BlendSourceOver,
		DepthTest:    false,
		DepthWrite:   false,
		CullMode:     backend.CullNone,
		Primitive:    backend.PrimitiveTriangles,
	})
	if err != nil {
		sh.Dispose()
		return fmt.Errorf("create pipeline: %w", err)
	}

	s.backend = sh
	s.pipeline = pip
	s.uniforms = rec.uniforms
	s.disposed = false

	// Re-register shader with renderer.
	if globalRenderer != nil && globalRenderer.registerShader != nil {
		globalRenderer.registerShader(s.id, s)
	}

	return nil
}

// globalTracker is the active resource tracker, set during engine init.
// It is nil until explicitly enabled.
var globalTracker *ResourceTracker
