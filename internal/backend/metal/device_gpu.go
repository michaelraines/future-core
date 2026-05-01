//go:build darwin && !soft

package metal

import (
	"fmt"
	"os"
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

	// frameCmdBuffer is the command buffer being filled this frame.
	// Lazily created via ensureFrameStarted on the first
	// BeginRenderPass; committed in Device.EndFrame. Holds every
	// render pass for the frame, so the GPU sees one batched
	// submission instead of dozens of small ones.
	frameCmdBuffer mtl.CommandBuffer

	// lastCmdBuffer is the most recently committed (committed in
	// EndFrame, not yet drained) command buffer. ReadScreen /
	// ResizeScreen / Dispose wait on it before touching the screen
	// texture's CPU-visible storage.
	lastCmdBuffer mtl.CommandBuffer

	// frameAutoreleaseToken bounds the lifetime of every Cocoa
	// autoreleased object created during a frame: the command buffer
	// itself, the per-pass MTLRenderPassDescriptors and their
	// colorAttachments arrays, the RenderCommandEncoders, etc.
	// Without this pool the heap balloons in any process that doesn't
	// run NSApp's run loop (e.g. our headless capture path), and
	// scenes with many small render passes — scene-selector spawns
	// ~50 thumbnail RTs per frame — slow to a crawl after seconds.
	// Opened on first BeginRenderPass, drained in EndFrame after the
	// frame buffer is committed (and post-Dispose-of-deferred RTs).
	frameAutoreleaseToken uintptr

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

	if err := d.allocScreen(); err != nil {
		return err
	}

	// Create sampler states.
	d.defaultSampler = mtl.DeviceNewSamplerState(d.device, mtl.SamplerMinMagFilterNearest)
	d.linearSampler = mtl.DeviceNewSamplerState(d.device, mtl.SamplerMinMagFilterLinear)

	return nil
}

// allocScreen creates a shared buffer plus a buffer-backed texture sized to
// d.width × d.height. Used by Init and ResizeScreen. The buffer-backed
// texture lets ReadScreen copy via buffer.contents instead of
// getBytes:fromRegion: (which has arm64 struct ABI issues with purego).
func (d *Device) allocScreen() error {
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
		mtl.BufferRelease(d.screenBuffer)
		d.screenBuffer = 0
		return fmt.Errorf("metal: failed to create default color texture")
	}
	return nil
}

// ResizeScreen reallocates the default color texture and screen readback
// buffer when the framebuffer dimensions change — typically when the
// window moves between displays with different backingScaleFactor (Retina
// vs non-Retina). Without this the engine keeps drawing into a stale-size
// texture and ReadScreen memcpys a partial slice into a presenter buffer
// of a different size, producing torn/garbled output.
func (d *Device) ResizeScreen(width, height int) {
	if width <= 0 || height <= 0 || (width == d.width && height == d.height) {
		return
	}
	// Drain any in-flight work that targets the current screen texture
	// before we tear it down — releasing a texture the GPU is still
	// writing to is undefined behavior.
	if d.lastCmdBuffer != 0 {
		mtl.CommandBufferWaitUntilCompleted(d.lastCmdBuffer)
		d.lastCmdBuffer = 0
	}
	if d.defaultColorTex != 0 {
		mtl.TextureRelease(d.defaultColorTex)
		d.defaultColorTex = 0
	}
	if d.screenBuffer != 0 {
		mtl.BufferRelease(d.screenBuffer)
		d.screenBuffer = 0
	}
	d.width = width
	d.height = height
	if err := d.allocScreen(); err != nil {
		// Allocation failure leaves the device without a screen target;
		// next BeginRenderPass will see colorTex==0 and the frame is
		// dropped. Log via stderr so the failure isn't completely silent.
		fmt.Fprintf(os.Stderr, "metal: ResizeScreen(%d,%d) failed: %v\n", width, height, err)
	}
}

// Dispose releases all Metal resources.
func (d *Device) Dispose() {
	if d.lastCmdBuffer != 0 {
		mtl.CommandBufferWaitUntilCompleted(d.lastCmdBuffer)
		d.lastCmdBuffer = 0
	}
	if d.defaultColorTex != 0 {
		mtl.TextureRelease(d.defaultColorTex)
		d.defaultColorTex = 0
	}
	if d.screenBuffer != 0 {
		mtl.BufferRelease(d.screenBuffer)
		d.screenBuffer = 0
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
	// Drain the queue before reading. Defensive: the engine flow
	// calls EndFrame before ReadScreen, but standalone tests / mid-
	// frame readbacks may not. Either path lands here.
	d.EndFrame()
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

// ensureFrameStarted lazily opens this frame's autorelease pool and
// command buffer. Called from Encoder.BeginRenderPass — frames with
// no rendering work avoid the pool/buffer allocation entirely.
func (d *Device) ensureFrameStarted() {
	if d.frameAutoreleaseToken == 0 {
		d.frameAutoreleaseToken = mtl.AutoreleasePoolPush()
	}
	if d.frameCmdBuffer == 0 {
		d.frameCmdBuffer = mtl.CommandQueueCommandBuffer(d.commandQueue)
	}
}

// BeginFrame prepares for a new frame.
func (d *Device) BeginFrame() {}

// EndFrame commits this frame's batched command buffer. Encoders
// accumulated render passes into d.frameCmdBuffer across the frame;
// committing here gives the GPU one submission containing every pass.
// The wait is deferred to ReadScreen / ResizeScreen / Dispose so the
// CPU can race ahead to the next frame's CPU work.
//
// To avoid the cmdBuffer being released by the autorelease pool
// before the GPU finishes, we drain the queue here before popping the
// pool. (The CPU still pipelines vs the next frame's Update() — the
// next frame's render pool isn't pushed until the next BeginRenderPass.)
func (d *Device) EndFrame() {
	// Drain anything still in flight. Per-pass commits are committed
	// asynchronously; we wait on the most recent so the autorelease
	// pool drain that follows is safe — popping the pool releases the
	// committed command buffers, and that's only OK once the GPU is
	// done with them.
	if d.frameCmdBuffer != 0 {
		mtl.CommandBufferCommit(d.frameCmdBuffer)
		d.lastCmdBuffer = d.frameCmdBuffer
		d.frameCmdBuffer = 0
	}
	if d.lastCmdBuffer != 0 {
		mtl.CommandBufferWaitUntilCompleted(d.lastCmdBuffer)
		d.lastCmdBuffer = 0
	}
	if d.frameAutoreleaseToken != 0 {
		mtl.AutoreleasePoolPop(d.frameAutoreleaseToken)
		d.frameAutoreleaseToken = 0
	}
}

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
	} else {
		// Zero-fill new textures. Metal does NOT zero-initialize newly-
		// allocated texture storage, so a fresh RT — sampled before
		// anything is rendered into it — reads garbage. Other backends
		// either zero by default (soft) or pre-register a pending clear
		// (WebGPU's newImageLabeled). Metal needs the same treatment;
		// the cost is one Upload of zeros at allocation time, paid once
		// per RT instead of once per frame's load action.
		bpp := bytesPerPixel(desc.Format)
		zeros := make([]byte, desc.Width*desc.Height*bpp)
		tex.Upload(zeros, 0)
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
	return &Encoder{dev: d}
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
