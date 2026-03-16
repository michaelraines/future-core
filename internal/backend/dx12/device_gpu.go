//go:build dx12native

package dx12

import (
	"fmt"
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/d3d12"
)

// Device implements backend.Device for DirectX 12 via COM vtable dispatch.
type Device struct {
	factory      d3d12.Factory
	adapter      d3d12.Adapter
	device       d3d12.Device
	commandQueue d3d12.CommandQueue
	commandAlloc d3d12.CommandAllocator
	commandList  d3d12.GraphicsCommandList
	fence        d3d12.Fence
	fenceValue   uint64
	rtvHeap      d3d12.DescriptorHeap
	rtvDescSize  uint32

	width  int
	height int

	// Default render target.
	defaultColorRes  d3d12.Resource
	defaultRTVHandle d3d12.CPUDescriptorHandle

	// Upload heap for CPU→GPU transfers.
	uploadBuffer d3d12.Resource
	uploadMapped uintptr
	uploadSize   int

	// DX12-specific state modeled for API compatibility.
	adapterDesc  AdapterDesc
	featureLevel FeatureLevel
	debugLayer   bool
}

// AdapterDesc mirrors DXGI_ADAPTER_DESC1 fields.
type AdapterDesc struct {
	Description  string
	VendorID     uint32
	DeviceID     uint32
	DedicatedMem uint64
	SharedMem    uint64
}

// FeatureLevel represents a D3D feature level.
type FeatureLevel int

// Feature level constants.
const (
	FeatureLevel11_0 FeatureLevel = 0xb000
	FeatureLevel11_1 FeatureLevel = 0xb100
	FeatureLevel12_0 FeatureLevel = 0xc000
	FeatureLevel12_1 FeatureLevel = 0xc100
	FeatureLevel12_2 FeatureLevel = 0xc200
)

// New creates a new DirectX 12 device.
func New() *Device {
	return &Device{
		adapterDesc: AdapterDesc{
			Description: "DirectX 12 Device",
		},
		featureLevel: FeatureLevel12_0,
	}
}

// Init initializes the DX12 device by loading d3d12.dll/dxgi.dll and creating
// the device, command queue, and default render target.
func (d *Device) Init(cfg backend.DeviceConfig) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("dx12: invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
	d.width = cfg.Width
	d.height = cfg.Height
	d.debugLayer = cfg.Debug

	if err := d3d12.Init(); err != nil {
		return fmt.Errorf("dx12: %w", err)
	}

	// Create DXGI factory.
	factory, err := d3d12.CreateFactory()
	if err != nil {
		return fmt.Errorf("dx12: %w", err)
	}
	d.factory = factory

	// Enumerate first adapter.
	d.adapter = d3d12.FactoryEnumAdapters(d.factory, 0)

	// Create device.
	dev, err := d3d12.CreateDevice(d.adapter, int32(d.featureLevel))
	if err != nil {
		return fmt.Errorf("dx12: %w", err)
	}
	d.device = dev

	// Create command queue.
	queue, err := d3d12.DeviceCreateCommandQueue(d.device, d3d12.CommandListTypeDirect)
	if err != nil {
		return fmt.Errorf("dx12: %w", err)
	}
	d.commandQueue = queue

	// Create command allocator.
	alloc, err := d3d12.DeviceCreateCommandAllocator(d.device, d3d12.CommandListTypeDirect)
	if err != nil {
		return fmt.Errorf("dx12: %w", err)
	}
	d.commandAlloc = alloc

	// Create command list.
	list, err := d3d12.DeviceCreateCommandList(d.device, d3d12.CommandListTypeDirect, d.commandAlloc)
	if err != nil {
		return fmt.Errorf("dx12: %w", err)
	}
	d.commandList = list
	_ = d3d12.CmdClose(d.commandList) // Start in closed state.

	// Create fence.
	fence, err := d3d12.DeviceCreateFence(d.device, 0)
	if err != nil {
		return fmt.Errorf("dx12: %w", err)
	}
	d.fence = fence

	// Create RTV descriptor heap.
	rtvHeap, err := d3d12.DeviceCreateDescriptorHeap(d.device, d3d12.DescriptorHeapTypeRTV, 16)
	if err != nil {
		return fmt.Errorf("dx12: %w", err)
	}
	d.rtvHeap = rtvHeap

	// Create default color render target.
	if err := d.createDefaultRenderTarget(); err != nil {
		return fmt.Errorf("dx12: %w", err)
	}

	// Create upload buffer for CPU→GPU transfers.
	d.uploadSize = 4 * 1024 * 1024 // 4 MB
	heapProps := d3d12.HeapProperties{Type: d3d12.HeapTypeUpload}
	resDesc := d3d12.ResourceDesc{
		Dimension:        d3d12.ResourceDimensionBuffer,
		Width:            uint64(d.uploadSize),
		Height:           1,
		DepthOrArraySize: 1,
		MipLevels:        1,
		SampleCount:      1,
		Layout:           1, // D3D12_TEXTURE_LAYOUT_ROW_MAJOR
	}
	buf, err := d3d12.DeviceCreateCommittedResource(d.device, &heapProps, &resDesc, d3d12.ResourceStateGenericRead)
	if err != nil {
		return fmt.Errorf("dx12: upload buffer: %w", err)
	}
	d.uploadBuffer = buf
	d.uploadMapped, err = d3d12.ResourceMap(d.uploadBuffer)
	if err != nil {
		return fmt.Errorf("dx12: upload buffer map: %w", err)
	}

	return nil
}

// createDefaultRenderTarget creates the default screen render target.
func (d *Device) createDefaultRenderTarget() error {
	heapProps := d3d12.HeapProperties{Type: d3d12.HeapTypeDefault}
	resDesc := d3d12.ResourceDesc{
		Dimension:        d3d12.ResourceDimensionTexture2D,
		Width:            uint64(d.width),
		Height:           uint32(d.height),
		DepthOrArraySize: 1,
		MipLevels:        1,
		Format:           d3d12.FormatR8G8B8A8UNorm,
		SampleCount:      1,
		Flags:            0x2, // D3D12_RESOURCE_FLAG_ALLOW_RENDER_TARGET
	}
	res, err := d3d12.DeviceCreateCommittedResource(d.device, &heapProps, &resDesc, d3d12.ResourceStateRenderTarget)
	if err != nil {
		return err
	}
	d.defaultColorRes = res

	// Create RTV for the default color resource.
	// Get the CPU start handle of the RTV heap — for now we store it directly.
	d.defaultRTVHandle = d3d12.CPUDescriptorHandle{Ptr: uintptr(d.rtvHeap)}
	d3d12.DeviceCreateRenderTargetView(d.device, d.defaultColorRes, d.defaultRTVHandle)

	return nil
}

// Dispose releases all DX12 resources.
func (d *Device) Dispose() {
	if d.uploadBuffer != 0 {
		d3d12.ResourceUnmap(d.uploadBuffer)
		d3d12.Release(uintptr(d.uploadBuffer))
		d.uploadBuffer = 0
	}
	if d.defaultColorRes != 0 {
		d3d12.Release(uintptr(d.defaultColorRes))
		d.defaultColorRes = 0
	}
	if d.rtvHeap != 0 {
		d3d12.Release(uintptr(d.rtvHeap))
		d.rtvHeap = 0
	}
	if d.fence != 0 {
		d3d12.Release(uintptr(d.fence))
		d.fence = 0
	}
	if d.commandList != 0 {
		d3d12.Release(uintptr(d.commandList))
		d.commandList = 0
	}
	if d.commandAlloc != 0 {
		d3d12.Release(uintptr(d.commandAlloc))
		d.commandAlloc = 0
	}
	if d.commandQueue != 0 {
		d3d12.Release(uintptr(d.commandQueue))
		d.commandQueue = 0
	}
	if d.device != 0 {
		d3d12.Release(uintptr(d.device))
		d.device = 0
	}
	if d.adapter != 0 {
		d3d12.Release(uintptr(d.adapter))
		d.adapter = 0
	}
	if d.factory != 0 {
		d3d12.Release(uintptr(d.factory))
		d.factory = 0
	}
}

// BeginFrame prepares for a new frame.
func (d *Device) BeginFrame() {
	// Wait for previous frame to complete.
	completed := d3d12.FenceGetCompletedValue(d.fence)
	if completed < d.fenceValue {
		// Spin-wait (in production, use an event handle).
		for d3d12.FenceGetCompletedValue(d.fence) < d.fenceValue {
			// busy wait
		}
	}
	// Reset command allocator and list.
	_ = d3d12.CmdReset(d.commandList, d.commandAlloc)
}

// EndFrame finalizes the frame by closing and submitting the command list.
func (d *Device) EndFrame() {
	_ = d3d12.CmdClose(d.commandList)
	d3d12.QueueExecuteCommandLists(d.commandQueue, d.commandList)
	d.fenceValue++
	_ = d3d12.QueueSignal(d.commandQueue, d.fence, d.fenceValue)
}

// NewTexture creates a DX12 texture (ID3D12Resource).
func (d *Device) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("dx12: invalid texture dimensions %dx%d", desc.Width, desc.Height)
	}

	dxFmt := dxgiFormatFromBackend(desc.Format)
	heapProps := d3d12.HeapProperties{Type: d3d12.HeapTypeDefault}
	resDesc := d3d12.ResourceDesc{
		Dimension:        d3d12.ResourceDimensionTexture2D,
		Width:            uint64(desc.Width),
		Height:           uint32(desc.Height),
		DepthOrArraySize: 1,
		MipLevels:        1,
		Format:           int32(dxFmt),
		SampleCount:      1,
	}

	res, err := d3d12.DeviceCreateCommittedResource(d.device, &heapProps, &resDesc, d3d12.ResourceStateCopyDest)
	if err != nil {
		return nil, fmt.Errorf("dx12: %w", err)
	}

	tex := &Texture{
		dev:        d,
		resource:   res,
		w:          desc.Width,
		h:          desc.Height,
		format:     desc.Format,
		dxgiFormat: dxFmt,
	}

	if len(desc.Data) > 0 {
		tex.Upload(desc.Data, 0)
	}

	return tex, nil
}

// NewBuffer creates a DX12 buffer (ID3D12Resource).
func (d *Device) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	size := desc.Size
	if len(desc.Data) > 0 {
		size = len(desc.Data)
	}
	if size <= 0 {
		return nil, fmt.Errorf("dx12: invalid buffer size %d", size)
	}

	// Align to 256 bytes (DX12 constant buffer alignment).
	alignedSize := uint64((size + 255) &^ 255)

	heapProps := d3d12.HeapProperties{Type: d3d12.HeapTypeUpload}
	resDesc := d3d12.ResourceDesc{
		Dimension:        d3d12.ResourceDimensionBuffer,
		Width:            alignedSize,
		Height:           1,
		DepthOrArraySize: 1,
		MipLevels:        1,
		SampleCount:      1,
		Layout:           1, // D3D12_TEXTURE_LAYOUT_ROW_MAJOR
	}

	res, err := d3d12.DeviceCreateCommittedResource(d.device, &heapProps, &resDesc, d3d12.ResourceStateGenericRead)
	if err != nil {
		return nil, fmt.Errorf("dx12: %w", err)
	}

	mapped, err := d3d12.ResourceMap(res)
	if err != nil {
		d3d12.Release(uintptr(res))
		return nil, fmt.Errorf("dx12: buffer map: %w", err)
	}

	buf := &Buffer{
		dev:      d,
		resource: res,
		size:     size,
		mapped:   mapped,
		gpuAddr:  d3d12.ResourceGetGPUVirtualAddress(res),
		heapType: d3d12HeapTypeUpload,
	}

	if len(desc.Data) > 0 {
		buf.Upload(desc.Data)
	}

	return buf, nil
}

// NewShader creates a DX12 shader (stores HLSL source for later compilation).
func (d *Device) NewShader(desc backend.ShaderDescriptor) (backend.Shader, error) {
	return &Shader{
		dev:            d,
		vertexSource:   desc.VertexSource,
		fragmentSource: desc.FragmentSource,
		attributes:     desc.Attributes,
		uniforms:       make(map[string]interface{}),
	}, nil
}

// NewRenderTarget creates a DX12 render target.
func (d *Device) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("dx12: invalid render target dimensions %dx%d", desc.Width, desc.Height)
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
		return nil, fmt.Errorf("dx12: render target color: %w", err)
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
			return nil, fmt.Errorf("dx12: render target depth: %w", err)
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

// NewPipeline creates a DX12 pipeline state object.
func (d *Device) NewPipeline(desc backend.PipelineDescriptor) (backend.Pipeline, error) {
	return &Pipeline{
		dev:  d,
		desc: desc,
	}, nil
}

// Capabilities returns DirectX 12 device capabilities.
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
	return &Encoder{
		dev:    d,
		width:  d.width,
		height: d.height,
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
