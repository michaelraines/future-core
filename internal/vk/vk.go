//go:build (darwin || linux || freebsd || windows) && !soft

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
	SurfaceKHR          uintptr
	SwapchainKHR        uintptr
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
	ErrorOutOfDateKHR         Result = -1000001004
	SuboptimalKHR             Result = 1000001003
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
	StructureTypeImageMemoryBarrier                   = 46
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
	StructureTypePipelineShaderStageCreateInfo        = 18

	// KHR extension structure types.
	StructureTypeSwapchainCreateInfoKHR    = 1000001000
	StructureTypePresentInfoKHR            = 1000001001
	StructureTypeMetalSurfaceCreateInfoEXT = 1000217000
	StructureTypeWin32SurfaceCreateInfoKHR = 1000009000
	StructureTypeXlibSurfaceCreateInfoKHR  = 1000004000
)

// VkFormat constants.
const (
	FormatUndefined          = 0
	FormatR8UNorm            = 9
	FormatR8G8B8UNorm        = 23
	FormatR8G8B8A8UNorm      = 37
	FormatB8G8R8A8UNorm      = 44
	FormatB8G8R8A8SRGB       = 50
	FormatR16G16B16A16SFloat = 97
	FormatR32G32SFloat       = 103
	FormatR32G32B32SFloat    = 106
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
	BlendFactorDstColor         = 4
	BlendFactorSrcAlpha         = 6
	BlendFactorOneMinusSrcAlpha = 7
	BlendFactorDstAlpha         = 8
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
	FrontFaceCW               = 1
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

// VkVertexInputRate.
const (
	VertexInputRateVertex = 0
)

// QueueFamilyIgnored is VK_QUEUE_FAMILY_IGNORED.
const QueueFamilyIgnored = 0xFFFFFFFF

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
	PipelineStageTopOfPipe             = 0x00000001
	PipelineStageVertexInput           = 0x00000004
	PipelineStageFragmentShader        = 0x00000080
	PipelineStageColorAttachmentOutput = 0x00000400
	PipelineStageTransfer              = 0x00001000
	PipelineStageBottomOfPipe          = 0x00002000
)

// VkAccessFlags.
const (
	AccessTransferRead         = 0x00000800
	AccessTransferWrite        = 0x00001000
	AccessShaderRead           = 0x00000020
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

// VkPresentModeKHR.
const (
	PresentModeImmediateKHR = 0
	PresentModeMailboxKHR   = 1
	PresentModeFifoKHR      = 2
)

// VkColorSpaceKHR.
const (
	ColorSpaceSRGBNonLinearKHR = 0
)

// VkCompositeAlphaFlagBitsKHR.
const (
	CompositeAlphaOpaqueKHR = 0x00000001
)

// VkSurfaceTransformFlagBitsKHR.
const (
	SurfaceTransformIdentityKHR = 0x00000001
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
// Graphics pipeline state structs
// ---------------------------------------------------------------------------

// PipelineShaderStageCreateInfo mirrors VkPipelineShaderStageCreateInfo.
type PipelineShaderStageCreateInfo struct {
	SType               uint32
	PNext               uintptr
	Flags               uint32
	Stage               uint32
	Module              ShaderModule
	PName               uintptr
	PSpecializationInfo uintptr
}

// PipelineVertexInputStateCreateInfo mirrors VkPipelineVertexInputStateCreateInfo.
type PipelineVertexInputStateCreateInfo struct {
	SType                           uint32
	PNext                           uintptr
	Flags                           uint32
	VertexBindingDescriptionCount   uint32
	PVertexBindingDescriptions      uintptr
	VertexAttributeDescriptionCount uint32
	PVertexAttributeDescriptions    uintptr
}

// VertexInputBindingDescription mirrors VkVertexInputBindingDescription.
type VertexInputBindingDescription struct {
	Binding   uint32
	Stride    uint32
	InputRate uint32
}

// VertexInputAttributeDescription mirrors VkVertexInputAttributeDescription.
type VertexInputAttributeDescription struct {
	Location uint32
	Binding  uint32
	Format   uint32
	Offset   uint32
}

// PipelineInputAssemblyStateCreateInfo mirrors VkPipelineInputAssemblyStateCreateInfo.
type PipelineInputAssemblyStateCreateInfo struct {
	SType                  uint32
	PNext                  uintptr
	Flags                  uint32
	Topology               uint32
	PrimitiveRestartEnable uint32
}

// PipelineViewportStateCreateInfo mirrors VkPipelineViewportStateCreateInfo.
type PipelineViewportStateCreateInfo struct {
	SType         uint32
	PNext         uintptr
	Flags         uint32
	ViewportCount uint32
	PViewports    uintptr
	ScissorCount  uint32
	PScissors     uintptr
}

// PipelineRasterizationStateCreateInfo mirrors VkPipelineRasterizationStateCreateInfo.
type PipelineRasterizationStateCreateInfo struct {
	SType                   uint32
	PNext                   uintptr
	Flags                   uint32
	DepthClampEnable        uint32
	RasterizerDiscardEnable uint32
	PolygonMode             uint32
	CullMode                uint32
	FrontFace               uint32
	DepthBiasEnable         uint32
	DepthBiasConstantFactor float32
	DepthBiasClamp          float32
	DepthBiasSlopeFactor    float32
	LineWidth               float32
}

// PipelineMultisampleStateCreateInfo mirrors VkPipelineMultisampleStateCreateInfo.
type PipelineMultisampleStateCreateInfo struct {
	SType                 uint32
	PNext                 uintptr
	Flags                 uint32
	RasterizationSamples  uint32
	SampleShadingEnable   uint32
	MinSampleShading      float32
	PSampleMask           uintptr
	AlphaToCoverageEnable uint32
	AlphaToOneEnable      uint32
}

// PipelineDepthStencilStateCreateInfo mirrors VkPipelineDepthStencilStateCreateInfo.
type PipelineDepthStencilStateCreateInfo struct {
	SType                 uint32
	PNext                 uintptr
	Flags                 uint32
	DepthTestEnable       uint32
	DepthWriteEnable      uint32
	DepthCompareOp        uint32
	DepthBoundsTestEnable uint32
	StencilTestEnable     uint32
	FrontFailOp           uint32
	FrontPassOp           uint32
	FrontDepthFailOp      uint32
	FrontCompareOp        uint32
	FrontCompareMask      uint32
	FrontWriteMask        uint32
	FrontReference        uint32
	BackFailOp            uint32
	BackPassOp            uint32
	BackDepthFailOp       uint32
	BackCompareOp         uint32
	BackCompareMask       uint32
	BackWriteMask         uint32
	BackReference         uint32
	MinDepthBounds        float32
	MaxDepthBounds        float32
}

// PipelineColorBlendAttachmentState mirrors VkPipelineColorBlendAttachmentState.
type PipelineColorBlendAttachmentState struct {
	BlendEnable         uint32
	SrcColorBlendFactor uint32
	DstColorBlendFactor uint32
	ColorBlendOp        uint32
	SrcAlphaBlendFactor uint32
	DstAlphaBlendFactor uint32
	AlphaBlendOp        uint32
	ColorWriteMask      uint32
}

// PipelineColorBlendStateCreateInfo mirrors VkPipelineColorBlendStateCreateInfo.
type PipelineColorBlendStateCreateInfo struct {
	SType           uint32
	PNext           uintptr
	Flags           uint32
	LogicOpEnable   uint32
	LogicOp         uint32
	AttachmentCount uint32
	PAttachments    uintptr
	BlendConstants  [4]float32
}

// PipelineDynamicStateCreateInfo mirrors VkPipelineDynamicStateCreateInfo.
type PipelineDynamicStateCreateInfo struct {
	SType             uint32
	PNext             uintptr
	Flags             uint32
	DynamicStateCount uint32
	PDynamicStates    uintptr
}

// GraphicsPipelineCreateInfo mirrors VkGraphicsPipelineCreateInfo.
type GraphicsPipelineCreateInfo struct {
	SType               uint32
	PNext               uintptr
	Flags               uint32
	StageCount          uint32
	PStages             uintptr
	PVertexInputState   uintptr
	PInputAssemblyState uintptr
	PTessellationState  uintptr
	PViewportState      uintptr
	PRasterizationState uintptr
	PMultisampleState   uintptr
	PDepthStencilState  uintptr
	PColorBlendState    uintptr
	PDynamicState       uintptr
	Layout              PipelineLayout
	RenderPass_         RenderPass
	Subpass             uint32
	BasePipeline        Pipeline
	BasePipelineIndex   int32
}

// PipelineLayoutCreateInfo mirrors VkPipelineLayoutCreateInfo.
type PipelineLayoutCreateInfo struct {
	SType                  uint32
	PNext                  uintptr
	Flags                  uint32
	SetLayoutCount         uint32
	PSetLayouts            uintptr
	PushConstantRangeCount uint32
	PPushConstantRanges    uintptr
}

// DescriptorSetLayoutBinding mirrors VkDescriptorSetLayoutBinding.
type DescriptorSetLayoutBinding struct {
	Binding            uint32
	DescriptorType     uint32
	DescriptorCount    uint32
	StageFlags         uint32
	PImmutableSamplers uintptr
}

// DescriptorSetLayoutCreateInfo mirrors VkDescriptorSetLayoutCreateInfo.
type DescriptorSetLayoutCreateInfo struct {
	SType        uint32
	PNext        uintptr
	Flags        uint32
	BindingCount uint32
	PBindings    uintptr
}

// DescriptorPoolSize mirrors VkDescriptorPoolSize.
type DescriptorPoolSize struct {
	Type_           uint32
	DescriptorCount uint32
}

// DescriptorPoolCreateInfo mirrors VkDescriptorPoolCreateInfo.
type DescriptorPoolCreateInfo struct {
	SType         uint32
	PNext         uintptr
	Flags         uint32
	MaxSets       uint32
	PoolSizeCount uint32
	PPoolSizes    uintptr
}

// DescriptorSetAllocateInfo mirrors VkDescriptorSetAllocateInfo.
type DescriptorSetAllocateInfo struct {
	SType              uint32
	PNext              uintptr
	DescriptorPool_    DescriptorPool
	DescriptorSetCount uint32
	PSetLayouts        uintptr
}

// WriteDescriptorSet mirrors VkWriteDescriptorSet.
type WriteDescriptorSet struct {
	SType            uint32
	PNext            uintptr
	DstSet           DescriptorSet
	DstBinding       uint32
	DstArrayElement  uint32
	DescriptorCount  uint32
	DescriptorType   uint32
	PImageInfo       uintptr
	PBufferInfo      uintptr
	PTexelBufferView uintptr
}

// DescriptorImageInfo mirrors VkDescriptorImageInfo.
type DescriptorImageInfo struct {
	Sampler     Sampler
	ImageView   ImageView
	ImageLayout uint32
}

// PushConstantRange mirrors VkPushConstantRange.
type PushConstantRange struct {
	StageFlags uint32
	Offset     uint32
	Size       uint32
}

// DescriptorBufferInfo mirrors VkDescriptorBufferInfo.
type DescriptorBufferInfo struct {
	Buffer_ Buffer
	Offset  uint64
	Range_  uint64
}

// ImageMemoryBarrier mirrors VkImageMemoryBarrier.
type ImageMemoryBarrier struct {
	SType               uint32
	PNext               uintptr
	SrcAccessMask       uint32
	DstAccessMask       uint32
	OldLayout           uint32
	NewLayout           uint32
	SrcQueueFamilyIndex uint32
	DstQueueFamilyIndex uint32
	Image_              Image
	SubresAspectMask    uint32
	SubresBaseMip       uint32
	SubresLevelCount    uint32
	SubresBaseLayer     uint32
	SubresLayerCount    uint32
}

// ---------------------------------------------------------------------------
// KHR extension structs (surface + swapchain)
// ---------------------------------------------------------------------------

// SurfaceCapabilitiesKHR mirrors VkSurfaceCapabilitiesKHR.
type SurfaceCapabilitiesKHR struct {
	MinImageCount           uint32
	MaxImageCount           uint32
	CurrentExtentWidth      uint32
	CurrentExtentHeight     uint32
	MinImageExtentWidth     uint32
	MinImageExtentHeight    uint32
	MaxImageExtentWidth     uint32
	MaxImageExtentHeight    uint32
	MaxImageArrayLayers     uint32
	SupportedTransforms     uint32
	CurrentTransform        uint32
	SupportedCompositeAlpha uint32
	SupportedUsageFlags     uint32
}

// SurfaceFormatKHR mirrors VkSurfaceFormatKHR.
type SurfaceFormatKHR struct {
	Format     uint32
	ColorSpace uint32
}

// SwapchainCreateInfoKHR mirrors VkSwapchainCreateInfoKHR.
type SwapchainCreateInfoKHR struct {
	SType                 uint32
	PNext                 uintptr
	Flags                 uint32
	Surface               SurfaceKHR
	MinImageCount         uint32
	ImageFormat           uint32
	ImageColorSpace       uint32
	ImageExtentWidth      uint32
	ImageExtentHeight     uint32
	ImageArrayLayers      uint32
	ImageUsage            uint32
	ImageSharingMode      uint32
	QueueFamilyIndexCount uint32
	PQueueFamilyIndices   uintptr
	PreTransform          uint32
	CompositeAlpha        uint32
	PresentMode           uint32
	Clipped               uint32
	OldSwapchain          SwapchainKHR
}

// PresentInfoKHR mirrors VkPresentInfoKHR.
type PresentInfoKHR struct {
	SType              uint32
	PNext              uintptr
	WaitSemaphoreCount uint32
	PWaitSemaphores    uintptr
	SwapchainCount     uint32
	PSwapchains        uintptr
	PImageIndices      uintptr
	PResults           uintptr
}

// MetalSurfaceCreateInfoEXT mirrors VkMetalSurfaceCreateInfoEXT.
type MetalSurfaceCreateInfoEXT struct {
	SType  uint32
	PNext  uintptr
	Flags  uint32
	PLayer uintptr // CAMetalLayer*
}

// Win32SurfaceCreateInfoKHR mirrors VkWin32SurfaceCreateInfoKHR.
type Win32SurfaceCreateInfoKHR struct {
	SType     uint32
	PNext     uintptr
	Flags     uint32
	Hinstance uintptr
	Hwnd      uintptr
}

// XlibSurfaceCreateInfoKHR mirrors VkXlibSurfaceCreateInfoKHR.
type XlibSurfaceCreateInfoKHR struct {
	SType   uint32
	PNext   uintptr
	Flags   uint32
	Display uintptr // Display*
	Window  uintptr // Window (X11)
}

// ---------------------------------------------------------------------------
// Internal function variables — populated by Init()
// ---------------------------------------------------------------------------

//nolint:unused // populated dynamically
var (
	fnEnumerateInstanceExtensionProperties   func(pLayerName uintptr, pPropertyCount *uint32, pProperties uintptr) Result
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
	fnCmdPushConstants      func(cmd CommandBuffer, layout PipelineLayout, stageFlags, offset, size uint32, pValues uintptr)

	// vkGetInstanceProcAddr — loaded from the Vulkan library directly.
	fnGetInstanceProcAddr func(instance Instance, pName uintptr) uintptr
)

// KHR extension function pointers — loaded via InitSwapchainFunctions after
// instance creation. These are nil until that call.
//
//nolint:unused // populated dynamically
var (
	fnDestroySurfaceKHR                       func(instance Instance, surface SurfaceKHR, pAllocator uintptr)
	fnGetPhysicalDeviceSurfaceSupportKHR      func(physicalDevice PhysicalDevice, queueFamilyIndex uint32, surface SurfaceKHR, pSupported *uint32) Result
	fnGetPhysicalDeviceSurfaceCapabilitiesKHR func(physicalDevice PhysicalDevice, surface SurfaceKHR, pCapabilities uintptr) Result
	fnGetPhysicalDeviceSurfaceFormatsKHR      func(physicalDevice PhysicalDevice, surface SurfaceKHR, pFormatCount *uint32, pFormats uintptr) Result
	fnGetPhysicalDeviceSurfacePresentModesKHR func(physicalDevice PhysicalDevice, surface SurfaceKHR, pModeCount *uint32, pModes uintptr) Result
	fnCreateSwapchainKHR                      func(device Device, pCreateInfo uintptr, pAllocator uintptr, pSwapchain *SwapchainKHR) Result
	fnDestroySwapchainKHR                     func(device Device, swapchain SwapchainKHR, pAllocator uintptr)
	fnGetSwapchainImagesKHR                   func(device Device, swapchain SwapchainKHR, pCount *uint32, pImages uintptr) Result
	fnAcquireNextImageKHR                     func(device Device, swapchain SwapchainKHR, timeout uint64, semaphore Semaphore, fence Fence, pImageIndex *uint32) Result
	fnQueuePresentKHR                         func(queue Queue, pPresentInfo uintptr) Result
	fnCreateMetalSurfaceEXT                   func(instance Instance, pCreateInfo uintptr, pAllocator uintptr, pSurface *SurfaceKHR) Result
	fnCreateWin32SurfaceKHR                   func(instance Instance, pCreateInfo uintptr, pAllocator uintptr, pSurface *SurfaceKHR) Result
	fnCreateXlibSurfaceKHR                    func(instance Instance, pCreateInfo uintptr, pAllocator uintptr, pSurface *SurfaceKHR) Result
)

// lib holds the loaded Vulkan library handle.
var lib uintptr

// ---------------------------------------------------------------------------
// Public wrappers
// ---------------------------------------------------------------------------

// ExtensionProperties mirrors VkExtensionProperties (partial).
type ExtensionProperties struct {
	ExtensionName [256]byte
	SpecVersion   uint32
}

// EnumerateInstanceExtensionProperties returns available instance extensions.
func EnumerateInstanceExtensionProperties() ([]string, error) {
	var count uint32
	r := fnEnumerateInstanceExtensionProperties(0, &count, 0)
	if r != Success {
		return nil, fmt.Errorf("vkEnumerateInstanceExtensionProperties (count): %w", r)
	}
	if count == 0 {
		return nil, nil
	}
	props := make([]ExtensionProperties, count)
	r = fnEnumerateInstanceExtensionProperties(0, &count, uintptr(unsafe.Pointer(&props[0])))
	if r != Success {
		return nil, fmt.Errorf("vkEnumerateInstanceExtensionProperties: %w", r)
	}
	names := make([]string, count)
	for i, p := range props[:count] {
		n := 0
		for n < len(p.ExtensionName) && p.ExtensionName[n] != 0 {
			n++
		}
		names[i] = string(p.ExtensionName[:n])
	}
	return names, nil
}

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

// CreateSemaphore wraps vkCreateSemaphore.
func CreateSemaphore(dev Device) (Semaphore, error) {
	info := struct {
		SType uint32
		PNext uintptr
		Flags uint32
	}{SType: StructureTypeSemaphoreCreateInfo}
	var sem Semaphore
	r := fnCreateSemaphore(dev, uintptr(unsafe.Pointer(&info)), 0, &sem)
	if r != Success {
		return 0, fmt.Errorf("vkCreateSemaphore: %w", r)
	}
	return sem, nil
}

// DestroySemaphore wraps vkDestroySemaphore.
func DestroySemaphore(dev Device, sem Semaphore) { fnDestroySemaphore(dev, sem, 0) }

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

// CmdCopyImageToBuffer wraps vkCmdCopyImageToBuffer.
func CmdCopyImageToBuffer(cmd CommandBuffer, srcImage Image, srcLayout uint32, dstBuffer Buffer, region BufferImageCopy) {
	fnCmdCopyImageToBuffer(cmd, srcImage, srcLayout, dstBuffer, 1, uintptr(unsafe.Pointer(&region)))
}

// CmdPipelineBarrier wraps vkCmdPipelineBarrier for image memory barriers.
func CmdPipelineBarrier(cmd CommandBuffer, srcStage, dstStage uint32, barriers []ImageMemoryBarrier) {
	var ptr uintptr
	if len(barriers) > 0 {
		ptr = uintptr(unsafe.Pointer(&barriers[0]))
	}
	fnCmdPipelineBarrier(cmd, srcStage, dstStage, 0, 0, 0, 0, 0, uint32(len(barriers)), ptr)
}

// CmdBindDescriptorSets wraps vkCmdBindDescriptorSets.
func CmdBindDescriptorSets(cmd CommandBuffer, layout PipelineLayout, firstSet uint32, sets []DescriptorSet) {
	if len(sets) == 0 {
		return
	}
	fnCmdBindDescriptorSets(cmd, PipelineBindPointGraphics, layout, firstSet, uint32(len(sets)),
		uintptr(unsafe.Pointer(&sets[0])), 0, 0)
}

// CmdPushConstants wraps vkCmdPushConstants.
func CmdPushConstants(cmd CommandBuffer, layout PipelineLayout, stageFlags, offset, size uint32, data unsafe.Pointer) {
	fnCmdPushConstants(cmd, layout, stageFlags, offset, size, uintptr(data))
}

// CreateDescriptorSetLayout wraps vkCreateDescriptorSetLayout.
func CreateDescriptorSetLayout(dev Device, info *DescriptorSetLayoutCreateInfo) (DescriptorSetLayout, error) {
	var layout DescriptorSetLayout
	r := fnCreateDescriptorSetLayout(dev, uintptr(unsafe.Pointer(info)), 0, &layout)
	if r != Success {
		return 0, fmt.Errorf("vkCreateDescriptorSetLayout: %w", r)
	}
	return layout, nil
}

// DestroyDescriptorSetLayout wraps vkDestroyDescriptorSetLayout.
func DestroyDescriptorSetLayout(dev Device, layout DescriptorSetLayout) {
	fnDestroyDescriptorSetLayout(dev, layout, 0)
}

// CreateDescriptorPool wraps vkCreateDescriptorPool.
func CreateDescriptorPool(dev Device, info *DescriptorPoolCreateInfo) (DescriptorPool, error) {
	var pool DescriptorPool
	r := fnCreateDescriptorPool(dev, uintptr(unsafe.Pointer(info)), 0, &pool)
	if r != Success {
		return 0, fmt.Errorf("vkCreateDescriptorPool: %w", r)
	}
	return pool, nil
}

// DestroyDescriptorPool wraps vkDestroyDescriptorPool.
func DestroyDescriptorPool(dev Device, pool DescriptorPool) {
	fnDestroyDescriptorPool(dev, pool, 0)
}

// AllocateDescriptorSet wraps vkAllocateDescriptorSets for a single set.
func AllocateDescriptorSet(dev Device, pool DescriptorPool, layout DescriptorSetLayout) (DescriptorSet, error) {
	info := DescriptorSetAllocateInfo{
		SType:              StructureTypeDescriptorSetAllocateInfo,
		DescriptorPool_:    pool,
		DescriptorSetCount: 1,
		PSetLayouts:        uintptr(unsafe.Pointer(&layout)),
	}
	var set DescriptorSet
	r := fnAllocateDescriptorSets(dev, uintptr(unsafe.Pointer(&info)), &set)
	if r != Success {
		return 0, fmt.Errorf("vkAllocateDescriptorSets: %w", r)
	}
	return set, nil
}

// UpdateDescriptorSets wraps vkUpdateDescriptorSets.
func UpdateDescriptorSets(dev Device, writes []WriteDescriptorSet) {
	if len(writes) == 0 {
		return
	}
	fnUpdateDescriptorSets(dev, uint32(len(writes)), uintptr(unsafe.Pointer(&writes[0])), 0, 0)
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

	// Load vkGetInstanceProcAddr first (needed for KHR extensions later).
	if ferr := must(&fnGetInstanceProcAddr, "vkGetInstanceProcAddr"); ferr != nil {
		return ferr
	}

	// Instance-level functions.
	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnEnumerateInstanceExtensionProperties, "vkEnumerateInstanceExtensionProperties"},
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
		{&fnCmdPushConstants, "vkCmdPushConstants"},
	} {
		if ferr := must(e.fn, e.name); ferr != nil {
			return ferr
		}
	}

	return nil
}

// InitSwapchainFunctions loads KHR surface and swapchain extension functions
// via vkGetInstanceProcAddr. Call after vkCreateInstance. Functions that are
// not available (e.g. platform-specific surface creators) are silently skipped.
func InitSwapchainFunctions(instance Instance) error {
	if fnGetInstanceProcAddr == nil {
		return fmt.Errorf("vk: vkGetInstanceProcAddr not loaded")
	}

	resolve := func(fn interface{}, name string) error {
		cname := CStr(name)
		addr := fnGetInstanceProcAddr(instance, uintptr(unsafe.Pointer(cname)))
		runtime.KeepAlive(cname)
		if addr == 0 {
			return fmt.Errorf("vk: %s not available", name)
		}
		purego.RegisterFunc(fn, addr)
		return nil
	}

	// Required surface + swapchain functions.
	for _, e := range []struct {
		fn   interface{}
		name string
	}{
		{&fnDestroySurfaceKHR, "vkDestroySurfaceKHR"},
		{&fnGetPhysicalDeviceSurfaceSupportKHR, "vkGetPhysicalDeviceSurfaceSupportKHR"},
		{&fnGetPhysicalDeviceSurfaceCapabilitiesKHR, "vkGetPhysicalDeviceSurfaceCapabilitiesKHR"},
		{&fnGetPhysicalDeviceSurfaceFormatsKHR, "vkGetPhysicalDeviceSurfaceFormatsKHR"},
		{&fnGetPhysicalDeviceSurfacePresentModesKHR, "vkGetPhysicalDeviceSurfacePresentModesKHR"},
		{&fnCreateSwapchainKHR, "vkCreateSwapchainKHR"},
		{&fnDestroySwapchainKHR, "vkDestroySwapchainKHR"},
		{&fnGetSwapchainImagesKHR, "vkGetSwapchainImagesKHR"},
		{&fnAcquireNextImageKHR, "vkAcquireNextImageKHR"},
		{&fnQueuePresentKHR, "vkQueuePresentKHR"},
	} {
		if err := resolve(e.fn, e.name); err != nil {
			return err
		}
	}

	// Platform surface creators — optional, only one will succeed per platform.
	_ = resolve(&fnCreateMetalSurfaceEXT, "vkCreateMetalSurfaceEXT")
	_ = resolve(&fnCreateWin32SurfaceKHR, "vkCreateWin32SurfaceKHR")
	_ = resolve(&fnCreateXlibSurfaceKHR, "vkCreateXlibSurfaceKHR")

	return nil
}

// ---------------------------------------------------------------------------
// KHR public wrappers
// ---------------------------------------------------------------------------

// DestroySurfaceKHR wraps vkDestroySurfaceKHR.
func DestroySurfaceKHR(instance Instance, surface SurfaceKHR) {
	fnDestroySurfaceKHR(instance, surface, 0)
}

// GetPhysicalDeviceSurfaceSupportKHR checks if a queue family supports presentation.
func GetPhysicalDeviceSurfaceSupportKHR(physDev PhysicalDevice, queueFamily uint32, surface SurfaceKHR) (bool, error) {
	var supported uint32
	r := fnGetPhysicalDeviceSurfaceSupportKHR(physDev, queueFamily, surface, &supported)
	if r != Success {
		return false, fmt.Errorf("vkGetPhysicalDeviceSurfaceSupportKHR: %w", r)
	}
	return supported != 0, nil
}

// GetPhysicalDeviceSurfaceCapabilitiesKHR queries surface capabilities.
func GetPhysicalDeviceSurfaceCapabilitiesKHR(physDev PhysicalDevice, surface SurfaceKHR) (SurfaceCapabilitiesKHR, error) {
	var caps SurfaceCapabilitiesKHR
	r := fnGetPhysicalDeviceSurfaceCapabilitiesKHR(physDev, surface, uintptr(unsafe.Pointer(&caps)))
	if r != Success {
		return caps, fmt.Errorf("vkGetPhysicalDeviceSurfaceCapabilitiesKHR: %w", r)
	}
	return caps, nil
}

// GetPhysicalDeviceSurfaceFormatsKHR queries supported surface formats.
func GetPhysicalDeviceSurfaceFormatsKHR(physDev PhysicalDevice, surface SurfaceKHR) ([]SurfaceFormatKHR, error) {
	var count uint32
	r := fnGetPhysicalDeviceSurfaceFormatsKHR(physDev, surface, &count, 0)
	if r != Success {
		return nil, fmt.Errorf("vkGetPhysicalDeviceSurfaceFormatsKHR (count): %w", r)
	}
	if count == 0 {
		return nil, nil
	}
	formats := make([]SurfaceFormatKHR, count)
	r = fnGetPhysicalDeviceSurfaceFormatsKHR(physDev, surface, &count, uintptr(unsafe.Pointer(&formats[0])))
	if r != Success {
		return nil, fmt.Errorf("vkGetPhysicalDeviceSurfaceFormatsKHR: %w", r)
	}
	return formats[:count], nil
}

// GetPhysicalDeviceSurfacePresentModesKHR queries supported present modes.
func GetPhysicalDeviceSurfacePresentModesKHR(physDev PhysicalDevice, surface SurfaceKHR) ([]uint32, error) {
	var count uint32
	r := fnGetPhysicalDeviceSurfacePresentModesKHR(physDev, surface, &count, 0)
	if r != Success {
		return nil, fmt.Errorf("vkGetPhysicalDeviceSurfacePresentModesKHR (count): %w", r)
	}
	if count == 0 {
		return nil, nil
	}
	modes := make([]uint32, count)
	r = fnGetPhysicalDeviceSurfacePresentModesKHR(physDev, surface, &count, uintptr(unsafe.Pointer(&modes[0])))
	if r != Success {
		return nil, fmt.Errorf("vkGetPhysicalDeviceSurfacePresentModesKHR: %w", r)
	}
	return modes[:count], nil
}

// CreateSwapchainKHR wraps vkCreateSwapchainKHR.
func CreateSwapchainKHR(device Device, info *SwapchainCreateInfoKHR) (SwapchainKHR, error) {
	var sc SwapchainKHR
	r := fnCreateSwapchainKHR(device, uintptr(unsafe.Pointer(info)), 0, &sc)
	if r != Success {
		return 0, fmt.Errorf("vkCreateSwapchainKHR: %w", r)
	}
	return sc, nil
}

// DestroySwapchainKHR wraps vkDestroySwapchainKHR.
func DestroySwapchainKHR(device Device, swapchain SwapchainKHR) {
	fnDestroySwapchainKHR(device, swapchain, 0)
}

// GetSwapchainImagesKHR retrieves swapchain images.
func GetSwapchainImagesKHR(device Device, swapchain SwapchainKHR) ([]Image, error) {
	var count uint32
	r := fnGetSwapchainImagesKHR(device, swapchain, &count, 0)
	if r != Success {
		return nil, fmt.Errorf("vkGetSwapchainImagesKHR (count): %w", r)
	}
	if count == 0 {
		return nil, nil
	}
	images := make([]Image, count)
	r = fnGetSwapchainImagesKHR(device, swapchain, &count, uintptr(unsafe.Pointer(&images[0])))
	if r != Success {
		return nil, fmt.Errorf("vkGetSwapchainImagesKHR: %w", r)
	}
	return images[:count], nil
}

// AcquireNextImageKHR acquires the next swapchain image.
func AcquireNextImageKHR(device Device, swapchain SwapchainKHR, timeout uint64, semaphore Semaphore, fence Fence) (uint32, Result) {
	var idx uint32
	r := fnAcquireNextImageKHR(device, swapchain, timeout, semaphore, fence, &idx)
	return idx, r
}

// QueuePresentKHR presents a rendered image to the swapchain.
func QueuePresentKHR(queue Queue, info *PresentInfoKHR) Result {
	return fnQueuePresentKHR(queue, uintptr(unsafe.Pointer(info)))
}

// CreateMetalSurfaceEXT creates a Vulkan surface from a CAMetalLayer (macOS).
func CreateMetalSurfaceEXT(instance Instance, info *MetalSurfaceCreateInfoEXT) (SurfaceKHR, error) {
	if fnCreateMetalSurfaceEXT == nil {
		return 0, fmt.Errorf("vk: vkCreateMetalSurfaceEXT not available")
	}
	var surface SurfaceKHR
	r := fnCreateMetalSurfaceEXT(instance, uintptr(unsafe.Pointer(info)), 0, &surface)
	if r != Success {
		return 0, fmt.Errorf("vkCreateMetalSurfaceEXT: %w", r)
	}
	return surface, nil
}

// CreateWin32SurfaceKHR creates a Vulkan surface from a Win32 window.
func CreateWin32SurfaceKHR(instance Instance, info *Win32SurfaceCreateInfoKHR) (SurfaceKHR, error) {
	if fnCreateWin32SurfaceKHR == nil {
		return 0, fmt.Errorf("vk: vkCreateWin32SurfaceKHR not available")
	}
	var surface SurfaceKHR
	r := fnCreateWin32SurfaceKHR(instance, uintptr(unsafe.Pointer(info)), 0, &surface)
	if r != Success {
		return 0, fmt.Errorf("vkCreateWin32SurfaceKHR: %w", r)
	}
	return surface, nil
}

// CreateXlibSurfaceKHR creates a Vulkan surface from an X11 window.
func CreateXlibSurfaceKHR(instance Instance, info *XlibSurfaceCreateInfoKHR) (SurfaceKHR, error) {
	if fnCreateXlibSurfaceKHR == nil {
		return 0, fmt.Errorf("vk: vkCreateXlibSurfaceKHR not available")
	}
	var surface SurfaceKHR
	r := fnCreateXlibSurfaceKHR(instance, uintptr(unsafe.Pointer(info)), 0, &surface)
	if r != Success {
		return 0, fmt.Errorf("vkCreateXlibSurfaceKHR: %w", r)
	}
	return surface, nil
}

// openVulkanLib opens the platform-specific Vulkan shared library.
func openVulkanLib() (uintptr, error) {
	var names []string
	switch runtime.GOOS {
	case "darwin":
		names = []string{
			"/opt/homebrew/lib/libMoltenVK.dylib",
			"/usr/local/lib/libMoltenVK.dylib",
			"libMoltenVK.dylib",
			"libvulkan.1.dylib",
			"libvulkan.dylib",
		}
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
