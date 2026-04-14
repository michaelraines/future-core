//go:build darwin && !soft

// Package mtl provides pure-Go bindings to Apple's Metal framework via
// purego and the Objective-C runtime. All calls go through objc_msgSend
// loaded at runtime — no CGo required.
package mtl

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
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
// Sampler filter — MTLSamplerMinMagFilter
// ---------------------------------------------------------------------------

const (
	SamplerMinMagFilterNearest = 0
	SamplerMinMagFilterLinear  = 1
)

// ---------------------------------------------------------------------------
// Vertex format — MTLVertexFormat
// ---------------------------------------------------------------------------

const (
	VertexFormatFloat2 = 29
	VertexFormatFloat3 = 30
	VertexFormatFloat4 = 31
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

	fnObjcMsgSendAddr uintptr // raw address of objc_msgSend
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

	fnObjcMsgSendAddr, err = purego.Dlsym(objcLib, "objc_msgSend")
	if err != nil {
		return fmt.Errorf("mtl: failed to resolve objc_msgSend: %w", err)
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
// Uses alloc/init + individual setters instead of the multi-argument
// texture2DDescriptorWithPixelFormat:width:height:mipmapped: convenience
// method, because purego's variadic msgSend can misalign arguments on arm64.
func DeviceNewTexture(dev Device, desc *TextureDescriptor) Texture {
	cls := getClass("MTLTextureDescriptor")
	tdesc := msgSend(msgSend(uintptr(cls), sel("alloc")), sel("init"))
	msgSend(tdesc, sel("setTextureType:"), 2) // MTLTextureType2D
	msgSend(tdesc, sel("setPixelFormat:"), uintptr(desc.PixelFormat))
	msgSend(tdesc, sel("setWidth:"), uintptr(desc.Width))
	msgSend(tdesc, sel("setHeight:"), uintptr(desc.Height))
	msgSend(tdesc, sel("setDepth:"), 1)
	msgSend(tdesc, sel("setMipmapLevelCount:"), 1)
	msgSend(tdesc, sel("setSampleCount:"), 1)
	msgSend(tdesc, sel("setUsage:"), uintptr(desc.Usage))
	msgSend(tdesc, sel("setStorageMode:"), uintptr(desc.StorageMode))
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

// Typed function pointers for encoder methods with struct parameters.
var (
	fnSetViewport   func(enc uintptr, sel Selector, vp Viewport)
	fnSetScissor    func(enc uintptr, sel Selector, rect ScissorRect)
	encStructFnInit bool
)

var fnSetClearColor func(obj uintptr, sel Selector, color ClearColor)

func initEncStructFns() {
	if encStructFnInit {
		return
	}
	purego.RegisterFunc(&fnSetViewport, fnObjcMsgSendAddr)
	purego.RegisterFunc(&fnSetScissor, fnObjcMsgSendAddr)
	purego.RegisterFunc(&fnSetClearColor, fnObjcMsgSendAddr)
	encStructFnInit = true
}

// SetClearColor sets the clear color on a color attachment descriptor.
func SetClearColor(obj uintptr, color ClearColor) {
	initEncStructFns()
	fnSetClearColor(obj, sel("setClearColor:"), color)
}

// RenderCommandEncoderSetViewport sets the viewport.
func RenderCommandEncoderSetViewport(enc RenderCommandEncoder, vp Viewport) {
	initEncStructFns()
	fnSetViewport(uintptr(enc), selSetViewport, vp)
}

// RenderCommandEncoderSetScissorRect sets the scissor rectangle.
func RenderCommandEncoderSetScissorRect(enc RenderCommandEncoder, rect ScissorRect) {
	initEncStructFns()
	fnSetScissor(uintptr(enc), selSetScissorRect, rect)
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
	objc.ID(enc).Send(objc.SEL(selCopyFromBuffer),
		uintptr(srcBuffer), srcOffset, srcBytesPerRow, srcBytesPerImage,
		srcSize,
		uintptr(dstTexture), dstSlice, dstLevel,
		dstOrigin)
}

// ---------------------------------------------------------------------------
// Texture functions
// ---------------------------------------------------------------------------

// Typed function pointers for ObjC methods with struct parameters.
// purego.RegisterFunc handles arm64 ABI correctly when it can see the full type.
var (
	fnTexReplaceRegion func(tex uintptr, sel Selector, region Region, level uint64, data uintptr, bytesPerRow uint64)
	fnTexGetBytes      func(tex uintptr, sel Selector, dst uintptr, bytesPerRow uint64, region Region, level uint64)
	structFnsReady     bool
)

func initStructFns() {
	if structFnsReady {
		return
	}
	purego.RegisterFunc(&fnTexReplaceRegion, fnObjcMsgSendAddr)
	purego.RegisterFunc(&fnTexGetBytes, fnObjcMsgSendAddr)
	structFnsReady = true
}

// TextureReplaceRegion uploads pixel data directly to a texture (shared/managed storage).
func TextureReplaceRegion(tex Texture, region Region, level uint64, data unsafe.Pointer, bytesPerRow uint64) {
	initStructFns()
	fnTexReplaceRegion(uintptr(tex), selReplaceRegion, region, level, uintptr(data), bytesPerRow)
}

// TextureGetBytes reads pixel data from a texture.
func TextureGetBytes(tex Texture, dst unsafe.Pointer, bytesPerRow uint64, region Region, level uint64) {
	initStructFns()
	fnTexGetBytes(uintptr(tex), selGetBytes, uintptr(dst), bytesPerRow, region, level)
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

// BufferNewTexture creates a texture backed by a buffer's storage.
// This avoids needing getBytes: (which has arm64 struct ABI issues) —
// the CPU can read pixels directly from the buffer's contents pointer.
func BufferNewTexture(buf Buffer, dev Device, pixelFormat int, width, height, bytesPerRow uint64, usage int) Texture {
	// Create a texture descriptor.
	cls := getClass("MTLTextureDescriptor")
	desc := msgSend(msgSend(uintptr(cls), sel("alloc")), sel("init"))
	msgSend(desc, sel("setTextureType:"), 2) // MTLTextureType2D
	msgSend(desc, sel("setPixelFormat:"), uintptr(pixelFormat))
	msgSend(desc, sel("setWidth:"), uintptr(width))
	msgSend(desc, sel("setHeight:"), uintptr(height))
	msgSend(desc, sel("setDepth:"), 1)
	msgSend(desc, sel("setMipmapLevelCount:"), 1)
	msgSend(desc, sel("setSampleCount:"), 1)
	msgSend(desc, sel("setUsage:"), uintptr(usage))
	msgSend(desc, sel("setStorageMode:"), uintptr(StorageModeShared))

	// newTextureWithDescriptor:offset:bytesPerRow:
	tex := Texture(msgSend(uintptr(buf),
		sel("newTextureWithDescriptor:offset:bytesPerRow:"),
		desc, 0, uintptr(bytesPerRow)))
	msgSend(desc, sel("release"))
	return tex
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

// msgSend calls objc_msgSend via purego.SyscallN, which correctly maps
// each argument to an arm64 register. Using purego.RegisterFunc with a
// variadic Go function misaligns arguments on arm64.
func msgSend(obj uintptr, sel Selector, args ...uintptr) uintptr {
	callArgs := make([]uintptr, 0, 2+len(args))
	callArgs = append(callArgs, obj, uintptr(sel))
	callArgs = append(callArgs, args...)
	ret, _, _ := purego.SyscallN(fnObjcMsgSendAddr, callArgs...)
	return ret
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

// ---------------------------------------------------------------------------
// Render pipeline state creation
// ---------------------------------------------------------------------------

// selectors for pipeline creation (cached lazily).
var (
	selNewRenderPipelineStateWithDescriptor Selector
	selSetVertexFunction                    Selector
	selSetFragmentFunction                  Selector
	selSetPixelFormat                       Selector
	selSetBlendingEnabled                   Selector
	selSetSourceRGBBlendFactor              Selector
	selSetDestinationRGBBlendFactor         Selector
	selSetSourceAlphaBlendFactor            Selector
	selSetDestinationAlphaBlendFactor       Selector
	selSetRGBBlendOperation                 Selector
	selSetAlphaBlendOperation               Selector
	selSetRenderPipelineState               Selector
	selSetFragmentTexture                   Selector
	selSetDepthStencilState                 Selector
	selSetCullMode                          Selector

	pipelineSelectorsOnce bool
)

func initPipelineSelectors() {
	if pipelineSelectorsOnce {
		return
	}
	pipelineSelectorsOnce = true
	selNewRenderPipelineStateWithDescriptor = sel("newRenderPipelineStateWithDescriptor:error:")
	selSetVertexFunction = sel("setVertexFunction:")
	selSetFragmentFunction = sel("setFragmentFunction:")
	selSetPixelFormat = sel("setPixelFormat:")
	selSetBlendingEnabled = sel("setBlendingEnabled:")
	selSetSourceRGBBlendFactor = sel("setSourceRGBBlendFactor:")
	selSetDestinationRGBBlendFactor = sel("setDestinationRGBBlendFactor:")
	selSetSourceAlphaBlendFactor = sel("setSourceAlphaBlendFactor:")
	selSetDestinationAlphaBlendFactor = sel("setDestinationAlphaBlendFactor:")
	selSetRGBBlendOperation = sel("setRgbBlendOperation:")
	selSetAlphaBlendOperation = sel("setAlphaBlendOperation:")
	selSetRenderPipelineState = sel("setRenderPipelineState:")
	selSetFragmentTexture = sel("setFragmentTexture:atIndex:")
	selSetDepthStencilState = sel("setDepthStencilState:")
	selSetCullMode = sel("setCullMode:")
}

// MTLBlendFactor constants.
// https://developer.apple.com/documentation/metal/mtlblendfactor
const (
	BlendFactorZero                     = 0
	BlendFactorOne                      = 1
	BlendFactorSourceColor              = 2
	BlendFactorOneMinusSourceColor      = 3
	BlendFactorSourceAlpha              = 4
	BlendFactorOneMinusSourceAlpha      = 5
	BlendFactorDestinationColor         = 8
	BlendFactorOneMinusDestinationColor = 9
	BlendFactorDestinationAlpha         = 10
	BlendFactorOneMinusDestinationAlpha = 11
)

// MTLBlendOperation constants.
// https://developer.apple.com/documentation/metal/mtlblendoperation
const (
	BlendOperationAdd             = 0
	BlendOperationSubtract        = 1
	BlendOperationReverseSubtract = 2
	BlendOperationMin             = 3
	BlendOperationMax             = 4
)

// MTLCullMode constants.
const (
	CullModeNone  = 0
	CullModeFront = 1
	CullModeBack  = 2
)

// MTLPrimitiveType constants.
const (
	PrimitiveTypeTriangle      = 3
	PrimitiveTypeTriangleStrip = 4
	PrimitiveTypeLine          = 1
	PrimitiveTypeLineStrip     = 2
	PrimitiveTypePoint         = 0
)

// DeviceNewLibraryWithSource compiles Metal Shading Language source to a library.
func DeviceNewLibraryWithSource(dev Device, source string) (Library, error) {
	initPipelineSelectors()
	src := nsString(source)
	var errObj uintptr
	lib := Library(msgSend(uintptr(dev), selNewLibraryWithSource, src, 0, uintptr(unsafe.Pointer(&errObj))))
	if lib == 0 {
		return 0, fmt.Errorf("mtl: shader compilation failed")
	}
	return lib, nil
}

// LibraryNewFunctionWithName gets a function from a library.
func LibraryNewFunctionWithName(lib Library, name string) Function {
	initPipelineSelectors()
	nsName := nsString(name)
	return Function(msgSend(uintptr(lib), selNewFunctionWithName, nsName))
}

// VertexAttr describes a vertex attribute for the vertex descriptor.
type VertexAttr struct {
	Format int // MTLVertexFormat (e.g. VertexFormatFloat2)
	Offset int
	Index  int // attribute index
}

// CreateRenderPipelineState creates a render pipeline state from vertex/fragment functions.
// If vertexAttrs is non-empty, a vertex descriptor is configured with the given
// attributes at buffer index 0 with the given stride.
func CreateRenderPipelineState(dev Device, vertexFn, fragmentFn Function, pixelFormat int, blendEnabled bool, srcRGB, dstRGB, srcAlpha, dstAlpha, opRGB, opAlpha int, vertexAttrs []VertexAttr, vertexStride int) (RenderPipelineState, error) {
	initPipelineSelectors()

	// Create MTLRenderPipelineDescriptor.
	cls := getClass("MTLRenderPipelineDescriptor")
	alloc := msgSend(uintptr(cls), sel("alloc"))
	desc := msgSend(alloc, sel("init"))

	// Set functions.
	msgSend(desc, selSetVertexFunction, uintptr(vertexFn))
	msgSend(desc, selSetFragmentFunction, uintptr(fragmentFn))

	// Configure vertex descriptor if attributes are provided.
	if len(vertexAttrs) > 0 {
		vdCls := getClass("MTLVertexDescriptor")
		vd := msgSend(msgSend(uintptr(vdCls), sel("alloc")), sel("init"))

		attrs := msgSend(vd, sel("attributes"))
		for _, a := range vertexAttrs {
			attr := msgSend(attrs, sel("objectAtIndexedSubscript:"), uintptr(a.Index))
			msgSend(attr, sel("setFormat:"), uintptr(a.Format))
			msgSend(attr, sel("setOffset:"), uintptr(a.Offset))
			msgSend(attr, sel("setBufferIndex:"), 0) // buffer slot 0
		}

		layouts := msgSend(vd, sel("layouts"))
		layout0 := msgSend(layouts, sel("objectAtIndexedSubscript:"), 0)
		msgSend(layout0, sel("setStride:"), uintptr(vertexStride))
		msgSend(layout0, sel("setStepFunction:"), 1) // MTLVertexStepFunctionPerVertex

		msgSend(desc, sel("setVertexDescriptor:"), vd)
	}

	// Configure color attachment 0.
	colorAttachments := msgSend(desc, sel("colorAttachments"))
	ca0 := msgSend(colorAttachments, sel("objectAtIndexedSubscript:"), 0)
	msgSend(ca0, selSetPixelFormat, uintptr(pixelFormat))

	if blendEnabled {
		msgSend(ca0, selSetBlendingEnabled, 1)
		msgSend(ca0, selSetSourceRGBBlendFactor, uintptr(srcRGB))
		msgSend(ca0, selSetDestinationRGBBlendFactor, uintptr(dstRGB))
		msgSend(ca0, selSetSourceAlphaBlendFactor, uintptr(srcAlpha))
		msgSend(ca0, selSetDestinationAlphaBlendFactor, uintptr(dstAlpha))
		msgSend(ca0, selSetRGBBlendOperation, uintptr(opRGB))
		msgSend(ca0, selSetAlphaBlendOperation, uintptr(opAlpha))
	}

	// Create pipeline state.
	var errObj uintptr
	pso := RenderPipelineState(msgSend(uintptr(dev), selNewRenderPipelineStateWithDescriptor, desc, uintptr(unsafe.Pointer(&errObj))))

	// Release descriptor.
	msgSend(desc, selRelease)

	if pso == 0 {
		return 0, fmt.Errorf("mtl: failed to create render pipeline state")
	}
	return pso, nil
}

// RenderCommandEncoderSetRenderPipelineState binds a pipeline state.
func RenderCommandEncoderSetRenderPipelineState(enc RenderCommandEncoder, pso RenderPipelineState) {
	initPipelineSelectors()
	msgSend(uintptr(enc), selSetRenderPipelineState, uintptr(pso))
}

// RenderCommandEncoderSetFragmentTexture binds a texture to a fragment shader slot.
func RenderCommandEncoderSetFragmentTexture(enc RenderCommandEncoder, tex Texture, index uint64) {
	initPipelineSelectors()
	msgSend(uintptr(enc), selSetFragmentTexture, uintptr(tex), uintptr(index))
}

// RenderCommandEncoderSetCullMode sets the cull mode.
func RenderCommandEncoderSetCullMode(enc RenderCommandEncoder, mode int) {
	initPipelineSelectors()
	msgSend(uintptr(enc), selSetCullMode, uintptr(mode))
}

// RenderCommandEncoderSetFragmentSamplerState binds a sampler state to a fragment slot.
func RenderCommandEncoderSetFragmentSamplerState(enc RenderCommandEncoder, sampler uintptr, index uint64) {
	msgSend(uintptr(enc), sel("setFragmentSamplerState:atIndex:"), sampler, uintptr(index))
}

// RenderCommandEncoderSetVertexBytes sets inline vertex shader constant data.
func RenderCommandEncoderSetVertexBytes(enc RenderCommandEncoder, data unsafe.Pointer, length, index uint64) {
	msgSend(uintptr(enc), sel("setVertexBytes:length:atIndex:"), uintptr(data), uintptr(length), uintptr(index))
}

// RenderCommandEncoderSetFragmentBytes sets inline fragment shader constant data.
func RenderCommandEncoderSetFragmentBytes(enc RenderCommandEncoder, data unsafe.Pointer, length, index uint64) {
	msgSend(uintptr(enc), sel("setFragmentBytes:length:atIndex:"), uintptr(data), uintptr(length), uintptr(index))
}

// DeviceNewSamplerState creates a sampler state with the given min/mag filter.
func DeviceNewSamplerState(dev Device, minMagFilter int) uintptr {
	cls := getClass("MTLSamplerDescriptor")
	desc := msgSend(msgSend(uintptr(cls), sel("alloc")), sel("init"))
	msgSend(desc, sel("setMinFilter:"), uintptr(minMagFilter))
	msgSend(desc, sel("setMagFilter:"), uintptr(minMagFilter))
	sampler := msgSend(uintptr(dev), sel("newSamplerStateWithDescriptor:"), desc)
	msgSend(desc, sel("release"))
	return sampler
}

// nsString creates an NSString from a Go string.
func nsString(s string) uintptr {
	cls := getClass("NSString")
	cstr_ := cstr(s)
	return msgSend(uintptr(cls), sel("stringWithUTF8String:"), uintptr(unsafe.Pointer(cstr_)))
}

// RenderPipelineStateRelease releases a render pipeline state.
func RenderPipelineStateRelease(pso RenderPipelineState) {
	if pso != 0 {
		msgSend(uintptr(pso), selRelease)
	}
}

// LibraryRelease releases a library.
func LibraryRelease(lib Library) {
	if lib != 0 {
		msgSend(uintptr(lib), selRelease)
	}
}

// Keep compiler happy.
var _ = unsafe.Pointer(nil)
