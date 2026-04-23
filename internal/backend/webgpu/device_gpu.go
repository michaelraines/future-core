//go:build (darwin || linux || freebsd || windows) && !soft

package webgpu

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/wgpu"
)

// uniformRingBufSize must hold ALL per-frame uniform writes without wrapping.
// With single-command-encoder batching, all passes are recorded before the
// GPU executes any of them, so writes must not be overwritten. 256 KB
// supports ~1024 draws per frame at 256-byte alignment.
const uniformRingBufSize = 256 * 1024

// uniformAlignOffset is the minimum offset alignment for uniform buffers.
// WebGPU requires 256-byte alignment for dynamic buffer offsets.
const uniformAlignOffset = 256

// Device implements backend.Device for WebGPU via wgpu-native.
type Device struct {
	instance wgpu.Instance
	adapter  wgpu.Adapter
	device   wgpu.Device
	queue    wgpu.Queue

	width  int
	height int

	// Default render target for screen rendering (offscreen path).
	defaultColorTex  wgpu.Texture
	defaultColorView wgpu.TextureView

	// Surface/presentation support.
	surface        wgpu.Surface
	hasSurface     bool
	surfaceFormat  wgpu.TextureFormat
	currentTexView wgpu.TextureView // view for this frame's surface texture

	// Persistent uniform ring buffer.
	uniformBuf    wgpu.Buffer
	uniformCursor int

	adapterInfo AdapterInfo
	limits      Limits

	// Sampler cache: keyed by filter mode.
	samplers map[wgpu.FilterMode]wgpu.Sampler

	// Default 1x1 white texture used as a placeholder for bind group 1
	// when SetTexture has not been called before a draw call.
	defaultWhiteTex *Texture

	// Cached encoder so Texture.ReadPixels can flush pending render work
	// before its own readback command buffer runs. Without this, a render
	// pass that writes to an RT then immediately reads the RT's color
	// texture returns uninitialized memory because the render-pass's
	// command buffer was never submitted.
	encoder *Encoder
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
	device, err := wgpu.AdapterRequestDeviceSync(d.adapter, d.instance)
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

		// Create surface: prefer MetalLayer (native wgpu surface), fall back
		// to SurfaceFactory (Vulkan-style).
		if cfg.MetalLayer != 0 {
			if err := d.createSurfaceFromMetalLayer(cfg.MetalLayer); err != nil {
				d.hasSurface = false
			}
		} else if cfg.SurfaceFactory != nil {
			if err := d.createSurface(cfg); err != nil {
				d.hasSurface = false
			}
		}

		if !d.hasSurface {
			// Offscreen path: create default color texture.
			texDesc := wgpu.TextureDescriptor{
				Usage:         wgpu.TextureUsage(wgpu.TextureUsageTextureBinding | wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopyDst | wgpu.TextureUsageCopySrc),
				Dimension:     2, // WGPUTextureDimension_2D
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

		// Create persistent uniform ring buffer.
		d.createUniformRingBuffer()

		// Create a 1x1 white placeholder texture for default bind group 1.
		d.createDefaultWhiteTexture()
	}

	return nil
}

// createSurfaceFromMetalLayer creates a wgpu surface from a CAMetalLayer
// using wgpu.InstanceCreateSurface with SurfaceDescriptorFromMetalLayer.
func (d *Device) createSurfaceFromMetalLayer(metalLayer uintptr) error {
	chainedDesc := wgpu.SurfaceDescriptorFromMetalLayer{
		Chain: wgpu.SChainedStruct{SType: wgpu.STypeSurfaceDescriptorFromMetalLayer},
		Layer: metalLayer,
	}
	surfaceDesc := wgpu.SurfaceDescriptor{
		NextInChain: uintptr(unsafe.Pointer(&chainedDesc)),
	}
	d.surface = wgpu.InstanceCreateSurface(d.instance, &surfaceDesc)
	runtime.KeepAlive(chainedDesc)
	if d.surface == 0 {
		return fmt.Errorf("wgpu: InstanceCreateSurface returned nil")
	}
	return d.configureSurface()
}

// createSurface creates a presentation surface from the SurfaceFactory.
func (d *Device) createSurface(cfg backend.DeviceConfig) error {
	surfaceHandle, err := cfg.SurfaceFactory(uintptr(d.instance))
	if err != nil {
		return fmt.Errorf("surface factory: %w", err)
	}
	d.surface = wgpu.Surface(surfaceHandle)
	if d.surface == 0 {
		return fmt.Errorf("surface factory returned nil")
	}
	return d.configureSurface()
}

// configureSurface queries the preferred format and configures the surface
// for presentation. Called after d.surface is set.
func (d *Device) configureSurface() error {
	// Query the preferred surface format from the adapter. On macOS
	// the first advertised format is typically BGRA8UnormSrgb — but
	// our shaders already output linear values that the compositor
	// then treats as sRGB-encoded, so picking the sRGB surface format
	// makes the driver apply gamma encoding a second time and the
	// scene looks washed-out / lighter than the WebGPU browser and
	// Vulkan backends. Prefer the linear (Unorm) variant when the
	// surface offers it; fall back to the first advertised format if
	// nothing linear is available.
	d.surfaceFormat = wgpu.TextureFormatBGRA8Unorm
	var caps wgpu.SurfaceCapabilities
	wgpu.SurfaceGetCapabilities(d.surface, d.adapter, &caps)
	if caps.FormatCount > 0 && caps.Formats != 0 {
		formats := unsafe.Slice((*wgpu.TextureFormat)(unsafe.Pointer(caps.Formats)), caps.FormatCount)
		d.surfaceFormat = formats[0]
		for _, f := range formats {
			if f == wgpu.TextureFormatBGRA8Unorm || f == wgpu.TextureFormatRGBA8Unorm {
				d.surfaceFormat = f
				break
			}
		}
	}

	surfaceCfg := wgpu.SurfaceConfiguration{
		Device:      d.device,
		Format:      d.surfaceFormat,
		Usage:       wgpu.TextureUsageRenderAttachment,
		AlphaMode:   wgpu.CompositeAlphaModeAuto,
		Width:       uint32(d.width),
		Height:      uint32(d.height),
		PresentMode: wgpu.PresentModeFifo, // VSync
	}
	wgpu.SurfaceConfigure(d.surface, &surfaceCfg)
	d.hasSurface = true
	return nil
}

// createUniformRingBuffer creates a persistent GPU buffer for uniforms.
func (d *Device) createUniformRingBuffer() {
	if d.device == 0 {
		return
	}
	bufDesc := wgpu.BufferDescriptor{
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
		Size:  uniformRingBufSize,
	}
	d.uniformBuf = wgpu.DeviceCreateBuffer(d.device, &bufDesc)
	d.uniformCursor = 0
}

// writeUniformRing writes data into the ring buffer and returns (offset, alignedSize).
// Returns (0, 0) if the ring buffer is not available.
func (d *Device) writeUniformRing(data []byte) (offset, alignedSize int) {
	if d.uniformBuf == 0 || d.queue == 0 || len(data) == 0 {
		return 0, 0
	}

	// Align size to uniformAlignOffset (256 bytes).
	alignedSize = (len(data) + uniformAlignOffset - 1) &^ (uniformAlignOffset - 1)

	// Wrap if we'd exceed the buffer.
	if d.uniformCursor+alignedSize > uniformRingBufSize {
		d.uniformCursor = 0
	}

	offset = d.uniformCursor
	wgpu.QueueWriteBuffer(d.queue, d.uniformBuf, uint64(offset),
		unsafe.Pointer(&data[0]), uint64(len(data)))
	runtime.KeepAlive(data)

	d.uniformCursor += alignedSize
	return offset, alignedSize
}

// createDefaultWhiteTexture creates a 1x1 RGBA white texture used as a
// placeholder binding for group 1 when no texture has been explicitly set.
func (d *Device) createDefaultWhiteTexture() {
	if d.device == 0 {
		return
	}
	tex, err := d.NewTexture(backend.TextureDescriptor{
		Width:  1,
		Height: 1,
		Format: backend.TextureFormatRGBA8,
		Data:   []byte{255, 255, 255, 255},
	})
	if err != nil {
		return
	}
	d.defaultWhiteTex = tex.(*Texture)
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
	if d.defaultWhiteTex != nil {
		d.defaultWhiteTex.Dispose()
		d.defaultWhiteTex = nil
	}
	for k, s := range d.samplers {
		wgpu.SamplerRelease(s)
		delete(d.samplers, k)
	}
	if d.uniformBuf != 0 {
		wgpu.BufferDestroy(d.uniformBuf)
		wgpu.BufferRelease(d.uniformBuf)
		d.uniformBuf = 0
	}
	if d.currentTexView != 0 {
		wgpu.TextureViewRelease(d.currentTexView)
		d.currentTexView = 0
	}
	if d.surface != 0 {
		wgpu.SurfaceUnconfigure(d.surface)
		wgpu.SurfaceRelease(d.surface)
		d.surface = 0
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
//
// Flushes any pending render work on the cached encoder before reading
// so tests and headless callers that do Clear/Draw → ReadScreen without
// an explicit Flush see the rendered output (WebGPU native doesn't
// auto-submit on EndRenderPass).
func (d *Device) ReadScreen(dst []byte) bool {
	if d.defaultColorTex == 0 || d.device == 0 || d.queue == 0 || len(dst) == 0 {
		return false
	}
	if d.encoder != nil {
		d.encoder.Flush()
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
		Aspect:   1, // WGPUTextureAspect_All
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

	// Ensure the GPU copy completes before mapping.
	wgpu.DevicePoll(d.device, true)

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
func (d *Device) BeginFrame() {
	// Reset uniform ring buffer cursor for the new frame.
	d.uniformCursor = 0

	// Clear the encoder's sticky blend override so a SetBlendMode call
	// from a prior frame doesn't leak into the new frame's first
	// SetPipeline. Keeping the override sticky across draws (inside a
	// frame) is intentional — matches Vulkan's behaviour — but across
	// frames it's a correctness hazard.
	if d.encoder != nil {
		d.encoder.pendingBlend = backend.BlendMode{}
		d.encoder.pendingBlendSet = false
	}

	if !d.hasSurface {
		return
	}

	// Acquire the next surface texture for rendering.
	var surfTex wgpu.SurfaceTexture
	wgpu.SurfaceGetCurrentTexture(d.surface, &surfTex)

	// In wgpu-native v27 status 0 = SuccessOptimal, 1 = SuccessSuboptimal.
	// Both are usable; the latter just means the surface should be
	// reconfigured soon (e.g. after a DPR change on macOS). Only
	// Timeout / Outdated / Lost / Error require a reconfigure+retry.
	//
	// wgpu-native keeps an implicit SurfaceOutput alive as long as the
	// returned texture is held, and SurfaceConfigure refuses to run
	// while one is outstanding ("SurfaceOutput must be dropped before
	// a new Surface is made"). Release the texture before reconfiguring.
	const (
		statusSuccessOptimal    = 0
		statusSuccessSuboptimal = 1
	)
	if surfTex.Status != statusSuccessOptimal && surfTex.Status != statusSuccessSuboptimal {
		if surfTex.Texture_ != 0 {
			wgpu.TextureRelease(surfTex.Texture_)
			surfTex.Texture_ = 0
		}
		d.reconfigureSurface()
		wgpu.SurfaceGetCurrentTexture(d.surface, &surfTex)
		if (surfTex.Status != statusSuccessOptimal && surfTex.Status != statusSuccessSuboptimal) ||
			surfTex.Texture_ == 0 {
			return
		}
	}

	if surfTex.Texture_ == 0 {
		return
	}

	// Release previous frame's view if still held.
	if d.currentTexView != 0 {
		wgpu.TextureViewRelease(d.currentTexView)
	}
	d.currentTexView = wgpu.TextureCreateView(surfTex.Texture_)
}

// EndFrame finalizes the frame and presents to the surface.
func (d *Device) EndFrame() {
	if !d.hasSurface {
		return
	}
	wgpu.SurfacePresent(d.surface)
	// Release the view — the surface texture is owned by the surface.
	if d.currentTexView != 0 {
		wgpu.TextureViewRelease(d.currentTexView)
		d.currentTexView = 0
	}
}

// Resize updates the device dimensions and reconfigures the surface.
// Called by the platform layer when the window is resized.
func (d *Device) Resize(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	d.width = width
	d.height = height

	if d.hasSurface {
		d.reconfigureSurface()
	} else {
		// Offscreen path: recreate the default color texture.
		d.recreateDefaultTexture()
	}

	// Keep the cached encoder's viewport dimensions in sync. Without
	// this, BeginRenderPass would call SetViewport with the original
	// size on every screen pass, scaling the rendered scene into the
	// top-left corner (or clipping it) until the next restart.
	if d.encoder != nil {
		d.encoder.width = width
		d.encoder.height = height
	}
}

// ResizeScreen is the interface method the engine invokes on every
// frame (see engine_desktop.go). For WebGPU native, screen and device
// dimensions are the same — forward to Resize, which reconfigures the
// swapchain and updates the encoder's cached viewport.
func (d *Device) ResizeScreen(width, height int) {
	if width == d.width && height == d.height {
		return
	}
	d.Resize(width, height)
}

// reconfigureSurface reconfigures the surface with current dimensions.
// Any outstanding view of the previous frame's surface texture is
// released first — wgpu-native rejects SurfaceConfigure while a
// SurfaceOutput from this surface is still held.
func (d *Device) reconfigureSurface() {
	if d.surface == 0 || d.device == 0 {
		return
	}
	if d.currentTexView != 0 {
		wgpu.TextureViewRelease(d.currentTexView)
		d.currentTexView = 0
	}
	surfaceCfg := wgpu.SurfaceConfiguration{
		Device:      d.device,
		Format:      d.surfaceFormat,
		Usage:       wgpu.TextureUsageRenderAttachment,
		AlphaMode:   wgpu.CompositeAlphaModeAuto,
		Width:       uint32(d.width),
		Height:      uint32(d.height),
		PresentMode: wgpu.PresentModeFifo,
	}
	wgpu.SurfaceConfigure(d.surface, &surfaceCfg)
}

// recreateDefaultTexture rebuilds the offscreen color texture after resize.
func (d *Device) recreateDefaultTexture() {
	if d.defaultColorView != 0 {
		wgpu.TextureViewRelease(d.defaultColorView)
		d.defaultColorView = 0
	}
	if d.defaultColorTex != 0 {
		wgpu.TextureRelease(d.defaultColorTex)
		d.defaultColorTex = 0
	}
	if d.device == 0 {
		return
	}
	texDesc := wgpu.TextureDescriptor{
		Usage:         wgpu.TextureUsage(wgpu.TextureUsageTextureBinding | wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopyDst | wgpu.TextureUsageCopySrc),
		Dimension:     2, // WGPUTextureDimension_2D
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

// NewTexture creates a WebGPU texture (GPUTexture).
func (d *Device) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("webgpu: invalid texture dimensions %dx%d", desc.Width, desc.Height)
	}

	wgpuFmt := wgpuTextureFormatEnum(desc.Format)
	usage := wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst | wgpu.TextureUsageCopySrc
	if desc.RenderTarget {
		usage |= wgpu.TextureUsageRenderAttachment
	}

	texDesc := wgpu.TextureDescriptor{
		Usage:         wgpu.TextureUsage(usage),
		Dimension:     2, // WGPUTextureDimension_2D
		Size:          wgpu.Extent3D{Width: uint32(desc.Width), Height: uint32(desc.Height), DepthOrArrayLayers: 1},
		Format:        wgpuFmt,
		MipLevelCount: 1,
		SampleCount:   1,
	}
	var labelBuf []byte
	if desc.Label != "" {
		texDesc.Label, labelBuf = wgpu.MakeStringView(desc.Label)
	}

	handle := wgpu.DeviceCreateTexture(d.device, &texDesc)
	runtime.KeepAlive(labelBuf)
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
	var labelBuf []byte
	if desc.Label != "" {
		bufDesc.Label, labelBuf = wgpu.MakeStringView(desc.Label)
	}

	handle := wgpu.DeviceCreateBuffer(d.device, &bufDesc)
	runtime.KeepAlive(labelBuf)
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
		Width:        desc.Width,
		Height:       desc.Height,
		Format:       colorFmt,
		RenderTarget: true,
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
			Width:        desc.Width,
			Height:       desc.Height,
			Format:       depthFmt,
			RenderTarget: true,
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
//
// The encoder is cached on first call and reused across the device's
// lifetime so Texture.ReadPixels can flush pending render work through
// it before issuing its own copy-texture-to-buffer command — the
// conformance suite's render-then-read-RT pattern relies on this.
func (d *Device) Encoder() backend.CommandEncoder {
	if d.encoder == nil {
		d.encoder = &Encoder{
			dev:    d,
			width:  d.width,
			height: d.height,
		}
	}
	return d.encoder
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
		return wgpu.BufferUsageVertex | wgpu.BufferUsageIndex
	case backend.BufferUsageIndex:
		return wgpu.BufferUsageIndex | wgpu.BufferUsageVertex
	case backend.BufferUsageUniform:
		return wgpu.BufferUsageUniform
	default:
		return wgpu.BufferUsageVertex | wgpu.BufferUsageIndex
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
