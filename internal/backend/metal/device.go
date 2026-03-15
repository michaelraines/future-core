// Package metal implements backend.Device targeting Apple's Metal API.
//
// In its current form this backend delegates all rendering to the software
// rasterizer so that conformance tests pass in any environment. When real
// Metal bindings are added (via purego + objc_msgSend on darwin), the
// delegation will be replaced by actual Metal calls.
package metal

import (
	"fmt"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/backend/soft"
	"github.com/michaelraines/future-render/internal/backend/softdelegate"
)

// Device implements backend.Device for Metal.
type Device struct {
	inner *soft.Device

	// Metal-specific state modeled for when real bindings are added.
	deviceName string
	featureSet FeatureSet
	maxThreads int
}

// FeatureSet represents a Metal GPU feature set / family.
type FeatureSet int

// Metal feature set constants.
const (
	FeatureSetMacFamily1v1 FeatureSet = iota
	FeatureSetMacFamily1v2
	FeatureSetMacFamily2v1
	FeatureSetIOSFamily1v1
	FeatureSetIOSFamily2v1
)

// New creates a new Metal device.
func New() *Device {
	return &Device{
		inner:      soft.New(),
		deviceName: "Software Rasterizer (Metal delegation)",
		featureSet: FeatureSetMacFamily2v1,
		maxThreads: 256,
	}
}

// Init initializes the Metal device.
func (d *Device) Init(cfg backend.DeviceConfig) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("metal: invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
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

// EndFrame finalizes the frame. In a real Metal implementation this would
// call [MTLCommandBuffer presentDrawable:] and commit.
func (d *Device) EndFrame() {
	d.inner.EndFrame()
}

// NewTexture creates a Metal texture (MTLTexture).
func (d *Device) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	inner, err := d.inner.NewTexture(desc)
	if err != nil {
		return nil, fmt.Errorf("metal: %w", err)
	}
	return &Texture{
		Texture:     inner,
		pixelFormat: mtlPixelFormatFromBackend(desc.Format),
		usage:       mtlTextureUsageShaderRead,
	}, nil
}

// NewBuffer creates a Metal buffer (MTLBuffer).
func (d *Device) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	inner, err := d.inner.NewBuffer(desc)
	if err != nil {
		return nil, fmt.Errorf("metal: %w", err)
	}
	return &Buffer{
		Buffer:      inner,
		storageMode: mtlStorageModeShared,
	}, nil
}

// NewShader creates a Metal shader library (MTLLibrary + MTLFunction pair).
func (d *Device) NewShader(desc backend.ShaderDescriptor) (backend.Shader, error) {
	inner, err := d.inner.NewShader(desc)
	if err != nil {
		return nil, fmt.Errorf("metal: %w", err)
	}
	return &Shader{Shader: inner}, nil
}

// NewRenderTarget creates a Metal render target.
func (d *Device) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	inner, err := d.inner.NewRenderTarget(desc)
	if err != nil {
		return nil, fmt.Errorf("metal: %w", err)
	}
	return &RenderTarget{RenderTarget: inner}, nil
}

// NewPipeline creates a Metal render pipeline state (MTLRenderPipelineState).
func (d *Device) NewPipeline(desc backend.PipelineDescriptor) (backend.Pipeline, error) {
	// Unwrap shader so the inner soft device receives the raw soft.Shader.
	innerDesc := desc
	if s, ok := desc.Shader.(*Shader); ok {
		innerDesc.Shader = s.Shader
	}
	inner, err := d.inner.NewPipeline(innerDesc)
	if err != nil {
		return nil, fmt.Errorf("metal: %w", err)
	}
	return &Pipeline{Pipeline: inner, desc: desc}, nil
}

// Capabilities returns Metal device capabilities.
func (d *Device) Capabilities() backend.DeviceCapabilities {
	return backend.DeviceCapabilities{
		MaxTextureSize:    16384,
		MaxRenderTargets:  8,
		SupportsInstanced: true,
		SupportsCompute:   true,
		SupportsMSAA:      true,
		MaxMSAASamples:    8,
		SupportsFloat16:   true,
	}
}

// Encoder returns the command encoder.
func (d *Device) Encoder() backend.CommandEncoder {
	return &Encoder{Encoder: softdelegate.Encoder{Inner: d.inner.Encoder()}}
}
