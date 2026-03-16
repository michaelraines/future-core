//go:build !dx12native

// Package dx12 implements backend.Device targeting DirectX 12.
//
// In its current form this backend delegates all rendering to the software
// rasterizer so that conformance tests pass in any environment. When real
// DX12 bindings are added (via purego loading of d3d12.dll/dxgi.dll on
// Windows), the delegation will be replaced by actual D3D12 COM calls.
package dx12

import (
	"fmt"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/backend/soft"
	"github.com/michaelraines/future-render/internal/backend/softdelegate"
)

// Device implements backend.Device for DirectX 12.
type Device struct {
	inner *soft.Device

	// DX12-specific state modeled for when real bindings are added.
	adapterDesc  AdapterDesc
	featureLevel FeatureLevel
	debugLayer   bool
}

// AdapterDesc mirrors DXGI_ADAPTER_DESC1 fields.
type AdapterDesc struct {
	Description  string
	VendorID     uint32
	DeviceID     uint32
	DedicatedMem uint64
	SharedMem    uint64
}

// FeatureLevel represents a D3D feature level.
type FeatureLevel int

// Feature level constants.
const (
	FeatureLevel11_0 FeatureLevel = 0xb000
	FeatureLevel11_1 FeatureLevel = 0xb100
	FeatureLevel12_0 FeatureLevel = 0xc000
	FeatureLevel12_1 FeatureLevel = 0xc100
	FeatureLevel12_2 FeatureLevel = 0xc200
)

// New creates a new DirectX 12 device.
func New() *Device {
	return &Device{
		inner: soft.New(),
		adapterDesc: AdapterDesc{
			Description: "Software Rasterizer (DX12 delegation)",
		},
		featureLevel: FeatureLevel12_0,
	}
}

// Init initializes the DX12 device.
func (d *Device) Init(cfg backend.DeviceConfig) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("dx12: invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
	d.debugLayer = cfg.Debug
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

// EndFrame finalizes the frame. In a real DX12 implementation this would
// call IDXGISwapChain::Present.
func (d *Device) EndFrame() {
	d.inner.EndFrame()
}

// NewTexture creates a DX12 texture (ID3D12Resource as committed resource).
func (d *Device) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	inner, err := d.inner.NewTexture(desc)
	if err != nil {
		return nil, fmt.Errorf("dx12: %w", err)
	}
	return &Texture{
		Texture:    inner,
		dxgiFormat: dxgiFormatFromBackend(desc.Format),
	}, nil
}

// NewBuffer creates a DX12 buffer (ID3D12Resource).
func (d *Device) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	inner, err := d.inner.NewBuffer(desc)
	if err != nil {
		return nil, fmt.Errorf("dx12: %w", err)
	}
	return &Buffer{
		Buffer:   inner,
		heapType: d3d12HeapTypeDefault,
	}, nil
}

// NewShader creates a DX12 shader (compiled HLSL bytecode / DXBC / DXIL).
func (d *Device) NewShader(desc backend.ShaderDescriptor) (backend.Shader, error) {
	inner, err := d.inner.NewShader(desc)
	if err != nil {
		return nil, fmt.Errorf("dx12: %w", err)
	}
	return &Shader{Shader: inner}, nil
}

// NewRenderTarget creates a DX12 render target.
func (d *Device) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	inner, err := d.inner.NewRenderTarget(desc)
	if err != nil {
		return nil, fmt.Errorf("dx12: %w", err)
	}
	return &RenderTarget{RenderTarget: inner}, nil
}

// NewPipeline creates a DX12 pipeline state object (ID3D12PipelineState).
func (d *Device) NewPipeline(desc backend.PipelineDescriptor) (backend.Pipeline, error) {
	// Unwrap shader so the inner soft device receives the raw soft.Shader.
	innerDesc := desc
	if s, ok := desc.Shader.(*Shader); ok {
		innerDesc.Shader = s.Shader
	}
	inner, err := d.inner.NewPipeline(innerDesc)
	if err != nil {
		return nil, fmt.Errorf("dx12: %w", err)
	}
	return &Pipeline{Pipeline: inner, desc: desc}, nil
}

// Capabilities returns DirectX 12 device capabilities.
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
