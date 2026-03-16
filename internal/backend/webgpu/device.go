//go:build !wgpunative

// Package webgpu implements backend.Device targeting the WebGPU API.
//
// WebGPU is a next-generation cross-platform GPU API that runs natively
// (via Dawn/wgpu-native) and in browsers (via the WebGPU JS API). In its
// current form this backend delegates all rendering to the software
// rasterizer so that conformance tests pass in any environment.
package webgpu

import (
	"fmt"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/backend/soft"
	"github.com/michaelraines/future-render/internal/backend/softdelegate"
)

// Device implements backend.Device for WebGPU.
type Device struct {
	inner *soft.Device

	// WebGPU-specific state modeled for when real bindings are added.
	adapterInfo AdapterInfo
	limits      Limits
}

// AdapterInfo mirrors GPUAdapterInfo properties.
type AdapterInfo struct {
	Vendor       string
	Architecture string
	Device       string
	Description  string
	BackendType  BackendType
}

// BackendType represents the underlying GPU API used by wgpu.
type BackendType int

// BackendType constants.
const (
	BackendTypeNull BackendType = iota
	BackendTypeWebGPU
	BackendTypeD3D11
	BackendTypeD3D12
	BackendTypeMetal
	BackendTypeVulkan
	BackendTypeOpenGL
	BackendTypeOpenGLES
)

// Limits mirrors GPUSupportedLimits.
type Limits struct {
	MaxTextureDimension2D      int
	MaxTextureArrayLayers      int
	MaxBindGroups              int
	MaxSampledTexturesPerStage int
	MaxSamplersPerStage        int
	MaxColorAttachments        int
}

// DefaultLimits returns WebGPU default limits.
func DefaultLimits() Limits {
	return Limits{
		MaxTextureDimension2D:      8192,
		MaxTextureArrayLayers:      256,
		MaxBindGroups:              4,
		MaxSampledTexturesPerStage: 16,
		MaxSamplersPerStage:        16,
		MaxColorAttachments:        8,
	}
}

// New creates a new WebGPU device.
func New() *Device {
	return &Device{
		inner: soft.New(),
		adapterInfo: AdapterInfo{
			Vendor:      "software",
			Device:      "Software Rasterizer (WebGPU delegation)",
			BackendType: BackendTypeNull,
		},
		limits: DefaultLimits(),
	}
}

// Init initializes the WebGPU device.
func (d *Device) Init(cfg backend.DeviceConfig) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("webgpu: invalid dimensions %dx%d", cfg.Width, cfg.Height)
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

// EndFrame finalizes the frame.
func (d *Device) EndFrame() {
	d.inner.EndFrame()
}

// NewTexture creates a WebGPU texture (GPUTexture).
func (d *Device) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	inner, err := d.inner.NewTexture(desc)
	if err != nil {
		return nil, fmt.Errorf("webgpu: %w", err)
	}
	return &Texture{
		Texture: inner,
		format:  wgpuTextureFormatFromBackend(desc.Format),
		usage:   wgpuTextureUsageSampled | wgpuTextureUsageCopyDst,
	}, nil
}

// NewBuffer creates a WebGPU buffer (GPUBuffer).
func (d *Device) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	inner, err := d.inner.NewBuffer(desc)
	if err != nil {
		return nil, fmt.Errorf("webgpu: %w", err)
	}
	return &Buffer{
		Buffer: inner,
		usage:  wgpuBufferUsageFromBackend(desc.Usage),
	}, nil
}

// NewShader creates a WebGPU shader module (GPUShaderModule).
func (d *Device) NewShader(desc backend.ShaderDescriptor) (backend.Shader, error) {
	inner, err := d.inner.NewShader(desc)
	if err != nil {
		return nil, fmt.Errorf("webgpu: %w", err)
	}
	return &Shader{Shader: inner}, nil
}

// NewRenderTarget creates a WebGPU render target.
func (d *Device) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	inner, err := d.inner.NewRenderTarget(desc)
	if err != nil {
		return nil, fmt.Errorf("webgpu: %w", err)
	}
	return &RenderTarget{RenderTarget: inner}, nil
}

// NewPipeline creates a WebGPU render pipeline (GPURenderPipeline).
func (d *Device) NewPipeline(desc backend.PipelineDescriptor) (backend.Pipeline, error) {
	// Unwrap shader so the inner soft device receives the raw soft.Shader.
	innerDesc := desc
	if s, ok := desc.Shader.(*Shader); ok {
		innerDesc.Shader = s.Shader
	}
	inner, err := d.inner.NewPipeline(innerDesc)
	if err != nil {
		return nil, fmt.Errorf("webgpu: %w", err)
	}
	return &Pipeline{Pipeline: inner, desc: desc}, nil
}

// Capabilities returns WebGPU device capabilities.
func (d *Device) Capabilities() backend.DeviceCapabilities {
	return backend.DeviceCapabilities{
		MaxTextureSize:    d.limits.MaxTextureDimension2D,
		MaxRenderTargets:  d.limits.MaxColorAttachments,
		SupportsInstanced: true,
		SupportsCompute:   true,
		SupportsMSAA:      true,
		MaxMSAASamples:    4,
		SupportsFloat16:   true,
	}
}

// Encoder returns the command encoder.
func (d *Device) Encoder() backend.CommandEncoder {
	return &Encoder{Encoder: softdelegate.Encoder{Inner: d.inner.Encoder()}}
}
