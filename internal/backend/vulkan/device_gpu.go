//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/vk"
)

// Device implements backend.Device for Vulkan with real GPU bindings.
type Device struct {
	instance       vk.Instance
	physicalDevice vk.PhysicalDevice
	device         vk.Device
	graphicsQueue  vk.Queue
	queueFamily    uint32
	commandPool    vk.CommandPool
	commandBuffer  vk.CommandBuffer
	fence          vk.Fence
	memProps       vk.PhysicalDeviceMemoryProperties
	devProps       vk.PhysicalDeviceProperties
	encoder        *Encoder

	// Default render pass for the screen target.
	defaultRenderPass     vk.RenderPass // LoadOp=Clear
	defaultRenderPassLoad vk.RenderPass // LoadOp=Load (for screen re-entries)
	defaultFramebuffer    vk.Framebuffer
	defaultColorImage     vk.Image
	defaultColorView      vk.ImageView
	defaultColorMem       vk.DeviceMemory

	// Default depth-stencil attachment: D24_UNORM_S8_UINT image + view
	// allocated alongside the default color attachment. Used by both
	// the offscreen default RT and the swapchain RT so every render
	// pass has a matching depth-stencil attachment for pipelines that
	// declare stencil state.
	defaultDepthStencilImage vk.Image
	defaultDepthStencilView  vk.ImageView
	defaultDepthStencilMem   vk.DeviceMemory

	width, height int

	// Staging buffer for texture uploads/readbacks.
	stagingBuffer vk.Buffer
	stagingMemory vk.DeviceMemory
	stagingSize   int
	stagingMapped unsafe.Pointer

	// Sampler cache keyed by filter mode. `SetTextureFilter` triggers
	// on-demand creation of per-filter samplers; the descriptor binding
	// path picks the one matching the encoder's current filter state.
	// Before this cache existed, Vulkan always bound a Nearest sampler
	// and silently ignored every FilterLinear request — which broke the
	// AA buffer downsample (supposed to be linear) and any other linear
	// texture sample through this backend.
	samplers map[backend.TextureFilter]vk.Sampler
	// defaultTexture is a 1x1 white texture used as a fallback when no
	// texture is bound at a draw (e.g. vector fills with src=nil).
	defaultTexture *Texture

	// Shared uniform buffer for UBO descriptors (persistently mapped).
	// Uses a ring-buffer write cursor so each draw's UBO data persists
	// until the command buffer executes.
	uniformBuffer  vk.Buffer
	uniformMemory  vk.DeviceMemory
	uniformMapped  unsafe.Pointer
	uniformBufSize int
	uniformCursor  int // next write offset (advanced per draw, reset per frame)

	// Vulkan-specific state for public API compatibility.
	instanceInfo       InstanceCreateInfo
	physicalDeviceInfo PhysicalDeviceInfo
	debugEnabled       bool

	// Swapchain state (populated when presenting directly to a surface).
	surfaceFactory          func(uintptr) (uintptr, error)
	surface                 vk.SurfaceKHR
	swapchain               vk.SwapchainKHR
	swapchainImages         []vk.Image
	swapchainViews          []vk.ImageView
	swapchainFormat         uint32
	swapchainExtent         [2]uint32
	swapchainFBs            []vk.Framebuffer
	swapchainRenderPass     vk.RenderPass // LoadOp=Clear
	swapchainRenderPassLoad vk.RenderPass // LoadOp=Load (for sprite-pass screen re-entries)
	currentImageIndex       uint32
	hasSwapchain            bool
	imageAvailableSem       vk.Semaphore
	renderFinishedSem       vk.Semaphore

	// disposing is set by Device.Dispose before cascading to per-resource
	// teardown. Every Texture.Dispose / RenderTarget.Dispose was calling
	// vk.DeviceWaitIdle individually as a "wait for in-flight GPU work"
	// safety measure — correct in isolation but catastrophic at shutdown
	// when hundreds of resources each re-wait the whole device (quadratic
	// behaviour on Metal via MoltenVK; multi-second window-close hangs,
	// macOS "application not responding" dialogs, and orphaned windows
	// when the user force-quits). With this flag, per-resource Dispose
	// code paths skip their own wait after Device.Dispose has already
	// done a single DeviceWaitIdle up top — one idle-wait for the whole
	// teardown instead of O(resources).
	disposing bool
}

// samplerFor returns a cached VkSampler configured for the requested
// filter mode, creating one on first request. The cache is tiny in
// practice — the engine only uses Nearest and Linear — but a map keeps
// this ready for future filter additions (mipmap modes, anisotropy)
// without reshuffling callers.
func (d *Device) samplerFor(filter backend.TextureFilter) vk.Sampler {
	if d.samplers == nil {
		d.samplers = make(map[backend.TextureFilter]vk.Sampler)
	}
	if s, ok := d.samplers[filter]; ok && s != 0 {
		return s
	}

	vkFilter := uint32(vk.FilterNearest)
	vkMipmap := uint32(vk.SamplerMipmapModeNearest)
	if filter == backend.FilterLinear {
		vkFilter = uint32(vk.FilterLinear)
		vkMipmap = uint32(vk.SamplerMipmapModeLinear)
	}
	ci := vk.SamplerCreateInfo{
		SType:        vk.StructureTypeSamplerCreateInfo,
		MagFilter:    vkFilter,
		MinFilter:    vkFilter,
		MipmapMode:   vkMipmap,
		AddressModeU: vk.SamplerAddressModeClampToEdge,
		AddressModeV: vk.SamplerAddressModeClampToEdge,
		AddressModeW: vk.SamplerAddressModeClampToEdge,
		MaxLod:       1.0,
	}
	s, err := vk.CreateSampler(d.device, &ci)
	if err != nil {
		return 0
	}
	d.samplers[filter] = s
	return s
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

// PhysicalDeviceInfo holds properties queried from vkGetPhysicalDeviceProperties.
type PhysicalDeviceInfo struct {
	DeviceName  string
	DeviceType  int
	VendorID    uint32
	MaxImageDim int
	MaxSamples  int
}

// New creates a new Vulkan device (uninitialized — call Init after window creation).
func New() *Device {
	return &Device{
		instanceInfo: InstanceCreateInfo{
			AppName:    "future-core",
			EngineName: "future-core",
			APIVersion: vkAPIVersion1_2,
		},
	}
}

// Init initializes the Vulkan device: loads the library, creates instance,
// selects a physical device, creates a logical device and command infrastructure.
func (d *Device) Init(cfg backend.DeviceConfig) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("vulkan: invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
	d.width = cfg.Width
	d.height = cfg.Height
	d.debugEnabled = cfg.Debug || os.Getenv("FUTURE_CORE_VK_VALIDATION") == "1"
	d.surfaceFactory = cfg.SurfaceFactory

	// Load Vulkan library.
	if err := vk.Init(); err != nil {
		return fmt.Errorf("vulkan: %w", err)
	}

	// If we have a surface factory, request the necessary instance extensions.
	if d.surfaceFactory != nil {
		// Query available extensions so we only request ones that exist.
		availExts, _ := vk.EnumerateInstanceExtensionProperties()
		hasExt := func(name string) bool {
			for _, e := range availExts {
				if e == name {
					return true
				}
			}
			return false
		}

		d.instanceInfo.Extensions = appendUnique(d.instanceInfo.Extensions, "VK_KHR_surface")
		switch runtime.GOOS {
		case "darwin":
			d.instanceInfo.Extensions = appendUnique(d.instanceInfo.Extensions, "VK_EXT_metal_surface")
			// MoltenVK may require portability enumeration.
			if hasExt("VK_KHR_portability_enumeration") {
				d.instanceInfo.Extensions = appendUnique(d.instanceInfo.Extensions, "VK_KHR_portability_enumeration")
			}
		case "windows":
			d.instanceInfo.Extensions = appendUnique(d.instanceInfo.Extensions, "VK_KHR_win32_surface")
		case "android":
			d.instanceInfo.Extensions = appendUnique(d.instanceInfo.Extensions, "VK_KHR_android_surface")
		default: // linux, freebsd
			d.instanceInfo.Extensions = appendUnique(d.instanceInfo.Extensions, "VK_KHR_xlib_surface")
		}
	}

	// Set up validation layers if debug mode.
	if d.debugEnabled {
		const validationLayer = "VK_LAYER_KHRONOS_validation"
		availLayers, _ := vk.EnumerateInstanceLayerProperties()
		hasLayer := false
		for _, l := range availLayers {
			if l == validationLayer {
				hasLayer = true
				break
			}
		}
		if hasLayer {
			d.instanceInfo.Layers = appendUnique(d.instanceInfo.Layers, validationLayer)
			fmt.Println("[vulkan] validation layer enabled")
		} else if d.debugEnabled {
			fmt.Println("[vulkan] validation layer not available")
		}
	}

	// Create Vulkan instance.
	appName := vk.CStr(d.instanceInfo.AppName)
	engineName := vk.CStr(d.instanceInfo.EngineName)
	runtime.KeepAlive(appName)
	runtime.KeepAlive(engineName)

	appInfo := vk.ApplicationInfo{
		SType:              vk.StructureTypeApplicationInfo,
		PApplicationName:   uintptr(unsafe.Pointer(appName)),
		ApplicationVersion: d.instanceInfo.AppVersion,
		PEngineName:        uintptr(unsafe.Pointer(engineName)),
		EngineVersion:      d.instanceInfo.EngineVersion,
		APIVersion:         d.instanceInfo.APIVersion,
	}

	createInfo := vk.InstanceCreateInfo{
		SType:            vk.StructureTypeInstanceCreateInfo,
		PApplicationInfo: uintptr(unsafe.Pointer(&appInfo)),
	}

	if len(d.instanceInfo.Layers) > 0 {
		cLayers := vk.CStrSlice(d.instanceInfo.Layers)
		createInfo.EnabledLayerCount = uint32(len(cLayers))
		createInfo.PPEnabledLayerNames = uintptr(unsafe.Pointer(&cLayers[0]))
		runtime.KeepAlive(cLayers)
	}

	if len(d.instanceInfo.Extensions) > 0 {
		cExts := vk.CStrSlice(d.instanceInfo.Extensions)
		createInfo.EnabledExtensionCount = uint32(len(cExts))
		createInfo.PPEnabledExtensionNames = uintptr(unsafe.Pointer(&cExts[0]))
		runtime.KeepAlive(cExts)
	}

	// MoltenVK on macOS may require portability enumeration flag.
	if runtime.GOOS == "darwin" {
		for _, ext := range d.instanceInfo.Extensions {
			if ext == "VK_KHR_portability_enumeration" {
				createInfo.Flags |= 0x00000001 // VK_INSTANCE_CREATE_ENUMERATE_PORTABILITY_BIT_KHR
				break
			}
		}
	}

	inst, err := vk.CreateInstance(&createInfo)
	if err != nil {
		return fmt.Errorf("vulkan: %w", err)
	}
	d.instance = inst

	// Load KHR extension functions if we have a surface factory.
	if d.surfaceFactory != nil {
		if err := vk.InitSwapchainFunctions(inst); err != nil {
			return fmt.Errorf("vulkan: %w", err)
		}
	}

	// Select physical device (prefer discrete GPU).
	physDevices, err := vk.EnumeratePhysicalDevices(inst)
	if err != nil {
		return fmt.Errorf("vulkan: %w", err)
	}
	if len(physDevices) == 0 {
		return fmt.Errorf("vulkan: no physical devices found")
	}

	d.physicalDevice = physDevices[0]
	for _, pd := range physDevices {
		props := vk.GetPhysicalDeviceProperties(pd)
		if props.DeviceType == 2 { // discrete GPU
			d.physicalDevice = pd
			break
		}
	}

	d.devProps = vk.GetPhysicalDeviceProperties(d.physicalDevice)
	d.memProps = vk.GetPhysicalDeviceMemoryProperties(d.physicalDevice)

	// Parse device name from null-terminated bytes.
	nameBytes := d.devProps.DeviceName[:]
	nameLen := 0
	for i, b := range nameBytes {
		if b == 0 {
			nameLen = i
			break
		}
	}
	d.physicalDeviceInfo = PhysicalDeviceInfo{
		DeviceName:  string(nameBytes[:nameLen]),
		DeviceType:  int(d.devProps.DeviceType),
		VendorID:    d.devProps.VendorID,
		MaxImageDim: 8192,
		MaxSamples:  4,
	}

	// Find a graphics queue family.
	queueFamilies := vk.GetPhysicalDeviceQueueFamilyProperties(d.physicalDevice)
	d.queueFamily = 0
	found := false
	for i, qf := range queueFamilies {
		if qf.QueueFlags&vk.QueueGraphics != 0 {
			d.queueFamily = uint32(i)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("vulkan: no graphics queue family found")
	}

	// Create logical device with one graphics queue.
	queuePriority := float32(1.0)
	queueCI := vk.DeviceQueueCreateInfo{
		SType:            vk.StructureTypeDeviceQueueCreateInfo,
		QueueFamilyIndex: d.queueFamily,
		QueueCount:       1,
		PQueuePriorities: uintptr(unsafe.Pointer(&queuePriority)),
	}

	deviceCI := vk.DeviceCreateInfo{
		SType:                vk.StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount: 1,
		PQueueCreateInfos:    uintptr(unsafe.Pointer(&queueCI)),
	}

	// Enable swapchain device extension if presenting.
	var devExts []*byte
	if d.surfaceFactory != nil {
		devExts = vk.CStrSlice([]string{"VK_KHR_swapchain"})
		deviceCI.EnabledExtensionCount = uint32(len(devExts))
		deviceCI.PPEnabledExtensionNames = uintptr(unsafe.Pointer(&devExts[0]))
	}

	dev, err := vk.CreateDevice(d.physicalDevice, &deviceCI)
	runtime.KeepAlive(devExts)
	if err != nil {
		return fmt.Errorf("vulkan: %w", err)
	}
	d.device = dev
	d.graphicsQueue = vk.GetDeviceQueue(dev, d.queueFamily, 0)

	// Create command pool and buffer.
	poolCI := vk.CommandPoolCreateInfo{
		SType:            vk.StructureTypeCommandPoolCreateInfo,
		Flags:            vk.CommandPoolCreateResetCommandBuffer,
		QueueFamilyIndex: d.queueFamily,
	}
	pool, err := vk.CreateCommandPool(dev, &poolCI)
	if err != nil {
		return fmt.Errorf("vulkan: %w", err)
	}
	d.commandPool = pool

	cmd, err := vk.AllocateCommandBuffer(dev, pool)
	if err != nil {
		return fmt.Errorf("vulkan: %w", err)
	}
	d.commandBuffer = cmd

	// Create fence for synchronization.
	fence, err := vk.CreateFence(dev, true)
	if err != nil {
		return fmt.Errorf("vulkan: %w", err)
	}
	d.fence = fence

	// Create default render target (offscreen).
	if err := d.createDefaultRenderTarget(); err != nil {
		return fmt.Errorf("vulkan: %w", err)
	}

	// Create staging buffer for transfers. Must be at least as large as the
	// framebuffer for ReadScreen to work (RGBA8 = 4 bytes per pixel).
	stagingSize := 4 * 1024 * 1024 // 4 MB minimum
	framebufferSize := d.width * d.height * 4
	if framebufferSize > stagingSize {
		stagingSize = framebufferSize
	}
	if err := d.createStagingBuffer(stagingSize); err != nil {
		return fmt.Errorf("vulkan: staging: %w", err)
	}

	// Create shared uniform buffer for UBO descriptors (1 MB, persistently
	// mapped). Sized to comfortably absorb a worst-case multi-RT frame:
	// scene-selector records ~100 sprite-pass batches per frame, each
	// consuming vtxAligned+fragAligned = 512 bytes of uniform space — so
	// 16 KB (the original size) wraps ~3 times mid-frame and overwrites
	// uniforms the GPU is still reading for earlier draws, silently
	// corrupting them and rendering the final composite blank.
	// 1 MB fits ~2000 draws and is still trivial on any desktop GPU.
	if err := d.createUniformBuffer(1024 * 1024); err != nil {
		return fmt.Errorf("vulkan: uniform buffer: %w", err)
	}

	// Create a 1x1 white default texture for fallback sampler binding.
	defTex, terr := d.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1,
		Format: backend.TextureFormatRGBA8,
		Data:   []byte{255, 255, 255, 255},
	})
	if terr != nil {
		return fmt.Errorf("vulkan: default texture: %w", terr)
	}
	d.defaultTexture = defTex.(*Texture)

	// Create encoder.
	d.encoder = &Encoder{dev: d, cmd: d.commandBuffer}

	// Set up swapchain if we have a surface factory.
	if d.surfaceFactory != nil {
		surfaceHandle, serr := d.surfaceFactory(uintptr(d.instance))
		if serr != nil {
			return fmt.Errorf("vulkan: surface creation: %w", serr)
		}
		d.surface = vk.SurfaceKHR(surfaceHandle)

		// Verify graphics queue supports presentation.
		supported, serr := vk.GetPhysicalDeviceSurfaceSupportKHR(d.physicalDevice, d.queueFamily, d.surface)
		if serr != nil {
			return fmt.Errorf("vulkan: %w", serr)
		}
		if !supported {
			return fmt.Errorf("vulkan: graphics queue family does not support presentation")
		}

		if serr := d.createSwapchain(); serr != nil {
			return fmt.Errorf("vulkan: %w", serr)
		}

		// Create synchronization semaphores for swapchain.
		imgSem, serr := vk.CreateSemaphore(d.device)
		if serr != nil {
			return fmt.Errorf("vulkan: %w", serr)
		}
		d.imageAvailableSem = imgSem

		renSem, serr := vk.CreateSemaphore(d.device)
		if serr != nil {
			return fmt.Errorf("vulkan: %w", serr)
		}
		d.renderFinishedSem = renSem
		d.hasSwapchain = true
	}

	return nil
}

// createDepthStencilTexture allocates a D24_UNORM_S8_UINT image at the
// given dimensions with a matching image view. Memory lifetime is
// device-owned; callers pass the handles back in to a later
// destroyDepthStencilTexture call. Returned triple is (image, memory,
// view); on error the partially-allocated resources are freed before
// returning.
func (d *Device) createDepthStencilTexture(width, height uint32) (vk.Image, vk.DeviceMemory, vk.ImageView, error) {
	imgCI := vk.ImageCreateInfo{
		SType:       vk.StructureTypeImageCreateInfo,
		ImageType:   vk.ImageType2D,
		Format:      vk.FormatD24UNormS8UInt,
		ExtentWidth: width, ExtentHeight: height, ExtentDepth: 1,
		MipLevels: 1, ArrayLayers: 1,
		Samples:       vk.SampleCount1,
		Tiling:        vk.ImageTilingOptimal,
		Usage:         uint32(vk.ImageUsageDepthStencilAttach),
		SharingMode:   vk.SharingModeExclusive,
		InitialLayout: vk.ImageLayoutUndefined,
	}
	img, err := vk.CreateImageRaw(d.device, &imgCI)
	if err != nil {
		return 0, 0, 0, err
	}

	memReq := vk.GetImageMemoryRequirements(d.device, img)
	memIdx, err := vk.FindMemoryType(d.memProps, memReq.MemoryTypeBits, vk.MemoryPropertyDeviceLocal)
	if err != nil {
		vk.DestroyImage(d.device, img)
		return 0, 0, 0, err
	}
	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReq.Size,
		MemoryTypeIndex: memIdx,
	}
	mem, err := vk.AllocateMemory(d.device, &allocInfo)
	if err != nil {
		vk.DestroyImage(d.device, img)
		return 0, 0, 0, err
	}
	if err := vk.BindImageMemory(d.device, img, mem, 0); err != nil {
		vk.FreeMemory(d.device, mem)
		vk.DestroyImage(d.device, img)
		return 0, 0, 0, err
	}

	viewCI := vk.ImageViewCreateInfo{
		SType:            vk.StructureTypeImageViewCreateInfo,
		Image:            img,
		ViewType:         vk.ImageViewType2D,
		Format:           vk.FormatD24UNormS8UInt,
		ComponentR:       vk.ComponentSwizzleIdentity,
		ComponentG:       vk.ComponentSwizzleIdentity,
		ComponentB:       vk.ComponentSwizzleIdentity,
		ComponentA:       vk.ComponentSwizzleIdentity,
		SubresAspectMask: vk.ImageAspectDepthStencil,
		SubresBaseMip:    0, SubresLevelCount: 1,
		SubresBaseLayer: 0, SubresLayerCount: 1,
	}
	view, err := vk.CreateImageViewRaw(d.device, &viewCI)
	if err != nil {
		vk.FreeMemory(d.device, mem)
		vk.DestroyImage(d.device, img)
		return 0, 0, 0, err
	}
	return img, mem, view, nil
}

// destroyDepthStencilTexture releases a depth-stencil texture triple
// previously allocated by createDepthStencilTexture. Safe to call with
// zero handles.
func (d *Device) destroyDepthStencilTexture(img vk.Image, mem vk.DeviceMemory, view vk.ImageView) {
	if view != 0 {
		vk.DestroyImageView(d.device, view)
	}
	if img != 0 {
		vk.DestroyImage(d.device, img)
	}
	if mem != 0 {
		vk.FreeMemory(d.device, mem)
	}
}

// createDefaultRenderTarget creates the offscreen color attachment.
func (d *Device) createDefaultRenderTarget() error {
	imgCI := vk.ImageCreateInfo{
		SType:       vk.StructureTypeImageCreateInfo,
		ImageType:   vk.ImageType2D,
		Format:      vk.FormatR8G8B8A8UNorm,
		ExtentWidth: uint32(d.width), ExtentHeight: uint32(d.height), ExtentDepth: 1,
		MipLevels: 1, ArrayLayers: 1,
		Samples:       vk.SampleCount1,
		Tiling:        vk.ImageTilingOptimal,
		Usage:         uint32(vk.ImageUsageColorAttachment | vk.ImageUsageTransferSrc | vk.ImageUsageTransferDst),
		SharingMode:   vk.SharingModeExclusive,
		InitialLayout: vk.ImageLayoutUndefined,
	}
	img, err := vk.CreateImageRaw(d.device, &imgCI)
	if err != nil {
		return err
	}
	d.defaultColorImage = img

	// Allocate and bind memory.
	memReq := vk.GetImageMemoryRequirements(d.device, img)
	memIdx, err := vk.FindMemoryType(d.memProps, memReq.MemoryTypeBits, vk.MemoryPropertyDeviceLocal)
	if err != nil {
		return err
	}
	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReq.Size,
		MemoryTypeIndex: memIdx,
	}
	mem, err := vk.AllocateMemory(d.device, &allocInfo)
	if err != nil {
		return err
	}
	d.defaultColorMem = mem
	if err := vk.BindImageMemory(d.device, img, mem, 0); err != nil {
		return err
	}

	// Create image view.
	viewCI := vk.ImageViewCreateInfo{
		SType:            vk.StructureTypeImageViewCreateInfo,
		Image:            img,
		ViewType:         vk.ImageViewType2D,
		Format:           vk.FormatR8G8B8A8UNorm,
		ComponentR:       vk.ComponentSwizzleIdentity,
		ComponentG:       vk.ComponentSwizzleIdentity,
		ComponentB:       vk.ComponentSwizzleIdentity,
		ComponentA:       vk.ComponentSwizzleIdentity,
		SubresAspectMask: vk.ImageAspectColor,
		SubresBaseMip:    0, SubresLevelCount: 1,
		SubresBaseLayer: 0, SubresLayerCount: 1,
	}
	view, err := vk.CreateImageViewRaw(d.device, &viewCI)
	if err != nil {
		return err
	}
	d.defaultColorView = view

	// Depth-stencil attachment so pipelines that declare stencil state
	// (sprite pass fill-rule routing, etc.) can run against the offscreen
	// default target. Matches the format baked into the render pass's
	// second attachment below.
	dsImg, dsMem, dsView, err := d.createDepthStencilTexture(uint32(d.width), uint32(d.height))
	if err != nil {
		return err
	}
	d.defaultDepthStencilImage = dsImg
	d.defaultDepthStencilMem = dsMem
	d.defaultDepthStencilView = dsView

	// Create render pass with color + depth-stencil attachments.
	attachments := [2]vk.AttachmentDescription{
		{
			Format:         vk.FormatR8G8B8A8UNorm,
			Samples:        vk.SampleCount1,
			LoadOp:         vk.AttachmentLoadOpClear,
			StoreOp:        vk.AttachmentStoreOpStore,
			StencilLoadOp:  vk.AttachmentLoadOpDontCare,
			StencilStoreOp: vk.AttachmentStoreOpDontCare,
			InitialLayout:  vk.ImageLayoutUndefined,
			FinalLayout:    vk.ImageLayoutColorAttachmentOptimal,
		},
		{
			Format:         vk.FormatD24UNormS8UInt,
			Samples:        vk.SampleCount1,
			LoadOp:         vk.AttachmentLoadOpClear,
			StoreOp:        vk.AttachmentStoreOpStore,
			StencilLoadOp:  vk.AttachmentLoadOpClear,
			StencilStoreOp: vk.AttachmentStoreOpStore,
			InitialLayout:  vk.ImageLayoutUndefined,
			FinalLayout:    vk.ImageLayoutDepthStencilAttachOptimal,
		},
	}
	colorRef := vk.AttachmentReference{
		Attachment: 0,
		Layout:     vk.ImageLayoutColorAttachmentOptimal,
	}
	depthRef := vk.AttachmentReference{
		Attachment: 1,
		Layout:     vk.ImageLayoutDepthStencilAttachOptimal,
	}
	subpass := vk.SubpassDescription{
		PipelineBindPoint:       vk.PipelineBindPointGraphics,
		ColorAttachmentCount:    1,
		PColorAttachments:       uintptr(unsafe.Pointer(&colorRef)),
		PDepthStencilAttachment: uintptr(unsafe.Pointer(&depthRef)),
	}
	dependency := vk.SubpassDependency{
		SrcSubpass:    0xFFFFFFFF, // VK_SUBPASS_EXTERNAL
		DstSubpass:    0,
		SrcStageMask:  vk.PipelineStageColorAttachmentOutput,
		DstStageMask:  vk.PipelineStageColorAttachmentOutput,
		DstAccessMask: vk.AccessColorAttachmentWrite,
	}
	rpCI := vk.RenderPassCreateInfo{
		SType:           vk.StructureTypeRenderPassCreateInfo,
		AttachmentCount: 2,
		PAttachments:    uintptr(unsafe.Pointer(&attachments[0])),
		SubpassCount:    1,
		PSubpasses:      uintptr(unsafe.Pointer(&subpass)),
		DependencyCount: 1,
		PDependencies:   uintptr(unsafe.Pointer(&dependency)),
	}
	rp, err := vk.CreateRenderPass(d.device, &rpCI)
	runtime.KeepAlive(attachments)
	if err != nil {
		return err
	}
	d.defaultRenderPass = rp

	// Load variant: same attachments, LoadOp=Load on color, so screen
	// re-entries within a frame preserve accumulated content. Without
	// this, the sprite pass's multi-composite pattern (draw offscreen
	// RT → composite to screen → draw another RT → composite again)
	// wipes prior composites on every screen re-entry, leaving only the
	// final composite visible. Framebuffer is render-pass-compatible
	// with both variants since attachment formats and counts match.
	// InitialLayout must match the prior pass's FinalLayout; for the
	// default offscreen target that's ColorAttachmentOptimal.
	attachmentsLoad := attachments
	attachmentsLoad[0].LoadOp = vk.AttachmentLoadOpLoad
	attachmentsLoad[0].InitialLayout = vk.ImageLayoutColorAttachmentOptimal
	rpCILoad := rpCI
	rpCILoad.PAttachments = uintptr(unsafe.Pointer(&attachmentsLoad[0]))
	rpLoad, err := vk.CreateRenderPass(d.device, &rpCILoad)
	runtime.KeepAlive(attachmentsLoad)
	if err != nil {
		return err
	}
	d.defaultRenderPassLoad = rpLoad

	// Create framebuffer with both attachments.
	fbViews := [2]vk.ImageView{d.defaultColorView, d.defaultDepthStencilView}
	fbCI := vk.FramebufferCreateInfo{
		SType:           vk.StructureTypeFramebufferCreateInfo,
		RenderPass_:     rp,
		AttachmentCount: 2,
		PAttachments:    uintptr(unsafe.Pointer(&fbViews[0])),
		Width:           uint32(d.width),
		Height:          uint32(d.height),
		Layers:          1,
	}
	fb, err := vk.CreateFramebuffer(d.device, &fbCI)
	runtime.KeepAlive(fbViews)
	if err != nil {
		return err
	}
	d.defaultFramebuffer = fb

	return nil
}

// createStagingBuffer creates a host-visible buffer for CPU↔GPU transfers.
func (d *Device) createStagingBuffer(size int) error {
	bufCI := vk.BufferCreateInfo{
		SType:       vk.StructureTypeBufferCreateInfo,
		Size:        uint64(size),
		Usage:       uint32(vk.BufferUsageTransferSrc | vk.BufferUsageTransferDst),
		SharingMode: vk.SharingModeExclusive,
	}
	buf, err := vk.CreateBufferRaw(d.device, &bufCI)
	if err != nil {
		return err
	}
	d.stagingBuffer = buf
	d.stagingSize = size

	memReq := vk.GetBufferMemoryRequirements(d.device, buf)
	memIdx, err := vk.FindMemoryType(d.memProps, memReq.MemoryTypeBits,
		vk.MemoryPropertyHostVisible|vk.MemoryPropertyHostCoherent)
	if err != nil {
		return err
	}
	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReq.Size,
		MemoryTypeIndex: memIdx,
	}
	mem, err := vk.AllocateMemory(d.device, &allocInfo)
	if err != nil {
		return err
	}
	d.stagingMemory = mem
	if err := vk.BindBufferMemory(d.device, buf, mem, 0); err != nil {
		return err
	}

	ptr, err := vk.MapMemory(d.device, mem, 0, uint64(size))
	if err != nil {
		return err
	}
	d.stagingMapped = ptr

	return nil
}

// createUniformBuffer creates a host-visible, host-coherent buffer for UBO
// descriptors. The buffer is persistently mapped so uniform data can be
// written directly before each draw call.
func (d *Device) createUniformBuffer(size int) error {
	bufCI := vk.BufferCreateInfo{
		SType:       vk.StructureTypeBufferCreateInfo,
		Size:        uint64(size),
		Usage:       uint32(vk.BufferUsageUniformBuffer),
		SharingMode: vk.SharingModeExclusive,
	}
	buf, err := vk.CreateBufferRaw(d.device, &bufCI)
	if err != nil {
		return err
	}
	d.uniformBuffer = buf
	d.uniformBufSize = size

	memReq := vk.GetBufferMemoryRequirements(d.device, buf)
	memIdx, err := vk.FindMemoryType(d.memProps, memReq.MemoryTypeBits,
		vk.MemoryPropertyHostVisible|vk.MemoryPropertyHostCoherent)
	if err != nil {
		return err
	}
	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReq.Size,
		MemoryTypeIndex: memIdx,
	}
	mem, err := vk.AllocateMemory(d.device, &allocInfo)
	if err != nil {
		return err
	}
	d.uniformMemory = mem
	if err := vk.BindBufferMemory(d.device, buf, mem, 0); err != nil {
		return err
	}

	ptr, err := vk.MapMemory(d.device, mem, 0, uint64(size))
	if err != nil {
		return err
	}
	d.uniformMapped = ptr

	return nil
}

// submitAndWait submits a pre-built SubmitInfo on graphicsQueue, then
// blocks until it retires via vkQueueWaitIdle. We used to use a
// reusable fence + WaitForFence here, but the Android emulator's
// gfxstream Vulkan driver routes fence waits through its QSRI
// (queue-submit-retire-in-order) sync-fd bookkeeping, and that path
// has a known "Failed to dup() QSRI sync fd: errno 9" race that leaves
// the fence perpetually unsignaled — WaitForFence hangs forever.
// QueueWaitIdle takes a different path inside gfxstream that avoids
// QSRI, so it works on both the emulator and real devices.
func (d *Device) submitAndWait(submitInfo *vk.SubmitInfo) error {
	if err := vk.QueueSubmit(d.graphicsQueue, submitInfo, 0); err != nil {
		return err
	}
	return vk.QueueWaitIdle(d.graphicsQueue)
}

// BeginDispose signals that a whole-device teardown is starting.
// Per-resource Dispose calls check this flag and skip their individual
// vkDeviceWaitIdle — the cumulative wait cost at shutdown of a
// resource-heavy scene was causing multi-second window-close hangs and
// orphaned windows on macOS/MoltenVK. Idempotent: repeat calls do a
// single extra DeviceWaitIdle to re-sync if more work slipped in.
func (d *Device) BeginDispose() {
	if d.device == 0 {
		return
	}
	if !d.disposing {
		_ = vk.DeviceWaitIdle(d.device)
		d.disposing = true
	}
}

func (d *Device) Dispose() {
	if d.device == 0 {
		return
	}
	// Idempotent-idle + flag set; BeginDispose may have already run.
	d.BeginDispose()

	// Destroy swapchain resources.
	if d.hasSwapchain {
		d.destroySwapchain()
		if d.imageAvailableSem != 0 {
			vk.DestroySemaphore(d.device, d.imageAvailableSem)
		}
		if d.renderFinishedSem != 0 {
			vk.DestroySemaphore(d.device, d.renderFinishedSem)
		}
	}
	if d.surface != 0 {
		vk.DestroySurfaceKHR(d.instance, d.surface)
	}

	if d.defaultTexture != nil {
		d.defaultTexture.Dispose()
	}
	for _, s := range d.samplers {
		if s != 0 {
			vk.DestroySampler(d.device, s)
		}
	}
	d.samplers = nil
	if d.uniformBuffer != 0 {
		vk.UnmapMemory(d.device, d.uniformMemory)
		vk.DestroyBuffer(d.device, d.uniformBuffer)
		vk.FreeMemory(d.device, d.uniformMemory)
	}
	if d.stagingBuffer != 0 {
		vk.UnmapMemory(d.device, d.stagingMemory)
		vk.DestroyBuffer(d.device, d.stagingBuffer)
		vk.FreeMemory(d.device, d.stagingMemory)
	}
	if d.defaultFramebuffer != 0 {
		vk.DestroyFramebuffer(d.device, d.defaultFramebuffer)
	}
	if d.defaultRenderPass != 0 {
		vk.DestroyRenderPass(d.device, d.defaultRenderPass)
	}
	if d.defaultRenderPassLoad != 0 {
		vk.DestroyRenderPass(d.device, d.defaultRenderPassLoad)
	}
	if d.defaultColorView != 0 {
		vk.DestroyImageView(d.device, d.defaultColorView)
	}
	if d.defaultColorImage != 0 {
		vk.DestroyImage(d.device, d.defaultColorImage)
	}
	if d.defaultColorMem != 0 {
		vk.FreeMemory(d.device, d.defaultColorMem)
	}
	// Default depth-stencil attachment. destroySwapchain also releases
	// it, but that path only runs when hasSwapchain — the offscreen
	// headless path (conformance tests, ReadScreen-driven capture,
	// WASM soft fallback) leaves createDefaultRenderTarget's dsImg /
	// dsMem / dsView alive until vkDestroyDevice, which
	// validation-layer builds (lavapipe) flag as "VkImage has not been
	// destroyed". Destroy here if it's still held.
	if d.defaultDepthStencilView != 0 || d.defaultDepthStencilImage != 0 || d.defaultDepthStencilMem != 0 {
		d.destroyDepthStencilTexture(
			d.defaultDepthStencilImage,
			d.defaultDepthStencilMem,
			d.defaultDepthStencilView,
		)
		d.defaultDepthStencilImage = 0
		d.defaultDepthStencilMem = 0
		d.defaultDepthStencilView = 0
	}
	// Encoder's descriptor pool. Created lazily in ensureDescriptorPool
	// on first draw; never freed prior to this fix. Safe to call
	// unconditionally — guards against a nil encoder or zero handle.
	if d.encoder != nil && d.encoder.descriptorPool != 0 {
		vk.DestroyDescriptorPool(d.device, d.encoder.descriptorPool)
		d.encoder.descriptorPool = 0
	}
	if d.fence != 0 {
		vk.DestroyFence(d.device, d.fence)
	}
	if d.commandPool != 0 {
		vk.DestroyCommandPool(d.device, d.commandPool)
	}
	vk.DestroyDevice(d.device)
	vk.DestroyInstance(d.instance)
	d.device = 0
}

// ReadScreen copies the default color image pixels into dst via the staging buffer.
// Returns false when the device presents directly via swapchain (no readback needed).
func (d *Device) ReadScreen(dst []byte) bool {
	if d.hasSwapchain {
		return d.readSwapchainScreen(dst)
	}
	if d.defaultColorImage == 0 || d.stagingMapped == nil {
		return false
	}
	if len(dst) == 0 {
		return true // probe: yes, this backend needs presentation
	}

	dataSize := d.width * d.height * 4 // RGBA8
	if dataSize > d.stagingSize {
		return false
	}

	// Ensure all prior GPU work is complete before we issue the readback.
	// EndFrame already waits on its fence, but on MoltenVK we observed
	// stochastic empty-frame flicker (ReadScreen sometimes saw the color
	// image in a partially-rendered state), implying the render-pass-end
	// layout transition wasn't fully committed by the time the fence
	// signaled. DeviceWaitIdle forces every queue to drain, which is
	// overkill for the fast path but appropriate for the deferred
	// readback workflow (headless capture + GL-presenter blit).
	_ = vk.DeviceWaitIdle(d.device)

	// Allocate a one-shot command buffer for the copy.
	cmd, err := vk.AllocateCommandBuffer(d.device, d.commandPool)
	if err != nil {
		return false
	}
	if err := vk.BeginCommandBuffer(cmd, vk.CommandBufferUsageOneTimeSubmit); err != nil {
		return false
	}

	// Transition default color image to transfer src.
	barriers := []vk.ImageMemoryBarrier{{
		SType:               vk.StructureTypeImageMemoryBarrier,
		SrcAccessMask:       vk.AccessColorAttachmentWrite,
		DstAccessMask:       vk.AccessTransferRead,
		OldLayout:           vk.ImageLayoutColorAttachmentOptimal,
		NewLayout:           vk.ImageLayoutTransferSrcOptimal,
		SrcQueueFamilyIndex: vk.QueueFamilyIgnored,
		DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		Image_:              d.defaultColorImage,
		SubresAspectMask:    vk.ImageAspectColor,
		SubresLevelCount:    1,
		SubresLayerCount:    1,
	}}
	vk.CmdPipelineBarrier(cmd,
		vk.PipelineStageColorAttachmentOutput, vk.PipelineStageTransfer,
		barriers)

	// Copy image to staging buffer.
	region := vk.BufferImageCopy{
		AspectMask:   vk.ImageAspectColor,
		LayerCount:   1,
		ImageExtentW: uint32(d.width),
		ImageExtentH: uint32(d.height),
		ImageExtentD: 1,
	}
	vk.CmdCopyImageToBuffer(cmd, d.defaultColorImage,
		vk.ImageLayoutTransferSrcOptimal, d.stagingBuffer, region)

	// Transition back to color attachment.
	barriers[0].SrcAccessMask = vk.AccessTransferRead
	barriers[0].DstAccessMask = vk.AccessColorAttachmentWrite
	barriers[0].OldLayout = vk.ImageLayoutTransferSrcOptimal
	barriers[0].NewLayout = vk.ImageLayoutColorAttachmentOptimal
	vk.CmdPipelineBarrier(cmd,
		vk.PipelineStageTransfer, vk.PipelineStageColorAttachmentOutput,
		barriers)

	_ = vk.EndCommandBuffer(cmd)

	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    uintptr(unsafe.Pointer(&cmd)),
	}
	_ = vk.QueueSubmit(d.graphicsQueue, &submitInfo, 0)
	_ = vk.DeviceWaitIdle(d.device)

	// Copy from mapped staging memory to dst.
	n := len(dst)
	if n > dataSize {
		n = dataSize
	}
	src := unsafe.Slice((*byte)(d.stagingMapped), n)
	copy(dst[:n], src)

	// FUTURE_CORE_VK_READSCREEN_DUMP=1 prints a 3×3 pixel grid from
	// the staging buffer to stderr, confirming whether a blank/solid
	// captured PNG reflects the actual GPU-image contents or a
	// readback-path bug. This existed as a one-off during the
	// scene-selector / bubble-pop triage and proved valuable enough
	// to leave env-gated for future Vulkan rendering investigations.
	if os.Getenv("FUTURE_CORE_VK_READSCREEN_DUMP") == "1" {
		for _, yFrac := range []float64{0.25, 0.5, 0.75} {
			for _, xFrac := range []float64{0.25, 0.5, 0.75} {
				x := int(float64(d.width) * xFrac)
				y := int(float64(d.height) * yFrac)
				off := (y*d.width + x) * 4
				if off+3 < n {
					fmt.Fprintf(os.Stderr, "vk.ReadScreen[%d,%d]=(%d,%d,%d,%d)\n",
						x, y, src[off], src[off+1], src[off+2], src[off+3])
				}
			}
		}
	}

	vk.FreeCommandBuffers(d.device, d.commandPool, cmd)

	return true
}

// readSwapchainScreen reads pixels from the current swapchain image into dst.
// When dst is nil (probe), returns false because swapchain presents directly
// and doesn't need a GL presenter. When dst is non-nil, performs readback.
func (d *Device) readSwapchainScreen(dst []byte) bool {
	if d.stagingMapped == nil {
		return false
	}
	if len(dst) == 0 {
		return false // probe: swapchain presents directly, no GL presenter needed
	}

	w := int(d.swapchainExtent[0])
	h := int(d.swapchainExtent[1])
	dataSize := w * h * 4
	if dataSize > d.stagingSize {
		return false
	}

	img := d.swapchainImages[d.currentImageIndex]

	cmd, err := vk.AllocateCommandBuffer(d.device, d.commandPool)
	if err != nil {
		return false
	}
	if err := vk.BeginCommandBuffer(cmd, vk.CommandBufferUsageOneTimeSubmit); err != nil {
		return false
	}

	// Transition swapchain image to transfer src.
	barriers := []vk.ImageMemoryBarrier{{
		SType:               vk.StructureTypeImageMemoryBarrier,
		SrcAccessMask:       vk.AccessColorAttachmentWrite,
		DstAccessMask:       vk.AccessTransferRead,
		OldLayout:           vk.ImageLayoutPresentSrcKHR,
		NewLayout:           vk.ImageLayoutTransferSrcOptimal,
		SrcQueueFamilyIndex: vk.QueueFamilyIgnored,
		DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		Image_:              img,
		SubresAspectMask:    vk.ImageAspectColor,
		SubresLevelCount:    1,
		SubresLayerCount:    1,
	}}
	vk.CmdPipelineBarrier(cmd,
		vk.PipelineStageColorAttachmentOutput, vk.PipelineStageTransfer,
		barriers)

	region := vk.BufferImageCopy{
		AspectMask:   vk.ImageAspectColor,
		LayerCount:   1,
		ImageExtentW: uint32(w),
		ImageExtentH: uint32(h),
		ImageExtentD: 1,
	}
	vk.CmdCopyImageToBuffer(cmd, img,
		vk.ImageLayoutTransferSrcOptimal, d.stagingBuffer, region)

	// Transition back to present src.
	barriers[0].SrcAccessMask = vk.AccessTransferRead
	barriers[0].DstAccessMask = 0
	barriers[0].OldLayout = vk.ImageLayoutTransferSrcOptimal
	barriers[0].NewLayout = vk.ImageLayoutPresentSrcKHR
	vk.CmdPipelineBarrier(cmd,
		vk.PipelineStageTransfer, vk.PipelineStageBottomOfPipe,
		barriers)

	_ = vk.EndCommandBuffer(cmd)

	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    uintptr(unsafe.Pointer(&cmd)),
	}
	_ = vk.QueueSubmit(d.graphicsQueue, &submitInfo, 0)
	_ = vk.DeviceWaitIdle(d.device)

	n := len(dst)
	if n > dataSize {
		n = dataSize
	}

	// Copy from staging and convert BGRA → RGBA if the swapchain format is BGRA.
	src := unsafe.Slice((*byte)(d.stagingMapped), n)
	copy(dst[:n], src)
	if d.swapchainFormat == vk.FormatB8G8R8A8UNorm || d.swapchainFormat == vk.FormatB8G8R8A8SRGB {
		for i := 0; i+3 < n; i += 4 {
			dst[i], dst[i+2] = dst[i+2], dst[i] // swap R and B
		}
	}

	vk.FreeCommandBuffers(d.device, d.commandPool, cmd)
	return true
}

// BeginFrame waits for the previous frame's fence and resets the command buffer.
// When a swapchain is active, acquires the next image.
func (d *Device) BeginFrame() {
	if d.device == 0 {
		return
	}
	// WaitForFence is the correct per-frame GPU→CPU sync everywhere.
	_ = vk.WaitForFence(d.device, d.fence, ^uint64(0))
	// Extra DeviceWaitIdle is a MoltenVK-only workaround for a
	// stochastic empty-frame flicker (~15-20% of frames on
	// scene-selector composited a blank tile grid even though the
	// fence had signaled). Suspected cause: Metal command-queue
	// commitment runs asynchronously relative to Vulkan fence
	// signaling, so the next frame's command buffer could begin
	// recording before prior writes to persistent render targets were
	// fully committed. Paired with a similar DeviceWaitIdle in
	// ReadScreen; together they drop the flicker from ~17% → 0%.
	//
	// Confined to darwin because:
	//   - On native Vulkan (Linux/Windows drivers) it's a significant
	//     per-frame stall that gates pipelining with zero benefit.
	//   - On the Android emulator's gfxstream Vulkan translation the
	//     vkDeviceWaitIdle round-trip over the qemu pipe never returns
	//     — the render thread hangs forever after the first submitted
	//     frame. Gating this restores the render loop on emulators.
	if runtime.GOOS == "darwin" {
		_ = vk.DeviceWaitIdle(d.device)
	}
	// GPU work from the previous frame is complete — safe to free resources.
	if d.encoder != nil {
		d.encoder.resetFrame()
	}
	_ = vk.ResetFence(d.device, d.fence)
	_ = vk.ResetCommandBuffer(d.commandBuffer)
	_ = vk.BeginCommandBuffer(d.commandBuffer, vk.CommandBufferUsageOneTimeSubmit)
	if d.encoder != nil {
		d.encoder.markRecording()
	}
	d.uniformCursor = 0

	if d.hasSwapchain {
		// Check if the surface size changed (e.g., Retina scale propagated
		// after Init, or window was resized). Recreate the swapchain if needed.
		caps, _ := vk.GetPhysicalDeviceSurfaceCapabilitiesKHR(d.physicalDevice, d.surface)
		if caps.CurrentExtentWidth != d.swapchainExtent[0] || caps.CurrentExtentHeight != d.swapchainExtent[1] {
			if caps.CurrentExtentWidth > 0 && caps.CurrentExtentHeight > 0 {
				_ = d.recreateSwapchain(int(caps.CurrentExtentWidth), int(caps.CurrentExtentHeight))
			}
		}

		idx, r := vk.AcquireNextImageKHR(d.device, d.swapchain, ^uint64(0), d.imageAvailableSem, 0)
		if r == vk.ErrorOutOfDateKHR {
			caps, _ := vk.GetPhysicalDeviceSurfaceCapabilitiesKHR(d.physicalDevice, d.surface)
			_ = d.recreateSwapchain(int(caps.CurrentExtentWidth), int(caps.CurrentExtentHeight))
			// Re-acquire after recreation.
			idx, _ = vk.AcquireNextImageKHR(d.device, d.swapchain, ^uint64(0), d.imageAvailableSem, 0)
		}
		d.currentImageIndex = idx
	}
}

// EndFrame ends command recording, submits to the queue, and presents
// when a swapchain is active.
func (d *Device) EndFrame() {
	if d.device == 0 || d.graphicsQueue == 0 {
		return
	}
	_ = vk.EndCommandBuffer(d.commandBuffer)
	if d.encoder != nil {
		d.encoder.markNotRecording()
	}

	cmd := d.commandBuffer

	if d.hasSwapchain {
		// Submit with semaphore synchronization for presentation.
		waitStage := uint32(vk.PipelineStageColorAttachmentOutput)
		submitInfo := vk.SubmitInfo{
			SType:                vk.StructureTypeSubmitInfo,
			WaitSemaphoreCount:   1,
			PWaitSemaphores:      uintptr(unsafe.Pointer(&d.imageAvailableSem)),
			PWaitDstStageMask:    uintptr(unsafe.Pointer(&waitStage)),
			CommandBufferCount:   1,
			PCommandBuffers:      uintptr(unsafe.Pointer(&cmd)),
			SignalSemaphoreCount: 1,
			PSignalSemaphores:    uintptr(unsafe.Pointer(&d.renderFinishedSem)),
		}
		err := vk.QueueSubmit(d.graphicsQueue, &submitInfo, d.fence)
		runtime.KeepAlive(cmd)
		runtime.KeepAlive(submitInfo)
		runtime.KeepAlive(waitStage)
		if err != nil {
			return
		}

		// Present the rendered image.
		imageIndex := d.currentImageIndex
		presentInfo := vk.PresentInfoKHR{
			SType:              vk.StructureTypePresentInfoKHR,
			WaitSemaphoreCount: 1,
			PWaitSemaphores:    uintptr(unsafe.Pointer(&d.renderFinishedSem)),
			SwapchainCount:     1,
			PSwapchains:        uintptr(unsafe.Pointer(&d.swapchain)),
			PImageIndices:      uintptr(unsafe.Pointer(&imageIndex)),
		}
		r := vk.QueuePresentKHR(d.graphicsQueue, &presentInfo)
		runtime.KeepAlive(presentInfo)
		runtime.KeepAlive(imageIndex)

		if r == vk.ErrorOutOfDateKHR || r == vk.SuboptimalKHR {
			caps, _ := vk.GetPhysicalDeviceSurfaceCapabilitiesKHR(d.physicalDevice, d.surface)
			_ = d.recreateSwapchain(int(caps.CurrentExtentWidth), int(caps.CurrentExtentHeight))
		}
	} else {
		// Non-swapchain path: submit and wait.
		submitInfo := vk.SubmitInfo{
			SType:              vk.StructureTypeSubmitInfo,
			CommandBufferCount: 1,
			PCommandBuffers:    uintptr(unsafe.Pointer(&cmd)),
		}
		err := vk.QueueSubmit(d.graphicsQueue, &submitInfo, d.fence)
		runtime.KeepAlive(cmd)
		runtime.KeepAlive(submitInfo)
		if err != nil {
			return
		}
		_ = vk.WaitForFence(d.device, d.fence, ^uint64(0))
	}
}

// NewTexture creates a VkImage + VkImageView + VkDeviceMemory.
func (d *Device) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("vulkan: invalid texture dimensions %dx%d", desc.Width, desc.Height)
	}
	format := uint32(vkFormatFromTextureFormat(desc.Format))
	usage := uint32(vk.ImageUsageSampled | vk.ImageUsageTransferDst | vk.ImageUsageTransferSrc)
	if desc.RenderTarget {
		usage |= vk.ImageUsageColorAttachment
	}

	imgCI := vk.ImageCreateInfo{
		SType:       vk.StructureTypeImageCreateInfo,
		ImageType:   vk.ImageType2D,
		Format:      format,
		ExtentWidth: uint32(desc.Width), ExtentHeight: uint32(desc.Height), ExtentDepth: 1,
		MipLevels: 1, ArrayLayers: 1,
		Samples:       vk.SampleCount1,
		Tiling:        vk.ImageTilingOptimal,
		Usage:         usage,
		SharingMode:   vk.SharingModeExclusive,
		InitialLayout: vk.ImageLayoutUndefined,
	}
	img, err := vk.CreateImageRaw(d.device, &imgCI)
	if err != nil {
		return nil, fmt.Errorf("vulkan: %w", err)
	}

	memReq := vk.GetImageMemoryRequirements(d.device, img)
	memIdx, err := vk.FindMemoryType(d.memProps, memReq.MemoryTypeBits, vk.MemoryPropertyDeviceLocal)
	if err != nil {
		vk.DestroyImage(d.device, img)
		return nil, fmt.Errorf("vulkan: %w", err)
	}
	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReq.Size,
		MemoryTypeIndex: memIdx,
	}
	mem, err := vk.AllocateMemory(d.device, &allocInfo)
	if err != nil {
		vk.DestroyImage(d.device, img)
		return nil, fmt.Errorf("vulkan: %w", err)
	}
	if err := vk.BindImageMemory(d.device, img, mem, 0); err != nil {
		vk.FreeMemory(d.device, mem)
		vk.DestroyImage(d.device, img)
		return nil, fmt.Errorf("vulkan: %w", err)
	}

	// Create image view.
	aspect := uint32(vk.ImageAspectColor)
	if desc.Format == backend.TextureFormatDepth24 || desc.Format == backend.TextureFormatDepth32F {
		aspect = vk.ImageAspectDepth
	}
	viewCI := vk.ImageViewCreateInfo{
		SType:            vk.StructureTypeImageViewCreateInfo,
		Image:            img,
		ViewType:         vk.ImageViewType2D,
		Format:           format,
		ComponentR:       vk.ComponentSwizzleIdentity,
		ComponentG:       vk.ComponentSwizzleIdentity,
		ComponentB:       vk.ComponentSwizzleIdentity,
		ComponentA:       vk.ComponentSwizzleIdentity,
		SubresAspectMask: aspect,
		SubresLevelCount: 1,
		SubresLayerCount: 1,
	}
	view, err := vk.CreateImageViewRaw(d.device, &viewCI)
	if err != nil {
		vk.FreeMemory(d.device, mem)
		vk.DestroyImage(d.device, img)
		return nil, fmt.Errorf("vulkan: %w", err)
	}

	tex := &Texture{
		dev:       d,
		image:     img,
		view:      view,
		memory:    mem,
		w:         desc.Width,
		h:         desc.Height,
		format:    desc.Format,
		vkFormat:  int(format),
		vkUsage:   int(usage),
		mipLevels: 1,
	}

	// Upload initial data if provided.
	if len(desc.Data) > 0 {
		tex.Upload(desc.Data, 0)
	}

	return tex, nil
}

// NewBuffer creates a VkBuffer + VkDeviceMemory.
func (d *Device) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	size := desc.Size
	if len(desc.Data) > 0 && size == 0 {
		size = len(desc.Data)
	}
	if size <= 0 {
		return nil, fmt.Errorf("vulkan: invalid buffer size %d", size)
	}

	vkUsage := uint32(vkBufferUsageFromBackend(desc.Usage))
	bufCI := vk.BufferCreateInfo{
		SType:       vk.StructureTypeBufferCreateInfo,
		Size:        uint64(size),
		Usage:       vkUsage | uint32(vk.BufferUsageTransferDst),
		SharingMode: vk.SharingModeExclusive,
	}
	buf, err := vk.CreateBufferRaw(d.device, &bufCI)
	if err != nil {
		return nil, fmt.Errorf("vulkan: %w", err)
	}

	// Use host-visible memory so we can map and upload directly.
	memReq := vk.GetBufferMemoryRequirements(d.device, buf)
	memIdx, err := vk.FindMemoryType(d.memProps, memReq.MemoryTypeBits,
		vk.MemoryPropertyHostVisible|vk.MemoryPropertyHostCoherent)
	if err != nil {
		vk.DestroyBuffer(d.device, buf)
		return nil, fmt.Errorf("vulkan: %w", err)
	}
	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReq.Size,
		MemoryTypeIndex: memIdx,
	}
	mem, err := vk.AllocateMemory(d.device, &allocInfo)
	if err != nil {
		vk.DestroyBuffer(d.device, buf)
		return nil, fmt.Errorf("vulkan: %w", err)
	}
	if err := vk.BindBufferMemory(d.device, buf, mem, 0); err != nil {
		vk.FreeMemory(d.device, mem)
		vk.DestroyBuffer(d.device, buf)
		return nil, fmt.Errorf("vulkan: %w", err)
	}

	// Persistently map to avoid per-frame map/unmap overhead.
	ptr, err := vk.MapMemory(d.device, mem, 0, uint64(size))
	if err != nil {
		vk.FreeMemory(d.device, mem)
		vk.DestroyBuffer(d.device, buf)
		return nil, fmt.Errorf("vulkan: map buffer: %w", err)
	}

	b := &Buffer{
		dev:     d,
		buffer:  buf,
		memory:  mem,
		mapped:  ptr,
		size:    size,
		vkUsage: int(vkUsage),
	}

	if len(desc.Data) > 0 {
		b.Upload(desc.Data)
	}

	return b, nil
}

// NewShader creates VkShaderModule pair from GLSL source.
// Note: In a production Vulkan backend, GLSL must be compiled to SPIR-V
// at runtime (via shaderc/glslang) or provided as pre-compiled SPIR-V.
// This implementation stores the GLSL source for future SPIR-V compilation.
func (d *Device) NewShader(desc backend.ShaderDescriptor) (backend.Shader, error) {
	return &Shader{
		dev:            d,
		vertexSource:   desc.VertexSource,
		fragmentSource: desc.FragmentSource,
		attributes:     desc.Attributes,
		uniforms:       make(map[string]interface{}),
	}, nil
}

// NewRenderTarget creates a VkFramebuffer with color + depth-stencil
// attachments and two render passes (Clear/Load) bound to that layout.
//
// The color attachment reuses the VkImageView already created by
// NewTexture (which sets `ImageUsageColorAttachment` when
// `RenderTarget: true`). The depth-stencil attachment is allocated
// privately per-RT so fill-rule stencil work on one RT doesn't spill
// into another.
//
// Two render passes are created — one with LoadOp=Clear and
// InitialLayout=Undefined for the first batch entering the RT in a
// frame, and one with LoadOp=Load and InitialLayout=ShaderReadOnlyOptimal
// for subsequent re-entries that must preserve accumulated content.
// The encoder picks between them based on the caller's LoadAction.
// FinalLayout on both is ShaderReadOnlyOptimal so later passes can
// sample the RT's color texture directly without an explicit barrier.
func (d *Device) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("vulkan: invalid render target dimensions %dx%d", desc.Width, desc.Height)
	}

	// Create color texture with ColorAttachment usage.
	colorTex, err := d.NewTexture(backend.TextureDescriptor{
		Width: desc.Width, Height: desc.Height,
		Format:       desc.ColorFormat,
		RenderTarget: true,
	})
	if err != nil {
		return nil, err
	}
	color := colorTex.(*Texture)

	// Create optional depth texture (backend-owned texture; not attached
	// to our per-RT framebuffer — it exists for callers that want to
	// read depth, which the sprite pass does not do yet).
	var depthTex backend.Texture
	if desc.HasDepth {
		dt, nterr := d.NewTexture(backend.TextureDescriptor{
			Width: desc.Width, Height: desc.Height,
			Format:       desc.DepthFormat,
			RenderTarget: true,
		})
		if nterr != nil {
			colorTex.Dispose()
			return nil, nterr
		}
		depthTex = dt
	}

	// Private depth-stencil attachment for this RT's framebuffer.
	dsImg, dsMem, dsView, err := d.createDepthStencilTexture(uint32(desc.Width), uint32(desc.Height))
	if err != nil {
		if depthTex != nil {
			depthTex.Dispose()
		}
		colorTex.Dispose()
		return nil, fmt.Errorf("vulkan: rt depth-stencil: %w", err)
	}

	colorFmt := uint32(vkFormatFromTextureFormat(desc.ColorFormat))
	rpClear, err := d.createOffscreenRenderPass(colorFmt, vk.AttachmentLoadOpClear, vk.ImageLayoutUndefined)
	if err != nil {
		d.destroyDepthStencilTexture(dsImg, dsMem, dsView)
		if depthTex != nil {
			depthTex.Dispose()
		}
		colorTex.Dispose()
		return nil, fmt.Errorf("vulkan: rt rp(clear): %w", err)
	}
	rpLoad, err := d.createOffscreenRenderPass(colorFmt, vk.AttachmentLoadOpLoad, vk.ImageLayoutShaderReadOnlyOptimal)
	if err != nil {
		vk.DestroyRenderPass(d.device, rpClear)
		d.destroyDepthStencilTexture(dsImg, dsMem, dsView)
		if depthTex != nil {
			depthTex.Dispose()
		}
		colorTex.Dispose()
		return nil, fmt.Errorf("vulkan: rt rp(load): %w", err)
	}

	// Framebuffer is render-pass-compatible with both RPs (same attachments).
	fbViews := [2]vk.ImageView{color.view, dsView}
	fbCI := vk.FramebufferCreateInfo{
		SType:           vk.StructureTypeFramebufferCreateInfo,
		RenderPass_:     rpClear,
		AttachmentCount: 2,
		PAttachments:    uintptr(unsafe.Pointer(&fbViews[0])),
		Width:           uint32(desc.Width),
		Height:          uint32(desc.Height),
		Layers:          1,
	}
	fb, err := vk.CreateFramebuffer(d.device, &fbCI)
	runtime.KeepAlive(fbViews)
	if err != nil {
		vk.DestroyRenderPass(d.device, rpLoad)
		vk.DestroyRenderPass(d.device, rpClear)
		d.destroyDepthStencilTexture(dsImg, dsMem, dsView)
		if depthTex != nil {
			depthTex.Dispose()
		}
		colorTex.Dispose()
		return nil, fmt.Errorf("vulkan: rt framebuffer: %w", err)
	}

	// Initialise the RT to transparent black by running a one-shot
	// render pass that just begins-and-ends the Clear variant. This
	// honours the Canvas contract ("NewCanvas returns a transparent
	// all-zero RT") on MoltenVK, whose VK_IMAGE_LAYOUT_UNDEFINED
	// contents sometimes seed as debug magenta (255,0,255) and leak
	// through subsequent LoadAction=Load passes — visible as the
	// magenta bubble-pop game RT, pink lighting-demo tint, and
	// empty-panel blots in vector-showcase.
	//
	// The Clear render pass's FinalLayout is ShaderReadOnlyOptimal,
	// matching what the Load-variant expects on subsequent use, so
	// the very first sprite-pass render pass on this RT (whether
	// Clear or Load) sees the image in the correct layout with
	// predictable zero contents.
	if initErr := d.clearFreshRenderTarget(fb, rpClear, uint32(desc.Width), uint32(desc.Height)); initErr != nil {
		vk.DestroyFramebuffer(d.device, fb)
		vk.DestroyRenderPass(d.device, rpLoad)
		vk.DestroyRenderPass(d.device, rpClear)
		d.destroyDepthStencilTexture(dsImg, dsMem, dsView)
		if depthTex != nil {
			depthTex.Dispose()
		}
		colorTex.Dispose()
		return nil, fmt.Errorf("vulkan: rt init clear: %w", initErr)
	}

	return &RenderTarget{
		dev:            d,
		colorTex:       color,
		depthTex:       depthTex,
		w:              desc.Width,
		h:              desc.Height,
		hasStencil:     true,
		renderPass:     rpClear,
		renderPassLoad: rpLoad,
		framebuffer:    fb,
		dsImage:        dsImg,
		dsMem:          dsMem,
		dsView:         dsView,
	}, nil
}

// clearFreshRenderTarget submits a one-shot empty render pass against
// the given framebuffer + Clear-variant render pass. The render pass
// clears the color attachment to transparent black (the first entry
// of the clearValues array) and transitions the image to
// ShaderReadOnlyOptimal on EndRenderPass via the attachment's
// FinalLayout. No draws are recorded — the clear itself does the
// work.
func (d *Device) clearFreshRenderTarget(fb vk.Framebuffer, rp vk.RenderPass, w, h uint32) error {
	cmd, err := vk.AllocateCommandBuffer(d.device, d.commandPool)
	if err != nil {
		return err
	}
	defer vk.FreeCommandBuffers(d.device, d.commandPool, cmd)

	if err := vk.BeginCommandBuffer(cmd, vk.CommandBufferUsageOneTimeSubmit); err != nil {
		return err
	}

	clearValues := [2]vk.ClearValue{
		{Color: [4]float32{0, 0, 0, 0}},
		makeDepthStencilClearValue(1.0, 0),
	}
	rpBegin := vk.RenderPassBeginInfo{
		SType:           vk.StructureTypeRenderPassBeginInfo,
		RenderPass_:     rp,
		Framebuffer_:    fb,
		RenderAreaW:     w,
		RenderAreaH:     h,
		ClearValueCount: 2,
		PClearValues:    uintptr(unsafe.Pointer(&clearValues[0])),
	}
	vk.CmdBeginRenderPass(cmd, &rpBegin)
	runtime.KeepAlive(clearValues)
	vk.CmdEndRenderPass(cmd)

	if err := vk.EndCommandBuffer(cmd); err != nil {
		return err
	}

	// Submit + wait on a one-shot fence instead of DeviceWaitIdle. Using
	// DeviceWaitIdle here would drain the whole queue, which on MoltenVK
	// includes any in-flight work from the caller's frame (this function
	// is reachable mid-frame via NewRenderTarget, e.g. the AA buffer's
	// lazy allocation). A scoped fence waits only for this one-shot
	// submission and keeps the main frame's submission ordering /
	// fence timing intact.
	fence, err := vk.CreateFence(d.device, false)
	if err != nil {
		return err
	}
	defer vk.DestroyFence(d.device, fence)

	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    uintptr(unsafe.Pointer(&cmd)),
	}
	if err := vk.QueueSubmit(d.graphicsQueue, &submitInfo, fence); err != nil {
		return err
	}
	runtime.KeepAlive(cmd)
	runtime.KeepAlive(submitInfo)
	return vk.WaitForFence(d.device, fence, ^uint64(0))
}

// createOffscreenRenderPass builds a 2-attachment (color + D24S8) render
// pass with the requested color LoadOp and InitialLayout. FinalLayout
// on the color attachment is always ShaderReadOnlyOptimal so subsequent
// passes can sample the RT without an explicit barrier. Two subpass
// dependencies frame the pass: an external→0 dependency ensures prior
// fragment-shader reads finish before we start writing, and a 0→external
// dependency flushes our writes before the next shader read.
func (d *Device) createOffscreenRenderPass(colorFormat, colorLoadOp, colorInitialLayout uint32) (vk.RenderPass, error) {
	attachments := [2]vk.AttachmentDescription{
		{
			Format:         colorFormat,
			Samples:        vk.SampleCount1,
			LoadOp:         colorLoadOp,
			StoreOp:        vk.AttachmentStoreOpStore,
			StencilLoadOp:  vk.AttachmentLoadOpDontCare,
			StencilStoreOp: vk.AttachmentStoreOpDontCare,
			InitialLayout:  colorInitialLayout,
			FinalLayout:    vk.ImageLayoutShaderReadOnlyOptimal,
		},
		{
			Format:         vk.FormatD24UNormS8UInt,
			Samples:        vk.SampleCount1,
			LoadOp:         vk.AttachmentLoadOpClear,
			StoreOp:        vk.AttachmentStoreOpDontCare,
			StencilLoadOp:  vk.AttachmentLoadOpClear,
			StencilStoreOp: vk.AttachmentStoreOpDontCare,
			InitialLayout:  vk.ImageLayoutUndefined,
			FinalLayout:    vk.ImageLayoutDepthStencilAttachOptimal,
		},
	}
	colorRef := vk.AttachmentReference{
		Attachment: 0,
		Layout:     vk.ImageLayoutColorAttachmentOptimal,
	}
	depthRef := vk.AttachmentReference{
		Attachment: 1,
		Layout:     vk.ImageLayoutDepthStencilAttachOptimal,
	}
	subpass := vk.SubpassDescription{
		PipelineBindPoint:       vk.PipelineBindPointGraphics,
		ColorAttachmentCount:    1,
		PColorAttachments:       uintptr(unsafe.Pointer(&colorRef)),
		PDepthStencilAttachment: uintptr(unsafe.Pointer(&depthRef)),
	}
	dependencies := [2]vk.SubpassDependency{
		{
			SrcSubpass:    0xFFFFFFFF, // VK_SUBPASS_EXTERNAL
			DstSubpass:    0,
			SrcStageMask:  vk.PipelineStageFragmentShader,
			DstStageMask:  vk.PipelineStageColorAttachmentOutput,
			SrcAccessMask: vk.AccessShaderRead,
			DstAccessMask: vk.AccessColorAttachmentWrite,
		},
		{
			SrcSubpass:    0,
			DstSubpass:    0xFFFFFFFF,
			SrcStageMask:  vk.PipelineStageColorAttachmentOutput,
			DstStageMask:  vk.PipelineStageFragmentShader,
			SrcAccessMask: vk.AccessColorAttachmentWrite,
			DstAccessMask: vk.AccessShaderRead,
		},
	}
	rpCI := vk.RenderPassCreateInfo{
		SType:           vk.StructureTypeRenderPassCreateInfo,
		AttachmentCount: 2,
		PAttachments:    uintptr(unsafe.Pointer(&attachments[0])),
		SubpassCount:    1,
		PSubpasses:      uintptr(unsafe.Pointer(&subpass)),
		DependencyCount: 2,
		PDependencies:   uintptr(unsafe.Pointer(&dependencies[0])),
	}
	rp, err := vk.CreateRenderPass(d.device, &rpCI)
	runtime.KeepAlive(attachments)
	runtime.KeepAlive(dependencies)
	return rp, err
}

// NewPipeline creates a VkPipeline (currently stores descriptor for deferred creation).
func (d *Device) NewPipeline(desc backend.PipelineDescriptor) (backend.Pipeline, error) {
	return &Pipeline{
		dev:  d,
		desc: desc,
	}, nil
}

// Capabilities returns Vulkan device capabilities.
func (d *Device) Capabilities() backend.DeviceCapabilities {
	maxDim := d.physicalDeviceInfo.MaxImageDim
	if maxDim == 0 {
		maxDim = 8192
	}
	maxSamples := d.physicalDeviceInfo.MaxSamples
	if maxSamples == 0 {
		maxSamples = 4
	}
	return backend.DeviceCapabilities{
		MaxTextureSize:    maxDim,
		MaxRenderTargets:  8,
		SupportsInstanced: true,
		SupportsCompute:   true,
		SupportsMSAA:      true,
		MaxMSAASamples:    maxSamples,
		SupportsFloat16:   true,
		SupportsStencil:   true,
	}
}

// Encoder returns the command encoder.
func (d *Device) Encoder() backend.CommandEncoder {
	return d.encoder
}

// ---------------------------------------------------------------------------
// Swapchain management
// ---------------------------------------------------------------------------

// createSwapchain queries surface capabilities and creates a VkSwapchainKHR
// with image views, a compatible render pass, and framebuffers.
func (d *Device) createSwapchain() error {
	caps, err := vk.GetPhysicalDeviceSurfaceCapabilitiesKHR(d.physicalDevice, d.surface)
	if err != nil {
		return err
	}

	formats, err := vk.GetPhysicalDeviceSurfaceFormatsKHR(d.physicalDevice, d.surface)
	if err != nil {
		return err
	}
	if len(formats) == 0 {
		return fmt.Errorf("no surface formats available")
	}

	// Prefer B8G8R8A8 SRGB non-linear; fall back to first available.
	// Prefer R8G8B8A8 so the shader's RGBA output matches the swapchain
	// without channel swizzling. Fall back to B8G8R8A8 (common on macOS).
	chosenFmt := formats[0]
	for _, f := range formats {
		if f.Format == vk.FormatR8G8B8A8UNorm {
			chosenFmt = f
			break
		}
	}
	if chosenFmt.Format != vk.FormatR8G8B8A8UNorm {
		for _, f := range formats {
			if f.Format == vk.FormatB8G8R8A8UNorm {
				chosenFmt = f
				break
			}
		}
	}

	// Present mode: prefer FIFO (guaranteed, acts as vsync).
	presentMode := uint32(vk.PresentModeFifoKHR)
	modes, _ := vk.GetPhysicalDeviceSurfacePresentModesKHR(d.physicalDevice, d.surface)
	for _, m := range modes {
		if m == vk.PresentModeMailboxKHR {
			presentMode = vk.PresentModeMailboxKHR
			break
		}
	}

	// Extent: use current extent if defined, else clamp to our desired size.
	extent := [2]uint32{caps.CurrentExtentWidth, caps.CurrentExtentHeight}
	if caps.CurrentExtentWidth == 0xFFFFFFFF {
		extent[0] = clampU32(uint32(d.width), caps.MinImageExtentWidth, caps.MaxImageExtentWidth)
		extent[1] = clampU32(uint32(d.height), caps.MinImageExtentHeight, caps.MaxImageExtentHeight)
	}

	imageCount := caps.MinImageCount + 1
	if caps.MaxImageCount > 0 && imageCount > caps.MaxImageCount {
		imageCount = caps.MaxImageCount
	}

	sci := vk.SwapchainCreateInfoKHR{
		SType:             vk.StructureTypeSwapchainCreateInfoKHR,
		Surface:           d.surface,
		MinImageCount:     imageCount,
		ImageFormat:       chosenFmt.Format,
		ImageColorSpace:   chosenFmt.ColorSpace,
		ImageExtentWidth:  extent[0],
		ImageExtentHeight: extent[1],
		ImageArrayLayers:  1,
		ImageUsage:        uint32(vk.ImageUsageColorAttachment),
		ImageSharingMode:  vk.SharingModeExclusive,
		PreTransform:      caps.CurrentTransform,
		CompositeAlpha:    vk.CompositeAlphaOpaqueKHR,
		PresentMode:       presentMode,
		Clipped:           1,
		OldSwapchain:      d.swapchain, // reuse old swapchain if recreating
	}

	sc, err := vk.CreateSwapchainKHR(d.device, &sci)
	if err != nil {
		return err
	}
	d.swapchain = sc
	d.swapchainFormat = chosenFmt.Format
	d.swapchainExtent = extent

	// Get swapchain images.
	images, err := vk.GetSwapchainImagesKHR(d.device, sc)
	if err != nil {
		return err
	}
	d.swapchainImages = images

	// Create image views.
	d.swapchainViews = make([]vk.ImageView, len(images))
	for i, img := range images {
		viewCI := vk.ImageViewCreateInfo{
			SType:            vk.StructureTypeImageViewCreateInfo,
			Image:            img,
			ViewType:         vk.ImageViewType2D,
			Format:           chosenFmt.Format,
			ComponentR:       vk.ComponentSwizzleIdentity,
			ComponentG:       vk.ComponentSwizzleIdentity,
			ComponentB:       vk.ComponentSwizzleIdentity,
			ComponentA:       vk.ComponentSwizzleIdentity,
			SubresAspectMask: vk.ImageAspectColor,
			SubresLevelCount: 1,
			SubresLayerCount: 1,
		}
		view, verr := vk.CreateImageViewRaw(d.device, &viewCI)
		if verr != nil {
			return verr
		}
		d.swapchainViews[i] = view
	}

	// Depth-stencil attachment shared across swapchain images. The engine
	// uses a single-frame-in-flight model (one fence / semaphore pair) so
	// a shared D24_UNORM_S8_UINT texture is safe. Clear-on-load on every
	// pass means no cross-frame data dependency either.
	dsImg, dsMem, dsView, err := d.createDepthStencilTexture(extent[0], extent[1])
	if err != nil {
		return err
	}
	d.defaultDepthStencilImage = dsImg
	d.defaultDepthStencilMem = dsMem
	d.defaultDepthStencilView = dsView

	// Create render pass for swapchain (final layout = PresentSrcKHR).
	attachments := [2]vk.AttachmentDescription{
		{
			Format:         chosenFmt.Format,
			Samples:        vk.SampleCount1,
			LoadOp:         vk.AttachmentLoadOpClear,
			StoreOp:        vk.AttachmentStoreOpStore,
			StencilLoadOp:  vk.AttachmentLoadOpDontCare,
			StencilStoreOp: vk.AttachmentStoreOpDontCare,
			InitialLayout:  vk.ImageLayoutUndefined,
			FinalLayout:    vk.ImageLayoutPresentSrcKHR,
		},
		{
			Format:         vk.FormatD24UNormS8UInt,
			Samples:        vk.SampleCount1,
			LoadOp:         vk.AttachmentLoadOpClear,
			StoreOp:        vk.AttachmentStoreOpStore,
			StencilLoadOp:  vk.AttachmentLoadOpClear,
			StencilStoreOp: vk.AttachmentStoreOpStore,
			InitialLayout:  vk.ImageLayoutUndefined,
			FinalLayout:    vk.ImageLayoutDepthStencilAttachOptimal,
		},
	}
	colorRef := vk.AttachmentReference{
		Attachment: 0,
		Layout:     vk.ImageLayoutColorAttachmentOptimal,
	}
	depthRef := vk.AttachmentReference{
		Attachment: 1,
		Layout:     vk.ImageLayoutDepthStencilAttachOptimal,
	}
	subpass := vk.SubpassDescription{
		PipelineBindPoint:       vk.PipelineBindPointGraphics,
		ColorAttachmentCount:    1,
		PColorAttachments:       uintptr(unsafe.Pointer(&colorRef)),
		PDepthStencilAttachment: uintptr(unsafe.Pointer(&depthRef)),
	}
	dependency := vk.SubpassDependency{
		SrcSubpass:    0xFFFFFFFF,
		DstSubpass:    0,
		SrcStageMask:  vk.PipelineStageColorAttachmentOutput,
		DstStageMask:  vk.PipelineStageColorAttachmentOutput,
		DstAccessMask: vk.AccessColorAttachmentWrite,
	}
	rpCI := vk.RenderPassCreateInfo{
		SType:           vk.StructureTypeRenderPassCreateInfo,
		AttachmentCount: 2,
		PAttachments:    uintptr(unsafe.Pointer(&attachments[0])),
		SubpassCount:    1,
		PSubpasses:      uintptr(unsafe.Pointer(&subpass)),
		DependencyCount: 1,
		PDependencies:   uintptr(unsafe.Pointer(&dependency)),
	}
	rp, err := vk.CreateRenderPass(d.device, &rpCI)
	runtime.KeepAlive(attachments)
	if err != nil {
		return err
	}
	d.swapchainRenderPass = rp

	// A second render pass with LoadOp=Load + InitialLayout=PresentSrcKHR
	// is needed for screen re-entries within a frame (e.g. the sprite
	// pass composites multiple offscreen RTs onto the screen in back-to-
	// back render passes). Without this variant, every screen re-entry
	// clears the swapchain image and only the final composite survives.
	// Both render passes are compatible with the same swapchain FBs
	// since attachment formats and counts match.
	attachmentsLoad := attachments
	attachmentsLoad[0].LoadOp = vk.AttachmentLoadOpLoad
	attachmentsLoad[0].InitialLayout = vk.ImageLayoutPresentSrcKHR
	rpCILoad := rpCI
	rpCILoad.PAttachments = uintptr(unsafe.Pointer(&attachmentsLoad[0]))
	rpLoad, err := vk.CreateRenderPass(d.device, &rpCILoad)
	runtime.KeepAlive(attachmentsLoad)
	if err != nil {
		return err
	}
	d.swapchainRenderPassLoad = rpLoad

	// Create framebuffers with color + shared depth-stencil attachments.
	d.swapchainFBs = make([]vk.Framebuffer, len(images))
	for i := range images {
		fbViews := [2]vk.ImageView{d.swapchainViews[i], d.defaultDepthStencilView}
		fbCI := vk.FramebufferCreateInfo{
			SType:           vk.StructureTypeFramebufferCreateInfo,
			RenderPass_:     rp,
			AttachmentCount: 2,
			PAttachments:    uintptr(unsafe.Pointer(&fbViews[0])),
			Width:           extent[0],
			Height:          extent[1],
			Layers:          1,
		}
		fb, ferr := vk.CreateFramebuffer(d.device, &fbCI)
		runtime.KeepAlive(fbViews)
		if ferr != nil {
			return ferr
		}
		d.swapchainFBs[i] = fb
	}

	return nil
}

// destroySwapchain destroys framebuffers, image views, render pass, and swapchain.
func (d *Device) destroySwapchain() {
	for _, fb := range d.swapchainFBs {
		if fb != 0 {
			vk.DestroyFramebuffer(d.device, fb)
		}
	}
	d.swapchainFBs = nil

	if d.swapchainRenderPass != 0 {
		vk.DestroyRenderPass(d.device, d.swapchainRenderPass)
		d.swapchainRenderPass = 0
	}
	if d.swapchainRenderPassLoad != 0 {
		vk.DestroyRenderPass(d.device, d.swapchainRenderPassLoad)
		d.swapchainRenderPassLoad = 0
	}

	for _, v := range d.swapchainViews {
		if v != 0 {
			vk.DestroyImageView(d.device, v)
		}
	}
	d.swapchainViews = nil
	d.swapchainImages = nil

	// Release the shared depth-stencil attachment. Safe to call
	// unconditionally — destroyDepthStencilTexture handles zero handles.
	d.destroyDepthStencilTexture(
		d.defaultDepthStencilImage,
		d.defaultDepthStencilMem,
		d.defaultDepthStencilView,
	)
	d.defaultDepthStencilImage = 0
	d.defaultDepthStencilMem = 0
	d.defaultDepthStencilView = 0

	if d.swapchain != 0 {
		vk.DestroySwapchainKHR(d.device, d.swapchain)
		d.swapchain = 0
	}
}

// recreateSwapchain rebuilds the swapchain after a resize or out-of-date error.
func (d *Device) recreateSwapchain(w, h int) error {
	// Wait for in-flight work on the graphics queue before destroying
	// the old swapchain images. QueueWaitIdle rather than
	// DeviceWaitIdle or WaitForFence: DeviceWaitIdle hangs on the
	// Android emulator's gfxstream, and WaitForFence on d.fence routes
	// through gfxstream's QSRI sync-fd path which can leave the fence
	// perpetually unsignaled.
	if d.graphicsQueue != 0 {
		_ = vk.QueueWaitIdle(d.graphicsQueue)
	}
	d.width = w
	d.height = h
	d.destroySwapchain()
	return d.createSwapchain()
}

// appendUnique appends s to slice if not already present.
func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

// clampU32 clamps v to [lo, hi].
func clampU32(v, lo, hi uint32) uint32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
