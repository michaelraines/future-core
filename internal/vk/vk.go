//go:build vulkan

// Package vk provides pure Go Vulkan 1.2 bindings loaded at runtime via purego.
// No CGo is required. The shared library (libvulkan.so on Linux,
// vulkan-1.dll on Windows, libMoltenVK.dylib on macOS) must be available at
// runtime.
package vk

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// ---------------------------------------------------------------------------
// Vulkan handle types (opaque pointers)
// ---------------------------------------------------------------------------

type (
	Instance            uintptr
	PhysicalDevice      uintptr
	Device              uintptr
	Queue               uintptr
	CommandPool         uintptr
	CommandBuffer       uintptr
	Fence               uintptr
	Semaphore           uintptr
	RenderPass          uintptr
	Framebuffer         uintptr
	Image               uintptr
	ImageView           uintptr
	DeviceMemory        uintptr
	Buffer              uintptr
	Sampler             uintptr
	ShaderModule        uintptr
	PipelineLayout      uintptr
	Pipeline            uintptr
	DescriptorSetLayout uintptr
	DescriptorPool      uintptr
	DescriptorSet       uintptr
)

// Result is VkResult.
type Result int32

// VkResult constants.
const (
	Success                   Result = 0
	NotReady                  Result = 1
	Timeout                   Result = 2
	ErrorOutOfHostMemory      Result = -1
	ErrorOutOfDeviceMemory    Result = -2
	ErrorInitializationFailed Result = -3
	ErrorDeviceLost           Result = -4
	ErrorMemoryMapFailed      Result = -5
	ErrorLayerNotPresent      Result = -6
	ErrorExtensionNotPresent  Result = -7
)

func (r Result) Error() string { return fmt.Sprintf("VkResult(%d)", int32(r)) }

// ---------------------------------------------------------------------------
// Vulkan constants
// ---------------------------------------------------------------------------

const (
	StructureTypeInstanceCreateInfo                   = 1
	StructureTypeDeviceCreateInfo                     = 3
	StructureTypeDeviceQueueCreateInfo                = 2
	StructureTypeCommandPoolCreateInfo                = 39
	StructureTypeCommandBufferAllocateInfo            = 40
	StructureTypeCommandBufferBeginInfo               = 42
	StructureTypeFenceCreateInfo                      = 8
	StructureTypeSemaphoreCreateInfo                  = 9
	StructureTypeRenderPassCreateInfo                 = 38
	StructureTypeFramebufferCreateInfo                = 37
	StructureTypeImageCreateInfo                      = 14
	StructureTypeImageViewCreateInfo                  = 15
	StructureTypeBufferCreateInfo                     = 12
	StructureTypeSamplerCreateInfo                    = 31
	StructureTypeShaderModuleCreateInfo               = 16
	StructureTypePipelineLayoutCreateInfo             = 30
	StructureTypeGraphicsPipelineCreateInfo           = 28
	StructureTypeMemoryAllocateInfo                   = 5
	StructureTypeSubmitInfo                           = 4
	StructureTypeRenderPassBeginInfo                  = 43
	StructureTypeMappedMemoryRange                    = 6
	StructureTypeWriteDescriptorSet                   = 35
	StructureTypeDescriptorSetLayoutCreateInfo        = 32
	StructureTypeDescriptorPoolCreateInfo             = 33
	StructureTypeDescriptorSetAllocateInfo            = 34
	StructureTypePipelineVertexInputStateCreateInfo   = 19
	StructureTypePipelineInputAssemblyStateCreateInfo = 20
	StructureTypePipelineViewportStateCreateInfo      = 22
	StructureTypePipelineRasterizationStateCreateInfo = 23
	StructureTypePipelineMultisampleStateCreateInfo   = 24
	StructureTypePipelineDepthStencilStateCreateInfo  = 25
	StructureTypePipelineColorBlendStateCreateInfo    = 26
	StructureTypePipelineDynamicStateCreateInfo       = 27
	StructureTypeApplicationInfo                      = 0
)

// VkFormat constants.
const (
	FormatUndefined          = 0
	FormatR8UNorm            = 9
	FormatR8G8B8UNorm        = 23
	FormatR8G8B8A8UNorm      = 37
	FormatB8G8R8A8UNorm      = 44
	FormatR16G16B16A16SFloat = 97
	FormatR32G32B32A32SFloat = 109
	FormatD16UNorm           = 124
	FormatD32SFloat          = 126
	FormatD24UNormS8UInt     = 129
)

// VkImageUsageFlags.
const (
	ImageUsageTransferSrc        = 0x00000001
	ImageUsageTransferDst        = 0x00000002
	ImageUsageSampled            = 0x00000004
	ImageUsageColorAttachment    = 0x00000010
	ImageUsageDepthStencilAttach = 0x00000020
)

// VkBufferUsageFlags.
const (
	BufferUsageTransferSrc   = 0x00000001
	BufferUsageTransferDst   = 0x00000002
	BufferUsageUniformBuffer = 0x00000010
	BufferUsageIndexBuffer   = 0x00000040
	BufferUsageVertexBuffer  = 0x00000080
)

// VkMemoryPropertyFlags.
const (
	MemoryPropertyDeviceLocal  = 0x00000001
	MemoryPropertyHostVisible  = 0x00000002
	MemoryPropertyHostCoherent = 0x00000004
)

// VkImageType, VkImageViewType, VkImageLayout.
const (
	ImageType2D     = 1
	ImageViewType2D = 1

	ImageLayoutUndefined                 = 0
	ImageLayoutGeneral                   = 1
	ImageLayoutColorAttachmentOptimal    = 2
	ImageLayoutDepthStencilAttachOptimal = 3
	ImageLayoutTransferSrcOptimal        = 6
	ImageLayoutTransferDstOptimal        = 7
	ImageLayoutShaderReadOnlyOptimal     = 5
	ImageLayoutPresentSrcKHR             = 1000001002
)

// VkImageAspectFlags.
const (
	ImageAspectColor = 0x00000001
	ImageAspectDepth = 0x00000002
)

// VkSharingMode.
const (
	SharingModeExclusive = 0
)

// VkSampleCountFlags.
const (
	SampleCount1 = 0x00000001
	SampleCount4 = 0x00000004
)

// VkImageTiling.
const (
	ImageTilingOptimal = 0
	ImageTilingLinear  = 1
)

// VkComponentSwizzle.
const (
	ComponentSwizzleIdentity = 0
)

// VkFilter, VkSamplerMipmapMode.
const (
	FilterNearest = 0
	FilterLinear  = 1

	SamplerMipmapModeNearest = 0
	SamplerMipmapModeLinear  = 1
)

// VkSamplerAddressMode.
const (
	SamplerAddressModeRepeat         = 0
	SamplerAddressModeMirroredRepeat = 1
	SamplerAddressModeClampToEdge    = 2
)

// VkBlendFactor.
const (
	BlendFactorZero             = 0
	BlendFactorOne              = 1
	BlendFactorSrcAlpha         = 6
	BlendFactorOneMinusSrcAlpha = 7
	BlendFactorDstColor         = 8
)

// VkBlendOp.
const (
	BlendOpAdd = 0
)

// VkColorComponentFlags.
const (
	ColorComponentR   = 0x00000001
	ColorComponentG   = 0x00000002
	ColorComponentB   = 0x00000004
	ColorComponentA   = 0x00000008
	ColorComponentAll = ColorComponentR | ColorComponentG | ColorComponentB | ColorComponentA
)

// VkCompareOp.
const (
	CompareOpNever          = 0
	CompareOpLess           = 1
	CompareOpEqual          = 2
	CompareOpLessOrEqual    = 3
	CompareOpGreater        = 4
	CompareOpNotEqual       = 5
	CompareOpGreaterOrEqual = 6
	CompareOpAlways         = 7
)

// VkCullModeFlags, VkFrontFace.
const (
	CullModeNone  = 0
	CullModeFront = 0x00000001
	CullModeBack  = 0x00000002

	FrontFaceCounterClockwise = 0
)

// VkPrimitiveTopology.
const (
	PrimitiveTopologyPointList     = 0
	PrimitiveTopologyLineList      = 1
	PrimitiveTopologyLineStrip     = 2
	PrimitiveTopologyTriangleList  = 3
	PrimitiveTopologyTriangleStrip = 4
)

// VkPolygonMode.
const (
	PolygonModeFill = 0
)

// VkDynamicState.
const (
	DynamicStateViewport = 0
	DynamicStateScissor  = 1
)

// VkShaderStageFlags.
const (
	ShaderStageVertex      = 0x00000001
	ShaderStageFragment    = 0x00000010
	ShaderStageAllGraphics = ShaderStageVertex | ShaderStageFragment
)

// VkDescriptorType.
const (
	DescriptorTypeUniformBuffer        = 6
	DescriptorTypeCombinedImageSampler = 1
)

// VkAttachmentLoadOp, VkAttachmentStoreOp.
const (
	AttachmentLoadOpLoad     = 0
	AttachmentLoadOpClear    = 1
	AttachmentLoadOpDontCare = 2

	AttachmentStoreOpStore    = 0
	AttachmentStoreOpDontCare = 1
)

// VkPipelineBindPoint.
const (
	PipelineBindPointGraphics = 0
)

// VkSubpassContents.
const (
	SubpassContentsInline = 0
)

// VkPipelineStageFlags.
const (
	PipelineStageColorAttachmentOutput = 0x00000400
)

// VkAccessFlags.
const (
	AccessColorAttachmentWrite = 0x00000100
)

// VkFenceCreateFlags.
const (
	FenceCreateSignaled = 0x00000001
)

// VkCommandPoolCreateFlags.
const (
	CommandPoolCreateResetCommandBuffer = 0x00000002
)

// VkCommandBufferLevel.
const (
	CommandBufferLevelPrimary = 0
)

// VkCommandBufferUsageFlags.
const (
	CommandBufferUsageOneTimeSubmit = 0x00000001
)

// VkIndexType.
const (
	IndexTypeUint16 = 0
	IndexTypeUint32 = 1
)

// VkQueueFlags.
const (
	QueueGraphics = 0x00000001
)

// Null handle.
const NullHandle = 0

// Whole size constant.
const WholeSize = ^uint64(0)

// ---------------------------------------------------------------------------
// Vulkan structs (C-compatible layout)
// ---------------------------------------------------------------------------

// ApplicationInfo mirrors VkApplicationInfo.
type ApplicationInfo struct {
	SType              uint32
	PNext              uintptr
	PApplicationName   uintptr
	ApplicationVersion uint32
	PEngineName        uintptr
	EngineVersion      uint32
	APIVersion         uint32
}

// InstanceCreateInfo mirrors VkInstanceCreateInfo.
type InstanceCreateInfo struct {
	SType                   uint32
	PNext                   uintptr
	Flags                   uint32
	PApplicationInfo        uintptr
	EnabledLayerCount       uint32
	PPEnabledLayerNames     uintptr
	EnabledExtensionCount   uint32
	PPEnabledExtensionNames uintptr
}

// DeviceQueueCreateInfo mirrors VkDeviceQueueCreateInfo.
type DeviceQueueCreateInfo struct {
	SType            uint32
	PNext            uintptr
	Flags            uint32
	QueueFamilyIndex uint32
	QueueCount       uint32
	PQueuePriorities uintptr
}

// DeviceCreateInfo mirrors VkDeviceCreateInfo.
type DeviceCreateInfo struct {
	SType                   uint32
	PNext                   uintptr
	Flags                   uint32
	QueueCreateInfoCount    uint32
	PQueueCreateInfos       uintptr
	EnabledLayerCount       uint32
	PPEnabledLayerNames     uintptr
	EnabledExtensionCount   uint32
	PPEnabledExtensionNames uintptr
	PEnabledFeatures        uintptr
}

// PhysicalDeviceProperties mirrors VkPhysicalDeviceProperties (partial).
type PhysicalDeviceProperties struct {
	APIVersion    uint32
	DriverVersion uint32
	VendorID      uint32
	DeviceID      uint32
	DeviceType    uint32
	DeviceName    [256]byte
	// ... remaining fields omitted, we use offsets
}

// PhysicalDeviceMemoryProperties mirrors VkPhysicalDeviceMemoryProperties.
type PhysicalDeviceMemoryProperties struct {
	MemoryTypeCount uint32
	MemoryTypes     [32]MemoryType
	MemoryHeapCount uint32
	MemoryHeaps     [16]MemoryHeap
}

// MemoryType mirrors VkMemoryType.
type MemoryType struct {
	PropertyFlags uint32
	HeapIndex     uint32
}

// MemoryHeap mirrors VkMemoryHeap.
type MemoryHeap struct {
	Size  uint64
	Flags uint32
	_     uint32 // padding
}

// QueueFamilyProperties mirrors VkQueueFamilyProperties.
type QueueFamilyProperties struct {
	QueueFlags                  uint32
	QueueCount                  uint32
	TimestampValidBits          uint32
	MinImageTransferGranularity [3]uint32
}

// MemoryAllocateInfo mirrors VkMemoryAllocateInfo.
type MemoryAllocateInfo struct {
	SType           uint32
	PNext           uintptr
	AllocationSize  uint64
	MemoryTypeIndex uint32
	_               uint32 // padding
}

// MemoryRequirements mirrors VkMemoryRequirements.
type MemoryRequirements struct {
	Size           uint64
	Alignment      uint64
	MemoryTypeBits uint32
	_              uint32 // padding
}

// ImageCreateInfo mirrors VkImageCreateInfo.
type ImageCreateInfo struct {
	SType                 uint32
	PNext                 uintptr
	Flags                 uint32
	ImageType             uint32
	Format                uint32
	ExtentWidth           uint32
	ExtentHeight          uint32
	ExtentDepth           uint32
	MipLevels             uint32
	ArrayLayers           uint32
	Samples               uint32
	Tiling                uint32
	Usage                 uint32
	SharingMode           uint32
	QueueFamilyIndexCount uint32
	PQueueFamilyIndices   uintptr
	InitialLayout         uint32
}

// ImageViewCreateInfo mirrors VkImageViewCreateInfo.
type ImageViewCreateInfo struct {
	SType            uint32
	PNext            uintptr
	Flags            uint32
	Image            Image
	ViewType         uint32
	Format           uint32
	ComponentR       uint32
	ComponentG       uint32
	ComponentB       uint32
	ComponentA       uint32
	SubresAspectMask uint32
	SubresBaseMip    uint32
	SubresLevelCount uint32
	SubresBaseLayer  uint32
	SubresLayerCount uint32
}

// BufferCreateInfo mirrors VkBufferCreateInfo.
type BufferCreateInfo struct {
	SType                 uint32
	PNext                 uintptr
	Flags                 uint32
	Size                  uint64
	Usage                 uint32
	SharingMode           uint32
	QueueFamilyIndexCount uint32
	PQueueFamilyIndices   uintptr
}

// SamplerCreateInfo mirrors VkSamplerCreateInfo.
type SamplerCreateInfo struct {
	SType                   uint32
	PNext                   uintptr
	Flags                   uint32
	MagFilter               uint32
	MinFilter               uint32
	MipmapMode              uint32
	AddressModeU            uint32
	AddressModeV            uint32
	AddressModeW            uint32
	MipLodBias              float32
	AnisotropyEnable        uint32
	MaxAnisotropy           float32
	CompareEnable           uint32
	CompareOp               uint32
	MinLod                  float32
	MaxLod                  float32
	BorderColor             uint32
	UnnormalizedCoordinates uint32
}

// ShaderModuleCreateInfo mirrors VkShaderModuleCreateInfo.
type ShaderModuleCreateInfo struct {
	SType    uint32
	PNext    uintptr
	Flags    uint32
	CodeSize uint64
	PCode    uintptr
}

// SubpassDescription mirrors VkSubpassDescription.
type SubpassDescription struct {
	Flags                   uint32
	PipelineBindPoint       uint32
	InputAttachmentCount    uint32
	PInputAttachments       uintptr
	ColorAttachmentCount    uint32
	PColorAttachments       uintptr
	PResolveAttachments     uintptr
	PDepthStencilAttachment uintptr
	PreserveAttachmentCount uint32
	PPreserveAttachments    uintptr
}

// AttachmentDescription mirrors VkAttachmentDescription.
type AttachmentDescription struct {
	Flags          uint32
	Format         uint32
	Samples        uint32
	LoadOp         uint32
	StoreOp        uint32
	StencilLoadOp  uint32
	StencilStoreOp uint32
	InitialLayout  uint32
	FinalLayout    uint32
}

// AttachmentReference mirrors VkAttachmentReference.
type AttachmentReference struct {
	Attachment uint32
	Layout     uint32
}

// SubpassDependency mirrors VkSubpassDependency.
type SubpassDependency struct {
	SrcSubpass      uint32
	DstSubpass      uint32
	SrcStageMask    uint32
	DstStageMask    uint32
	SrcAccessMask   uint32
	DstAccessMask   uint32
	DependencyFlags uint32
}

// RenderPassCreateInfo mirrors VkRenderPassCreateInfo.
type RenderPassCreateInfo struct {
	SType           uint32
	PNext           uintptr
	Flags           uint32
	AttachmentCount uint32
	PAttachments    uintptr
	SubpassCount    uint32
	PSubpasses      uintptr
	DependencyCount uint32
	PDependencies   uintptr
}

// FramebufferCreateInfo mirrors VkFramebufferCreateInfo.
type FramebufferCreateInfo struct {
	SType           uint32
	PNext           uintptr
	Flags           uint32
	RenderPass_     RenderPass
	AttachmentCount uint32
	PAttachments    uintptr
	Width           uint32
	Height          uint32
	Layers          uint32
}

// CommandPoolCreateInfo mirrors VkCommandPoolCreateInfo.
type CommandPoolCreateInfo struct {
	SType            uint32
	PNext            uintptr
	Flags            uint32
	QueueFamilyIndex uint32
}

// CommandBufferAllocateInfo mirrors VkCommandBufferAllocateInfo.
type CommandBufferAllocateInfo struct {
	SType              uint32
	PNext              uintptr
	CommandPool_       CommandPool
	Level              uint32
	CommandBufferCount uint32
}

// CommandBufferBeginInfo mirrors VkCommandBufferBeginInfo.
type CommandBufferBeginInfo struct {
	SType            uint32
	PNext            uintptr
	Flags            uint32
	PInheritanceInfo uintptr
}

// RenderPassBeginInfo mirrors VkRenderPassBeginInfo.
type RenderPassBeginInfo struct {
	SType           uint32
	PNext           uintptr
	RenderPass_     RenderPass
	Framebuffer_    Framebuffer
	RenderAreaX     int32
	RenderAreaY     int32
	RenderAreaW     uint32
	RenderAreaH     uint32
	ClearValueCount uint32
	PClearValues    uintptr
}

// ClearValue holds a clear color (as 4 float32) or depth/stencil.
type ClearValue struct {
	Color [4]float32
}

// ClearValueDepthStencil for depth/stencil clears.
type ClearValueDepthStencil struct {
	Depth   float32
	Stencil uint32
}

// FenceCreateInfo mirrors VkFenceCreateInfo.
type FenceCreateInfo struct {
	SType uint32
	PNext uintptr
	Flags uint32
}

// SemaphoreCreateInfo mirrors VkSemaphoreCreateInfo.
type SemaphoreCreateInfo struct {
	SType uint32
	PNext uintptr
	Flags uint32
}

// SubmitInfo mirrors VkSubmitInfo.
type SubmitInfo struct {
	SType                uint32
	PNext                uintptr
	WaitSemaphoreCount   uint32
	PWaitSemaphores      uintptr
	PWaitDstStageMask    uintptr
	CommandBufferCount   uint32
	PCommandBuffers      uintptr
	SignalSemaphoreCount uint32
	PSignalSemaphores    uintptr
}

// Viewport mirrors VkViewport.
type Viewport struct {
	X, Y, Width, Height, MinDepth, MaxDepth float32
}

// Rect2D mirrors VkRect2D.
type Rect2D struct {
	OffsetX, OffsetY int32
	ExtentW, ExtentH uint32
}

// BufferImageCopy mirrors VkBufferImageCopy.
type BufferImageCopy struct {
	BufferOffset      uint64
	BufferRowLength   uint32
	BufferImageHeight uint32
	// ImageSubresourceLayers fields (inlined for C-compatibility).
	AspectMask     uint32
	MipLevel       uint32
	BaseArrayLayer uint32
	LayerCount     uint32
	// ImageOffset (VkOffset3D).
	ImageOffsetX int32
	ImageOffsetY int32
	ImageOffsetZ int32
	// ImageExtent (VkExtent3D).
	ImageExtentW uint32
	ImageExtentH uint32
	ImageExtentD uint32
}

// ---------------------------------------------------------------------------
// Internal function variables — populated by Init()
// ---------------------------------------------------------------------------

//nolint:unused // populated dynamically
var (
	fnCreateInstance                         func(pCreateInfo uintptr, pAllocator uintptr, pInstance *Instance) Result
	fnDestroyInstance                        func(instance Instance, pAllocator uintptr)
	fnEnumeratePhysicalDevices               func(instance Instance, pCount *uint32, pDevices uintptr) Result
	fnGetPhysicalDeviceProperties            func(device PhysicalDevice, pProperties uintptr)
	fnGetPhysicalDeviceMemoryProperties      func(device PhysicalDevice, pProperties uintptr)
	fnGetPhysicalDeviceQueueFamilyProperties func(device PhysicalDevice, pCount *uint32, pProperties uintptr)
	fnCreateDevice                           func(physicalDevice PhysicalDevice, pCreateInfo uintptr, pAllocator uintptr, pDevice *Device) Result
	fnDestroyDevice                          func(device Device, pAllocator uintptr)
	fnGetDeviceQueue                         func(device Device, queueFamilyIndex, queueIndex uint32, pQueue *Queue)
	fnDeviceWaitIdle                         func(device Device) Result

	fnCreateCommandPool      func(device Device, pCreateInfo uintptr, pAllocator uintptr, pPool *CommandPool) Result
	fnDestroyCommandPool     func(device Device, pool CommandPool, pAllocator uintptr)
	fnAllocateCommandBuffers func(device Device, pAllocateInfo uintptr, pCommandBuffers *CommandBuffer) Result
	fnFreeCommandBuffers     func(device Device, pool CommandPool, count uint32, pCommandBuffers *CommandBuffer)
	fnBeginCommandBuffer     func(commandBuffer CommandBuffer, pBeginInfo uintptr) Result
	fnEndCommandBuffer       func(commandBuffer CommandBuffer) Result
	fnResetCommandBuffer     func(commandBuffer CommandBuffer, flags uint32) Result

	fnCreateFence      func(device Device, pCreateInfo uintptr, pAllocator uintptr, pFence *Fence) Result
	fnDestroyFence     func(device Device, fence Fence, pAllocator uintptr)
	fnWaitForFences    func(device Device, fenceCount uint32, pFences *Fence, waitAll uint32, timeout uint64) Result
	fnResetFences      func(device Device, fenceCount uint32, pFences *Fence) Result
	fnCreateSemaphore  func(device Device, pCreateInfo uintptr, pAllocator uintptr, pSemaphore *Semaphore) Result
	fnDestroySemaphore func(device Device, semaphore Semaphore, pAllocator uintptr)
	fnQueueSubmit      func(queue Queue, submitCount uint32, pSubmits uintptr, fence Fence) Result

	fnCreateImage                func(device Device, pCreateInfo uintptr, pAllocator uintptr, pImage *Image) Result
	fnDestroyImage               func(device Device, image Image, pAllocator uintptr)
	fnCreateImageView            func(device Device, pCreateInfo uintptr, pAllocator uintptr, pView *ImageView) Result
	fnDestroyImageView           func(device Device, imageView ImageView, pAllocator uintptr)
	fnGetImageMemoryRequirements func(device Device, image Image, pRequirements uintptr)
	fnBindImageMemory            func(device Device, image Image, memory DeviceMemory, offset uint64) Result

	fnCreateBuffer                func(device Device, pCreateInfo uintptr, pAllocator uintptr, pBuffer *Buffer) Result
	fnDestroyBuffer               func(device Device, buffer Buffer, pAllocator uintptr)
	fnGetBufferMemoryRequirements func(device Device, buffer Buffer, pRequirements uintptr)
	fnBindBufferMemory            func(device Device, buffer Buffer, memory DeviceMemory, offset uint64) Result

	fnAllocateMemory func(device Device, pAllocateInfo uintptr, pAllocator uintptr, pMemory *DeviceMemory) Result
	fnFreeMemory     func(device Device, memory DeviceMemory, pAllocator uintptr)
	fnMapMemory      func(device Device, memory DeviceMemory, offset, size uint64, flags uint32, ppData *unsafe.Pointer) Result
	fnUnmapMemory    func(device Device, memory DeviceMemory)

	fnCreateSampler  func(device Device, pCreateInfo uintptr, pAllocator uintptr, pSampler *Sampler) Result
	fnDestroySampler func(device Device, sampler Sampler, pAllocator uintptr)

	fnCreateShaderModule  func(device Device, pCreateInfo uintptr, pAllocator uintptr, pModule *ShaderModule) Result
	fnDestroyShaderModule func(device Device, module ShaderModule, pAllocator uintptr)

	fnCreateRenderPass  func(device Device, pCreateInfo uintptr, pAllocator uintptr, pRenderPass *RenderPass) Result
	fnDestroyRenderPass func(device Device, renderPass RenderPass, pAllocator uintptr)

	fnCreateFramebuffer  func(device Device, pCreateInfo uintptr, pAllocator uintptr, pFramebuffer *Framebuffer) Result
	fnDestroyFramebuffer func(device Device, framebuffer Framebuffer, pAllocator uintptr)

	fnCreatePipelineLayout  func(device Device, pCreateInfo uintptr, pAllocator uintptr, pLayout *PipelineLayout) Result
	fnDestroyPipelineLayout func(device Device, layout PipelineLayout, pAllocator uintptr)

	fnCreateGraphicsPipelines func(device Device, pipelineCache uintptr, createInfoCount uint32, pCreateInfos uintptr, pAllocator uintptr, pPipelines *Pipeline) Result
	fnDestroyPipeline         func(device Device, pipeline Pipeline, pAllocator uintptr)

	fnCreateDescriptorSetLayout  func(device Device, pCreateInfo uintptr, pAllocator uintptr, pLayout *DescriptorSetLayout) Result
	fnDestroyDescriptorSetLayout func(device Device, layout DescriptorSetLayout, pAllocator uintptr)
	fnCreateDescriptorPool       func(device Device, pCreateInfo uintptr, pAllocator uintptr, pPool *DescriptorPool) Result
	fnDestroyDescriptorPool      func(device Device, pool DescriptorPool, pAllocator uintptr)
	fnAllocateDescriptorSets     func(device Device, pAllocateInfo uintptr, pSets *DescriptorSet) Result
	fnUpdateDescriptorSets       func(device Device, writeCount uint32, pWrites uintptr, copyCount uint32, pCopies uintptr)

	// Command buffer recording.
	fnCmdBeginRenderPass    func(cmd CommandBuffer, pBeginInfo uintptr, contents uint32)
	fnCmdEndRenderPass      func(cmd CommandBuffer)
	fnCmdBindPipeline       func(cmd CommandBuffer, bindPoint uint32, pipeline Pipeline)
	fnCmdBindVertexBuffers  func(cmd CommandBuffer, firstBinding, bindingCount uint32, pBuffers uintptr, pOffsets uintptr)
	fnCmdBindIndexBuffer    func(cmd CommandBuffer, buffer Buffer, offset uint64, indexType uint32)
	fnCmdBindDescriptorSets func(cmd CommandBuffer, bindPoint uint32, layout PipelineLayout, firstSet, count uint32, pSets uintptr, dynamicOffsetCount uint32, pDynamicOffsets uintptr)
	fnCmdDraw               func(cmd CommandBuffer, vertexCount, instanceCount, firstVertex, firstInstance uint32)
	fnCmdDrawIndexed        func(cmd CommandBuffer, indexCount, instanceCount, firstIndex uint32, vertexOffset int32, firstInstance uint32)
	fnCmdSetViewport        func(cmd CommandBuffer, firstViewport, viewportCount uint32, pViewports uintptr)
	fnCmdSetScissor         func(cmd CommandBuffer, firstScissor, scissorCount uint32, pScissors uintptr)
	fnCmdCopyBufferToImage  func(cmd CommandBuffer, srcBuffer Buffer, dstImage Image, dstImageLayout uint32, regionCount uint32, pRegions uintptr)
	fnCmdCopyImageToBuffer  func(cmd CommandBuffer, srcImage Image, srcImageLayout uint32, dstBuffer Buffer, regionCount uint32, pRegions uintptr)
	fnCmdPipelineBarrier    func(cmd CommandBuffer, srcStageMask, dstStageMask, dependencyFlags uint32, memBarrierCount uint32, pMemBarriers uintptr, bufBarrierCount uint32, pBufBarriers uintptr, imgBarrierCount uint32, pImgBarriers uintptr)
)

// lib holds the loaded Vulkan library handle.
var lib uintptr

// ---------------------------------------------------------------------------
// Public wrappers
// ---------------------------------------------------------------------------

// CreateInstance wraps vkCreateInstance.
func CreateInstance(info *InstanceCreateInfo) (Instance, error) {
	var inst Instance
	r := fnCreateInstance(uintptr(unsafe.Pointer(info)), 0, &inst)
	if r != Success {
		return 0, fmt.Errorf("vkCreateInstance: %w", r)
	}
	return inst, nil
}

// DestroyInstance wraps vkDestroyInstance.
func DestroyInstance(inst Instance) { fnDestroyInstance(inst, 0) }

// EnumeratePhysicalDevices wraps vkEnumeratePhysicalDevices.
func EnumeratePhysicalDevices(inst Instance) ([]PhysicalDevice, error) {
	var count uint32
	r := fnEnumeratePhysicalDevices(inst, &count, 0)
	if r != Success {
		return nil, fmt.Errorf("vkEnumeratePhysicalDevices (count): %w", r)
	}
	if count == 0 {
		return nil, nil
	}
	devices := make([]PhysicalDevice, count)
	r = fnEnumeratePhysicalDevices(inst, &count, uintptr(unsafe.Pointer(&devices[0])))
	if r != Success {
		return nil, fmt.Errorf("vkEnumeratePhysicalDevices: %w", r)
	}
	return devices, nil
}

// GetPhysicalDeviceProperties wraps vkGetPhysicalDeviceProperties.
func GetPhysicalDeviceProperties(dev PhysicalDevice) PhysicalDeviceProperties {
	var props PhysicalDeviceProperties
	fnGetPhysicalDeviceProperties(dev, uintptr(unsafe.Pointer(&props)))
	return props
}

// GetPhysicalDeviceMemoryProperties wraps vkGetPhysicalDeviceMemoryProperties.
func GetPhysicalDeviceMemoryProperties(dev PhysicalDevice) PhysicalDeviceMemoryProperties {
	var props PhysicalDeviceMemoryProperties
	fnGetPhysicalDeviceMemoryProperties(dev, uintptr(unsafe.Pointer(&props)))
	return props
}

// GetPhysicalDeviceQueueFamilyProperties wraps the Vulkan function.
func GetPhysicalDeviceQueueFamilyProperties(dev PhysicalDevice) []QueueFamilyProperties {
	var count uint32
	fnGetPhysicalDeviceQueueFamilyProperties(dev, &count, 0)
	if count == 0 {
		return nil
	}
	props := make([]QueueFamilyProperties, count)
	fnGetPhysicalDeviceQueueFamilyProperties(dev, &count, uintptr(unsafe.Pointer(&props[0])))
	return props
}

// CreateDevice wraps vkCreateDevice.
func CreateDevice(physDev PhysicalDevice, info *DeviceCreateInfo) (Device, error) {
	var dev Device
	r := fnCreateDevice(physDev, uintptr(unsafe.Pointer(info)), 0, &dev)
	if r != Success {
		return 0, fmt.Errorf("vkCreateDevice: %w", r)
	}
	return dev, nil
}

// DestroyDevice wraps vkDestroyDevice.
func DestroyDevice(dev Device) { fnDestroyDevice(dev, 0) }

// GetDeviceQueue wraps vkGetDeviceQueue.
func GetDeviceQueue(dev Device, familyIndex, queueIndex uint32) Queue {
	var q Queue
	fnGetDeviceQueue(dev, familyIndex, queueIndex, &q)
	return q
}

// DeviceWaitIdle wraps vkDeviceWaitIdle.
func DeviceWaitIdle(dev Device) error {
	r := fnDeviceWaitIdle(dev)
	if r != Success {
		return fmt.Errorf("vkDeviceWaitIdle: %w", r)
	}
	return nil
}

// CreateCommandPool wraps vkCreateCommandPool.
func CreateCommandPool(dev Device, info *CommandPoolCreateInfo) (CommandPool, error) {
	var pool CommandPool
	r := fnCreateCommandPool(dev, uintptr(unsafe.Pointer(info)), 0, &pool)
	if r != Success {
		return 0, fmt.Errorf("vkCreateCommandPool: %w", r)
	}
	return pool, nil
}

// DestroyCommandPool wraps vkDestroyCommandPool.
func DestroyCommandPool(dev Device, pool CommandPool) { fnDestroyCommandPool(dev, pool, 0) }

// AllocateCommandBuffer allocates a single primary command buffer.
func AllocateCommandBuffer(dev Device, pool CommandPool) (CommandBuffer, error) {
	info := CommandBufferAllocateInfo{
		SType:              StructureTypeCommandBufferAllocateInfo,
		CommandPool_:       pool,
		Level:              CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	}
	var cmd CommandBuffer
	r := fnAllocateCommandBuffers(dev, uintptr(unsafe.Pointer(&info)), &cmd)
	if r != Success {
		return 0, fmt.Errorf("vkAllocateCommandBuffers: %w", r)
	}
	return cmd, nil
}

// BeginCommandBuffer wraps vkBeginCommandBuffer.
func BeginCommandBuffer(cmd CommandBuffer, flags uint32) error {
	info := CommandBufferBeginInfo{
		SType: StructureTypeCommandBufferBeginInfo,
		Flags: flags,
	}
	r := fnBeginCommandBuffer(cmd, uintptr(unsafe.Pointer(&info)))
	if r != Success {
		return fmt.Errorf("vkBeginCommandBuffer: %w", r)
	}
	return nil
}

// EndCommandBuffer wraps vkEndCommandBuffer.
func EndCommandBuffer(cmd CommandBuffer) error {
	r := fnEndCommandBuffer(cmd)
	if r != Success {
		return fmt.Errorf("vkEndCommandBuffer: %w", r)
	}
	return nil
}

// ResetCommandBuffer wraps vkResetCommandBuffer.
func ResetCommandBuffer(cmd CommandBuffer) error {
	r := fnResetCommandBuffer(cmd, 0)
	if r != Success {
		return fmt.Errorf("vkResetCommandBuffer: %w", r)
	}
	return nil
}

// CreateFence wraps vkCreateFence.
func CreateFence(dev Device, signaled bool) (Fence, error) {
	info := FenceCreateInfo{SType: StructureTypeFenceCreateInfo}
	if signaled {
		info.Flags = FenceCreateSignaled
	}
	var fence Fence
	r := fnCreateFence(dev, uintptr(unsafe.Pointer(&info)), 0, &fence)
	if r != Success {
		return 0, fmt.Errorf("vkCreateFence: %w", r)
	}
	return fence, nil
}

// DestroyFence wraps vkDestroyFence.
func DestroyFence(dev Device, fence Fence) { fnDestroyFence(dev, fence, 0) }

// WaitForFence wraps vkWaitForFences for a single fence.
func WaitForFence(dev Device, fence Fence, timeout uint64) error {
	r := fnWaitForFences(dev, 1, &fence, 1, timeout)
	if r != Success {
		return fmt.Errorf("vkWaitForFences: %w", r)
	}
	return nil
}

// ResetFence wraps vkResetFences for a single fence.
func ResetFence(dev Device, fence Fence) error {
	r := fnResetFences(dev, 1, &fence)
	if r != Success {
		return fmt.Errorf("vkResetFences: %w", r)
	}
	return nil
}

// QueueSubmit wraps vkQueueSubmit.
func QueueSubmit(queue Queue, info *SubmitInfo, fence Fence) error {
	r := fnQueueSubmit(queue, 1, uintptr(unsafe.Pointer(info)), fence)
	if r != Success {
		return fmt.Errorf("vkQueueSubmit: %w", r)
	}
	return nil
}

// CreateImageRaw wraps vkCreateImage.
func CreateImageRaw(dev Device, info *ImageCreateInfo) (Image, error) {
	var img Image
	r := fnCreateImage(dev, uintptr(unsafe.Pointer(info)), 0, &img)
	if r != Success {
		return 0, fmt.Errorf("vkCreateImage: %w", r)
	}
	return img, nil
}

// DestroyImage wraps vkDestroyImage.
func DestroyImage(dev Device, img Image) { fnDestroyImage(dev, img, 0) }

// CreateImageView wraps vkCreateImageView.
func CreateImageViewRaw(dev Device, info *ImageViewCreateInfo) (ImageView, error) {
	var view ImageView
	r := fnCreateImageView(dev, uintptr(unsafe.Pointer(info)), 0, &view)
	if r != Success {
		return 0, fmt.Errorf("vkCreateImageView: %w", r)
	}
	return view, nil
}

// DestroyImageView wraps vkDestroyImageView.
func DestroyImageView(dev Device, view ImageView) { fnDestroyImageView(dev, view, 0) }

// GetImageMemoryRequirements wraps vkGetImageMemoryRequirements.
func GetImageMemoryRequirements(dev Device, img Image) MemoryRequirements {
	var req MemoryRequirements
	fnGetImageMemoryRequirements(dev, img, uintptr(unsafe.Pointer(&req)))
	return req
}

// BindImageMemory wraps vkBindImageMemory.
func BindImageMemory(dev Device, img Image, mem DeviceMemory, offset uint64) error {
	r := fnBindImageMemory(dev, img, mem, offset)
	if r != Success {
		return fmt.Errorf("vkBindImageMemory: %w", r)
	}
	return nil
}

// CreateBufferRaw wraps vkCreateBuffer.
func CreateBufferRaw(dev Device, info *BufferCreateInfo) (Buffer, error) {
	var buf Buffer
	r := fnCreateBuffer(dev, uintptr(unsafe.Pointer(info)), 0, &buf)
	if r != Success {
		return 0, fmt.Errorf("vkCreateBuffer: %w", r)
	}
	return buf, nil
}

// DestroyBuffer wraps vkDestroyBuffer.
func DestroyBuffer(dev Device, buf Buffer) { fnDestroyBuffer(dev, buf, 0) }

// GetBufferMemoryRequirements wraps vkGetBufferMemoryRequirements.
func GetBufferMemoryRequirements(dev Device, buf Buffer) MemoryRequirements {
	var req MemoryRequirements
	fnGetBufferMemoryRequirements(dev, buf, uintptr(unsafe.Pointer(&req)))
	return req
}

// BindBufferMemory wraps vkBindBufferMemory.
func BindBufferMemory(dev Device, buf Buffer, mem DeviceMemory, offset uint64) error {
	r := fnBindBufferMemory(dev, buf, mem, offset)
	if r != Success {
		return fmt.Errorf("vkBindBufferMemory: %w", r)
	}
	return nil
}

// AllocateMemory wraps vkAllocateMemory.
func AllocateMemory(dev Device, info *MemoryAllocateInfo) (DeviceMemory, error) {
	var mem DeviceMemory
	r := fnAllocateMemory(dev, uintptr(unsafe.Pointer(info)), 0, &mem)
	if r != Success {
		return 0, fmt.Errorf("vkAllocateMemory: %w", r)
	}
	return mem, nil
}

// FreeMemory wraps vkFreeMemory.
func FreeMemory(dev Device, mem DeviceMemory) { fnFreeMemory(dev, mem, 0) }

// MapMemory wraps vkMapMemory.
func MapMemory(dev Device, mem DeviceMemory, offset, size uint64) (unsafe.Pointer, error) {
	var ptr unsafe.Pointer
	r := fnMapMemory(dev, mem, offset, size, 0, &ptr)
	if r != Success {
		return nil, fmt.Errorf("vkMapMemory: %w", r)
	}
	return ptr, nil
}

// UnmapMemory wraps vkUnmapMemory.
func UnmapMemory(dev Device, mem DeviceMemory) { fnUnmapMemory(dev, mem) }

// CreateSampler wraps vkCreateSampler.
func CreateSampler(dev Device, info *SamplerCreateInfo) (Sampler, error) {
	var s Sampler
	r := fnCreateSampler(dev, uintptr(unsafe.Pointer(info)), 0, &s)
	if r != Success {
		return 0, fmt.Errorf("vkCreateSampler: %w", r)
	}
	return s, nil
}

// DestroySampler wraps vkDestroySampler.
func DestroySampler(dev Device, s Sampler) { fnDestroySampler(dev, s, 0) }

// CreateShaderModule wraps vkCreateShaderModule.
func CreateShaderModule(dev Device, info *ShaderModuleCreateInfo) (ShaderModule, error) {
	var mod ShaderModule
	r := fnCreateShaderModule(dev, uintptr(unsafe.Pointer(info)), 0, &mod)
	if r != Success {
		return 0, fmt.Errorf("vkCreateShaderModule: %w", r)
	}
	return mod, nil
}

// DestroyShaderModule wraps vkDestroyShaderModule.
func DestroyShaderModule(dev Device, mod ShaderModule) { fnDestroyShaderModule(dev, mod, 0) }

// CreateRenderPass wraps vkCreateRenderPass.
func CreateRenderPass(dev Device, info *RenderPassCreateInfo) (RenderPass, error) {
	var rp RenderPass
	r := fnCreateRenderPass(dev, uintptr(unsafe.Pointer(info)), 0, &rp)
	if r != Success {
		return 0, fmt.Errorf("vkCreateRenderPass: %w", r)
	}
	return rp, nil
}

// DestroyRenderPass wraps vkDestroyRenderPass.
func DestroyRenderPass(dev Device, rp RenderPass) { fnDestroyRenderPass(dev, rp, 0) }

// CreateFramebuffer wraps vkCreateFramebuffer.
func CreateFramebuffer(dev Device, info *FramebufferCreateInfo) (Framebuffer, error) {
	var fb Framebuffer
	r := fnCreateFramebuffer(dev, uintptr(unsafe.Pointer(info)), 0, &fb)
	if r != Success {
		return 0, fmt.Errorf("vkCreateFramebuffer: %w", r)
	}
	return fb, nil
}

// DestroyFramebuffer wraps vkDestroyFramebuffer.
func DestroyFramebuffer(dev Device, fb Framebuffer) { fnDestroyFramebuffer(dev, fb, 0) }

// CreatePipelineLayout wraps vkCreatePipelineLayout.
func CreatePipelineLayout(dev Device, info uintptr) (PipelineLayout, error) {
	var layout PipelineLayout
	r := fnCreatePipelineLayout(dev, info, 0, &layout)
	if r != Success {
		return 0, fmt.Errorf("vkCreatePipelineLayout: %w", r)
	}
	return layout, nil
}

// DestroyPipelineLayout wraps vkDestroyPipelineLayout.
func DestroyPipelineLayout(dev Device, layout PipelineLayout) {
	fnDestroyPipelineLayout(dev, layout, 0)
}

// CreateGraphicsPipeline wraps vkCreateGraphicsPipelines for a single pipeline.
func CreateGraphicsPipeline(dev Device, info uintptr) (Pipeline, error) {
	var pip Pipeline
	r := fnCreateGraphicsPipelines(dev, 0, 1, info, 0, &pip)
	if r != Success {
		return 0, fmt.Errorf("vkCreateGraphicsPipelines: %w", r)
	}
	return pip, nil
}

// DestroyPipeline wraps vkDestroyPipeline.
func DestroyPipeline(dev Device, pip Pipeline) { fnDestroyPipeline(dev, pip, 0) }

// CmdBeginRenderPass wraps vkCmdBeginRenderPass.
func CmdBeginRenderPass(cmd CommandBuffer, info *RenderPassBeginInfo) {
	fnCmdBeginRenderPass(cmd, uintptr(unsafe.Pointer(info)), SubpassContentsInline)
}

// CmdEndRenderPass wraps vkCmdEndRenderPass.
func CmdEndRenderPass(cmd CommandBuffer) { fnCmdEndRenderPass(cmd) }

// CmdBindPipeline wraps vkCmdBindPipeline.
func CmdBindPipeline(cmd CommandBuffer, pip Pipeline) {
	fnCmdBindPipeline(cmd, PipelineBindPointGraphics, pip)
}

// CmdBindVertexBuffer wraps vkCmdBindVertexBuffers for a single buffer.
func CmdBindVertexBuffer(cmd CommandBuffer, binding uint32, buf Buffer, offset uint64) {
	fnCmdBindVertexBuffers(cmd, binding, 1, uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&offset)))
}

// CmdBindIndexBuffer wraps vkCmdBindIndexBuffer.
func CmdBindIndexBuffer(cmd CommandBuffer, buf Buffer, offset uint64, indexType uint32) {
	fnCmdBindIndexBuffer(cmd, buf, offset, indexType)
}

// CmdDraw wraps vkCmdDraw.
func CmdDraw(cmd CommandBuffer, vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	fnCmdDraw(cmd, vertexCount, instanceCount, firstVertex, firstInstance)
}

// CmdDrawIndexed wraps vkCmdDrawIndexed.
func CmdDrawIndexed(cmd CommandBuffer, indexCount, instanceCount, firstIndex uint32, vertexOffset int32, firstInstance uint32) {
	fnCmdDrawIndexed(cmd, indexCount, instanceCount, firstIndex, vertexOffset, firstInstance)
}

// CmdSetViewport wraps vkCmdSetViewport.
func CmdSetViewport(cmd CommandBuffer, vp Viewport) {
	fnCmdSetViewport(cmd, 0, 1, uintptr(unsafe.Pointer(&vp)))
}

// CmdSetScissor wraps vkCmdSetScissor.
func CmdSetScissor(cmd CommandBuffer, rect Rect2D) {
	fnCmdSetScissor(cmd, 0, 1, uintptr(unsafe.Pointer(&rect)))
}

// CmdCopyBufferToImage wraps vkCmdCopyBufferToImage.
func CmdCopyBufferToImage(cmd CommandBuffer, srcBuffer Buffer, dstImage Image, dstLayout uint32, region BufferImageCopy) {
	fnCmdCopyBufferToImage(cmd, srcBuffer, dstImage, dstLayout, 1, uintptr(unsafe.Pointer(&region)))
}

// FreeCommandBuffers wraps vkFreeCommandBuffers for a single command buffer.
func FreeCommandBuffers(dev Device, pool CommandPool, cmd CommandBuffer) {
	fnFreeCommandBuffers(dev, pool, 1, &cmd)
}

// ---------------------------------------------------------------------------
// Memory helpers
// ---------------------------------------------------------------------------

// FindMemoryType selects a memory type that satisfies the filter and property flags.
func FindMemoryType(memProps PhysicalDeviceMemoryProperties, filter uint32, flags uint32) (uint32, error) {
	for i := uint32(0); i < memProps.MemoryTypeCount; i++ {
		if filter&(1<<i) != 0 && memProps.MemoryTypes[i].PropertyFlags&flags == flags {
			return i, nil
		}
	}
	return 0, fmt.Errorf("vk: no suitable memory type found (filter=0x%x, flags=0x%x)", filter, flags)
}

// CStr converts a Go string to a null-terminated C string in a fresh []byte.
func CStr(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// CStrSlice converts a Go string slice to an array of C string pointers.
func CStrSlice(strs []string) []*byte {
	ptrs := make([]*byte, len(strs))
	for i, s := range strs {
		ptrs[i] = CStr(s)
	}
	return ptrs
}

// ---------------------------------------------------------------------------
// Initialization
// ---------------------------------------------------------------------------

// Init loads the Vulkan shared library and resolves all function pointers.
func Init() error {
	var err error
	lib, err = openVulkanLib()
	if err != nil {
		return fmt.Errorf("vk: %w", err)
	}

	must := func(fn interface{}, name string) error {
		addr, serr := purego.Dlsym(lib, name)
		if serr != nil {
			return fmt.Errorf("vk: symbol %s: %w", name, serr)
		}
		purego.RegisterFunc(fn, addr)
		return nil
	}

	// Instance-level functions.
	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnCreateInstance, "vkCreateInstance"},
		{&fnDestroyInstance, "vkDestroyInstance"},
		{&fnEnumeratePhysicalDevices, "vkEnumeratePhysicalDevices"},
		{&fnGetPhysicalDeviceProperties, "vkGetPhysicalDeviceProperties"},
		{&fnGetPhysicalDeviceMemoryProperties, "vkGetPhysicalDeviceMemoryProperties"},
		{&fnGetPhysicalDeviceQueueFamilyProperties, "vkGetPhysicalDeviceQueueFamilyProperties"},
		{&fnCreateDevice, "vkCreateDevice"},
		{&fnDestroyDevice, "vkDestroyDevice"},
		{&fnGetDeviceQueue, "vkGetDeviceQueue"},
		{&fnDeviceWaitIdle, "vkDeviceWaitIdle"},
	} {
		if ferr := must(e.fn, e.name); ferr != nil {
			return ferr
		}
	}

	// Command buffer functions.
	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnCreateCommandPool, "vkCreateCommandPool"},
		{&fnDestroyCommandPool, "vkDestroyCommandPool"},
		{&fnAllocateCommandBuffers, "vkAllocateCommandBuffers"},
		{&fnFreeCommandBuffers, "vkFreeCommandBuffers"},
		{&fnBeginCommandBuffer, "vkBeginCommandBuffer"},
		{&fnEndCommandBuffer, "vkEndCommandBuffer"},
		{&fnResetCommandBuffer, "vkResetCommandBuffer"},
	} {
		if ferr := must(e.fn, e.name); ferr != nil {
			return ferr
		}
	}

	// Synchronization functions.
	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnCreateFence, "vkCreateFence"},
		{&fnDestroyFence, "vkDestroyFence"},
		{&fnWaitForFences, "vkWaitForFences"},
		{&fnResetFences, "vkResetFences"},
		{&fnCreateSemaphore, "vkCreateSemaphore"},
		{&fnDestroySemaphore, "vkDestroySemaphore"},
		{&fnQueueSubmit, "vkQueueSubmit"},
	} {
		if ferr := must(e.fn, e.name); ferr != nil {
			return ferr
		}
	}

	// Resource creation functions.
	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnCreateImage, "vkCreateImage"},
		{&fnDestroyImage, "vkDestroyImage"},
		{&fnCreateImageView, "vkCreateImageView"},
		{&fnDestroyImageView, "vkDestroyImageView"},
		{&fnGetImageMemoryRequirements, "vkGetImageMemoryRequirements"},
		{&fnBindImageMemory, "vkBindImageMemory"},
		{&fnCreateBuffer, "vkCreateBuffer"},
		{&fnDestroyBuffer, "vkDestroyBuffer"},
		{&fnGetBufferMemoryRequirements, "vkGetBufferMemoryRequirements"},
		{&fnBindBufferMemory, "vkBindBufferMemory"},
		{&fnAllocateMemory, "vkAllocateMemory"},
		{&fnFreeMemory, "vkFreeMemory"},
		{&fnMapMemory, "vkMapMemory"},
		{&fnUnmapMemory, "vkUnmapMemory"},
		{&fnCreateSampler, "vkCreateSampler"},
		{&fnDestroySampler, "vkDestroySampler"},
		{&fnCreateShaderModule, "vkCreateShaderModule"},
		{&fnDestroyShaderModule, "vkDestroyShaderModule"},
	} {
		if ferr := must(e.fn, e.name); ferr != nil {
			return ferr
		}
	}

	// Render pass and pipeline functions.
	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnCreateRenderPass, "vkCreateRenderPass"},
		{&fnDestroyRenderPass, "vkDestroyRenderPass"},
		{&fnCreateFramebuffer, "vkCreateFramebuffer"},
		{&fnDestroyFramebuffer, "vkDestroyFramebuffer"},
		{&fnCreatePipelineLayout, "vkCreatePipelineLayout"},
		{&fnDestroyPipelineLayout, "vkDestroyPipelineLayout"},
		{&fnCreateGraphicsPipelines, "vkCreateGraphicsPipelines"},
		{&fnDestroyPipeline, "vkDestroyPipeline"},
		{&fnCreateDescriptorSetLayout, "vkCreateDescriptorSetLayout"},
		{&fnDestroyDescriptorSetLayout, "vkDestroyDescriptorSetLayout"},
		{&fnCreateDescriptorPool, "vkCreateDescriptorPool"},
		{&fnDestroyDescriptorPool, "vkDestroyDescriptorPool"},
		{&fnAllocateDescriptorSets, "vkAllocateDescriptorSets"},
		{&fnUpdateDescriptorSets, "vkUpdateDescriptorSets"},
	} {
		if ferr := must(e.fn, e.name); ferr != nil {
			return ferr
		}
	}

	// Command recording functions.
	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnCmdBeginRenderPass, "vkCmdBeginRenderPass"},
		{&fnCmdEndRenderPass, "vkCmdEndRenderPass"},
		{&fnCmdBindPipeline, "vkCmdBindPipeline"},
		{&fnCmdBindVertexBuffers, "vkCmdBindVertexBuffers"},
		{&fnCmdBindIndexBuffer, "vkCmdBindIndexBuffer"},
		{&fnCmdBindDescriptorSets, "vkCmdBindDescriptorSets"},
		{&fnCmdDraw, "vkCmdDraw"},
		{&fnCmdDrawIndexed, "vkCmdDrawIndexed"},
		{&fnCmdSetViewport, "vkCmdSetViewport"},
		{&fnCmdSetScissor, "vkCmdSetScissor"},
		{&fnCmdCopyBufferToImage, "vkCmdCopyBufferToImage"},
		{&fnCmdCopyImageToBuffer, "vkCmdCopyImageToBuffer"},
		{&fnCmdPipelineBarrier, "vkCmdPipelineBarrier"},
	} {
		if ferr := must(e.fn, e.name); ferr != nil {
			return ferr
		}
	}

	return nil
}

// openVulkanLib opens the platform-specific Vulkan shared library.
func openVulkanLib() (uintptr, error) {
	var names []string
	switch runtime.GOOS {
	case "darwin":
		names = []string{"libMoltenVK.dylib", "libvulkan.1.dylib", "libvulkan.dylib"}
	case "windows":
		names = []string{"vulkan-1.dll"}
	default: // linux, freebsd, android
		names = []string{"libvulkan.so.1", "libvulkan.so"}
	}

	var firstErr error
	for _, name := range names {
		h, err := purego.Dlopen(name, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err == nil {
			return h, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return 0, fmt.Errorf("failed to load Vulkan: %w", firstErr)
}
