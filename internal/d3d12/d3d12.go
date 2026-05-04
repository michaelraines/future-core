//go:build windows && !soft

// Package d3d12 provides pure-Go bindings to DirectX 12 and DXGI via purego
// (for ABI dispatch) plus internal/dlopen (for cross-platform library
// loading). All COM interface calls go through vtable dispatch — no CGo
// required.
//
// DX12 uses COM (Component Object Model) interfaces where each object has a
// vtable pointer. Method calls are dispatched by reading function pointers
// from the vtable at known offsets.
package d3d12

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/michaelraines/future-core/internal/dlopen"
)

// ---------------------------------------------------------------------------
// Handle types — COM interface pointers
// ---------------------------------------------------------------------------

type (
	// Factory is an IDXGIFactory4 COM interface pointer.
	Factory uintptr
	// Adapter is an IDXGIAdapter1 COM interface pointer.
	Adapter uintptr
	// Device is an ID3D12Device COM interface pointer.
	Device uintptr
	// CommandQueue is an ID3D12CommandQueue COM interface pointer.
	CommandQueue uintptr
	// CommandAllocator is an ID3D12CommandAllocator COM interface pointer.
	CommandAllocator uintptr
	// GraphicsCommandList is an ID3D12GraphicsCommandList COM interface pointer.
	GraphicsCommandList uintptr
	// Resource is an ID3D12Resource COM interface pointer (texture or buffer).
	Resource uintptr
	// DescriptorHeap is an ID3D12DescriptorHeap COM interface pointer.
	DescriptorHeap uintptr
	// Fence is an ID3D12Fence COM interface pointer.
	Fence uintptr
	// PipelineState is an ID3D12PipelineState COM interface pointer.
	PipelineState uintptr
	// RootSignature is an ID3D12RootSignature COM interface pointer.
	RootSignature uintptr
)

// HRESULT is a Windows COM result code.
type HRESULT int32

// Succeeded returns true if the HRESULT indicates success.
func (hr HRESULT) Succeeded() bool { return hr >= 0 }

// Error implements the error interface for HRESULT.
func (hr HRESULT) Error() string {
	return fmt.Sprintf("HRESULT 0x%08X", uint32(hr))
}

// ---------------------------------------------------------------------------
// DXGI_FORMAT constants
// ---------------------------------------------------------------------------

const (
	FormatUnknown           = 0
	FormatR32G32B32A32Float = 2
	FormatR16G16B16A16Float = 10
	FormatR8G8B8A8UNorm     = 28
	FormatB8G8R8A8UNorm     = 87
	FormatR8UNorm           = 61
	FormatD24UNormS8UInt    = 45
	FormatD32Float          = 40
	FormatR32Typeless       = 39
	FormatR24G8Typeless     = 44
)

// ---------------------------------------------------------------------------
// D3D12_HEAP_TYPE
// ---------------------------------------------------------------------------

const (
	HeapTypeDefault  = 1
	HeapTypeUpload   = 2
	HeapTypeReadback = 3
)

// ---------------------------------------------------------------------------
// D3D12_RESOURCE_DIMENSION
// ---------------------------------------------------------------------------

const (
	ResourceDimensionUnknown   = 0
	ResourceDimensionBuffer    = 1
	ResourceDimensionTexture1D = 2
	ResourceDimensionTexture2D = 3
	ResourceDimensionTexture3D = 4
)

// ---------------------------------------------------------------------------
// D3D12_RESOURCE_STATES
// ---------------------------------------------------------------------------

const (
	ResourceStateCommon               = 0
	ResourceStateVertexAndConstBuffer = 0x00000001
	ResourceStateIndexBuffer          = 0x00000002
	ResourceStateRenderTarget         = 0x00000004
	ResourceStateUnorderedAccess      = 0x00000008
	ResourceStateDepthWrite           = 0x00000010
	ResourceStateDepthRead            = 0x00000020
	ResourceStateNonPixelShaderResrc  = 0x00000040
	ResourceStatePixelShaderResource  = 0x00000080
	ResourceStateCopyDest             = 0x00000400
	ResourceStateCopySrc              = 0x00000800
	ResourceStateGenericRead          = 0x00000001 | 0x00000002 | 0x00000040 | 0x00000080 | 0x00000200 | 0x00000800
	ResourceStatePresent              = 0

	// ResourceStateVertexBuffer is the legacy alias for
	// VertexAndConstBuffer. Earlier bindings had this set to 0x4 by
	// mistake (which collided with RenderTarget); the value is now
	// the spec-correct 0x1, matching D3D12_RESOURCE_STATE_VERTEX_AND_CONSTANT_BUFFER.
	ResourceStateVertexBuffer = ResourceStateVertexAndConstBuffer
)

// ---------------------------------------------------------------------------
// D3D12_COMMAND_LIST_TYPE
// ---------------------------------------------------------------------------

const (
	CommandListTypeDirect  = 0
	CommandListTypeBundle  = 1
	CommandListTypeCompute = 2
	CommandListTypeCopy    = 3
)

// ---------------------------------------------------------------------------
// D3D12_DESCRIPTOR_HEAP_TYPE
// ---------------------------------------------------------------------------

const (
	DescriptorHeapTypeCBVSRVUAV = 0
	DescriptorHeapTypeSampler   = 1
	DescriptorHeapTypeRTV       = 2
	DescriptorHeapTypeDSV       = 3
)

// ---------------------------------------------------------------------------
// D3D12_INDEX_BUFFER_STRIP_CUT_VALUE / DXGI_FORMAT index types
// ---------------------------------------------------------------------------

const (
	IndexFormatUInt16 = FormatR8G8B8A8UNorm // placeholder — real format used inline
	IndexFormatUInt32 = FormatR32Typeless   // placeholder

	// Actual DXGI formats for index buffers.
	FormatR16UInt = 57
	FormatR32UInt = 42
)

// ---------------------------------------------------------------------------
// C-compatible structs
// ---------------------------------------------------------------------------

// HeapProperties mirrors D3D12_HEAP_PROPERTIES.
type HeapProperties struct {
	Type                 int32
	CPUPageProperty      int32
	MemoryPoolPreference int32
	CreationNodeMask     uint32
	VisibleNodeMask      uint32
}

// ResourceDesc mirrors D3D12_RESOURCE_DESC.
type ResourceDesc struct {
	Dimension        int32
	Alignment        uint64
	Width            uint64
	Height           uint32
	DepthOrArraySize uint16
	MipLevels        uint16
	Format           int32
	SampleCount      uint32
	SampleQuality    uint32
	Layout           int32
	Flags            int32
}

// Viewport mirrors D3D12_VIEWPORT.
type Viewport struct {
	TopLeftX, TopLeftY, Width, Height, MinDepth, MaxDepth float32
}

// Rect mirrors D3D12_RECT (RECT).
type Rect struct {
	Left, Top, Right, Bottom int32
}

// ClearColor represents an RGBA clear color.
type ClearColor struct {
	R, G, B, A float32
}

// CPUDescriptorHandle mirrors D3D12_CPU_DESCRIPTOR_HANDLE.
type CPUDescriptorHandle struct {
	Ptr uintptr
}

// TextureCopyLocation mirrors D3D12_TEXTURE_COPY_LOCATION.
//
// The C struct is { ID3D12Resource* pResource; D3D12_TEXTURE_COPY_TYPE Type;
// union { D3D12_PLACED_SUBRESOURCE_FOOTPRINT PlacedFootprint; UINT
// SubresourceIndex; }; } — total 48 bytes on x64. The 4-byte gap after Type
// is the union's 8-byte alignment requirement (PlacedFootprint contains a
// UINT64). Go won't insert that automatically because the next field is a
// byte array (alignment 1), so we add an explicit pad. Use the
// NewTextureCopyLocation* helpers below to construct one safely.
type TextureCopyLocation struct {
	Resource Resource // 8 bytes
	Type     int32    // 4 bytes — 0 = SubresourceIndex, 1 = PlacedFootprint
	_        [4]byte  // 4 bytes pad — keeps the union 8-byte aligned
	Union    [32]byte // PlacedSubresourceFootprint (32 bytes) OR SubresourceIndex (4 bytes, rest unused)
}

// PlacedSubresourceFootprint mirrors D3D12_PLACED_SUBRESOURCE_FOOTPRINT.
// 32 bytes on x64 (uint64 Offset is 8-byte aligned; trailing pad keeps the
// struct's tail 8-byte aligned).
type PlacedSubresourceFootprint struct {
	Offset   uint64 // 8 bytes
	Format   int32  // 4 bytes
	Width    uint32 // 4 bytes
	Height   uint32 // 4 bytes
	Depth    uint32 // 4 bytes
	RowPitch uint32 // 4 bytes — must be a multiple of D3D12_TEXTURE_DATA_PITCH_ALIGNMENT (256)
	_        uint32 // trailing pad to 32-byte total
}

// NewTextureCopyLocationSubresource constructs a TextureCopyLocation
// referencing a specific subresource (mip + array slice) of a texture
// resource. Used as the source for copies from a texture.
func NewTextureCopyLocationSubresource(res Resource, subresourceIndex uint32) TextureCopyLocation {
	loc := TextureCopyLocation{Resource: res, Type: 0}
	*(*uint32)(unsafe.Pointer(&loc.Union[0])) = subresourceIndex
	return loc
}

// NewTextureCopyLocationFootprint constructs a TextureCopyLocation
// referencing a buffer at a specific row-major footprint. Used as the
// destination for texture-to-buffer copies (ReadScreen, downloads).
func NewTextureCopyLocationFootprint(res Resource, fp PlacedSubresourceFootprint) TextureCopyLocation {
	loc := TextureCopyLocation{Resource: res, Type: 1}
	*(*PlacedSubresourceFootprint)(unsafe.Pointer(&loc.Union[0])) = fp
	return loc
}

// ResourceBarrier mirrors D3D12_RESOURCE_BARRIER for the Transition flavor
// (the only kind we use today — UAV / Aliasing barriers can be added later).
//
// C layout (48 bytes on x64):
//
//	D3D12_RESOURCE_BARRIER_TYPE Type;     // 4 bytes
//	D3D12_RESOURCE_BARRIER_FLAGS Flags;   // 4 bytes
//	union {
//	    D3D12_RESOURCE_TRANSITION_BARRIER Transition;  // 24 bytes
//	    D3D12_RESOURCE_ALIASING_BARRIER   Aliasing;
//	    D3D12_RESOURCE_UAV_BARRIER        UAV;
//	};
//
// The union region is 24 bytes wide (Transition has the largest layout:
// pResource ptr + 3*UINT). We pad the struct to 32 bytes for an 8-byte
// alignment of pResource within the transition.
type ResourceBarrier struct {
	Type       int32 // 0 = TRANSITION, 1 = ALIASING, 2 = UAV
	Flags      int32 // 0 = NONE, 1 = BEGIN_ONLY, 2 = END_ONLY
	Transition TransitionBarrier
}

// TransitionBarrier mirrors D3D12_RESOURCE_TRANSITION_BARRIER.
type TransitionBarrier struct {
	Resource    Resource // 8 bytes
	Subresource uint32   // 4 bytes — D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES = 0xffffffff
	StateBefore int32    // 4 bytes — D3D12_RESOURCE_STATES
	StateAfter  int32    // 4 bytes
	_           uint32   // 4 bytes trailing pad to 24
}

// NewTransitionBarrier constructs a ResourceBarrier that transitions a
// resource between two states.
func NewTransitionBarrier(res Resource, before, after int32) ResourceBarrier {
	return ResourceBarrier{
		Type: 0, // TRANSITION
		Transition: TransitionBarrier{
			Resource:    res,
			Subresource: 0xffffffff, // ALL_SUBRESOURCES
			StateBefore: before,
			StateAfter:  after,
		},
	}
}

// ---------------------------------------------------------------------------
// Internal function variables — populated by Init()
// ---------------------------------------------------------------------------

var (
	dxgiLib  uintptr
	d3d12Lib uintptr

	fnCreateDXGIFactory1 func(riid uintptr, ppFactory *Factory) HRESULT
	fnD3D12CreateDevice  func(pAdapter uintptr, minFeatureLevel int32, riid uintptr, ppDevice *Device) HRESULT

	// IID constants for COM interface identification.
	iidFactory4 [16]byte
	iidDevice   [16]byte
)

// ---------------------------------------------------------------------------
// Initialization
// ---------------------------------------------------------------------------

// Init loads the D3D12 and DXGI DLLs, resolving all function pointers.
func Init() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("d3d12: DirectX 12 is only available on Windows")
	}

	// Load DXGI.
	var err error
	dxgiLib, err = dlopen.Open("dxgi.dll")
	if err != nil {
		return fmt.Errorf("d3d12: failed to load dxgi.dll: %w", err)
	}

	// Load D3D12.
	d3d12Lib, err = dlopen.Open("d3d12.dll")
	if err != nil {
		return fmt.Errorf("d3d12: failed to load d3d12.dll: %w", err)
	}

	if err := resolveSymbol(dxgiLib, "CreateDXGIFactory1", &fnCreateDXGIFactory1); err != nil {
		return err
	}
	if err := resolveSymbol(d3d12Lib, "D3D12CreateDevice", &fnD3D12CreateDevice); err != nil {
		return err
	}

	// Initialize well-known IIDs.
	// IDXGIFactory4: {1bc6ea02-ef36-464f-bf0c-21ca39e5168a}
	copy(iidFactory4[:], []byte{0x02, 0xea, 0xc6, 0x1b, 0x36, 0xef, 0x4f, 0x46, 0xbf, 0x0c, 0x21, 0xca, 0x39, 0xe5, 0x16, 0x8a})
	// ID3D12Device: {189819f1-1db6-4b57-be54-1821339b85f7}
	copy(iidDevice[:], []byte{0xf1, 0x19, 0x98, 0x18, 0xb6, 0x1d, 0x57, 0x4b, 0xbe, 0x54, 0x18, 0x21, 0x33, 0x9b, 0x85, 0xf7})

	return nil
}

// ---------------------------------------------------------------------------
// Factory / Adapter functions
// ---------------------------------------------------------------------------

// CreateFactory creates an IDXGIFactory4.
func CreateFactory() (Factory, error) {
	var factory Factory
	hr := fnCreateDXGIFactory1(uintptr(unsafe.Pointer(&iidFactory4)), &factory)
	if !hr.Succeeded() {
		return 0, fmt.Errorf("CreateDXGIFactory1: %w", hr)
	}
	return factory, nil
}

// FactoryEnumAdapters enumerates adapters. Returns 0 when index is out of range.
func FactoryEnumAdapters(factory Factory, index uint32) Adapter {
	vtable := *(*[64]uintptr)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(factory))))
	// IDXGIFactory1::EnumAdapters1 is at vtable index 12.
	var adapter Adapter
	callCOM3(vtable[12], uintptr(factory), uintptr(index), uintptr(unsafe.Pointer(&adapter)))
	return adapter
}

// ---------------------------------------------------------------------------
// Device functions
// ---------------------------------------------------------------------------

// CreateDevice creates an ID3D12Device.
func CreateDevice(adapter Adapter, minFeatureLevel int32) (Device, error) {
	var dev Device
	hr := fnD3D12CreateDevice(uintptr(adapter), minFeatureLevel, uintptr(unsafe.Pointer(&iidDevice)), &dev)
	if !hr.Succeeded() {
		return 0, fmt.Errorf("D3D12CreateDevice: %w", hr)
	}
	return dev, nil
}

// DeviceCreateCommandQueue creates a command queue.
func DeviceCreateCommandQueue(dev Device, listType int32) (CommandQueue, error) {
	// D3D12_COMMAND_QUEUE_DESC
	type queueDesc struct {
		Type     int32
		Priority int32
		Flags    uint32
		NodeMask uint32
	}
	desc := queueDesc{Type: listType}
	vtable := comVtable(uintptr(dev))
	// ID3D12Device::CreateCommandQueue is at vtable index 8.
	var queue CommandQueue
	hr := HRESULT(callCOM3(vtable[8], uintptr(dev), uintptr(unsafe.Pointer(&desc)), uintptr(unsafe.Pointer(&queue))))
	if !hr.Succeeded() {
		return 0, fmt.Errorf("CreateCommandQueue: %w", hr)
	}
	return queue, nil
}

// DeviceCreateCommandAllocator creates a command allocator.
func DeviceCreateCommandAllocator(dev Device, listType int32) (CommandAllocator, error) {
	vtable := comVtable(uintptr(dev))
	// ID3D12Device::CreateCommandAllocator is at vtable index 9.
	var alloc CommandAllocator
	hr := HRESULT(callCOM3(vtable[9], uintptr(dev), uintptr(listType), uintptr(unsafe.Pointer(&alloc))))
	if !hr.Succeeded() {
		return 0, fmt.Errorf("CreateCommandAllocator: %w", hr)
	}
	return alloc, nil
}

// DeviceCreateCommandList creates a graphics command list.
func DeviceCreateCommandList(dev Device, listType int32, alloc CommandAllocator) (GraphicsCommandList, error) {
	vtable := comVtable(uintptr(dev))
	// ID3D12Device::CreateCommandList is at vtable index 12.
	var list GraphicsCommandList
	hr := HRESULT(callCOM5(vtable[12], uintptr(dev), 0, uintptr(listType), uintptr(alloc), uintptr(unsafe.Pointer(&list))))
	if !hr.Succeeded() {
		return 0, fmt.Errorf("CreateCommandList: %w", hr)
	}
	return list, nil
}

// DeviceCreateCommittedResource creates a committed resource (texture or buffer).
func DeviceCreateCommittedResource(dev Device, heapProps *HeapProperties, resDesc *ResourceDesc, initialState int32) (Resource, error) {
	vtable := comVtable(uintptr(dev))
	// ID3D12Device::CreateCommittedResource is at vtable index 27.
	var resource Resource
	hr := HRESULT(callCOM7(vtable[27], uintptr(dev),
		uintptr(unsafe.Pointer(heapProps)),
		0, // HeapFlags
		uintptr(unsafe.Pointer(resDesc)),
		uintptr(initialState),
		0, // pOptimizedClearValue
		uintptr(unsafe.Pointer(&resource))))
	if !hr.Succeeded() {
		return 0, fmt.Errorf("CreateCommittedResource: %w", hr)
	}
	return resource, nil
}

// DeviceCreateFence creates a fence for GPU/CPU synchronization.
func DeviceCreateFence(dev Device, initialValue uint64) (Fence, error) {
	vtable := comVtable(uintptr(dev))
	// ID3D12Device::CreateFence is at vtable index 31.
	var fence Fence
	hr := HRESULT(callCOM4(vtable[31], uintptr(dev), uintptr(initialValue), 0, uintptr(unsafe.Pointer(&fence))))
	if !hr.Succeeded() {
		return 0, fmt.Errorf("CreateFence: %w", hr)
	}
	return fence, nil
}

// DeviceCreateRenderTargetView creates a render target view.
func DeviceCreateRenderTargetView(dev Device, resource Resource, handle CPUDescriptorHandle) {
	vtable := comVtable(uintptr(dev))
	// ID3D12Device::CreateRenderTargetView is at vtable index 20.
	callCOM4(vtable[20], uintptr(dev), uintptr(resource), 0, handle.Ptr)
}

// DeviceCreateDescriptorHeap creates a descriptor heap.
func DeviceCreateDescriptorHeap(dev Device, heapType, numDescriptors int32) (DescriptorHeap, error) {
	type heapDesc struct {
		Type     int32
		Num      uint32
		Flags    uint32
		NodeMask uint32
	}
	desc := heapDesc{Type: heapType, Num: uint32(numDescriptors)}
	vtable := comVtable(uintptr(dev))
	// ID3D12Device::CreateDescriptorHeap is at vtable index 14.
	var heap DescriptorHeap
	hr := HRESULT(callCOM3(vtable[14], uintptr(dev), uintptr(unsafe.Pointer(&desc)), uintptr(unsafe.Pointer(&heap))))
	if !hr.Succeeded() {
		return 0, fmt.Errorf("CreateDescriptorHeap: %w", hr)
	}
	return heap, nil
}

// ---------------------------------------------------------------------------
// Command list functions
// ---------------------------------------------------------------------------

// CmdClose closes the command list.
func CmdClose(list GraphicsCommandList) error {
	vtable := comVtable(uintptr(list))
	// ID3D12GraphicsCommandList::Close is at vtable index 7.
	hr := HRESULT(callCOM1(vtable[7], uintptr(list)))
	if !hr.Succeeded() {
		return fmt.Errorf("CommandList::Close: %w", hr)
	}
	return nil
}

// CmdReset resets the command list.
func CmdReset(list GraphicsCommandList, alloc CommandAllocator) error {
	vtable := comVtable(uintptr(list))
	// ID3D12GraphicsCommandList::Reset is at vtable index 8.
	hr := HRESULT(callCOM3(vtable[8], uintptr(list), uintptr(alloc), 0))
	if !hr.Succeeded() {
		return fmt.Errorf("CommandList::Reset: %w", hr)
	}
	return nil
}

// CmdClearRenderTargetView clears a render target view.
func CmdClearRenderTargetView(list GraphicsCommandList, handle CPUDescriptorHandle, color ClearColor) {
	vtable := comVtable(uintptr(list))
	// ID3D12GraphicsCommandList::ClearRenderTargetView is at vtable index 47.
	callCOM4(vtable[47], uintptr(list), handle.Ptr, uintptr(unsafe.Pointer(&color)), 0)
}

// CmdSetViewports sets viewports.
func CmdSetViewports(list GraphicsCommandList, vp Viewport) {
	vtable := comVtable(uintptr(list))
	// RSSetViewports is at vtable index 43.
	callCOM3(vtable[43], uintptr(list), 1, uintptr(unsafe.Pointer(&vp)))
}

// CmdSetScissorRects sets scissor rectangles.
func CmdSetScissorRects(list GraphicsCommandList, rect Rect) {
	vtable := comVtable(uintptr(list))
	// RSSetScissorRects is at vtable index 44.
	callCOM3(vtable[44], uintptr(list), 1, uintptr(unsafe.Pointer(&rect)))
}

// CmdSetVertexBuffers binds vertex buffers.
func CmdSetVertexBuffers(list GraphicsCommandList, slot uint32, gpuAddr uintptr, sizeInBytes, strideInBytes uint32) {
	type vbView struct {
		BufferLocation uintptr
		SizeInBytes    uint32
		StrideInBytes  uint32
	}
	view := vbView{BufferLocation: gpuAddr, SizeInBytes: sizeInBytes, StrideInBytes: strideInBytes}
	vtable := comVtable(uintptr(list))
	// IASetVertexBuffers is at vtable index 39.
	callCOM4(vtable[39], uintptr(list), uintptr(slot), 1, uintptr(unsafe.Pointer(&view)))
}

// CmdSetIndexBuffer binds an index buffer.
func CmdSetIndexBuffer(list GraphicsCommandList, gpuAddr uintptr, sizeInBytes uint32, format int32) {
	type ibView struct {
		BufferLocation uintptr
		SizeInBytes    uint32
		Format         int32
	}
	view := ibView{BufferLocation: gpuAddr, SizeInBytes: sizeInBytes, Format: format}
	vtable := comVtable(uintptr(list))
	// IASetIndexBuffer is at vtable index 40.
	callCOM2(vtable[40], uintptr(list), uintptr(unsafe.Pointer(&view)))
}

// CmdDrawInstanced issues a non-indexed draw call.
func CmdDrawInstanced(list GraphicsCommandList, vertexCount, instanceCount, startVertex, startInstance uint32) {
	vtable := comVtable(uintptr(list))
	// DrawInstanced is at vtable index 12.
	callCOM5(vtable[12], uintptr(list), uintptr(vertexCount), uintptr(instanceCount), uintptr(startVertex), uintptr(startInstance))
}

// CmdDrawIndexedInstanced issues an indexed draw call.
func CmdDrawIndexedInstanced(list GraphicsCommandList, indexCount, instanceCount, startIndex uint32, baseVertex int32, startInstance uint32) {
	vtable := comVtable(uintptr(list))
	// DrawIndexedInstanced is at vtable index 13.
	callCOM6(vtable[13], uintptr(list), uintptr(indexCount), uintptr(instanceCount), uintptr(startIndex), uintptr(baseVertex), uintptr(startInstance))
}

// CmdOMSetRenderTargets sets render target(s).
func CmdOMSetRenderTargets(list GraphicsCommandList, numRTs uint32, rtvHandle CPUDescriptorHandle) {
	vtable := comVtable(uintptr(list))
	// OMSetRenderTargets is at vtable index 46.
	callCOM5(vtable[46], uintptr(list), uintptr(numRTs), uintptr(unsafe.Pointer(&rtvHandle)), 0, 0)
}

// CmdResourceBarrier inserts a transition / aliasing / UAV barrier into the
// command list. We typically pass a single transition barrier per call; the
// numBarriers + array form supports batching but the binding here keeps the
// shape simple.
//
// ID3D12GraphicsCommandList::ResourceBarrier vtable index = 26.
func CmdResourceBarrier(list GraphicsCommandList, barrier *ResourceBarrier) {
	vtable := comVtable(uintptr(list))
	callCOM3(vtable[26], uintptr(list), 1, uintptr(unsafe.Pointer(barrier)))
}

// CmdCopyTextureRegion copies a region from a texture (or buffer) into
// another. For the ReadScreen path we copy the default RT (a Texture2D) into
// a buffer using a placed-footprint destination.
//
// ID3D12GraphicsCommandList::CopyTextureRegion vtable index = 16.
//
// Args (per Microsoft docs):
//
//	pDst : *D3D12_TEXTURE_COPY_LOCATION
//	DstX, DstY, DstZ : UINT (destination offset within dst)
//	pSrc : *D3D12_TEXTURE_COPY_LOCATION
//	pSrcBox : *D3D12_BOX (NULL for whole resource)
func CmdCopyTextureRegion(list GraphicsCommandList, dst, src *TextureCopyLocation) {
	vtable := comVtable(uintptr(list))
	callCOM7(vtable[16], uintptr(list),
		uintptr(unsafe.Pointer(dst)),
		0, 0, 0, // DstX, DstY, DstZ
		uintptr(unsafe.Pointer(src)),
		0, // pSrcBox = NULL → copy entire source subresource
	)
}

// ---------------------------------------------------------------------------
// Queue functions
// ---------------------------------------------------------------------------

// QueueExecuteCommandLists submits command lists for execution.
func QueueExecuteCommandLists(queue CommandQueue, list GraphicsCommandList) {
	vtable := comVtable(uintptr(queue))
	// ExecuteCommandLists is at vtable index 10.
	callCOM3(vtable[10], uintptr(queue), 1, uintptr(unsafe.Pointer(&list)))
}

// QueueSignal signals a fence from the GPU.
func QueueSignal(queue CommandQueue, fence Fence, value uint64) error {
	vtable := comVtable(uintptr(queue))
	// Signal is at vtable index 14.
	hr := HRESULT(callCOM3(vtable[14], uintptr(queue), uintptr(fence), uintptr(value)))
	if !hr.Succeeded() {
		return fmt.Errorf("Queue::Signal: %w", hr)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Fence functions
// ---------------------------------------------------------------------------

// FenceGetCompletedValue returns the current fence value.
func FenceGetCompletedValue(fence Fence) uint64 {
	vtable := comVtable(uintptr(fence))
	// GetCompletedValue is at vtable index 8.
	return uint64(callCOM1(vtable[8], uintptr(fence)))
}

// ---------------------------------------------------------------------------
// Resource functions
// ---------------------------------------------------------------------------

// ResourceGetGPUVirtualAddress returns the GPU virtual address of a resource.
func ResourceGetGPUVirtualAddress(res Resource) uintptr {
	vtable := comVtable(uintptr(res))
	// GetGPUVirtualAddress is at vtable index 19.
	return callCOM1(vtable[19], uintptr(res))
}

// ResourceMap maps a resource for CPU access.
func ResourceMap(res Resource) (uintptr, error) {
	vtable := comVtable(uintptr(res))
	// Map is at vtable index 8.
	var ptr uintptr
	hr := HRESULT(callCOM4(vtable[8], uintptr(res), 0, 0, uintptr(unsafe.Pointer(&ptr))))
	if !hr.Succeeded() {
		return 0, fmt.Errorf("Resource::Map: %w", hr)
	}
	return ptr, nil
}

// ResourceUnmap unmaps a resource.
func ResourceUnmap(res Resource) {
	vtable := comVtable(uintptr(res))
	// Unmap is at vtable index 9.
	callCOM3(vtable[9], uintptr(res), 0, 0)
}

// ---------------------------------------------------------------------------
// COM release
// ---------------------------------------------------------------------------

// Release calls IUnknown::Release on a COM object.
func Release(obj uintptr) {
	if obj == 0 {
		return
	}
	vtable := comVtable(obj)
	// IUnknown::Release is at vtable index 2.
	callCOM1(vtable[2], obj)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// comVtable reads the vtable pointer from a COM object.
func comVtable(obj uintptr) [64]uintptr {
	return *(*[64]uintptr)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(obj))))
}

// COM call helpers that call through vtable function pointers.
// These use purego.SyscallN under the hood.

func callCOM1(fn, a1 uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, a1)
	return r
}

func callCOM2(fn, a1, a2 uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, a1, a2)
	return r
}

func callCOM3(fn, a1, a2, a3 uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, a1, a2, a3)
	return r
}

func callCOM4(fn, a1, a2, a3, a4 uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, a1, a2, a3, a4)
	return r
}

func callCOM5(fn, a1, a2, a3, a4, a5 uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, a1, a2, a3, a4, a5)
	return r
}

func callCOM6(fn, a1, a2, a3, a4, a5, a6 uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, a1, a2, a3, a4, a5, a6)
	return r
}

func callCOM7(fn, a1, a2, a3, a4, a5, a6, a7 uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, a1, a2, a3, a4, a5, a6, a7)
	return r
}

// resolveSymbol loads a symbol from a DLL into a function pointer.
// internal/dlopen routes to syscall.GetProcAddress on Windows and
// purego.Dlsym on Unix; the resolved function-pointer value is then
// registered via purego.RegisterFunc (cross-platform).
func resolveSymbol(handle uintptr, name string, fn interface{}) error {
	sym, err := dlopen.Sym(handle, name)
	if err != nil {
		return fmt.Errorf("d3d12: failed to resolve %s: %w", name, err)
	}
	purego.RegisterFunc(fn, sym)
	return nil
}

// Keep compiler happy.
var _ = unsafe.Pointer(nil)
