//go:build (darwin || linux || freebsd || windows) && !soft

package vulkan

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/vk"
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
	defaultRenderPass  vk.RenderPass
	defaultFramebuffer vk.Framebuffer
	defaultColorImage  vk.Image
	defaultColorView   vk.ImageView
	defaultColorMem    vk.DeviceMemory
	width, height      int

	// Staging buffer for texture uploads/readbacks.
	stagingBuffer vk.Buffer
	stagingMemory vk.DeviceMemory
	stagingSize   int
	stagingMapped unsafe.Pointer

	// Default sampler and 1x1 white texture for fallback binding.
	defaultSampler vk.Sampler
	defaultTexture *Texture

	// Shared uniform buffer for UBO descriptors (persistently mapped).
	// Uses a ring-buffer write cursor so each draw's UBO data persists
	// until the command buffer executes.
	uniformBuffer    vk.Buffer
	uniformMemory    vk.DeviceMemory
	uniformMapped    unsafe.Pointer
	uniformBufSize   int
	uniformCursor    int // next write offset (advanced per draw, reset per frame)

	// Vulkan-specific state for public API compatibility.
	instanceInfo       InstanceCreateInfo
	physicalDeviceInfo PhysicalDeviceInfo
	debugEnabled       bool

	// Swapchain state (populated when presenting directly to a surface).
	surfaceFactory      func(uintptr) (uintptr, error)
	surface             vk.SurfaceKHR
	swapchain           vk.SwapchainKHR
	swapchainImages     []vk.Image
	swapchainViews      []vk.ImageView
	swapchainFormat     uint32
	swapchainExtent     [2]uint32
	swapchainFBs        []vk.Framebuffer
	swapchainRenderPass vk.RenderPass
	currentImageIndex   uint32
	hasSwapchain        bool
	imageAvailableSem   vk.Semaphore
	renderFinishedSem   vk.Semaphore
}

// ensureDefaultSampler creates a default nearest-filter sampler if needed.
func (d *Device) ensureDefaultSampler() vk.Sampler {
	if d.defaultSampler != 0 {
		return d.defaultSampler
	}
	ci := vk.SamplerCreateInfo{
		SType:        vk.StructureTypeSamplerCreateInfo,
		MagFilter:    vk.FilterNearest,
		MinFilter:    vk.FilterNearest,
		MipmapMode:   vk.SamplerMipmapModeNearest,
		AddressModeU: vk.SamplerAddressModeClampToEdge,
		AddressModeV: vk.SamplerAddressModeClampToEdge,
		AddressModeW: vk.SamplerAddressModeClampToEdge,
		MaxLod:       1.0,
	}
	s, err := vk.CreateSampler(d.device, &ci)
	if err != nil {
		return 0
	}
	d.defaultSampler = s
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
			AppName:    "future-render",
			EngineName: "future-render",
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
	d.debugEnabled = cfg.Debug || os.Getenv("FUTURE_RENDER_VK_VALIDATION") == "1"
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

	// Create shared uniform buffer for UBO descriptors (16 KB, persistently mapped).
	if err := d.createUniformBuffer(16 * 1024); err != nil {
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

	// Create render pass.
	colorAttach := vk.AttachmentDescription{
		Format:         vk.FormatR8G8B8A8UNorm,
		Samples:        vk.SampleCount1,
		LoadOp:         vk.AttachmentLoadOpClear,
		StoreOp:        vk.AttachmentStoreOpStore,
		StencilLoadOp:  vk.AttachmentLoadOpDontCare,
		StencilStoreOp: vk.AttachmentStoreOpDontCare,
		InitialLayout:  vk.ImageLayoutUndefined,
		FinalLayout:    vk.ImageLayoutColorAttachmentOptimal,
	}
	colorRef := vk.AttachmentReference{
		Attachment: 0,
		Layout:     vk.ImageLayoutColorAttachmentOptimal,
	}
	subpass := vk.SubpassDescription{
		PipelineBindPoint:    vk.PipelineBindPointGraphics,
		ColorAttachmentCount: 1,
		PColorAttachments:    uintptr(unsafe.Pointer(&colorRef)),
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
		AttachmentCount: 1,
		PAttachments:    uintptr(unsafe.Pointer(&colorAttach)),
		SubpassCount:    1,
		PSubpasses:      uintptr(unsafe.Pointer(&subpass)),
		DependencyCount: 1,
		PDependencies:   uintptr(unsafe.Pointer(&dependency)),
	}
	rp, err := vk.CreateRenderPass(d.device, &rpCI)
	if err != nil {
		return err
	}
	d.defaultRenderPass = rp

	// Create framebuffer.
	fbCI := vk.FramebufferCreateInfo{
		SType:           vk.StructureTypeFramebufferCreateInfo,
		RenderPass_:     rp,
		AttachmentCount: 1,
		PAttachments:    uintptr(unsafe.Pointer(&d.defaultColorView)),
		Width:           uint32(d.width),
		Height:          uint32(d.height),
		Layers:          1,
	}
	fb, err := vk.CreateFramebuffer(d.device, &fbCI)
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

// Dispose releases all Vulkan resources.
func (d *Device) Dispose() {
	if d.device == 0 {
		return
	}
	_ = vk.DeviceWaitIdle(d.device)

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
	if d.defaultSampler != 0 {
		vk.DestroySampler(d.device, d.defaultSampler)
	}
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
	if d.defaultColorView != 0 {
		vk.DestroyImageView(d.device, d.defaultColorView)
	}
	if d.defaultColorImage != 0 {
		vk.DestroyImage(d.device, d.defaultColorImage)
	}
	if d.defaultColorMem != 0 {
		vk.FreeMemory(d.device, d.defaultColorMem)
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
	_ = vk.WaitForFence(d.device, d.fence, ^uint64(0))
	// GPU work from the previous frame is complete — safe to free resources.
	if d.encoder != nil {
		d.encoder.resetFrame()
	}
	_ = vk.ResetFence(d.device, d.fence)
	_ = vk.ResetCommandBuffer(d.commandBuffer)
	_ = vk.BeginCommandBuffer(d.commandBuffer, vk.CommandBufferUsageOneTimeSubmit)
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

// NewRenderTarget creates a VkFramebuffer with color (and optional depth) attachments.
func (d *Device) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("vulkan: invalid render target dimensions %dx%d", desc.Width, desc.Height)
	}

	// Create color texture.
	colorTex, err := d.NewTexture(backend.TextureDescriptor{
		Width: desc.Width, Height: desc.Height,
		Format:       desc.ColorFormat,
		RenderTarget: true,
	})
	if err != nil {
		return nil, err
	}

	// Create optional depth texture.
	var depthTex backend.Texture
	if desc.HasDepth {
		dt, err := d.NewTexture(backend.TextureDescriptor{
			Width: desc.Width, Height: desc.Height,
			Format:       desc.DepthFormat,
			RenderTarget: true,
		})
		if err != nil {
			colorTex.Dispose()
			return nil, err
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

	// Create render pass for swapchain (final layout = PresentSrcKHR).
	colorAttach := vk.AttachmentDescription{
		Format:         chosenFmt.Format,
		Samples:        vk.SampleCount1,
		LoadOp:         vk.AttachmentLoadOpClear,
		StoreOp:        vk.AttachmentStoreOpStore,
		StencilLoadOp:  vk.AttachmentLoadOpDontCare,
		StencilStoreOp: vk.AttachmentStoreOpDontCare,
		InitialLayout:  vk.ImageLayoutUndefined,
		FinalLayout:    vk.ImageLayoutPresentSrcKHR,
	}
	colorRef := vk.AttachmentReference{
		Attachment: 0,
		Layout:     vk.ImageLayoutColorAttachmentOptimal,
	}
	subpass := vk.SubpassDescription{
		PipelineBindPoint:    vk.PipelineBindPointGraphics,
		ColorAttachmentCount: 1,
		PColorAttachments:    uintptr(unsafe.Pointer(&colorRef)),
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
		AttachmentCount: 1,
		PAttachments:    uintptr(unsafe.Pointer(&colorAttach)),
		SubpassCount:    1,
		PSubpasses:      uintptr(unsafe.Pointer(&subpass)),
		DependencyCount: 1,
		PDependencies:   uintptr(unsafe.Pointer(&dependency)),
	}
	rp, err := vk.CreateRenderPass(d.device, &rpCI)
	if err != nil {
		return err
	}
	d.swapchainRenderPass = rp

	// Create framebuffers.
	d.swapchainFBs = make([]vk.Framebuffer, len(images))
	for i := range images {
		fbCI := vk.FramebufferCreateInfo{
			SType:           vk.StructureTypeFramebufferCreateInfo,
			RenderPass_:     rp,
			AttachmentCount: 1,
			PAttachments:    uintptr(unsafe.Pointer(&d.swapchainViews[i])),
			Width:           extent[0],
			Height:          extent[1],
			Layers:          1,
		}
		fb, ferr := vk.CreateFramebuffer(d.device, &fbCI)
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

	for _, v := range d.swapchainViews {
		if v != 0 {
			vk.DestroyImageView(d.device, v)
		}
	}
	d.swapchainViews = nil
	d.swapchainImages = nil

	if d.swapchain != 0 {
		vk.DestroySwapchainKHR(d.device, d.swapchain)
		d.swapchain = 0
	}
}

// recreateSwapchain rebuilds the swapchain after a resize or out-of-date error.
func (d *Device) recreateSwapchain(w, h int) error {
	_ = vk.DeviceWaitIdle(d.device)
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
