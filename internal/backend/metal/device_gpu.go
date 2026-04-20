//go:build darwin && !soft

package metal

import (
	"fmt"
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/mtl"
)

// Device implements backend.Device for Metal via the Objective-C runtime.
type Device struct {
	device       mtl.Device
	commandQueue mtl.CommandQueue

	width  int
	height int

	// Default render target: a buffer-backed texture for screen rendering.
	// The buffer allows CPU readback without getBytes (avoids arm64 struct ABI issues).
	defaultColorTex mtl.Texture
	screenBuffer    mtl.Buffer
	screenBufSize   int

	// Sampler states for texture filtering.
	defaultSampler uintptr // MTLSamplerState (nearest)
	linearSampler  uintptr // MTLSamplerState (linear)

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
		featureSet: FeatureSetMacFamily2v1,
		maxThreads: 256,
	}
}

// Init initializes the Metal device by loading the framework and creating
// a device, command queue, and default render target.
func (d *Device) Init(cfg backend.DeviceConfig) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("metal: invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
	d.width = cfg.Width
	d.height = cfg.Height

	if err := mtl.Init(); err != nil {
		return fmt.Errorf("metal: %w", err)
	}

	d.device = mtl.CreateSystemDefaultDevice()
	if d.device == 0 {
		return fmt.Errorf("metal: failed to create system default device")
	}

	d.deviceName = mtl.DeviceName(d.device)
	d.commandQueue = mtl.DeviceNewCommandQueue(d.device)
	if d.commandQueue == 0 {
		return fmt.Errorf("metal: failed to create command queue")
	}

	// Create a shared buffer and a texture backed by it for screen rendering.
	// Using a buffer-backed texture allows CPU readback via buffer.contents
	// instead of getBytes:fromRegion: (which has arm64 struct ABI issues with purego).
	bytesPerRow := d.width * 4
	d.screenBufSize = bytesPerRow * d.height
	d.screenBuffer = mtl.DeviceNewBuffer(d.device, uint64(d.screenBufSize), mtl.ResourceStorageModeShared)
	if d.screenBuffer == 0 {
		return fmt.Errorf("metal: failed to create screen buffer")
	}

	d.defaultColorTex = mtl.BufferNewTexture(d.screenBuffer, d.device,
		mtl.PixelFormatRGBA8Unorm, uint64(d.width), uint64(d.height),
		uint64(bytesPerRow), mtl.TextureUsageShaderRead|mtl.TextureUsageRenderTarget)
	if d.defaultColorTex == 0 {
		return fmt.Errorf("metal: failed to create default color texture")
	}

	// Create sampler states.
	d.defaultSampler = mtl.DeviceNewSamplerState(d.device, mtl.SamplerMinMagFilterNearest)
	d.linearSampler = mtl.DeviceNewSamplerState(d.device, mtl.SamplerMinMagFilterLinear)

	return nil
}

// Dispose releases all Metal resources.
func (d *Device) Dispose() {
	if d.defaultColorTex != 0 {
		mtl.TextureRelease(d.defaultColorTex)
		d.defaultColorTex = 0
	}
	if d.commandQueue != 0 {
		mtl.Release(uintptr(d.commandQueue))
		d.commandQueue = 0
	}
	// Device is autoreleased by the system; we don't release it.
	d.device = 0
}

// ReadScreen copies the rendered screen pixels from the default color
// texture into dst (RGBA, width*height*4 bytes).
func (d *Device) ReadScreen(dst []byte) bool {
	if d.screenBuffer == 0 {
		return false
	}
	if len(dst) == 0 {
		return true // probe: yes, this backend needs presentation
	}
	// Read directly from the buffer-backed texture's underlying buffer.
	src := mtl.BufferContents(d.screenBuffer)
	if src == 0 {
		return false
	}
	n := d.screenBufSize
	if n > len(dst) {
		n = len(dst)
	}
	copy(dst, unsafe.Slice((*byte)(unsafe.Pointer(src)), n))
	return true
}

// BeginFrame prepares for a new frame.
func (d *Device) BeginFrame() {}

// EndFrame finalizes the frame.
func (d *Device) EndFrame() {}

// NewTexture creates a Metal texture (MTLTexture).
func (d *Device) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("metal: invalid texture dimensions %dx%d", desc.Width, desc.Height)
	}

	pf := mtlPixelFormatFromBackend(desc.Format)
	usage := mtl.TextureUsageShaderRead | mtl.TextureUsageRenderTarget

	texDesc := mtl.TextureDescriptor{
		PixelFormat: pf,
		Width:       uint64(desc.Width),
		Height:      uint64(desc.Height),
		Depth:       1,
		MipmapCount: 1,
		SampleCount: 1,
		StorageMode: mtl.StorageModeShared,
		Usage:       usage,
	}

	handle := mtl.DeviceNewTexture(d.device, &texDesc)
	if handle == 0 {
		return nil, fmt.Errorf("metal: failed to create texture")
	}

	tex := &Texture{
		dev:         d,
		handle:      handle,
		w:           desc.Width,
		h:           desc.Height,
		format:      desc.Format,
		pixelFormat: pf,
		usage:       usage,
	}

	if len(desc.Data) > 0 {
		tex.Upload(desc.Data, 0)
	}

	return tex, nil
}

// NewBuffer creates a Metal buffer (MTLBuffer).
func (d *Device) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	size := desc.Size
	if len(desc.Data) > 0 {
		size = len(desc.Data)
	}
	if size <= 0 {
		return nil, fmt.Errorf("metal: invalid buffer size %d", size)
	}

	handle := mtl.DeviceNewBuffer(d.device, uint64(size), mtl.ResourceStorageModeShared)
	if handle == 0 {
		return nil, fmt.Errorf("metal: failed to create buffer")
	}

	buf := &Buffer{
		dev:         d,
		handle:      handle,
		size:        size,
		storageMode: mtlStorageModeShared,
	}

	if len(desc.Data) > 0 {
		buf.Upload(desc.Data)
	}

	return buf, nil
}

// NewShader creates a Metal shader (stores MSL source for later compilation).
func (d *Device) NewShader(desc backend.ShaderDescriptor) (backend.Shader, error) {
	return &Shader{
		dev:            d,
		vertexSource:   desc.VertexSource,
		fragmentSource: desc.FragmentSource,
		attributes:     desc.Attributes,
		uniforms:       make(map[string]interface{}),
	}, nil
}

// NewRenderTarget creates a Metal render target with color and optional depth textures.
func (d *Device) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("metal: invalid render target dimensions %dx%d", desc.Width, desc.Height)
	}

	colorFmt := desc.ColorFormat
	if colorFmt == 0 {
		colorFmt = backend.TextureFormatRGBA8
	}

	colorTex, err := d.NewTexture(backend.TextureDescriptor{
		Width:  desc.Width,
		Height: desc.Height,
		Format: colorFmt,
	})
	if err != nil {
		return nil, fmt.Errorf("metal: render target color: %w", err)
	}

	var depthTex backend.Texture
	if desc.HasDepth {
		depthFmt := desc.DepthFormat
		if depthFmt == 0 {
			depthFmt = backend.TextureFormatDepth24
		}
		dt, err := d.NewTexture(backend.TextureDescriptor{
			Width:  desc.Width,
			Height: desc.Height,
			Format: depthFmt,
		})
		if err != nil {
			colorTex.Dispose()
			return nil, fmt.Errorf("metal: render target depth: %w", err)
		}
		depthTex = dt
	}

	return &RenderTarget{
		dev:      d,
		colorTex: colorTex.(*Texture),
		depthTex: depthTex,
		w:        desc.Width,
		h:        desc.Height,
	}, nil
}

// NewPipeline creates a Metal render pipeline state.
//
// TODO(metal-native): this just stores the descriptor today — no
// MTLRenderPipelineState / MTLDepthStencilState is created. When native
// graphics rendering lands, build both objects from the descriptor
// including:
//   - MTLDepthStencilDescriptor.frontFaceStencil/backFaceStencil from
//     desc.Stencil when desc.StencilEnable is true
//   - depthAttachmentPixelFormat + stencilAttachmentPixelFormat set to
//     MTLPixelFormatDepth32Float_Stencil8 (macOS) or Depth24Stencil8
//     (iOS/tvOS)
//   - MTLRenderPipelineColorAttachmentDescriptor.writeMask = 0 when
//     desc.ColorWriteDisabled is true
//
// Then flip Capabilities.SupportsStencil=true.
func (d *Device) NewPipeline(desc backend.PipelineDescriptor) (backend.Pipeline, error) {
	return &Pipeline{
		dev:  d,
		desc: desc,
	}, nil
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
	return &Encoder{
		dev:    d,
		width:  d.width,
		height: d.height,
	}
}

// bytesPerPixel returns the bytes per pixel for a texture format.
func bytesPerPixel(f backend.TextureFormat) int {
	switch f {
	case backend.TextureFormatR8:
		return 1
	case backend.TextureFormatRGB8:
		return 3
	case backend.TextureFormatRGBA8:
		return 4
	case backend.TextureFormatRGBA16F:
		return 8
	case backend.TextureFormatRGBA32F:
		return 16
	case backend.TextureFormatDepth32F:
		return 4
	case backend.TextureFormatDepth24:
		return 4
	default:
		return 4
	}
}

// Keep the compiler happy.
var _ = unsafe.Pointer(nil)
