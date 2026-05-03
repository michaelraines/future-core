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

	// 1x1 white texture used to fill any sampler slot the shader's MSL
	// signature declares but the engine doesn't bind. Metal validation
	// drops draws that leave a declared texture/sampler binding nil,
	// which presented as fully-black render targets on cells that ran
	// effect shaders declaring uTexture0..3 but only sampling slot 0.
	whiteTex mtl.Texture

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

	// rtSyncFence serialises producer→consumer dependencies between
	// render encoders within a single command buffer. Apple's Metal
	// auto-tracker is supposed to handle the render-target →
	// fragment-shader-read pattern (pass A writes to texture T, pass B
	// samples T) without explicit sync, but in practice (especially on
	// the iso-combat / lighting-demo offscreen-RT-then-sample pattern)
	// the implicit transition isn't always honoured — pass B reads
	// stale or zero contents. The fix Apple recommends in their
	// developer-forum threads on this exact symptom is MTLFence:
	// updateFence on the producer's EndRenderPass with the fragment
	// stage, waitForFence on the consumer's BeginRenderPass with the
	// fragment stage. A single shared fence serialises all offscreen
	// passes into the next pass that reads from any RT, which costs
	// some pass-level parallelism but is correct.
	//
	// Apple Developer Forums "Coherency, synchronization, scheduling":
	// recommends MTLFence (Marek Simonik response) for cross-encoder
	// dependencies within a single command queue.
	rtSyncFence mtl.Fence

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

	// Create a 1x1 white texture for binding at unused sampler slots.
	whiteDesc := mtl.TextureDescriptor{
		PixelFormat: mtl.PixelFormatRGBA8Unorm,
		Width:       1, Height: 1, Depth: 1,
		MipmapCount: 1,
		SampleCount: 1,
		StorageMode: mtl.StorageModeShared,
		Usage:       mtl.TextureUsageShaderRead,
	}
	d.whiteTex = mtl.DeviceNewTexture(d.device, &whiteDesc)
	if d.whiteTex != 0 {
		whiteData := []byte{255, 255, 255, 255}
		region := mtl.Region{Size: mtl.Size{Width: 1, Height: 1, Depth: 1}}
		mtl.TextureReplaceRegion(d.whiteTex, region, 0, unsafe.Pointer(&whiteData[0]), 4)
	}

	// Allocate the cross-encoder sync fence. Per Apple Developer Forums
	// (thread 25270), fences are the recommended primitive for the
	// "pass A writes texture, pass B samples it within the same
	// command buffer" pattern when implicit hazard tracking misbehaves.
	d.rtSyncFence = mtl.DeviceNewFence(d.device)
	if d.rtSyncFence == 0 {
		return fmt.Errorf("metal: failed to allocate rtSyncFence")
	}

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
	if d.whiteTex != 0 {
		mtl.TextureRelease(d.whiteTex)
		d.whiteTex = 0
	}
	if d.screenBuffer != 0 {
		mtl.BufferRelease(d.screenBuffer)
		d.screenBuffer = 0
	}
	if d.rtSyncFence != 0 {
		mtl.FenceRelease(d.rtSyncFence)
		d.rtSyncFence = 0
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

	// Storage mode selection — this is the iso-combat fix.
	//
	// Shared storage gives the GPU's L2 cache and the CPU-visible
	// backing store the same view of the texture. For RTs that are
	// written via render passes and then sampled in a later pass
	// within the same command buffer, Apple's auto-tracker sometimes
	// fails to invalidate the sampler-side view of the L2 cache —
	// the sampling pass reads stale data (often whatever uniform
	// memory the engine's whiteTexture happens to occupy). Visible
	// in iso-combat as the terrain atlas rendering as the engine's
	// whiteTexture content.
	//
	// Private storage forces the texture to live in GPU-only memory.
	// Apple's Metal driver tracks producer-consumer dependencies for
	// Private textures correctly across encoders within a command
	// buffer (matches Vulkan's MemoryPropertyDeviceLocal behaviour).
	// CPU uploads (replaceRegion) are invalid on Private — those
	// route through a Shared staging buffer + blit copy in
	// Texture.Upload / Texture.UploadRegion.
	//
	// Heuristic: textures created with seeded Data go to Shared
	// (avoids the staging-buffer round-trip for the one-shot upload
	// at creation). Empty textures created for render-target use go
	// to Private. The sprite-atlas page (sprite_atlas.go) creates
	// without Data and then uploads via UploadRegion — those land on
	// Private and use the staging-buffer path; the cost is a single
	// extra blit copy per atlas-pack which is amortised across many
	// frames of sampling.
	storage := mtl.StorageModeShared
	if len(desc.Data) == 0 {
		storage = mtl.StorageModePrivate
	}

	texDesc := mtl.TextureDescriptor{
		PixelFormat: pf,
		Width:       uint64(desc.Width),
		Height:      uint64(desc.Height),
		Depth:       1,
		MipmapCount: 1,
		SampleCount: 1,
		StorageMode: storage,
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
		storageMode: storage,
	}

	if len(desc.Data) > 0 {
		tex.Upload(desc.Data, 0)
	} else {
		// Fresh RT — submit a synchronous one-shot render pass that
		// clears the texture to transparent black. Mirrors Vulkan's
		// clearFreshRenderTarget (commit 359f00b). Without it, an
		// RT-capable shared-storage texture contains undefined GPU
		// memory after creation; the engine's
		// `pendingClear → LoadActionClear-on-first-use` only fires
		// when the texture is *rendered into*, so an atlas RT that
		// is sampled before the engine issues a render pass against
		// it (e.g. mid-frame ordering when render-pass A samples
		// the atlas and render-pass B is the one that would have
		// done the LoadActionClear) reads garbage — visible in
		// isometric-combat as the terrain atlas sampling solid
		// white. A GPU-side render-pass clear is the analog of the
		// CPU zero-fill removed in d4223db, without the CPU↔GPU
		// race that motivated removal.
		d.clearFreshTexture(uintptr(handle), desc.Width, desc.Height, bytesPerPixel(desc.Format))
	}

	return tex, nil
}

// clearFreshTexture zero-initialises the given color texture by
// blit-copying zeros from a Shared staging buffer. Works on both
// Shared and Private storage textures; an empty render pass
// (LoadActionClear+StoreActionStore with no draws) is supposed to
// achieve the same on Apple Silicon, but the Metal driver
// optimises away passes with no encoded work — leaving a Private
// texture with uninitialised GPU memory. The blit copy is a real
// command the driver can't elide. Synchronous: blocks until the
// GPU is done so the texture has predictable contents before any
// subsequent use as a sample source or render target.
func (d *Device) clearFreshTexture(colorTex uintptr, w, h int, bpp int) {
	if d.commandQueue == 0 || colorTex == 0 || w <= 0 || h <= 0 {
		return
	}
	bytesPerRow := uint64(w * bpp)
	totalBytes := uint64(h) * bytesPerRow

	staging := mtl.DeviceNewBuffer(d.device, totalBytes, mtl.ResourceStorageModeShared)
	if staging == 0 {
		return
	}
	defer mtl.BufferRelease(staging)
	// Buffer contents are zero-initialised by Metal on creation; no
	// CPU-side memset needed.

	cmdBuf := mtl.CommandQueueCommandBuffer(d.commandQueue)
	if cmdBuf == 0 {
		return
	}
	blit := mtl.CommandBufferBlitCommandEncoder(cmdBuf)
	if blit == 0 {
		mtl.CommandBufferCommit(cmdBuf)
		return
	}
	mtl.BlitCommandEncoderCopyFromBufferToTexture(blit,
		staging, 0, bytesPerRow, totalBytes,
		mtl.Size{Width: uint64(w), Height: uint64(h), Depth: 1},
		mtl.Texture(colorTex), 0, 0,
		mtl.Origin{})
	mtl.BlitCommandEncoderEndEncoding(blit)
	mtl.CommandBufferCommit(cmdBuf)
	mtl.CommandBufferWaitUntilCompleted(cmdBuf)
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
