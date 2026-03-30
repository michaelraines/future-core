//go:build (darwin || linux || freebsd || windows) && !soft

package webgpu

import (
	"fmt"
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/wgpu"
)

// Device implements backend.Device for WebGPU via wgpu-native.
type Device struct {
	instance wgpu.Instance
	adapter  wgpu.Adapter
	device   wgpu.Device
	queue    wgpu.Queue

	width  int
	height int

	// Default render target for screen rendering.
	defaultColorTex  wgpu.Texture
	defaultColorView wgpu.TextureView

	adapterInfo AdapterInfo
	limits      Limits

	// Sampler cache: keyed by filter mode.
	samplers map[wgpu.FilterMode]wgpu.Sampler
}

// New creates a new WebGPU device.
func New() *Device {
	return &Device{
		adapterInfo: AdapterInfo{
			BackendType: BackendTypeNull,
		},
		limits:   DefaultLimits(),
		samplers: make(map[wgpu.FilterMode]wgpu.Sampler),
	}
}

// Init initializes the WebGPU device by loading wgpu-native and creating
// an instance, adapter, device, and queue.
func (d *Device) Init(cfg backend.DeviceConfig) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("webgpu: invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
	d.width = cfg.Width
	d.height = cfg.Height

	if err := wgpu.Init(); err != nil {
		return fmt.Errorf("webgpu: %w", err)
	}

	d.instance = wgpu.CreateInstance()
	if d.instance == 0 {
		return fmt.Errorf("webgpu: failed to create instance")
	}

	// Request adapter (synchronous — wgpu-native calls callback inline).
	adapter, err := wgpu.InstanceRequestAdapterSync(d.instance)
	if err != nil {
		wgpu.InstanceRelease(d.instance)
		d.instance = 0
		return fmt.Errorf("webgpu: %w", err)
	}
	d.adapter = adapter

	// Request device (synchronous — wgpu-native calls callback inline).
	device, err := wgpu.AdapterRequestDeviceSync(d.adapter)
	if err != nil {
		wgpu.AdapterRelease(d.adapter)
		d.adapter = 0
		wgpu.InstanceRelease(d.instance)
		d.instance = 0
		return fmt.Errorf("webgpu: %w", err)
	}
	d.device = device

	if d.device != 0 {
		d.queue = wgpu.DeviceGetQueue(d.device)

		// Create default color texture for screen rendering.
		texDesc := wgpu.TextureDescriptor{
			Usage:         wgpu.TextureUsage(wgpu.TextureUsageTextureBinding | wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopyDst | wgpu.TextureUsageCopySrc),
			Dimension:     1, // 2D
			Size:          wgpu.Extent3D{Width: uint32(d.width), Height: uint32(d.height), DepthOrArrayLayers: 1},
			Format:        wgpu.TextureFormatRGBA8Unorm,
			MipLevelCount: 1,
			SampleCount:   1,
		}
		d.defaultColorTex = wgpu.DeviceCreateTexture(d.device, &texDesc)
		if d.defaultColorTex != 0 {
			d.defaultColorView = wgpu.TextureCreateView(d.defaultColorTex)
		}
	}

	return nil
}

// getSampler returns a cached sampler for the given filter mode, creating one if needed.
func (d *Device) getSampler(filter wgpu.FilterMode) wgpu.Sampler {
	if d.device == 0 {
		return 0
	}
	if s, ok := d.samplers[filter]; ok {
		return s
	}
	desc := wgpu.SamplerDescriptor{
		AddressModeU:  wgpu.AddressModeClampToEdge,
		AddressModeV:  wgpu.AddressModeClampToEdge,
		AddressModeW:  wgpu.AddressModeClampToEdge,
		MagFilter:     filter,
		MinFilter:     filter,
		MipmapFilter:  filter,
		LodMinClamp:   0,
		LodMaxClamp:   32,
		Compare:       0, // undefined — no comparison
		MaxAnisotropy: 1,
	}
	s := wgpu.DeviceCreateSamplerWithDescriptor(d.device, &desc)
	if s != 0 {
		d.samplers[filter] = s
	}
	return s
}

// Dispose releases all WebGPU resources.
func (d *Device) Dispose() {
	for k, s := range d.samplers {
		wgpu.SamplerRelease(s)
		delete(d.samplers, k)
	}
	if d.defaultColorView != 0 {
		wgpu.TextureViewRelease(d.defaultColorView)
		d.defaultColorView = 0
	}
	if d.defaultColorTex != 0 {
		wgpu.TextureRelease(d.defaultColorTex)
		d.defaultColorTex = 0
	}
	if d.device != 0 {
		wgpu.DeviceRelease(d.device)
		d.device = 0
	}
	if d.adapter != 0 {
		wgpu.AdapterRelease(d.adapter)
		d.adapter = 0
	}
	if d.instance != 0 {
		wgpu.InstanceRelease(d.instance)
		d.instance = 0
	}
}

// ReadScreen copies the default color texture pixels into dst.
func (d *Device) ReadScreen(dst []byte) bool {
	if d.defaultColorTex == 0 || d.device == 0 || d.queue == 0 || len(dst) == 0 {
		return false
	}

	bpp := 4 // RGBA8
	bytesPerRow := uint32(d.width * bpp)
	alignedBytesPerRow := (bytesPerRow + 255) &^ 255
	dataSize := uint64(alignedBytesPerRow) * uint64(d.height)

	// Create a staging buffer for readback.
	bufDesc := wgpu.BufferDescriptor{
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
		Size:  dataSize,
	}
	stagingBuf := wgpu.DeviceCreateBuffer(d.device, &bufDesc)
	if stagingBuf == 0 {
		return false
	}

	// Encode copy texture -> buffer.
	enc := wgpu.DeviceCreateCommandEncoder(d.device)
	src := wgpu.ImageCopyTexture{
		Texture_: d.defaultColorTex,
		MipLevel: 0,
		Origin:   wgpu.Origin3D{},
		Aspect:   0,
	}
	dstCopy := wgpu.ImageCopyBuffer{
		Layout: wgpu.TextureDataLayout{
			BytesPerRow:  alignedBytesPerRow,
			RowsPerImage: uint32(d.height),
		},
		Buffer_: stagingBuf,
	}
	copySize := wgpu.Extent3D{
		Width:              uint32(d.width),
		Height:             uint32(d.height),
		DepthOrArrayLayers: 1,
	}
	wgpu.CommandEncoderCopyTextureToBuffer(enc, &src, &dstCopy, &copySize)
	cmdBuf := wgpu.CommandEncoderFinish(enc)
	wgpu.QueueSubmit(d.queue, []wgpu.CommandBuffer{cmdBuf})
	wgpu.CommandBufferRelease(cmdBuf)
	wgpu.CommandEncoderRelease(enc)

	// Map the staging buffer and copy data.
	wgpu.BufferMapAsync(stagingBuf, wgpu.MapModeRead, 0, dataSize)
	wgpu.DevicePoll(d.device, true)

	ptr := wgpu.BufferGetMappedRange(stagingBuf, 0, dataSize)
	if ptr == 0 {
		return false
	}

	// Copy row by row to handle alignment padding.
	srcSlice := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), dataSize)
	dstOffset := 0
	for row := 0; row < d.height; row++ {
		srcStart := int(alignedBytesPerRow) * row
		n := int(bytesPerRow)
		if dstOffset+n > len(dst) {
			n = len(dst) - dstOffset
		}
		if n <= 0 {
			break
		}
		copy(dst[dstOffset:dstOffset+n], srcSlice[srcStart:srcStart+n])
		dstOffset += n
	}

	wgpu.BufferUnmap(stagingBuf)
	wgpu.BufferDestroy(stagingBuf)
	wgpu.BufferRelease(stagingBuf)
	return true
}

// BeginFrame prepares for a new frame.
func (d *Device) BeginFrame() {}

// EndFrame finalizes the frame by submitting any pending work.
func (d *Device) EndFrame() {}

// NewTexture creates a WebGPU texture (GPUTexture).
func (d *Device) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("webgpu: invalid texture dimensions %dx%d", desc.Width, desc.Height)
	}

	wgpuFmt := wgpuTextureFormatEnum(desc.Format)
	usage := wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst | wgpu.TextureUsageCopySrc

	texDesc := wgpu.TextureDescriptor{
		Usage:         wgpu.TextureUsage(usage),
		Dimension:     1, // 2D
		Size:          wgpu.Extent3D{Width: uint32(desc.Width), Height: uint32(desc.Height), DepthOrArrayLayers: 1},
		Format:        wgpuFmt,
		MipLevelCount: 1,
		SampleCount:   1,
	}

	handle := wgpu.DeviceCreateTexture(d.device, &texDesc)
	if handle == 0 {
		return nil, fmt.Errorf("webgpu: failed to create texture")
	}

	view := wgpu.TextureCreateView(handle)

	tex := &Texture{
		dev:    d,
		handle: handle,
		view:   view,
		w:      desc.Width,
		h:      desc.Height,
		format: desc.Format,
	}

	if len(desc.Data) > 0 {
		tex.Upload(desc.Data, 0)
	}

	return tex, nil
}

// NewBuffer creates a WebGPU buffer (GPUBuffer).
func (d *Device) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	size := desc.Size
	if len(desc.Data) > 0 {
		size = len(desc.Data)
	}
	if size <= 0 {
		return nil, fmt.Errorf("webgpu: invalid buffer size %d", size)
	}

	usage := wgpuBufferUsageEnum(desc.Usage) | wgpu.BufferUsageCopyDst

	// Align size to 4 bytes (WebGPU requirement).
	alignedSize := uint64((size + 3) &^ 3)

	bufDesc := wgpu.BufferDescriptor{
		Usage: usage,
		Size:  alignedSize,
	}

	handle := wgpu.DeviceCreateBuffer(d.device, &bufDesc)
	if handle == 0 {
		return nil, fmt.Errorf("webgpu: failed to create buffer")
	}

	buf := &Buffer{
		dev:    d,
		handle: handle,
		size:   size,
	}

	if len(desc.Data) > 0 {
		buf.Upload(desc.Data)
	}

	return buf, nil
}

// NewShader creates a WebGPU shader module.
func (d *Device) NewShader(desc backend.ShaderDescriptor) (backend.Shader, error) {
	return &Shader{
		dev:            d,
		vertexSource:   desc.VertexSource,
		fragmentSource: desc.FragmentSource,
		attributes:     desc.Attributes,
		uniforms:       make(map[string]interface{}),
	}, nil
}

// NewRenderTarget creates a WebGPU render target with color and optional depth textures.
func (d *Device) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("webgpu: invalid render target dimensions %dx%d", desc.Width, desc.Height)
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
		return nil, fmt.Errorf("webgpu: render target color: %w", err)
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
			return nil, fmt.Errorf("webgpu: render target depth: %w", err)
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

// NewPipeline creates a WebGPU render pipeline.
func (d *Device) NewPipeline(desc backend.PipelineDescriptor) (backend.Pipeline, error) {
	return &Pipeline{
		dev:  d,
		desc: desc,
	}, nil
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
	return &Encoder{
		dev:    d,
		width:  d.width,
		height: d.height,
	}
}

// wgpuTextureFormatEnum maps backend format to wgpu TextureFormat.
func wgpuTextureFormatEnum(f backend.TextureFormat) wgpu.TextureFormat {
	switch f {
	case backend.TextureFormatRGBA8:
		return wgpu.TextureFormatRGBA8Unorm
	case backend.TextureFormatRGB8:
		return wgpu.TextureFormatRGBA8Unorm // No RGB8 in WebGPU
	case backend.TextureFormatR8:
		return wgpu.TextureFormatR8Unorm
	case backend.TextureFormatRGBA16F:
		return wgpu.TextureFormatRGBA16Float
	case backend.TextureFormatRGBA32F:
		return wgpu.TextureFormatRGBA32Float
	case backend.TextureFormatDepth24:
		return wgpu.TextureFormatDepth24Plus
	case backend.TextureFormatDepth32F:
		return wgpu.TextureFormatDepth32Float
	default:
		return wgpu.TextureFormatRGBA8Unorm
	}
}

// wgpuBufferUsageEnum maps backend buffer usage to wgpu BufferUsage.
func wgpuBufferUsageEnum(u backend.BufferUsage) wgpu.BufferUsage {
	switch u {
	case backend.BufferUsageVertex:
		return wgpu.BufferUsageVertex
	case backend.BufferUsageIndex:
		return wgpu.BufferUsageIndex
	case backend.BufferUsageUniform:
		return wgpu.BufferUsageUniform
	default:
		return wgpu.BufferUsageVertex
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
