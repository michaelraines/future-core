//go:build metal

// Package mtl provides pure-Go bindings to Apple's Metal framework via
// purego and the Objective-C runtime. All calls go through objc_msgSend
// loaded at runtime — no CGo required.
package mtl

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// ---------------------------------------------------------------------------
// Handle types — opaque Objective-C object pointers
// ---------------------------------------------------------------------------

type (
	// Device is an id<MTLDevice>.
	Device uintptr
	// CommandQueue is an id<MTLCommandQueue>.
	CommandQueue uintptr
	// CommandBuffer is an id<MTLCommandBuffer>.
	CommandBuffer uintptr
	// RenderCommandEncoder is an id<MTLRenderCommandEncoder>.
	RenderCommandEncoder uintptr
	// BlitCommandEncoder is an id<MTLBlitCommandEncoder>.
	BlitCommandEncoder uintptr
	// Texture is an id<MTLTexture>.
	Texture uintptr
	// Buffer is an id<MTLBuffer>.
	Buffer uintptr
	// Library is an id<MTLLibrary>.
	Library uintptr
	// Function is an id<MTLFunction>.
	Function uintptr
	// RenderPipelineState is an id<MTLRenderPipelineState>.
	RenderPipelineState uintptr
	// DepthStencilState is an id<MTLDepthStencilState>.
	DepthStencilState uintptr

	// Selector is an Objective-C SEL.
	Selector uintptr
	// Class is an Objective-C Class.
	Class uintptr
)

// ---------------------------------------------------------------------------
// Pixel format constants — MTLPixelFormat enum
// ---------------------------------------------------------------------------

const (
	PixelFormatInvalid         = 0
	PixelFormatR8Unorm         = 10
	PixelFormatRGBA8Unorm      = 70
	PixelFormatBGRA8Unorm      = 80
	PixelFormatRGBA16Float     = 115
	PixelFormatRGBA32Float     = 125
	PixelFormatDepth32Float    = 252
	PixelFormatDepth24Stencil8 = 255
	PixelFormatDepth32Stencil8 = 260
)

// ---------------------------------------------------------------------------
// Texture usage constants — MTLTextureUsage
// ---------------------------------------------------------------------------

const (
	TextureUsageShaderRead   = 0x0001
	TextureUsageShaderWrite  = 0x0002
	TextureUsageRenderTarget = 0x0004
)

// ---------------------------------------------------------------------------
// Storage mode constants — MTLStorageMode
// ---------------------------------------------------------------------------

const (
	StorageModeShared  = 0
	StorageModeManaged = 1
	StorageModePrivate = 2
)

// ---------------------------------------------------------------------------
// Load action / store action — MTLLoadAction, MTLStoreAction
// ---------------------------------------------------------------------------

const (
	LoadActionDontCare = 0
	LoadActionLoad     = 1
	LoadActionClear    = 2

	StoreActionDontCare = 0
	StoreActionStore    = 1
)

// ---------------------------------------------------------------------------
// Index type — MTLIndexType
// ---------------------------------------------------------------------------

const (
	IndexTypeUInt16 = 0
	IndexTypeUInt32 = 1
)

// ---------------------------------------------------------------------------
// Resource options — MTLResourceOptions
// ---------------------------------------------------------------------------

const (
	ResourceStorageModeShared   = StorageModeShared << 4
	ResourceStorageModeManaged  = StorageModeManaged << 4
	ResourceStorageModePrivate  = StorageModePrivate << 4
	ResourceCPUCacheModeDefault = 0
)

// ---------------------------------------------------------------------------
// C-compatible structs
// ---------------------------------------------------------------------------

// ClearColor mirrors MTLClearColor.
type ClearColor struct {
	Red, Green, Blue, Alpha float64
}

// Origin mirrors MTLOrigin.
type Origin struct {
	X, Y, Z uint64
}

// Size mirrors MTLSize.
type Size struct {
	Width, Height, Depth uint64
}

// Region mirrors MTLRegion for 2D textures.
type Region struct {
	Origin Origin
	Size   Size
}

// Viewport mirrors MTLViewport.
type Viewport struct {
	OriginX, OriginY, Width, Height, ZNear, ZFar float64
}

// ScissorRect mirrors MTLScissorRect.
type ScissorRect struct {
	X, Y, Width, Height uint64
}

// TextureDescriptor holds parameters for texture creation.
type TextureDescriptor struct {
	PixelFormat int
	Width       uint64
	Height      uint64
	Depth       uint64
	MipmapCount uint64
	SampleCount uint64
	StorageMode int
	Usage       int
	TextureType int // 0 = 1D, 1 = 1DArray, 2 = 2D, etc.
}

// RenderPassColorAttachmentDescriptor describes a color attachment.
type RenderPassColorAttachmentDescriptor struct {
	Texture     Texture
	LoadAction  int
	StoreAction int
	ClearColor  ClearColor
}

// ---------------------------------------------------------------------------
// Internal function variables — populated by Init()
// ---------------------------------------------------------------------------

var (
	lib uintptr

	fnObjcMsgSend     func(obj uintptr, sel Selector, args ...uintptr) uintptr
	fnObjcGetClass    func(name *byte) Class
	fnSelRegisterName func(name *byte) Selector

	// Cached selectors for Metal methods.
	selNewCommandQueue          Selector
	selCommandBuffer            Selector
	selRenderCommandEncoder     Selector
	selEndEncoding              Selector
	selCommit                   Selector
	selWaitUntilCompleted       Selector
	selNewTextureWithDescriptor Selector
	selNewBufferWithLength      Selector
	selNewBufferWithBytes       Selector
	selSetVertexBuffer          Selector
	selSetFragmentBuffer        Selector
	selSetVertexBytes           Selector
	selSetFragmentBytes         Selector
	selDrawPrimitives           Selector
	selDrawIndexedPrimitives    Selector
	selSetViewport              Selector
	selSetScissorRect           Selector
	selReplaceRegion            Selector
	selGetBytes                 Selector
	selContents                 Selector
	selLength                   Selector
	selWidth                    Selector
	selHeight                   Selector
	selPixelFormat              Selector
	selRelease                  Selector
	selRetain                   Selector
	selNewLibraryWithSource     Selector
	selNewFunctionWithName      Selector
	selLabel                    Selector
	selSetLabel                 Selector
	selBlitCommandEncoder       Selector
	selCopyFromTexture          Selector
	selCopyFromBuffer           Selector
	selSynchronizeResource      Selector
	selName                     Selector

	// MTLDevice creation function.
	fnMTLCreateSystemDefaultDevice func() Device
)

// ---------------------------------------------------------------------------
// Initialization
// ---------------------------------------------------------------------------

// Init loads the Metal and Objective-C runtime libraries, resolving all
// function pointers and caching commonly-used selectors.
func Init() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("mtl: Metal is only available on macOS/iOS")
	}

	var err error

	// Load Objective-C runtime.
	objcLib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("mtl: failed to load libobjc: %w", err)
	}

	if err := resolveSymbol(objcLib, "objc_msgSend", &fnObjcMsgSend); err != nil {
		return err
	}
	if err := resolveSymbol(objcLib, "objc_getClass", &fnObjcGetClass); err != nil {
		return err
	}
	if err := resolveSymbol(objcLib, "sel_registerName", &fnSelRegisterName); err != nil {
		return err
	}

	// Load Metal framework.
	lib, err = purego.Dlopen("/System/Library/Frameworks/Metal.framework/Metal", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("mtl: failed to load Metal.framework: %w", err)
	}

	if err := resolveSymbol(lib, "MTLCreateSystemDefaultDevice", &fnMTLCreateSystemDefaultDevice); err != nil {
		return err
	}

	// Cache selectors for common Metal methods.
	selNewCommandQueue = sel("newCommandQueue")
	selCommandBuffer = sel("commandBuffer")
	selRenderCommandEncoder = sel("renderCommandEncoderWithDescriptor:")
	selEndEncoding = sel("endEncoding")
	selCommit = sel("commit")
	selWaitUntilCompleted = sel("waitUntilCompleted")
	selNewTextureWithDescriptor = sel("newTextureWithDescriptor:")
	selNewBufferWithLength = sel("newBufferWithLength:options:")
	selNewBufferWithBytes = sel("newBufferWithBytesNoCopy:length:options:deallocator:")
	selSetVertexBuffer = sel("setVertexBuffer:offset:atIndex:")
	selSetFragmentBuffer = sel("setFragmentBuffer:offset:atIndex:")
	selSetVertexBytes = sel("setVertexBytes:length:atIndex:")
	selSetFragmentBytes = sel("setFragmentBytes:length:atIndex:")
	selDrawPrimitives = sel("drawPrimitives:vertexStart:vertexCount:instanceCount:")
	selDrawIndexedPrimitives = sel("drawIndexedPrimitives:indexCount:indexType:indexBuffer:indexBufferOffset:instanceCount:")
	selSetViewport = sel("setViewport:")
	selSetScissorRect = sel("setScissorRect:")
	selReplaceRegion = sel("replaceRegion:mipmapLevel:withBytes:bytesPerRow:")
	selGetBytes = sel("getBytes:bytesPerRow:fromRegion:mipmapLevel:")
	selContents = sel("contents")
	selLength = sel("length")
	selWidth = sel("width")
	selHeight = sel("height")
	selPixelFormat = sel("pixelFormat")
	selRelease = sel("release")
	selRetain = sel("retain")
	selNewLibraryWithSource = sel("newLibraryWithSource:options:error:")
	selNewFunctionWithName = sel("newFunctionWithName:")
	selLabel = sel("label")
	selSetLabel = sel("setLabel:")
	selBlitCommandEncoder = sel("blitCommandEncoder")
	selCopyFromTexture = sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toBuffer:destinationOffset:destinationBytesPerRow:destinationBytesPerImage:")
	selCopyFromBuffer = sel("copyFromBuffer:sourceOffset:sourceBytesPerRow:sourceBytesPerImage:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:")
	selSynchronizeResource = sel("synchronizeResource:")
	selName = sel("name")

	return nil
}

// ---------------------------------------------------------------------------
// Device functions
// ---------------------------------------------------------------------------

// CreateSystemDefaultDevice returns the system's default Metal device.
func CreateSystemDefaultDevice() Device {
	return fnMTLCreateSystemDefaultDevice()
}

// DeviceNewCommandQueue creates a new command queue.
func DeviceNewCommandQueue(dev Device) CommandQueue {
	return CommandQueue(msgSend(uintptr(dev), selNewCommandQueue))
}

// DeviceNewTexture creates a new texture from a descriptor.
// The caller must create an MTLTextureDescriptor ObjC object — for simplicity
// we use the raw struct approach with replaceRegion for data upload.
func DeviceNewTexture(dev Device, desc *TextureDescriptor) Texture {
	// Create an MTLTextureDescriptor via the ObjC runtime.
	cls := getClass("MTLTextureDescriptor")
	tdesc := msgSend(uintptr(cls), sel("texture2DDescriptorWithPixelFormat:width:height:mipmapped:"),
		uintptr(desc.PixelFormat), uintptr(desc.Width), uintptr(desc.Height), 0)
	// Set usage.
	msgSend(tdesc, sel("setUsage:"), uintptr(desc.Usage))
	// Set storage mode.
	msgSend(tdesc, sel("setStorageMode:"), uintptr(desc.StorageMode))
	// Create texture.
	return Texture(msgSend(uintptr(dev), selNewTextureWithDescriptor, tdesc))
}

// DeviceNewBuffer creates a new buffer with the given length and options.
func DeviceNewBuffer(dev Device, length uint64, options uint64) Buffer {
	return Buffer(msgSend(uintptr(dev), selNewBufferWithLength, uintptr(length), uintptr(options)))
}

// DeviceName returns the device name.
func DeviceName(dev Device) string {
	namePtr := msgSend(uintptr(dev), selName)
	if namePtr == 0 {
		return ""
	}
	// namePtr is an NSString; get UTF8 C string.
	cstr := msgSend(namePtr, sel("UTF8String"))
	if cstr == 0 {
		return ""
	}
	return goString(cstr)
}

// ---------------------------------------------------------------------------
// Command queue / buffer functions
// ---------------------------------------------------------------------------

// CommandQueueCommandBuffer creates a new command buffer from the queue.
func CommandQueueCommandBuffer(queue CommandQueue) CommandBuffer {
	return CommandBuffer(msgSend(uintptr(queue), selCommandBuffer))
}

// CommandBufferCommit commits the command buffer for execution.
func CommandBufferCommit(buf CommandBuffer) {
	msgSend(uintptr(buf), selCommit)
}

// CommandBufferWaitUntilCompleted blocks until the command buffer finishes.
func CommandBufferWaitUntilCompleted(buf CommandBuffer) {
	msgSend(uintptr(buf), selWaitUntilCompleted)
}

// CommandBufferRenderCommandEncoder creates a render command encoder.
func CommandBufferRenderCommandEncoder(buf CommandBuffer, desc uintptr) RenderCommandEncoder {
	return RenderCommandEncoder(msgSend(uintptr(buf), selRenderCommandEncoder, desc))
}

// CommandBufferBlitCommandEncoder creates a blit command encoder.
func CommandBufferBlitCommandEncoder(buf CommandBuffer) BlitCommandEncoder {
	return BlitCommandEncoder(msgSend(uintptr(buf), selBlitCommandEncoder))
}

// ---------------------------------------------------------------------------
// Render command encoder functions
// ---------------------------------------------------------------------------

// RenderCommandEncoderEndEncoding ends encoding.
func RenderCommandEncoderEndEncoding(enc RenderCommandEncoder) {
	msgSend(uintptr(enc), selEndEncoding)
}

// RenderCommandEncoderSetViewport sets the viewport.
func RenderCommandEncoderSetViewport(enc RenderCommandEncoder, vp Viewport) {
	msgSend(uintptr(enc), selSetViewport, *(*uintptr)(unsafe.Pointer(&vp)))
}

// RenderCommandEncoderSetScissorRect sets the scissor rectangle.
func RenderCommandEncoderSetScissorRect(enc RenderCommandEncoder, rect ScissorRect) {
	msgSend(uintptr(enc), selSetScissorRect, *(*uintptr)(unsafe.Pointer(&rect)))
}

// RenderCommandEncoderSetVertexBuffer binds a vertex buffer.
func RenderCommandEncoderSetVertexBuffer(enc RenderCommandEncoder, buf Buffer, offset, index uint64) {
	msgSend(uintptr(enc), selSetVertexBuffer, uintptr(buf), uintptr(offset), uintptr(index))
}

// RenderCommandEncoderDrawPrimitives issues a draw call.
func RenderCommandEncoderDrawPrimitives(enc RenderCommandEncoder, primType, vertexStart, vertexCount, instanceCount uint64) {
	msgSend(uintptr(enc), selDrawPrimitives, uintptr(primType), uintptr(vertexStart), uintptr(vertexCount), uintptr(instanceCount))
}

// RenderCommandEncoderDrawIndexedPrimitives issues an indexed draw call.
func RenderCommandEncoderDrawIndexedPrimitives(enc RenderCommandEncoder, primType, indexCount, indexType uint64, indexBuffer Buffer, indexBufferOffset, instanceCount uint64) {
	msgSend(uintptr(enc), selDrawIndexedPrimitives, uintptr(primType), uintptr(indexCount), uintptr(indexType), uintptr(indexBuffer), uintptr(indexBufferOffset), uintptr(instanceCount))
}

// ---------------------------------------------------------------------------
// Blit encoder functions
// ---------------------------------------------------------------------------

// BlitCommandEncoderEndEncoding ends the blit encoder.
func BlitCommandEncoderEndEncoding(enc BlitCommandEncoder) {
	msgSend(uintptr(enc), selEndEncoding)
}

// BlitCommandEncoderSynchronizeResource synchronizes a managed resource.
func BlitCommandEncoderSynchronizeResource(enc BlitCommandEncoder, resource uintptr) {
	msgSend(uintptr(enc), selSynchronizeResource, resource)
}

// BlitCommandEncoderCopyFromBufferToTexture copies data from a buffer to a texture.
func BlitCommandEncoderCopyFromBufferToTexture(enc BlitCommandEncoder, srcBuffer Buffer, srcOffset, srcBytesPerRow, srcBytesPerImage uint64, srcSize Size, dstTexture Texture, dstSlice, dstLevel uint64, dstOrigin Origin) {
	msgSend(uintptr(enc), selCopyFromBuffer,
		uintptr(srcBuffer), uintptr(srcOffset), uintptr(srcBytesPerRow), uintptr(srcBytesPerImage),
		*(*uintptr)(unsafe.Pointer(&srcSize)),
		uintptr(dstTexture), uintptr(dstSlice), uintptr(dstLevel),
		*(*uintptr)(unsafe.Pointer(&dstOrigin)))
}

// ---------------------------------------------------------------------------
// Texture functions
// ---------------------------------------------------------------------------

// TextureReplaceRegion uploads pixel data directly to a texture (shared/managed storage).
func TextureReplaceRegion(tex Texture, region Region, level uint64, data unsafe.Pointer, bytesPerRow uint64) {
	msgSend(uintptr(tex), selReplaceRegion,
		*(*uintptr)(unsafe.Pointer(&region)),
		uintptr(level),
		uintptr(data),
		uintptr(bytesPerRow))
}

// TextureGetBytes reads pixel data from a texture.
func TextureGetBytes(tex Texture, dst unsafe.Pointer, bytesPerRow uint64, region Region, level uint64) {
	msgSend(uintptr(tex), selGetBytes,
		uintptr(dst),
		uintptr(bytesPerRow),
		*(*uintptr)(unsafe.Pointer(&region)),
		uintptr(level))
}

// TextureWidth returns the texture width.
func TextureWidth(tex Texture) uint64 {
	return uint64(msgSend(uintptr(tex), selWidth))
}

// TextureHeight returns the texture height.
func TextureHeight(tex Texture) uint64 {
	return uint64(msgSend(uintptr(tex), selHeight))
}

// TextureRelease releases a texture.
func TextureRelease(tex Texture) {
	msgSend(uintptr(tex), selRelease)
}

// ---------------------------------------------------------------------------
// Buffer functions
// ---------------------------------------------------------------------------

// BufferContents returns the CPU-accessible pointer for a shared/managed buffer.
func BufferContents(buf Buffer) uintptr {
	return msgSend(uintptr(buf), selContents)
}

// BufferLength returns the buffer length in bytes.
func BufferLength(buf Buffer) uint64 {
	return uint64(msgSend(uintptr(buf), selLength))
}

// BufferRelease releases a buffer.
func BufferRelease(buf Buffer) {
	msgSend(uintptr(buf), selRelease)
}

// ---------------------------------------------------------------------------
// General ObjC resource management
// ---------------------------------------------------------------------------

// Release sends the release message to an Objective-C object.
func Release(obj uintptr) {
	if obj != 0 {
		msgSend(obj, selRelease)
	}
}

// ---------------------------------------------------------------------------
// Exported helpers for backend packages
// ---------------------------------------------------------------------------

// MsgSend sends an Objective-C message. Exported for use by the metal backend
// package for ObjC runtime calls not covered by typed wrappers.
func MsgSend(obj uintptr, s Selector, args ...uintptr) uintptr {
	return msgSend(obj, s, args...)
}

// Sel creates a selector from a name string.
func Sel(name string) Selector {
	return sel(name)
}

// GetClass returns an Objective-C class by name.
func GetClass(name string) Class {
	return getClass(name)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// msgSend wraps objc_msgSend.
func msgSend(obj uintptr, sel Selector, args ...uintptr) uintptr {
	switch len(args) {
	case 0:
		return fnObjcMsgSend(obj, sel)
	case 1:
		return fnObjcMsgSend(obj, sel, args[0])
	case 2:
		return fnObjcMsgSend(obj, sel, args[0], args[1])
	case 3:
		return fnObjcMsgSend(obj, sel, args[0], args[1], args[2])
	case 4:
		return fnObjcMsgSend(obj, sel, args[0], args[1], args[2], args[3])
	case 5:
		return fnObjcMsgSend(obj, sel, args[0], args[1], args[2], args[3], args[4])
	case 6:
		return fnObjcMsgSend(obj, sel, args[0], args[1], args[2], args[3], args[4], args[5])
	default:
		return fnObjcMsgSend(obj, sel, args...)
	}
}

// sel creates a selector from a Go string.
func sel(name string) Selector {
	b := cstr(name)
	return fnSelRegisterName(b)
}

// getClass returns an Objective-C class by name.
func getClass(name string) Class {
	b := cstr(name)
	return fnObjcGetClass(b)
}

// cstr converts a Go string to a null-terminated C string.
func cstr(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// goString converts a C string pointer to a Go string.
func goString(p uintptr) string {
	if p == 0 {
		return ""
	}
	var length int
	for {
		b := *(*byte)(unsafe.Pointer(p + uintptr(length)))
		if b == 0 {
			break
		}
		length++
		if length > 4096 {
			break
		}
	}
	buf := make([]byte, length)
	for i := range buf {
		buf[i] = *(*byte)(unsafe.Pointer(p + uintptr(i)))
	}
	return string(buf)
}

// resolveSymbol loads a symbol from a library into a function pointer.
func resolveSymbol(handle uintptr, name string, fn interface{}) error {
	sym, err := purego.Dlsym(handle, name)
	if err != nil {
		return fmt.Errorf("mtl: failed to resolve %s: %w", name, err)
	}
	purego.RegisterFunc(fn, sym)
	return nil
}

// Keep compiler happy.
var _ = unsafe.Pointer(nil)
