//go:build !vulkan

// Package vulkan implements backend.Device targeting the Vulkan API.
//
// In its current form this backend delegates all rendering to the software
// rasterizer so that conformance tests pass in any environment. When real
// Vulkan bindings are added (via purego loading of libvulkan), the delegation
// will be replaced by actual Vulkan calls while keeping the same type surface.
package vulkan

import (
	"fmt"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/backend/soft"
	"github.com/michaelraines/future-render/internal/backend/softdelegate"
)

// Device implements backend.Device for Vulkan.
type Device struct {
	inner *soft.Device

	// Vulkan-specific state modeled for when real bindings are added.
	instanceInfo   InstanceCreateInfo
	physicalDevice PhysicalDeviceInfo
	debugEnabled   bool
}

// InstanceCreateInfo mirrors VkInstanceCreateInfo fields.
type InstanceCreateInfo struct {
	AppName       string
	AppVersion    uint32
	EngineName    string
	EngineVersion uint32
	APIVersion    uint32
	Layers        []string
	Extensions    []string
}

// PhysicalDeviceInfo holds properties a real Vulkan implementation
// would query from vkGetPhysicalDeviceProperties.
type PhysicalDeviceInfo struct {
	DeviceName  string
	DeviceType  int // 0=other, 1=integrated, 2=discrete, 3=virtual, 4=CPU
	VendorID    uint32
	MaxImageDim int
	MaxSamples  int
}

// New creates a new Vulkan device.
func New() *Device {
	return &Device{
		inner: soft.New(),
		instanceInfo: InstanceCreateInfo{
			AppName:    "future-render",
			EngineName: "future-render",
			APIVersion: vkAPIVersion1_2,
		},
		physicalDevice: PhysicalDeviceInfo{
			DeviceName:  "Software Rasterizer (Vulkan delegation)",
			DeviceType:  4, // VK_PHYSICAL_DEVICE_TYPE_CPU
			MaxImageDim: 8192,
			MaxSamples:  4,
		},
	}
}

// Init initializes the Vulkan device.
func (d *Device) Init(cfg backend.DeviceConfig) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("vulkan: invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
	d.debugEnabled = cfg.Debug
	if d.debugEnabled {
		const validationLayer = "VK_LAYER_KHRONOS_validation"
		found := false
		for _, l := range d.instanceInfo.Layers {
			if l == validationLayer {
				found = true
				break
			}
		}
		if !found {
			d.instanceInfo.Layers = append(d.instanceInfo.Layers, validationLayer)
		}
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

// EndFrame finalizes the frame. In a real Vulkan implementation this would
// call vkQueuePresentKHR.
func (d *Device) EndFrame() {
	d.inner.EndFrame()
}

// NewTexture creates a Vulkan texture (VkImage + VkImageView).
func (d *Device) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	inner, err := d.inner.NewTexture(desc)
	if err != nil {
		return nil, fmt.Errorf("vulkan: %w", err)
	}
	return &Texture{
		Texture:   inner,
		vkFormat:  vkFormatFromTextureFormat(desc.Format),
		vkUsage:   vkImageUsageSampled | vkImageUsageTransferDst,
		mipLevels: 1,
	}, nil
}

// NewBuffer creates a Vulkan buffer (VkBuffer + VkDeviceMemory).
func (d *Device) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	inner, err := d.inner.NewBuffer(desc)
	if err != nil {
		return nil, fmt.Errorf("vulkan: %w", err)
	}
	return &Buffer{
		Buffer:  inner,
		vkUsage: vkBufferUsageFromBackend(desc.Usage),
	}, nil
}

// NewShader creates a Vulkan shader module (VkShaderModule pair).
func (d *Device) NewShader(desc backend.ShaderDescriptor) (backend.Shader, error) {
	inner, err := d.inner.NewShader(desc)
	if err != nil {
		return nil, fmt.Errorf("vulkan: %w", err)
	}
	return &Shader{
		Shader: inner,
	}, nil
}

// NewRenderTarget creates a Vulkan framebuffer (VkFramebuffer).
func (d *Device) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	inner, err := d.inner.NewRenderTarget(desc)
	if err != nil {
		return nil, fmt.Errorf("vulkan: %w", err)
	}
	return &RenderTarget{RenderTarget: inner}, nil
}

// NewPipeline creates a Vulkan graphics pipeline (VkPipeline).
func (d *Device) NewPipeline(desc backend.PipelineDescriptor) (backend.Pipeline, error) {
	// Unwrap shader so the inner soft device receives the raw soft.Shader.
	innerDesc := desc
	if s, ok := desc.Shader.(*Shader); ok {
		innerDesc.Shader = s.Shader
	}
	inner, err := d.inner.NewPipeline(innerDesc)
	if err != nil {
		return nil, fmt.Errorf("vulkan: %w", err)
	}
	return &Pipeline{Pipeline: inner, desc: desc}, nil
}

// Capabilities returns Vulkan device capabilities.
func (d *Device) Capabilities() backend.DeviceCapabilities {
	return backend.DeviceCapabilities{
		MaxTextureSize:    d.physicalDevice.MaxImageDim,
		MaxRenderTargets:  8,
		SupportsInstanced: true,
		SupportsCompute:   true,
		SupportsMSAA:      true,
		MaxMSAASamples:    d.physicalDevice.MaxSamples,
		SupportsFloat16:   true,
	}
}

// Encoder returns the command encoder.
func (d *Device) Encoder() backend.CommandEncoder {
	return &Encoder{Encoder: softdelegate.Encoder{Inner: d.inner.Encoder()}}
}
