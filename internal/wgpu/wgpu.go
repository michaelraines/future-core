//go:build (darwin || linux || freebsd || windows) && !soft

// Package wgpu provides pure Go WebGPU bindings loaded at runtime via purego
// against wgpu-native (libwgpu_native). No CGo is required. The shared library
// must be available at runtime.
package wgpu

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/michaelraines/future-core/internal/dlopen"
)

// ---------------------------------------------------------------------------
// Handle types (opaque pointers)
// ---------------------------------------------------------------------------

type (
	Instance          uintptr
	Adapter           uintptr
	Device            uintptr
	Queue             uintptr
	Surface           uintptr
	SwapChain         uintptr
	Texture           uintptr
	TextureView       uintptr
	Sampler           uintptr
	Buffer            uintptr
	ShaderModule      uintptr
	BindGroupLayout   uintptr
	BindGroup         uintptr
	PipelineLayout    uintptr
	RenderPipeline    uintptr
	ComputePipeline   uintptr
	CommandEncoder    uintptr
	RenderPassEncoder uintptr
	CommandBuffer     uintptr
	QuerySet          uintptr
)

// ---------------------------------------------------------------------------
// Enum types
// ---------------------------------------------------------------------------

// TextureFormat mirrors WGPUTextureFormat.
type TextureFormat uint32

// wgpu-native v27 texture format enum values (from
// /opt/homebrew/include/webgpu.h). The WebGPU spec shifted these in
// v27 so every named enum has an explicit `Undefined = 0` sentinel;
// always source new constants from the installed header rather than
// from a spec draft. Historical note: RGBA16Float / RGBA32Float /
// BGRA8Unorm used to be listed as 33 / 36 / 24 here — all off-by-one
// because the author of the bindings was reading a pre-v27 spec. The
// BGRA8Unorm bug in particular silently selected BGRA8UnormSrgb
// (=0x18=24) on macOS surfaces and produced a visible double-gamma
// encoding (scene-selector background noticeably brighter than the
// WebGPU browser and Vulkan native backends).
const (
	TextureFormatR8Unorm        TextureFormat = 0x01
	TextureFormatRGBA8Unorm     TextureFormat = 0x12 // 18
	TextureFormatRGBA8UnormSrgb TextureFormat = 0x13 // 19
	TextureFormatBGRA8Unorm     TextureFormat = 0x17 // 23
	TextureFormatBGRA8UnormSrgb TextureFormat = 0x18 // 24
	TextureFormatRGBA16Float    TextureFormat = 0x22 // 34
	TextureFormatRGBA32Float    TextureFormat = 0x23 // 35
	TextureFormatDepth24Plus    TextureFormat = 0x28 // 40
	TextureFormatDepth32Float   TextureFormat = 0x2A // 42
)

// TextureUsage mirrors WGPUTextureUsage flags (WGPUFlags = uint64).
type TextureUsage uint64

const (
	TextureUsageCopySrc          TextureUsage = 0x01
	TextureUsageCopyDst          TextureUsage = 0x02
	TextureUsageTextureBinding   TextureUsage = 0x04
	TextureUsageStorageBinding   TextureUsage = 0x08
	TextureUsageRenderAttachment TextureUsage = 0x10
)

// BufferUsage mirrors WGPUBufferUsage flags (WGPUFlags = uint64).
type BufferUsage uint64

const (
	BufferUsageMapRead  BufferUsage = 0x0001
	BufferUsageMapWrite BufferUsage = 0x0002
	BufferUsageCopySrc  BufferUsage = 0x0004
	BufferUsageCopyDst  BufferUsage = 0x0008
	BufferUsageIndex    BufferUsage = 0x0010
	BufferUsageVertex   BufferUsage = 0x0020
	BufferUsageUniform  BufferUsage = 0x0040
	BufferUsageStorage  BufferUsage = 0x0080
)

// LoadOp mirrors WGPULoadOp.
type LoadOp uint32

const (
	LoadOpLoad  LoadOp = 1
	LoadOpClear LoadOp = 2
)

// StoreOp mirrors WGPUStoreOp.
type StoreOp uint32

const (
	StoreOpStore   StoreOp = 1
	StoreOpDiscard StoreOp = 2
)

// BlendFactor mirrors WGPUBlendFactor.
type BlendFactor uint32

const (
	BlendFactorZero             BlendFactor = 1
	BlendFactorOne              BlendFactor = 2
	BlendFactorSrc              BlendFactor = 3
	BlendFactorOneMinusSrc      BlendFactor = 4
	BlendFactorSrcAlpha         BlendFactor = 5
	BlendFactorOneMinusSrcAlpha BlendFactor = 6
	BlendFactorDst              BlendFactor = 7
	BlendFactorOneMinusDst      BlendFactor = 8
	BlendFactorDstAlpha         BlendFactor = 9
	BlendFactorOneMinusDstAlpha BlendFactor = 10
)

// BlendOperation mirrors WGPUBlendOperation.
type BlendOperation uint32

const (
	BlendOperationAdd             BlendOperation = 1
	BlendOperationSubtract        BlendOperation = 2
	BlendOperationReverseSubtract BlendOperation = 3
	BlendOperationMin             BlendOperation = 4
	BlendOperationMax             BlendOperation = 5
)

// IndexFormat mirrors WGPUIndexFormat.
type IndexFormat uint32

const (
	IndexFormatUint16 IndexFormat = 1
	IndexFormatUint32 IndexFormat = 2
)

// PrimitiveTopology mirrors WGPUPrimitiveTopology.
type PrimitiveTopology uint32

const (
	PrimitiveTopologyPointList     PrimitiveTopology = 1
	PrimitiveTopologyLineList      PrimitiveTopology = 2
	PrimitiveTopologyLineStrip     PrimitiveTopology = 3
	PrimitiveTopologyTriangleList  PrimitiveTopology = 4
	PrimitiveTopologyTriangleStrip PrimitiveTopology = 5
)

// VertexFormat mirrors WGPUVertexFormat.
type VertexFormat uint32

const (
	VertexFormatUnorm8x4  VertexFormat = 0x09 // WGPUVertexFormat_Unorm8x4
	VertexFormatFloat32x2 VertexFormat = 0x1D // WGPUVertexFormat_Float32x2
	VertexFormatFloat32x3 VertexFormat = 0x1E // WGPUVertexFormat_Float32x3
	VertexFormatFloat32x4 VertexFormat = 0x1F // WGPUVertexFormat_Float32x4
	VertexFormatUint8x4   VertexFormat = 0x03 // WGPUVertexFormat_Uint8x4
)

// VertexStepMode mirrors WGPUVertexStepMode.
type VertexStepMode uint32

const (
	VertexStepModeVertex   VertexStepMode = 2
	VertexStepModeInstance VertexStepMode = 3
)

// CullMode mirrors WGPUCullMode.
type CullMode uint32

const (
	CullModeNone  CullMode = 1
	CullModeFront CullMode = 2
	CullModeBack  CullMode = 3
)

// FrontFace mirrors WGPUFrontFace.
type FrontFace uint32

const (
	FrontFaceCCW FrontFace = 1
	FrontFaceCW  FrontFace = 2
)

// ColorWriteMask mirrors WGPUColorWriteMask (WGPUFlags = uint64).
type ColorWriteMask uint64

const (
	ColorWriteMaskAll ColorWriteMask = 0xF
)

// CompareFunction mirrors WGPUCompareFunction.
type CompareFunction uint32

const (
	CompareFunctionNever        CompareFunction = 1
	CompareFunctionLess         CompareFunction = 2
	CompareFunctionLessEqual    CompareFunction = 3
	CompareFunctionEqual        CompareFunction = 4
	CompareFunctionGreaterEqual CompareFunction = 5
	CompareFunctionGreater      CompareFunction = 6
	CompareFunctionNotEqual     CompareFunction = 7
	CompareFunctionAlways       CompareFunction = 8
)

// StencilOperation mirrors WGPUStencilOperation.
type StencilOperation uint32

const (
	StencilOperationKeep           StencilOperation = 0
	StencilOperationZero           StencilOperation = 1
	StencilOperationReplace        StencilOperation = 2
	StencilOperationInvert         StencilOperation = 3
	StencilOperationIncrementClamp StencilOperation = 4
	StencilOperationDecrementClamp StencilOperation = 5
	StencilOperationIncrementWrap  StencilOperation = 6
	StencilOperationDecrementWrap  StencilOperation = 7
)

// AddressMode mirrors WGPUAddressMode.
type AddressMode uint32

const (
	AddressModeRepeat       AddressMode = 0
	AddressModeMirrorRepeat AddressMode = 1
	AddressModeClampToEdge  AddressMode = 2
)

// FilterMode mirrors WGPUFilterMode.
type FilterMode uint32

const (
	FilterModeNearest FilterMode = 1
	FilterModeLinear  FilterMode = 2
)

// RenderPassDepthStencilAttachment is WGPURenderPassDepthStencilAttachment.
type RenderPassDepthStencilAttachment struct {
	View              TextureView
	DepthLoadOp       LoadOp
	DepthStoreOp      StoreOp
	DepthClearValue   float32
	DepthReadOnly     uint32
	StencilLoadOp     LoadOp
	StencilStoreOp    StoreOp
	StencilClearValue uint32
	StencilReadOnly   uint32
}

// StringView is WGPUStringView (data pointer + length).
type StringView struct {
	Data   uintptr
	Length uintptr
}

// MakeStringView creates a StringView from a Go string. The caller must keep
// the byte slice alive (use runtime.KeepAlive) until the C call completes.
func MakeStringView(s string) (StringView, []byte) {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return StringView{
		Data:   uintptr(unsafe.Pointer(&b[0])),
		Length: uintptr(len(s)),
	}, b
}

// NullStringView returns a StringView with WGPU_STRING_VIEW_INIT (null data, 0 length).
func NullStringView() StringView {
	return StringView{}
}

// CallbackMode mirrors WGPUCallbackMode.
type CallbackMode uint32

const (
	CallbackModeWaitAnyOnly        CallbackMode = 1
	CallbackModeAllowProcessEvents CallbackMode = 2
	CallbackModeAllowSpontaneous   CallbackMode = 3
)

// RequestAdapterStatus mirrors WGPURequestAdapterStatus.
const RequestAdapterStatusSuccess uint32 = 1

// RequestDeviceStatus mirrors WGPURequestDeviceStatus.
const RequestDeviceStatusSuccess uint32 = 1

// MapAsyncStatusSuccess mirrors WGPUMapAsyncStatus_Success.
const MapAsyncStatusSuccess uint32 = 1

// Future is WGPUFuture.
type Future struct {
	ID uint64
}

// RequestAdapterCallbackInfo is WGPURequestAdapterCallbackInfo.
type RequestAdapterCallbackInfo struct {
	NextInChain uintptr
	Mode        CallbackMode
	_           [4]byte // padding to align Callback
	Callback    uintptr
	Userdata1   uintptr
	Userdata2   uintptr
}

// RequestDeviceCallbackInfo is WGPURequestDeviceCallbackInfo.
type RequestDeviceCallbackInfo struct {
	NextInChain uintptr
	Mode        CallbackMode
	_           [4]byte // padding to align Callback
	Callback    uintptr
	Userdata1   uintptr
	Userdata2   uintptr
}

// BufferMapCallbackInfo is WGPUBufferMapCallbackInfo.
type BufferMapCallbackInfo struct {
	NextInChain uintptr
	Mode        CallbackMode
	_           [4]byte // padding to align Callback
	Callback    uintptr
	Userdata1   uintptr
	Userdata2   uintptr
}

// SamplerDescriptor is WGPUSamplerDescriptor.
type SamplerDescriptor struct {
	NextInChain   uintptr
	Label         StringView
	AddressModeU  AddressMode
	AddressModeV  AddressMode
	AddressModeW  AddressMode
	MagFilter     FilterMode
	MinFilter     FilterMode
	MipmapFilter  FilterMode
	LodMinClamp   float32
	LodMaxClamp   float32
	Compare       CompareFunction
	MaxAnisotropy uint16
	_             [2]byte // padding
}

// ---------------------------------------------------------------------------
// Pipeline creation structs (C-compatible layout)
// ---------------------------------------------------------------------------

// ShaderModuleWGSLDescriptor is the WGSL chained struct (WGPUShaderSourceWGSL) for shader creation.
type ShaderModuleWGSLDescriptor struct {
	Chain SChainedStruct
	Code  StringView
}

// SChainedStruct is the chained struct header.
type SChainedStruct struct {
	Next  uintptr
	SType uint32
	_     [4]byte // padding
}

// ShaderModuleDescriptor is WGPUShaderModuleDescriptor.
type ShaderModuleDescriptor struct {
	NextInChain uintptr
	Label       StringView
}

// VertexAttribute is WGPUVertexAttribute.
type VertexAttribute struct {
	Format         VertexFormat
	_              [4]byte // padding
	Offset         uint64
	ShaderLocation uint32
	_              [4]byte // padding
}

// VertexBufferLayout is WGPUVertexBufferLayout.
type VertexBufferLayout struct {
	StepMode       VertexStepMode
	_              [4]byte // padding
	ArrayStride    uint64
	AttributeCount uintptr
	Attributes     uintptr
}

// VertexState is WGPUVertexState.
type VertexState struct {
	NextInChain   uintptr
	Module        ShaderModule
	EntryPoint    StringView
	ConstantCount uintptr
	Constants     uintptr
	BufferCount   uintptr
	Buffers       uintptr
}

// FragmentState is WGPUFragmentState.
type FragmentState struct {
	NextInChain   uintptr
	Module        ShaderModule
	EntryPoint    StringView
	ConstantCount uintptr
	Constants     uintptr
	TargetCount   uintptr
	Targets       uintptr
}

// ColorTargetState is WGPUColorTargetState.
type ColorTargetState struct {
	NextInChain uintptr
	Format      TextureFormat
	_           [4]byte        // padding to align Blend ptr
	Blend       uintptr        // *BlendState, 0 for no blending
	WriteMask   ColorWriteMask // uint64
}

// BlendState is WGPUBlendState.
type BlendState struct {
	Color BlendComponent
	Alpha BlendComponent
}

// BlendComponent is WGPUBlendComponent.
type BlendComponent struct {
	Operation BlendOperation
	SrcFactor BlendFactor
	DstFactor BlendFactor
}

// PrimitiveState is WGPUPrimitiveState.
type PrimitiveState struct {
	NextInChain      uintptr
	Topology         PrimitiveTopology
	StripIndexFormat IndexFormat
	FrontFace_       FrontFace
	CullMode_        CullMode
	UnclippedDepth   uint32
	_                [4]byte // padding
}

// MultisampleState is WGPUMultisampleState.
type MultisampleState struct {
	NextInChain            uintptr
	Count                  uint32
	Mask                   uint32
	AlphaToCoverageEnabled uint32
	_                      [4]byte // padding
}

// DepthStencilState is WGPUDepthStencilState.
type DepthStencilState struct {
	NextInChain         uintptr
	Format              TextureFormat
	DepthWriteEnabled   uint32
	DepthCompare        CompareFunction
	StencilFront        StencilFaceState
	StencilBack         StencilFaceState
	StencilReadMask     uint32
	StencilWriteMask    uint32
	DepthBias           int32
	DepthBiasSlopeScale float32
	DepthBiasClamp      float32
	_                   [4]byte
}

// StencilFaceState is WGPUStencilFaceState.
type StencilFaceState struct {
	Compare     CompareFunction
	FailOp      uint32
	DepthFailOp uint32
	PassOp      uint32
}

// RenderPipelineDescriptor is WGPURenderPipelineDescriptor.
type RenderPipelineDescriptor struct {
	NextInChain  uintptr
	Label        StringView
	Layout       PipelineLayout
	Vertex       VertexState
	Primitive    PrimitiveState
	DepthStencil uintptr // *DepthStencilState
	Multisample  MultisampleState
	Fragment     uintptr // *FragmentState
}

// BindGroupLayoutEntry is WGPUBindGroupLayoutEntry.
type BindGroupLayoutEntry struct {
	NextInChain    uintptr
	Binding        uint32
	_              [4]byte // padding
	Visibility     uint64  // WGPUShaderStage (WGPUFlags = uint64)
	Buffer_        BindGroupLayoutEntryBuffer
	Sampler_       BindGroupLayoutEntrySampler
	Texture_       BindGroupLayoutEntryTexture
	StorageTexture BindGroupLayoutEntryStorageTexture
}

// BindGroupLayoutEntryBuffer is the buffer part of a layout entry.
type BindGroupLayoutEntryBuffer struct {
	NextInChain      uintptr
	Type             uint32
	HasDynamicOffset uint32
	MinBindingSize   uint64
}

// BindGroupLayoutEntrySampler is the sampler part of a layout entry.
type BindGroupLayoutEntrySampler struct {
	NextInChain uintptr
	Type        uint32
	_           [4]byte
}

// BindGroupLayoutEntryTexture is the texture part of a layout entry.
type BindGroupLayoutEntryTexture struct {
	NextInChain   uintptr
	SampleType    uint32
	ViewDimension uint32
	Multisampled  uint32
	_             [4]byte
}

// BindGroupLayoutEntryStorageTexture is the storage texture part of a layout entry.
type BindGroupLayoutEntryStorageTexture struct {
	NextInChain   uintptr
	Access        uint32
	Format        TextureFormat
	ViewDimension uint32
	_             [4]byte
}

// BindGroupLayoutDescriptor is WGPUBindGroupLayoutDescriptor.
type BindGroupLayoutDescriptor struct {
	NextInChain uintptr
	Label       StringView
	EntryCount  uintptr
	Entries     uintptr
}

// BindGroupEntry is WGPUBindGroupEntry.
type BindGroupEntry struct {
	NextInChain  uintptr
	Binding      uint32
	_            [4]byte
	Buffer_      Buffer
	Offset       uint64
	Size         uint64
	Sampler_     Sampler
	TextureView_ TextureView
}

// BindGroupDescriptor is WGPUBindGroupDescriptor.
type BindGroupDescriptor struct {
	NextInChain uintptr
	Label       StringView
	Layout      BindGroupLayout
	EntryCount  uintptr
	Entries     uintptr
}

// PipelineLayoutDescriptor is WGPUPipelineLayoutDescriptor.
type PipelineLayoutDescriptor struct {
	NextInChain          uintptr
	Label                StringView
	BindGroupLayoutCount uintptr
	BindGroupLayouts     uintptr
}

// SType constants for chained structs.
const (
	STypeShaderModuleWGSLDescriptor uint32 = 2 // WGPUSType_ShaderSourceWGSL
)

// ---------------------------------------------------------------------------
// Struct types (C-compatible layout)
// ---------------------------------------------------------------------------

// Color is WGPUColor.
type Color struct {
	R, G, B, A float64
}

// Extent3D is WGPUExtent3D.
type Extent3D struct {
	Width, Height, DepthOrArrayLayers uint32
}

// Origin3D is WGPUOrigin3D.
type Origin3D struct {
	X, Y, Z uint32
}

// TextureDescriptor is WGPUTextureDescriptor.
type TextureDescriptor struct {
	NextInChain     uintptr
	Label           StringView
	Usage           TextureUsage // uint64
	Dimension       uint32
	Size            Extent3D // 12 bytes, starts at offset 36
	Format          TextureFormat
	MipLevelCount   uint32
	SampleCount     uint32
	_               [4]byte // padding to align ViewFormatCount to 8
	ViewFormatCount uintptr
	ViewFormats     uintptr
}

// BufferDescriptor is WGPUBufferDescriptor.
type BufferDescriptor struct {
	NextInChain      uintptr
	Label            StringView
	Usage            BufferUsage // uint64
	Size             uint64
	MappedAtCreation uint32
	_                [4]byte // padding
}

// TexelCopyTextureInfo is WGPUTexelCopyTextureInfo (formerly ImageCopyTexture).
type TexelCopyTextureInfo struct {
	Texture_ Texture
	MipLevel uint32
	Origin   Origin3D
	Aspect   uint32
}

// TexelCopyBufferInfo is WGPUTexelCopyBufferInfo (formerly ImageCopyBuffer).
type TexelCopyBufferInfo struct {
	Layout  TexelCopyBufferLayout
	Buffer_ Buffer
}

// TexelCopyBufferLayout is WGPUTexelCopyBufferLayout (formerly TextureDataLayout).
type TexelCopyBufferLayout struct {
	Offset       uint64
	BytesPerRow  uint32
	RowsPerImage uint32
}

// Aliases for backward compatibility within this package.
type ImageCopyTexture = TexelCopyTextureInfo
type ImageCopyBuffer = TexelCopyBufferInfo
type TextureDataLayout = TexelCopyBufferLayout

// RenderPassColorAttachment is WGPURenderPassColorAttachment.
type RenderPassColorAttachment struct {
	NextInChain   uintptr
	View          TextureView
	DepthSlice    uint32
	ResolveTarget TextureView
	LoadOp_       LoadOp
	StoreOp_      StoreOp
	ClearValue    Color
}

// PresentMode mirrors WGPUPresentMode.
type PresentMode uint32

const (
	PresentModeFifo    PresentMode = 0
	PresentModeMailbox PresentMode = 2
)

// CompositeAlphaMode mirrors WGPUCompositeAlphaMode.
type CompositeAlphaMode uint32

const (
	CompositeAlphaModeAuto CompositeAlphaMode = 0
)

// SurfaceConfiguration is WGPUSurfaceConfiguration.
type SurfaceConfiguration struct {
	NextInChain     uintptr
	Device          Device
	Format          TextureFormat
	_               [4]byte      // padding after format (uint32) to align usage (uint64)
	Usage           TextureUsage // uint64
	Width           uint32
	Height          uint32
	ViewFormatCount uintptr
	ViewFormats     uintptr
	AlphaMode       CompositeAlphaMode
	PresentMode     PresentMode
}

// SurfaceTexture is WGPUSurfaceTexture (returned by GetCurrentTexture).
type SurfaceTexture struct {
	NextInChain uintptr
	Texture_    Texture
	Status      uint32
	_           [4]byte // padding
}

// SurfaceCapabilities is WGPUSurfaceCapabilities.
type SurfaceCapabilities struct {
	NextInChain      uintptr
	Usages           TextureUsage // uint64
	FormatCount      uintptr
	Formats          uintptr
	PresentModeCount uintptr
	PresentModes     uintptr
	AlphaModeCount   uintptr
	AlphaModes       uintptr
}

// SurfaceDescriptorFromMetalLayer for creating a surface from a CAMetalLayer.
type SurfaceDescriptorFromMetalLayer struct {
	Chain SChainedStruct
	Layer uintptr
}

// SurfaceDescriptorFromWindowsHWND for creating a surface from an HWND.
type SurfaceDescriptorFromWindowsHWND struct {
	Chain     SChainedStruct
	Hinstance uintptr
	Hwnd      uintptr
}

// SurfaceDescriptorFromXlibWindow for creating a surface from an X11 window.
type SurfaceDescriptorFromXlibWindow struct {
	Chain   SChainedStruct
	Display uintptr
	Window  uint64
}

// SurfaceDescriptor is WGPUSurfaceDescriptor.
type SurfaceDescriptor struct {
	NextInChain uintptr
	Label       StringView
}

// SType constants for surface chained structs.
const (
	STypeSurfaceDescriptorFromMetalLayer  uint32 = 4 // WGPUSType_SurfaceSourceMetalLayer
	STypeSurfaceDescriptorFromWindowsHWND uint32 = 5 // WGPUSType_SurfaceSourceWindowsHWND
	STypeSurfaceDescriptorFromXlibWindow  uint32 = 6 // WGPUSType_SurfaceSourceXlibWindow
)

// RenderPassDescriptor is WGPURenderPassDescriptor.
type RenderPassDescriptor struct {
	NextInChain            uintptr
	Label                  StringView
	ColorAttachmentCount   uintptr
	ColorAttachments       uintptr
	DepthStencilAttachment uintptr
	OcclusionQuerySet      QuerySet
	TimestampWrites        uintptr
}

// ---------------------------------------------------------------------------
// Private function variables (populated by Init)
// ---------------------------------------------------------------------------

var (
	fnCreateInstance func(uintptr) Instance
	// fnInstanceRequestAdapter and fnAdapterRequestDevice use SyscallN
	// because they take callback-info structs by value and return WGPUFuture.
	symInstanceRequestAdapter          uintptr
	symAdapterRequestDevice            uintptr
	symInstanceWaitAny                 uintptr
	fnDeviceGetQueue                   func(Device) Queue
	fnDeviceCreateTexture              func(Device, *TextureDescriptor) Texture
	fnDeviceCreateBuffer               func(Device, *BufferDescriptor) Buffer
	fnDeviceCreateShaderModule         func(Device, uintptr) ShaderModule
	fnDeviceCreateRenderPipeline       func(Device, uintptr) RenderPipeline
	fnDeviceCreateCommandEncoder       func(Device, uintptr) CommandEncoder
	fnTextureCreateView                func(Texture, uintptr) TextureView
	fnTextureGetWidth                  func(Texture) uint32
	fnTextureGetHeight                 func(Texture) uint32
	fnTextureDestroy                   func(Texture)
	fnTextureRelease                   func(Texture)
	fnTextureViewRelease               func(TextureView)
	fnBufferGetSize                    func(Buffer) uint64
	fnBufferDestroy                    func(Buffer)
	fnBufferRelease                    func(Buffer)
	fnShaderModuleRelease              func(ShaderModule)
	fnRenderPipelineRelease            func(RenderPipeline)
	fnQueueWriteBuffer                 func(Queue, Buffer, uint64, uintptr, uint64)
	fnQueueWriteTexture                func(Queue, *ImageCopyTexture, uintptr, uint64, *TextureDataLayout, *Extent3D)
	fnQueueSubmit                      func(Queue, uintptr, uintptr)
	fnCommandEncoderBeginRenderPass    func(CommandEncoder, *RenderPassDescriptor) RenderPassEncoder
	fnCommandEncoderFinish             func(CommandEncoder, uintptr) CommandBuffer
	fnCommandEncoderRelease            func(CommandEncoder)
	fnCommandBufferRelease             func(CommandBuffer)
	fnRenderPassEncoderSetPipeline     func(RenderPassEncoder, RenderPipeline)
	fnRenderPassEncoderSetVertexBuffer func(RenderPassEncoder, uint32, Buffer, uint64, uint64)
	fnRenderPassEncoderSetIndexBuffer  func(RenderPassEncoder, Buffer, IndexFormat, uint64, uint64)
	fnRenderPassEncoderSetViewport     func(RenderPassEncoder, float32, float32, float32, float32, float32, float32)
	fnRenderPassEncoderSetScissorRect  func(RenderPassEncoder, uint32, uint32, uint32, uint32)
	fnRenderPassEncoderDraw            func(RenderPassEncoder, uint32, uint32, uint32, uint32)
	fnRenderPassEncoderDrawIndexed     func(RenderPassEncoder, uint32, uint32, uint32, int32, uint32)
	fnRenderPassEncoderEnd             func(RenderPassEncoder)
	fnRenderPassEncoderRelease         func(RenderPassEncoder)
	fnInstanceRelease                  func(Instance)
	fnAdapterRelease                   func(Adapter)
	fnDeviceRelease                    func(Device)

	// Surface / presentation functions.
	fnInstanceCreateSurface    func(Instance, uintptr) Surface
	fnSurfaceConfigure         func(Surface, *SurfaceConfiguration)
	fnSurfaceGetCurrentTexture func(Surface, *SurfaceTexture)
	fnSurfacePresent           func(Surface)
	fnSurfaceUnconfigure       func(Surface)
	fnSurfaceRelease           func(Surface)
	fnSurfaceGetCapabilities   func(Surface, Adapter, *SurfaceCapabilities)

	// Pipeline / bind group / readback functions.
	fnDeviceCreateBindGroupLayout       func(Device, *BindGroupLayoutDescriptor) BindGroupLayout
	fnDeviceCreateBindGroup             func(Device, *BindGroupDescriptor) BindGroup
	fnDeviceCreatePipelineLayout        func(Device, *PipelineLayoutDescriptor) PipelineLayout
	fnCommandEncoderCopyTextureToBuffer func(CommandEncoder, *TexelCopyTextureInfo, *TexelCopyBufferInfo, *Extent3D)
	symBufferMapAsync                   uintptr
	fnBufferGetMappedRange              func(Buffer, uint64, uint64) uintptr
	fnBufferUnmap                       func(Buffer)
	fnBindGroupLayoutRelease            func(BindGroupLayout)
	fnBindGroupRelease                  func(BindGroup)
	fnPipelineLayoutRelease             func(PipelineLayout)
	fnRenderPassEncoderSetBindGroup     func(RenderPassEncoder, uint32, BindGroup, uint32, uintptr)
	fnDevicePoll                        func(Device, uint32, uintptr) uint32
	fnDeviceCreateSampler               func(Device, uintptr) Sampler
	fnSamplerRelease                    func(Sampler)
)

// ---------------------------------------------------------------------------
// Public wrapper functions
// ---------------------------------------------------------------------------

// CreateInstance creates a wgpu Instance.
func CreateInstance() Instance {
	return fnCreateInstance(0)
}

// DeviceGetQueue returns the default queue for a device.
func DeviceGetQueue(dev Device) Queue {
	return fnDeviceGetQueue(dev)
}

// DeviceCreateTexture creates a GPU texture.
func DeviceCreateTexture(dev Device, desc *TextureDescriptor) Texture {
	return fnDeviceCreateTexture(dev, desc)
}

// DeviceCreateBuffer creates a GPU buffer.
func DeviceCreateBuffer(dev Device, desc *BufferDescriptor) Buffer {
	return fnDeviceCreateBuffer(dev, desc)
}

// DeviceCreateCommandEncoder creates a command encoder.
func DeviceCreateCommandEncoder(dev Device) CommandEncoder {
	return fnDeviceCreateCommandEncoder(dev, 0)
}

// TextureCreateView creates a default texture view.
func TextureCreateView(tex Texture) TextureView {
	return fnTextureCreateView(tex, 0)
}

// TextureGetWidth returns the texture width.
func TextureGetWidth(tex Texture) uint32 {
	return fnTextureGetWidth(tex)
}

// TextureGetHeight returns the texture height.
func TextureGetHeight(tex Texture) uint32 {
	return fnTextureGetHeight(tex)
}

// TextureDestroy destroys a texture.
func TextureDestroy(tex Texture) {
	fnTextureDestroy(tex)
}

// TextureRelease releases a texture reference.
func TextureRelease(tex Texture) {
	fnTextureRelease(tex)
}

// TextureViewRelease releases a texture view reference.
func TextureViewRelease(view TextureView) {
	fnTextureViewRelease(view)
}

// BufferGetSize returns the buffer size.
func BufferGetSize(buf Buffer) uint64 {
	return fnBufferGetSize(buf)
}

// BufferDestroy destroys a buffer.
func BufferDestroy(buf Buffer) {
	fnBufferDestroy(buf)
}

// BufferRelease releases a buffer reference.
func BufferRelease(buf Buffer) {
	fnBufferRelease(buf)
}

// ShaderModuleRelease releases a shader module reference.
func ShaderModuleRelease(mod ShaderModule) {
	fnShaderModuleRelease(mod)
}

// RenderPipelineRelease releases a render pipeline reference.
func RenderPipelineRelease(pipe RenderPipeline) {
	fnRenderPipelineRelease(pipe)
}

// QueueWriteBuffer writes data to a buffer via the queue.
func QueueWriteBuffer(queue Queue, buf Buffer, offset uint64, data unsafe.Pointer, size uint64) {
	fnQueueWriteBuffer(queue, buf, offset, uintptr(data), size)
}

// QueueWriteTexture writes data to a texture via the queue.
func QueueWriteTexture(queue Queue, dst *ImageCopyTexture, data unsafe.Pointer, dataSize uint64, layout *TextureDataLayout, size *Extent3D) {
	fnQueueWriteTexture(queue, dst, uintptr(data), dataSize, layout, size)
}

// QueueSubmit submits command buffers to the queue.
func QueueSubmit(queue Queue, cmds []CommandBuffer) {
	if len(cmds) == 0 {
		return
	}
	fnQueueSubmit(queue, uintptr(len(cmds)), uintptr(unsafe.Pointer(&cmds[0])))
}

// CommandEncoderBeginRenderPass begins a render pass.
func CommandEncoderBeginRenderPass(enc CommandEncoder, desc *RenderPassDescriptor) RenderPassEncoder {
	return fnCommandEncoderBeginRenderPass(enc, desc)
}

// CommandEncoderFinish finishes encoding and returns a command buffer.
func CommandEncoderFinish(enc CommandEncoder) CommandBuffer {
	return fnCommandEncoderFinish(enc, 0)
}

// CommandEncoderRelease releases a command encoder.
func CommandEncoderRelease(enc CommandEncoder) {
	fnCommandEncoderRelease(enc)
}

// CommandBufferRelease releases a command buffer.
func CommandBufferRelease(buf CommandBuffer) {
	fnCommandBufferRelease(buf)
}

// RenderPassSetPipeline binds a render pipeline.
func RenderPassSetPipeline(rpe RenderPassEncoder, pipe RenderPipeline) {
	fnRenderPassEncoderSetPipeline(rpe, pipe)
}

// RenderPassSetVertexBuffer binds a vertex buffer.
func RenderPassSetVertexBuffer(rpe RenderPassEncoder, slot uint32, buf Buffer, offset, size uint64) {
	fnRenderPassEncoderSetVertexBuffer(rpe, slot, buf, offset, size)
}

// RenderPassSetIndexBuffer binds an index buffer.
func RenderPassSetIndexBuffer(rpe RenderPassEncoder, buf Buffer, format IndexFormat, offset, size uint64) {
	fnRenderPassEncoderSetIndexBuffer(rpe, buf, format, offset, size)
}

// RenderPassSetViewport sets the viewport.
func RenderPassSetViewport(rpe RenderPassEncoder, x, y, w, h, minDepth, maxDepth float32) {
	fnRenderPassEncoderSetViewport(rpe, x, y, w, h, minDepth, maxDepth)
}

// RenderPassSetScissorRect sets the scissor rectangle.
func RenderPassSetScissorRect(rpe RenderPassEncoder, x, y, w, h uint32) {
	fnRenderPassEncoderSetScissorRect(rpe, x, y, w, h)
}

// RenderPassDraw issues a draw call.
func RenderPassDraw(rpe RenderPassEncoder, vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	fnRenderPassEncoderDraw(rpe, vertexCount, instanceCount, firstVertex, firstInstance)
}

// RenderPassDrawIndexed issues an indexed draw call.
func RenderPassDrawIndexed(rpe RenderPassEncoder, indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	fnRenderPassEncoderDrawIndexed(rpe, indexCount, instanceCount, firstIndex, baseVertex, firstInstance)
}

// RenderPassEnd ends a render pass.
func RenderPassEnd(rpe RenderPassEncoder) {
	fnRenderPassEncoderEnd(rpe)
}

// RenderPassRelease releases a render pass encoder.
func RenderPassRelease(rpe RenderPassEncoder) {
	fnRenderPassEncoderRelease(rpe)
}

// DeviceCreateShaderModuleWGSL creates a shader module from WGSL source.
func DeviceCreateShaderModuleWGSL(dev Device, code string) ShaderModule {
	codeSV, codeKeep := MakeStringView(code)
	wgslDesc := ShaderModuleWGSLDescriptor{
		Chain: SChainedStruct{SType: STypeShaderModuleWGSLDescriptor},
		Code:  codeSV,
	}
	desc := ShaderModuleDescriptor{
		NextInChain: uintptr(unsafe.Pointer(&wgslDesc)),
	}
	ret := fnDeviceCreateShaderModule(dev, uintptr(unsafe.Pointer(&desc)))
	runtime.KeepAlive(codeKeep)
	runtime.KeepAlive(wgslDesc)
	return ret
}

// DeviceCreateRenderPipelineTyped creates a render pipeline from a typed descriptor.
func DeviceCreateRenderPipelineTyped(dev Device, desc *RenderPipelineDescriptor) RenderPipeline {
	return fnDeviceCreateRenderPipeline(dev, uintptr(unsafe.Pointer(desc)))
}

// DeviceCreateBindGroupLayout creates a bind group layout.
func DeviceCreateBindGroupLayout(dev Device, desc *BindGroupLayoutDescriptor) BindGroupLayout {
	return fnDeviceCreateBindGroupLayout(dev, desc)
}

// DeviceCreateBindGroup creates a bind group.
func DeviceCreateBindGroup(dev Device, desc *BindGroupDescriptor) BindGroup {
	return fnDeviceCreateBindGroup(dev, desc)
}

// DeviceCreatePipelineLayout creates a pipeline layout.
func DeviceCreatePipelineLayout(dev Device, desc *PipelineLayoutDescriptor) PipelineLayout {
	return fnDeviceCreatePipelineLayout(dev, desc)
}

// CommandEncoderCopyTextureToBuffer copies texture data to a buffer.
func CommandEncoderCopyTextureToBuffer(enc CommandEncoder, src *ImageCopyTexture, dst *ImageCopyBuffer, size *Extent3D) {
	fnCommandEncoderCopyTextureToBuffer(enc, src, dst, size)
}

// BufferMapAsync maps a buffer for reading.
// v27: wgpuBufferMapAsync(buffer, mode, offset, size, callbackInfo) -> WGPUFuture
func BufferMapAsync(buf Buffer, mode uint32, offset, size uint64) {
	// Use AllowProcessEvents mode so DevicePoll triggers the callback.
	cb := purego.NewCallback(func(status uint32, msgData uintptr, msgLen uintptr, userdata1 uintptr, userdata2 uintptr) {
	})
	info := BufferMapCallbackInfo{
		Mode:     CallbackModeAllowProcessEvents,
		Callback: cb,
	}
	purego.SyscallN(symBufferMapAsync,
		uintptr(buf), uintptr(mode), uintptr(offset), uintptr(size),
		uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(info)
}

// BufferGetMappedRange returns the mapped pointer.
func BufferGetMappedRange(buf Buffer, offset, size uint64) uintptr {
	return fnBufferGetMappedRange(buf, offset, size)
}

// BufferUnmap unmaps a buffer.
func BufferUnmap(buf Buffer) {
	fnBufferUnmap(buf)
}

// BindGroupLayoutRelease releases a bind group layout.
func BindGroupLayoutRelease(layout BindGroupLayout) {
	fnBindGroupLayoutRelease(layout)
}

// BindGroupRelease releases a bind group.
func BindGroupRelease(bg BindGroup) {
	fnBindGroupRelease(bg)
}

// PipelineLayoutRelease releases a pipeline layout.
func PipelineLayoutRelease(layout PipelineLayout) {
	fnPipelineLayoutRelease(layout)
}

// RenderPassSetBindGroup binds a bind group to a slot.
func RenderPassSetBindGroup(rpe RenderPassEncoder, groupIndex uint32, group BindGroup) {
	fnRenderPassEncoderSetBindGroup(rpe, groupIndex, group, 0, 0)
}

// DevicePoll polls the device for completed work.
func DevicePoll(dev Device, wait bool) {
	w := uint32(0)
	if wait {
		w = 1
	}
	fnDevicePoll(dev, w, 0)
}

// DeviceCreateSampler creates a sampler with default settings (nearest filter).
func DeviceCreateSampler(dev Device) Sampler {
	return fnDeviceCreateSampler(dev, 0)
}

// DeviceCreateSamplerWithDescriptor creates a sampler with the given descriptor.
func DeviceCreateSamplerWithDescriptor(dev Device, desc *SamplerDescriptor) Sampler {
	return fnDeviceCreateSampler(dev, uintptr(unsafe.Pointer(desc)))
}

// SamplerRelease releases a sampler.
func SamplerRelease(s Sampler) {
	fnSamplerRelease(s)
}

// InstanceRequestAdapterSync synchronously requests an adapter from the instance.
// Uses AllowSpontaneous mode — wgpu-native fires the callback inline before returning.
func InstanceRequestAdapterSync(inst Instance) (Adapter, error) {
	var result Adapter
	var resultErr error

	// v27 callback: (status, adapter, message{data,len}, userdata1, userdata2)
	cb := purego.NewCallback(func(status uint32, adapter uintptr, msgData uintptr, msgLen uintptr, userdata1 uintptr, userdata2 uintptr) {
		if status != RequestAdapterStatusSuccess {
			resultErr = fmt.Errorf("adapter request failed (status %d)", status)
			return
		}
		result = Adapter(adapter)
	})

	info := RequestAdapterCallbackInfo{
		Mode:     CallbackModeAllowSpontaneous,
		Callback: cb,
	}

	// On ARM64, structs > 16 bytes are passed by hidden pointer.
	purego.SyscallN(symInstanceRequestAdapter,
		uintptr(inst), 0,
		uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(info)

	if resultErr != nil {
		return 0, resultErr
	}
	if result == 0 {
		return 0, fmt.Errorf("adapter request returned nil")
	}
	return result, nil
}

// AdapterRequestDeviceSync synchronously requests a device from the adapter.
func AdapterRequestDeviceSync(adapter Adapter, inst Instance) (Device, error) {
	var result Device
	var resultErr error

	cb := purego.NewCallback(func(status uint32, device uintptr, msgData uintptr, msgLen uintptr, userdata1 uintptr, userdata2 uintptr) {
		if status != RequestDeviceStatusSuccess {
			resultErr = fmt.Errorf("device request failed (status %d)", status)
			return
		}
		result = Device(device)
	})

	info := RequestDeviceCallbackInfo{
		Mode:     CallbackModeAllowSpontaneous,
		Callback: cb,
	}

	purego.SyscallN(symAdapterRequestDevice,
		uintptr(adapter), 0,
		uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(info)

	if resultErr != nil {
		return 0, resultErr
	}
	if result == 0 {
		return 0, fmt.Errorf("device request returned nil")
	}
	return result, nil
}

// MapModeRead is the read mode for buffer mapping.
const MapModeRead uint32 = 1

// InstanceRelease releases an instance.
func InstanceRelease(inst Instance) {
	fnInstanceRelease(inst)
}

// AdapterRelease releases an adapter.
func AdapterRelease(adapter Adapter) {
	fnAdapterRelease(adapter)
}

// DeviceRelease releases a device.
func DeviceRelease(dev Device) {
	fnDeviceRelease(dev)
}

// InstanceCreateSurface creates a presentation surface.
func InstanceCreateSurface(inst Instance, desc *SurfaceDescriptor) Surface {
	return fnInstanceCreateSurface(inst, uintptr(unsafe.Pointer(desc)))
}

// SurfaceConfigure configures a surface for presentation.
func SurfaceConfigure(surface Surface, config *SurfaceConfiguration) {
	fnSurfaceConfigure(surface, config)
}

// SurfaceGetCurrentTexture gets the current texture for rendering.
func SurfaceGetCurrentTexture(surface Surface, out *SurfaceTexture) {
	fnSurfaceGetCurrentTexture(surface, out)
}

// SurfacePresent presents the current texture to the screen.
func SurfacePresent(surface Surface) {
	fnSurfacePresent(surface)
}

// SurfaceUnconfigure unconfigures a surface.
func SurfaceUnconfigure(surface Surface) {
	fnSurfaceUnconfigure(surface)
}

// SurfaceRelease releases a surface.
func SurfaceRelease(surface Surface) {
	fnSurfaceRelease(surface)
}

// SurfaceGetCapabilities queries surface capabilities.
func SurfaceGetCapabilities(surface Surface, adapter Adapter, caps *SurfaceCapabilities) {
	fnSurfaceGetCapabilities(surface, adapter, caps)
}

// ---------------------------------------------------------------------------
// Library loading
// ---------------------------------------------------------------------------

// loadWGPULib attempts to load the wgpu-native shared library. It checks
// WGPU_NATIVE_LIB_PATH first (which may be an exact file path or a directory
// containing the library), then falls back to the platform default name.
func loadWGPULib(defaultName string) (uintptr, error) {
	var names []string
	if p := os.Getenv("WGPU_NATIVE_LIB_PATH"); p != "" {
		info, err := os.Stat(p)
		if err == nil && info.IsDir() {
			names = append(names, filepath.Join(p, defaultName))
		} else {
			names = append(names, p)
		}
	}
	names = append(names, defaultName)

	var firstErr error
	for _, name := range names {
		h, err := dlopen.Open(name)
		if err == nil {
			return h, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return 0, fmt.Errorf("failed to load %s: %w", defaultName, firstErr)
}

// ---------------------------------------------------------------------------
// Init loads libwgpu_native and resolves all function symbols
// ---------------------------------------------------------------------------

// Init loads the wgpu-native shared library and resolves symbols.
// The library search order is:
//  1. WGPU_NATIVE_LIB_PATH env var (exact file path or directory)
//  2. Platform default name via dynamic linker search (LD_LIBRARY_PATH, etc.)
func Init() error {
	var libName string
	switch runtime.GOOS {
	case "linux":
		libName = "libwgpu_native.so"
	case "windows":
		libName = "wgpu_native.dll"
	case "darwin":
		libName = "libwgpu_native.dylib"
	default:
		return fmt.Errorf("wgpu: unsupported platform %s", runtime.GOOS)
	}

	lib, err := loadWGPULib(libName)
	if err != nil {
		return fmt.Errorf("wgpu: %w", err)
	}

	reg := func(fn interface{}, name string) {
		if err != nil {
			return
		}
		sym, e := dlopen.Sym(lib, name)
		if e != nil {
			err = fmt.Errorf("wgpu: symbol %s: %w", name, e)
			return
		}
		purego.RegisterFunc(fn, sym)
	}

	reg(&fnCreateInstance, "wgpuCreateInstance")
	// InstanceRequestAdapter and AdapterRequestDevice use SyscallN (struct-by-value params).
	if err == nil {
		symInstanceRequestAdapter, err = dlopen.Sym(lib, "wgpuInstanceRequestAdapter")
	}
	if err == nil {
		symAdapterRequestDevice, err = dlopen.Sym(lib, "wgpuAdapterRequestDevice")
	}
	if err == nil {
		symInstanceWaitAny, err = dlopen.Sym(lib, "wgpuInstanceWaitAny")
	}
	reg(&fnDeviceGetQueue, "wgpuDeviceGetQueue")
	reg(&fnDeviceCreateTexture, "wgpuDeviceCreateTexture")
	reg(&fnDeviceCreateBuffer, "wgpuDeviceCreateBuffer")
	reg(&fnDeviceCreateShaderModule, "wgpuDeviceCreateShaderModule")
	reg(&fnDeviceCreateRenderPipeline, "wgpuDeviceCreateRenderPipeline")
	reg(&fnDeviceCreateCommandEncoder, "wgpuDeviceCreateCommandEncoder")
	reg(&fnTextureCreateView, "wgpuTextureCreateView")
	reg(&fnTextureGetWidth, "wgpuTextureGetWidth")
	reg(&fnTextureGetHeight, "wgpuTextureGetHeight")
	reg(&fnTextureDestroy, "wgpuTextureDestroy")
	reg(&fnTextureRelease, "wgpuTextureRelease")
	reg(&fnTextureViewRelease, "wgpuTextureViewRelease")
	reg(&fnBufferGetSize, "wgpuBufferGetSize")
	reg(&fnBufferDestroy, "wgpuBufferDestroy")
	reg(&fnBufferRelease, "wgpuBufferRelease")
	reg(&fnShaderModuleRelease, "wgpuShaderModuleRelease")
	reg(&fnRenderPipelineRelease, "wgpuRenderPipelineRelease")
	reg(&fnQueueWriteBuffer, "wgpuQueueWriteBuffer")
	reg(&fnQueueWriteTexture, "wgpuQueueWriteTexture")
	reg(&fnQueueSubmit, "wgpuQueueSubmit")
	reg(&fnCommandEncoderBeginRenderPass, "wgpuCommandEncoderBeginRenderPass")
	reg(&fnCommandEncoderFinish, "wgpuCommandEncoderFinish")
	reg(&fnCommandEncoderRelease, "wgpuCommandEncoderRelease")
	reg(&fnCommandBufferRelease, "wgpuCommandBufferRelease")
	reg(&fnRenderPassEncoderSetPipeline, "wgpuRenderPassEncoderSetPipeline")
	reg(&fnRenderPassEncoderSetVertexBuffer, "wgpuRenderPassEncoderSetVertexBuffer")
	reg(&fnRenderPassEncoderSetIndexBuffer, "wgpuRenderPassEncoderSetIndexBuffer")
	reg(&fnRenderPassEncoderSetViewport, "wgpuRenderPassEncoderSetViewport")
	reg(&fnRenderPassEncoderSetScissorRect, "wgpuRenderPassEncoderSetScissorRect")
	reg(&fnRenderPassEncoderDraw, "wgpuRenderPassEncoderDraw")
	reg(&fnRenderPassEncoderDrawIndexed, "wgpuRenderPassEncoderDrawIndexed")
	reg(&fnRenderPassEncoderEnd, "wgpuRenderPassEncoderEnd")
	reg(&fnRenderPassEncoderRelease, "wgpuRenderPassEncoderRelease")
	reg(&fnInstanceRelease, "wgpuInstanceRelease")
	reg(&fnAdapterRelease, "wgpuAdapterRelease")
	reg(&fnDeviceRelease, "wgpuDeviceRelease")
	reg(&fnDeviceCreateBindGroupLayout, "wgpuDeviceCreateBindGroupLayout")
	reg(&fnDeviceCreateBindGroup, "wgpuDeviceCreateBindGroup")
	reg(&fnDeviceCreatePipelineLayout, "wgpuDeviceCreatePipelineLayout")
	reg(&fnCommandEncoderCopyTextureToBuffer, "wgpuCommandEncoderCopyTextureToBuffer")
	// BufferMapAsync uses SyscallN (struct-by-value param).
	if err == nil {
		symBufferMapAsync, err = dlopen.Sym(lib, "wgpuBufferMapAsync")
	}
	reg(&fnBufferGetMappedRange, "wgpuBufferGetMappedRange")
	reg(&fnBufferUnmap, "wgpuBufferUnmap")
	reg(&fnBindGroupLayoutRelease, "wgpuBindGroupLayoutRelease")
	reg(&fnBindGroupRelease, "wgpuBindGroupRelease")
	reg(&fnPipelineLayoutRelease, "wgpuPipelineLayoutRelease")
	reg(&fnRenderPassEncoderSetBindGroup, "wgpuRenderPassEncoderSetBindGroup")
	reg(&fnDevicePoll, "wgpuDevicePoll")
	reg(&fnDeviceCreateSampler, "wgpuDeviceCreateSampler")
	reg(&fnSamplerRelease, "wgpuSamplerRelease")
	reg(&fnInstanceCreateSurface, "wgpuInstanceCreateSurface")
	reg(&fnSurfaceConfigure, "wgpuSurfaceConfigure")
	reg(&fnSurfaceGetCurrentTexture, "wgpuSurfaceGetCurrentTexture")
	reg(&fnSurfacePresent, "wgpuSurfacePresent")
	reg(&fnSurfaceUnconfigure, "wgpuSurfaceUnconfigure")
	reg(&fnSurfaceRelease, "wgpuSurfaceRelease")
	reg(&fnSurfaceGetCapabilities, "wgpuSurfaceGetCapabilities")

	return err
}
