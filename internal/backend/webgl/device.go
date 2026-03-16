//go:build !js

// Package webgl implements backend.Device targeting the WebGL2 API.
//
// In its current form this backend delegates all rendering to the software
// rasterizer so that conformance tests pass in any environment. When real
// WebGL2 bindings are added (via syscall/js under GOOS=js GOARCH=wasm),
// the delegation will be replaced by actual WebGL2 calls while keeping the
// same public types and API surface.
package webgl

import (
	"fmt"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/backend/soft"
	"github.com/michaelraines/future-render/internal/backend/softdelegate"
)

// Device implements backend.Device for WebGL2.
// It wraps a software rasterizer for conformance testing and will be
// replaced with real syscall/js WebGL2 bindings for browser targets.
type Device struct {
	inner *soft.Device

	// WebGL2-specific state that a real implementation would use.
	canvasWidth  int
	canvasHeight int
	contextAttrs ContextAttributes
}

// ContextAttributes mirrors WebGL2 context creation attributes.
type ContextAttributes struct {
	Alpha                 bool
	Depth                 bool
	Stencil               bool
	Antialias             bool
	PremultipliedAlpha    bool
	PreserveDrawingBuffer bool
	PowerPreference       string // "default", "high-performance", "low-power"
}

// DefaultContextAttributes returns sensible defaults for WebGL2.
func DefaultContextAttributes() ContextAttributes {
	return ContextAttributes{
		Alpha:              true,
		Depth:              true,
		Antialias:          true,
		PremultipliedAlpha: true,
		PowerPreference:    "default",
	}
}

// New creates a new WebGL2 device.
func New() *Device {
	return &Device{
		inner:        soft.New(),
		contextAttrs: DefaultContextAttributes(),
	}
}

// Init initializes the WebGL2 device.
func (d *Device) Init(cfg backend.DeviceConfig) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("webgl: invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
	d.canvasWidth = cfg.Width
	d.canvasHeight = cfg.Height
	return d.inner.Init(cfg)
}

// Dispose releases device resources.
func (d *Device) Dispose() {
	d.inner.Dispose()
}

// BeginFrame prepares for a new frame.
func (d *Device) BeginFrame() {
	d.inner.BeginFrame()
}

// EndFrame finalizes the frame. In a real WebGL2 implementation this would
// call requestAnimationFrame and present the canvas.
func (d *Device) EndFrame() {
	d.inner.EndFrame()
}

// NewTexture creates a WebGL2 texture backed by the software rasterizer.
func (d *Device) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	inner, err := d.inner.NewTexture(desc)
	if err != nil {
		return nil, fmt.Errorf("webgl: %w", err)
	}
	return &Texture{
		Texture:  inner,
		glTarget: glTexture2D,
		glFormat: glFormatFromTextureFormat(desc.Format),
	}, nil
}

// NewBuffer creates a WebGL2 buffer backed by the software rasterizer.
func (d *Device) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	inner, err := d.inner.NewBuffer(desc)
	if err != nil {
		return nil, fmt.Errorf("webgl: %w", err)
	}
	return &Buffer{
		Buffer:  inner,
		glUsage: glUsageFromBufferUsage(desc.Usage),
	}, nil
}

// NewShader creates a WebGL2 shader program backed by the software rasterizer.
func (d *Device) NewShader(desc backend.ShaderDescriptor) (backend.Shader, error) {
	inner, err := d.inner.NewShader(desc)
	if err != nil {
		return nil, fmt.Errorf("webgl: %w", err)
	}
	return &Shader{
		Shader:       inner,
		vertexSource: translateGLSLES(desc.VertexSource),
		fragSource:   translateGLSLES(desc.FragmentSource),
	}, nil
}

// NewRenderTarget creates a WebGL2 framebuffer backed by the software rasterizer.
func (d *Device) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	inner, err := d.inner.NewRenderTarget(desc)
	if err != nil {
		return nil, fmt.Errorf("webgl: %w", err)
	}
	return &RenderTarget{RenderTarget: inner}, nil
}

// NewPipeline creates a WebGL2 pipeline state backed by the software rasterizer.
func (d *Device) NewPipeline(desc backend.PipelineDescriptor) (backend.Pipeline, error) {
	// Unwrap shader so the inner soft device receives the raw soft.Shader.
	innerDesc := desc
	if s, ok := desc.Shader.(*Shader); ok {
		innerDesc.Shader = s.Shader
	}
	inner, err := d.inner.NewPipeline(innerDesc)
	if err != nil {
		return nil, fmt.Errorf("webgl: %w", err)
	}
	return &Pipeline{Pipeline: inner, desc: desc}, nil
}

// Capabilities returns WebGL2 device capabilities.
func (d *Device) Capabilities() backend.DeviceCapabilities {
	return backend.DeviceCapabilities{
		MaxTextureSize:    4096,
		MaxRenderTargets:  4,
		SupportsInstanced: true,
		SupportsCompute:   false, // WebGL2 has no compute shaders
		SupportsMSAA:      true,
		MaxMSAASamples:    4,
		SupportsFloat16:   false, // Extension-dependent in WebGL2
	}
}

// Encoder returns the command encoder.
func (d *Device) Encoder() backend.CommandEncoder {
	return &Encoder{Encoder: softdelegate.Encoder{Inner: d.inner.Encoder()}}
}
