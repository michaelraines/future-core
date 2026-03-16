//go:build wgpunative

// Package wgpu provides pure Go WebGPU bindings loaded at runtime via purego
// against wgpu-native (libwgpu_native). No CGo is required. The shared library
// must be available at runtime.
package wgpu

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
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

const (
	TextureFormatRGBA8Unorm   TextureFormat = 18
	TextureFormatBGRA8Unorm   TextureFormat = 24
	TextureFormatR8Unorm      TextureFormat = 1
	TextureFormatRGBA16Float  TextureFormat = 33
	TextureFormatRGBA32Float  TextureFormat = 36
	TextureFormatDepth24Plus  TextureFormat = 40
	TextureFormatDepth32Float TextureFormat = 42
)

// TextureUsage mirrors WGPUTextureUsage flags.
type TextureUsage uint32

const (
	TextureUsageCopySrc          TextureUsage = 0x01
	TextureUsageCopyDst          TextureUsage = 0x02
	TextureUsageTextureBinding   TextureUsage = 0x04
	TextureUsageStorageBinding   TextureUsage = 0x08
	TextureUsageRenderAttachment TextureUsage = 0x10
)

// BufferUsage mirrors WGPUBufferUsage flags.
type BufferUsage uint32

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
	LoadOpClear LoadOp = 1
	LoadOpLoad  LoadOp = 2
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
	BlendFactorZero             BlendFactor = 0
	BlendFactorOne              BlendFactor = 1
	BlendFactorSrcAlpha         BlendFactor = 6
	BlendFactorOneMinusSrcAlpha BlendFactor = 7
)

// BlendOperation mirrors WGPUBlendOperation.
type BlendOperation uint32

const (
	BlendOperationAdd BlendOperation = 0
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
	PrimitiveTopologyTriangleList PrimitiveTopology = 3
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
	Label           uintptr
	Usage           TextureUsage
	Dimension       uint32
	Size            Extent3D
	Format          TextureFormat
	MipLevelCount   uint32
	SampleCount     uint32
	ViewFormatCount uint32
	ViewFormats     uintptr
}

// BufferDescriptor is WGPUBufferDescriptor.
type BufferDescriptor struct {
	NextInChain      uintptr
	Label            uintptr
	Usage            BufferUsage
	Size             uint64
	MappedAtCreation uint32
	_                [4]byte // padding
}

// ImageCopyTexture is WGPUImageCopyTexture.
type ImageCopyTexture struct {
	NextInChain uintptr
	Texture_    Texture
	MipLevel    uint32
	Origin      Origin3D
	Aspect      uint32
}

// ImageCopyBuffer is WGPUImageCopyBuffer.
type ImageCopyBuffer struct {
	NextInChain uintptr
	Layout      TextureDataLayout
	Buffer_     Buffer
}

// TextureDataLayout is WGPUTextureDataLayout.
type TextureDataLayout struct {
	NextInChain  uintptr
	Offset       uint64
	BytesPerRow  uint32
	RowsPerImage uint32
}

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

// RenderPassDescriptor is WGPURenderPassDescriptor.
type RenderPassDescriptor struct {
	NextInChain            uintptr
	Label                  uintptr
	ColorAttachmentCount   uint32
	_                      [4]byte // padding
	ColorAttachments       uintptr
	DepthStencilAttachment uintptr
	OcclusionQuerySet      QuerySet
	TimestampWrites        uintptr
}

// ---------------------------------------------------------------------------
// Private function variables (populated by Init)
// ---------------------------------------------------------------------------

var (
	fnCreateInstance                   func(uintptr) Instance
	fnInstanceRequestAdapter           func(Instance, uintptr, uintptr, uintptr)
	fnAdapterRequestDevice             func(Adapter, uintptr, uintptr, uintptr)
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
	fnQueueSubmit                      func(Queue, uint32, uintptr)
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
	fnQueueSubmit(queue, uint32(len(cmds)), uintptr(unsafe.Pointer(&cmds[0])))
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

// ---------------------------------------------------------------------------
// Init loads libwgpu_native and resolves all function symbols
// ---------------------------------------------------------------------------

// Init loads the wgpu-native shared library and resolves symbols.
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

	lib, err := purego.Dlopen(libName, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("wgpu: failed to load %s: %w", libName, err)
	}

	reg := func(fn interface{}, name string) {
		if err != nil {
			return
		}
		sym, e := purego.Dlsym(lib, name)
		if e != nil {
			err = fmt.Errorf("wgpu: symbol %s: %w", name, e)
			return
		}
		purego.RegisterFunc(fn, sym)
	}

	reg(&fnCreateInstance, "wgpuCreateInstance")
	reg(&fnInstanceRequestAdapter, "wgpuInstanceRequestAdapter")
	reg(&fnAdapterRequestDevice, "wgpuAdapterRequestDevice")
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

	return err
}
