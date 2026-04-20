package backend

import "image"

// Device represents a graphics device (GPU context). This is the primary
// entry point for creating GPU resources. Implementations exist per-backend
// (OpenGL, Metal, Vulkan, etc.).
type Device interface {
	// Init initializes the device with the given window handle and configuration.
	Init(cfg DeviceConfig) error

	// Dispose releases all device resources.
	Dispose()

	// BeginFrame prepares the device for a new frame of rendering.
	BeginFrame()

	// EndFrame finalizes the frame and presents it to the screen.
	EndFrame()

	// NewTexture creates a new texture with the given descriptor.
	NewTexture(desc TextureDescriptor) (Texture, error)

	// NewBuffer creates a new GPU buffer with the given descriptor.
	NewBuffer(desc BufferDescriptor) (Buffer, error)

	// NewShader compiles and creates a shader program from source.
	NewShader(desc ShaderDescriptor) (Shader, error)

	// NewRenderTarget creates a new render target (framebuffer).
	NewRenderTarget(desc RenderTargetDescriptor) (RenderTarget, error)

	// NewPipeline creates a new render pipeline state.
	NewPipeline(desc PipelineDescriptor) (Pipeline, error)

	// Capabilities returns the capabilities of this device.
	Capabilities() DeviceCapabilities

	// Encoder returns the command encoder for recording rendering commands.
	Encoder() CommandEncoder

	// ReadScreen copies the rendered screen pixels (RGBA, width*height*4 bytes)
	// into dst. Returns true if pixels were copied. Returns false if the
	// backend renders directly to the display surface (e.g. OpenGL) and no
	// copy is needed.
	ReadScreen(dst []byte) bool
}

// DeviceConfig holds configuration for device initialization.
type DeviceConfig struct {
	// WindowHandle is a platform-specific window handle.
	WindowHandle uintptr

	// Width and Height are the initial framebuffer dimensions.
	Width, Height int

	// VSync enables vertical synchronization.
	VSync bool

	// SampleCount is the MSAA sample count (1 = no MSAA).
	SampleCount int

	// Debug enables GPU debug/validation layers.
	Debug bool

	// SurfaceFactory creates a presentation surface for the given graphics
	// API instance handle. For Vulkan, the parameter is a VkInstance and the
	// return value is a VkSurfaceKHR. If nil, the backend renders to an
	// offscreen target and relies on ReadScreen + GL presenter.
	SurfaceFactory func(instance uintptr) (surface uintptr, err error)

	// MetalLayer is a CAMetalLayer pointer for creating a WebGPU surface
	// on macOS. Used by the WebGPU backend's InstanceCreateSurface. If 0,
	// the WebGPU backend falls back to offscreen rendering.
	MetalLayer uintptr
}

// DeviceCapabilities reports what the device supports.
type DeviceCapabilities struct {
	MaxTextureSize    int
	MaxRenderTargets  int
	SupportsInstanced bool
	SupportsCompute   bool
	SupportsMSAA      bool
	MaxMSAASamples    int
	SupportsFloat16   bool
	// SupportsStencil is true when the device has a working stencil path
	// (attachment + pipeline ops + dynamic reference value). The sprite pass
	// only routes FillRuleNonZero/FillRuleEvenOdd batches through the stencil
	// draw path when both the device and the current render target have
	// stencil support; otherwise it falls back to a plain indexed draw.
	SupportsStencil bool
}

// Texture represents a GPU texture resource.
type Texture interface {
	// Upload uploads pixel data to the texture.
	Upload(data []byte, level int)

	// UploadRegion uploads pixel data to a rectangular region.
	UploadRegion(data []byte, x, y, width, height, level int)

	// ReadPixels reads RGBA pixel data from the texture into dst.
	// dst must be at least 4*width*height bytes.
	ReadPixels(dst []byte)

	// Width returns the texture width.
	Width() int

	// Height returns the texture height.
	Height() int

	// Format returns the texture format.
	Format() TextureFormat

	// Dispose releases the texture's GPU resources.
	Dispose()
}

// TextureDescriptor describes a texture to be created.
type TextureDescriptor struct {
	Width, Height int
	Format        TextureFormat
	Filter        TextureFilter
	WrapU, WrapV  TextureWrap
	MipMapped     bool
	RenderTarget  bool        // can this texture be used as a render target attachment?
	Data          []byte      // optional initial data
	Image         *image.RGBA // optional initial image
	// Label is a human-readable name shown in WebGPU/Vulkan validation
	// errors and debugger captures (e.g. "sprite-atlas-page-0",
	// "aa-buffer-for-frame-bg"). Optional — empty means unlabeled.
	// Costs nothing at runtime since backends that don't expose a
	// labeling facility just ignore it.
	Label string
}

// Buffer represents a GPU buffer (vertex or index data).
type Buffer interface {
	// Upload uploads data to the buffer.
	Upload(data []byte)

	// UploadRegion uploads data to a region of the buffer.
	UploadRegion(data []byte, offset int)

	// Size returns the buffer size in bytes.
	Size() int

	// Dispose releases the buffer's GPU resources.
	Dispose()
}

// BufferDescriptor describes a buffer to be created.
type BufferDescriptor struct {
	Size    int
	Usage   BufferUsage
	Dynamic bool   // hint: buffer will be updated frequently
	Data    []byte // optional initial data
	// Label is a human-readable name shown in WebGPU/Vulkan validation
	// errors and debugger captures (e.g. "sprite-pass-vertex",
	// "uniform-ring"). Optional — empty means unlabeled.
	Label string
}

// BufferUsage specifies how a buffer will be used.
type BufferUsage int

// BufferUsage constants.
const (
	BufferUsageVertex BufferUsage = iota
	BufferUsageIndex
	BufferUsageUniform
)

// Shader represents a compiled shader program.
type Shader interface {
	// SetUniformFloat sets a float uniform.
	SetUniformFloat(name string, v float32)

	// SetUniformVec2 sets a vec2 uniform.
	SetUniformVec2(name string, v [2]float32)

	// SetUniformVec3 sets a vec3 uniform. The three floats occupy the
	// declared 12 bytes of the vec3 slot in the uniform struct; callers
	// that historically padded to vec4 and used SetUniformVec4 would
	// corrupt the following struct member (because WGSL packs a scalar
	// after a vec3 at offset+12, not offset+16).
	SetUniformVec3(name string, v [3]float32)

	// SetUniformVec4 sets a vec4 uniform.
	SetUniformVec4(name string, v [4]float32)

	// SetUniformMat4 sets a mat4 uniform.
	SetUniformMat4(name string, v [16]float32)

	// SetUniformInt sets an int uniform.
	SetUniformInt(name string, v int32)

	// SetUniformBlock sets a uniform block's data.
	SetUniformBlock(name string, data []byte)

	// PackCurrentUniforms returns a byte snapshot of the shader's current
	// uniform values, packed according to the backend's uniform layout.
	// Returns nil if the shader has no uniforms. Used by DrawRectShader/
	// DrawTrianglesShader to snapshot per-draw uniforms before the sprite
	// pass runs (preventing later draws from overwriting earlier ones).
	PackCurrentUniforms() []byte

	// Dispose releases the shader's GPU resources.
	Dispose()
}

// ShaderDescriptor describes a shader program to be created.
type ShaderDescriptor struct {
	VertexSource   string
	FragmentSource string

	// Attributes declares the vertex attributes this shader expects.
	Attributes []VertexAttribute
}

// RenderTarget represents an off-screen render target (framebuffer).
type RenderTarget interface {
	// ColorTexture returns the color attachment texture.
	ColorTexture() Texture

	// DepthTexture returns the depth attachment texture, if any.
	DepthTexture() Texture

	// Width returns the render target width.
	Width() int

	// Height returns the render target height.
	Height() int

	// HasStencil reports whether the render target carries a stencil
	// attachment. Consumers (e.g. the sprite pass) combine this with
	// DeviceCapabilities.SupportsStencil to decide whether fill-rule
	// batches can be routed through the stencil draw path on this target.
	HasStencil() bool

	// Dispose releases the render target's GPU resources.
	Dispose()
}

// RenderTargetDescriptor describes a render target to be created.
type RenderTargetDescriptor struct {
	Width, Height int
	ColorFormat   TextureFormat
	HasDepth      bool
	DepthFormat   TextureFormat
	// HasStencil indicates whether the render target needs a stencil
	// attachment. On GPUs that require packed depth-stencil attachments
	// (most 2D GPUs), setting HasStencil implies a packed depth+stencil
	// texture is allocated — the depth half is unused if HasDepth is false
	// but the packing is required by the API.
	HasStencil  bool
	SampleCount int
	// Label is a human-readable name shown in WebGPU/Vulkan validation
	// errors and debugger captures (e.g. "image-rt-<textureID>",
	// "aa-buffer-rt"). Optional — empty means unlabeled.
	Label string
}

// Pipeline represents a configured render pipeline state.
// This bundles shader, vertex format, blend mode, and other state into
// a single object that can be bound efficiently.
type Pipeline interface {
	// Dispose releases the pipeline's GPU resources.
	Dispose()
}

// PipelineDescriptor describes a render pipeline to be created.
type PipelineDescriptor struct {
	Shader       Shader
	VertexFormat VertexFormat
	BlendMode    BlendMode
	DepthTest    bool
	DepthWrite   bool
	DepthFunc    CompareFunc
	CullMode     CullMode
	Primitive    PrimitiveType

	// StencilEnable enables the stencil test for this pipeline. On
	// pipeline-baked backends (WebGPU, Vulkan, Metal, DX12) the Stencil
	// ops/func/masks are compiled into the pipeline object; the reference
	// value is dynamic and set via CommandEncoder.SetStencilReference at
	// draw time. On GL-style backends (OpenGL, WebGL, soft) the pipeline's
	// Stencil is applied eagerly in SetPipeline.
	StencilEnable bool
	// Stencil holds the baked stencil state. Ignored when StencilEnable
	// is false.
	Stencil StencilDescriptor
	// DepthStencilFormat is the format of the depth-stencil attachment
	// the pipeline will render against. Must match the bound render
	// target's attachment for pipeline-native backends (WebGPU, Vulkan,
	// Metal, DX12). Zero value is fine for pipelines without a depth or
	// stencil attachment.
	DepthStencilFormat TextureFormat

	// ColorWriteDisabled masks out all color channel writes on this
	// pipeline. Used by stencil-only passes (e.g. the sprite pass's
	// fill-rule stencil-write pipelines) so that the draw updates the
	// stencil buffer without touching color. Baked into pipeline state
	// on WebGPU/Vulkan/Metal/DX12 where per-draw color-write toggling
	// requires a pipeline swap anyway; GL-style backends apply it
	// eagerly in SetPipeline via glColorMask.
	ColorWriteDisabled bool
}

// CommandEncoder records rendering commands for a single render pass.
// This is the primary interface for issuing draw calls.
type CommandEncoder interface {
	// BeginRenderPass begins a render pass to the given target.
	// If target is nil, renders to the default framebuffer (screen).
	BeginRenderPass(desc RenderPassDescriptor)

	// EndRenderPass ends the current render pass.
	EndRenderPass()

	// SetPipeline binds a render pipeline.
	SetPipeline(pipeline Pipeline)

	// SetBlendMode sets the blend mode for subsequent draws. On backends
	// where blend is baked into the pipeline (WebGPU), this triggers a
	// pipeline recreation if the mode differs from the pipeline's current
	// blend. No-op for backends that handle blend as mutable state.
	SetBlendMode(mode BlendMode)

	// SetVertexBuffer binds a vertex buffer at the given slot.
	SetVertexBuffer(buf Buffer, slot int)

	// SetIndexBuffer binds an index buffer.
	SetIndexBuffer(buf Buffer, format IndexFormat)

	// SetTexture binds a texture to a texture slot.
	SetTexture(tex Texture, slot int)

	// SetTextureFilter overrides the texture filter for a texture slot.
	// This uses sampler objects to decouple filter state from the texture.
	SetTextureFilter(slot int, filter TextureFilter)

	// SetStencilReference updates the dynamic stencil reference value used
	// by the currently-bound pipeline's stencil test. The stencil enable
	// flag, ops, compare func, and masks are baked into the pipeline via
	// PipelineDescriptor.StencilEnable/Stencil. No-op on backends that do
	// not support stencil (Capabilities.SupportsStencil == false).
	SetStencilReference(ref uint32)

	// SetColorWrite enables or disables writing to the color buffer.
	SetColorWrite(enabled bool)

	// SetViewport sets the rendering viewport.
	SetViewport(vp Viewport)

	// SetScissor sets the scissor rectangle. Pass nil to disable scissor test.
	SetScissor(rect *ScissorRect)

	// Draw issues a non-indexed draw call.
	Draw(vertexCount, instanceCount, firstVertex int)

	// DrawIndexed issues an indexed draw call.
	DrawIndexed(indexCount, instanceCount, firstIndex int)

	// Flush submits all recorded commands to the GPU.
	Flush()
}

// RenderPassDescriptor describes a render pass.
type RenderPassDescriptor struct {
	Target       RenderTarget // nil = default framebuffer
	ClearColor   [4]float32   // RGBA clear color
	ClearDepth   float32
	ClearStencil uint32
	LoadAction   LoadAction
	StoreAction  StoreAction
	// StencilLoadAction controls the stencil attachment's load op when the
	// target has stencil. Zero value (LoadActionClear) is safe for RTs
	// without a stencil attachment since the field is ignored there.
	StencilLoadAction  LoadAction
	StencilStoreAction StoreAction
}

// LoadAction specifies what happens to render target contents at pass start.
type LoadAction int

// LoadAction constants.
const (
	LoadActionClear    LoadAction = iota // Clear to ClearColor/ClearDepth
	LoadActionLoad                       // Preserve existing contents
	LoadActionDontCare                   // Contents are undefined
)

// StoreAction specifies what happens to render target contents at pass end.
type StoreAction int

// StoreAction constants.
const (
	StoreActionStore    StoreAction = iota // Preserve rendered contents
	StoreActionDontCare                    // Contents may be discarded
)
