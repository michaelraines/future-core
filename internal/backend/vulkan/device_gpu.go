//go:build vulkan

package vulkan

import (
	"fmt"
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

	// Vulkan-specific state for public API compatibility.
	instanceInfo       InstanceCreateInfo
	physicalDeviceInfo PhysicalDeviceInfo
	debugEnabled       bool
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
	d.debugEnabled = cfg.Debug

	// Load Vulkan library.
	if err := vk.Init(); err != nil {
		return fmt.Errorf("vulkan: %w", err)
	}

	// Set up validation layers if debug mode.
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

	inst, err := vk.CreateInstance(&createInfo)
	if err != nil {
		return fmt.Errorf("vulkan: %w", err)
	}
	d.instance = inst

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

	dev, err := vk.CreateDevice(d.physicalDevice, &deviceCI)
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

	// Create staging buffer for transfers.
	if err := d.createStagingBuffer(4 * 1024 * 1024); err != nil {
		return fmt.Errorf("vulkan: staging: %w", err)
	}

	// Create encoder.
	d.encoder = &Encoder{dev: d, cmd: d.commandBuffer}

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

// Dispose releases all Vulkan resources.
func (d *Device) Dispose() {
	if d.device == 0 {
		return
	}
	_ = vk.DeviceWaitIdle(d.device)

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

// BeginFrame waits for the previous frame's fence and resets the command buffer.
func (d *Device) BeginFrame() {
	_ = vk.WaitForFence(d.device, d.fence, ^uint64(0))
	_ = vk.ResetFence(d.device, d.fence)
	_ = vk.ResetCommandBuffer(d.commandBuffer)
	_ = vk.BeginCommandBuffer(d.commandBuffer, vk.CommandBufferUsageOneTimeSubmit)
}

// EndFrame ends command recording and submits to the queue.
func (d *Device) EndFrame() {
	_ = vk.EndCommandBuffer(d.commandBuffer)
	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    uintptr(unsafe.Pointer(&d.commandBuffer)),
	}
	_ = vk.QueueSubmit(d.graphicsQueue, &submitInfo, d.fence)
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

	b := &Buffer{
		dev:     d,
		buffer:  buf,
		memory:  mem,
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
