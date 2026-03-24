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

	// SetUniformVec4 sets a vec4 uniform.
	SetUniformVec4(name string, v [4]float32)

	// SetUniformMat4 sets a mat4 uniform.
	SetUniformMat4(name string, v [16]float32)

	// SetUniformInt sets an int uniform.
	SetUniformInt(name string, v int32)

	// SetUniformBlock sets a uniform block's data.
	SetUniformBlock(name string, data []byte)

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

	// Dispose releases the render target's GPU resources.
	Dispose()
}

// RenderTargetDescriptor describes a render target to be created.
type RenderTargetDescriptor struct {
	Width, Height int
	ColorFormat   TextureFormat
	HasDepth      bool
	DepthFormat   TextureFormat
	SampleCount   int
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

	// SetVertexBuffer binds a vertex buffer at the given slot.
	SetVertexBuffer(buf Buffer, slot int)

	// SetIndexBuffer binds an index buffer.
	SetIndexBuffer(buf Buffer, format IndexFormat)

	// SetTexture binds a texture to a texture slot.
	SetTexture(tex Texture, slot int)

	// SetTextureFilter overrides the texture filter for a texture slot.
	// This uses sampler objects to decouple filter state from the texture.
	SetTextureFilter(slot int, filter TextureFilter)

	// SetStencil configures and enables/disables the stencil test.
	SetStencil(enabled bool, desc StencilDescriptor)

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
	Target      RenderTarget // nil = default framebuffer
	ClearColor  [4]float32   // RGBA clear color
	ClearDepth  float32
	LoadAction  LoadAction
	StoreAction StoreAction
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
