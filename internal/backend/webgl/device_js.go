//go:build js

package webgl

import (
	"fmt"
	"syscall/js"

	"github.com/michaelraines/future-render/internal/backend"
)

// Device implements backend.Device for WebGL2 using syscall/js.
type Device struct {
	canvas js.Value
	gl     js.Value

	width  int
	height int

	contextAttrs ContextAttributes

	// Default framebuffer state.
	defaultRT *RenderTarget
}

// New creates a new WebGL2 device.
func New() *Device {
	return &Device{
		contextAttrs: DefaultContextAttributes(),
	}
}

// Init initializes the WebGL2 device by acquiring a canvas and WebGL2 context.
func (d *Device) Init(cfg backend.DeviceConfig) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("webgl: invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
	d.width = cfg.Width
	d.height = cfg.Height

	doc := js.Global().Get("document")
	if doc.IsUndefined() || doc.IsNull() {
		return fmt.Errorf("webgl: document not available")
	}

	d.canvas = doc.Call("createElement", "canvas")
	d.canvas.Set("width", d.width)
	d.canvas.Set("height", d.height)

	attrs := js.Global().Get("Object").New()
	attrs.Set("alpha", d.contextAttrs.Alpha)
	attrs.Set("depth", d.contextAttrs.Depth)
	attrs.Set("stencil", d.contextAttrs.Stencil)
	attrs.Set("antialias", d.contextAttrs.Antialias)
	attrs.Set("premultipliedAlpha", d.contextAttrs.PremultipliedAlpha)
	attrs.Set("preserveDrawingBuffer", d.contextAttrs.PreserveDrawingBuffer)
	attrs.Set("powerPreference", d.contextAttrs.PowerPreference)

	d.gl = d.canvas.Call("getContext", "webgl2", attrs)
	if d.gl.IsNull() || d.gl.IsUndefined() {
		return fmt.Errorf("webgl: WebGL2 context not available")
	}

	// Enable standard defaults.
	d.gl.Call("enable", d.gl.Get("BLEND").Int())
	d.gl.Call("blendFunc",
		d.gl.Get("SRC_ALPHA").Int(),
		d.gl.Get("ONE_MINUS_SRC_ALPHA").Int(),
	)

	d.gl.Call("viewport", 0, 0, d.width, d.height)

	return nil
}

// Dispose releases WebGL2 resources.
func (d *Device) Dispose() {
	if d.defaultRT != nil {
		d.defaultRT.Dispose()
		d.defaultRT = nil
	}
	// WebGL context is garbage-collected by the browser.
}

// BeginFrame prepares for a new frame.
func (d *Device) BeginFrame() {}

// EndFrame finalizes the frame.
func (d *Device) EndFrame() {
	d.gl.Call("flush")
}

// NewTexture creates a WebGL2 texture.
func (d *Device) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("webgl: invalid texture dimensions %dx%d", desc.Width, desc.Height)
	}

	glTex := d.gl.Call("createTexture")
	if glTex.IsNull() || glTex.IsUndefined() {
		return nil, fmt.Errorf("webgl: failed to create texture")
	}

	tex2D := d.gl.Get("TEXTURE_2D").Int()
	d.gl.Call("bindTexture", tex2D, glTex)

	// Set default sampling parameters.
	d.gl.Call("texParameteri", tex2D,
		d.gl.Get("TEXTURE_MIN_FILTER").Int(), d.gl.Get("NEAREST").Int())
	d.gl.Call("texParameteri", tex2D,
		d.gl.Get("TEXTURE_MAG_FILTER").Int(), d.gl.Get("NEAREST").Int())
	d.gl.Call("texParameteri", tex2D,
		d.gl.Get("TEXTURE_WRAP_S").Int(), d.gl.Get("CLAMP_TO_EDGE").Int())
	d.gl.Call("texParameteri", tex2D,
		d.gl.Get("TEXTURE_WRAP_T").Int(), d.gl.Get("CLAMP_TO_EDGE").Int())

	internalFmt := glInternalFormat(d.gl, desc.Format)
	glFmt := glBaseFormat(d.gl, desc.Format)
	glType := glPixelType(d.gl, desc.Format)

	if len(desc.Data) > 0 {
		arr := js.Global().Get("Uint8Array").New(len(desc.Data))
		js.CopyBytesToJS(arr, desc.Data)
		d.gl.Call("texImage2D", tex2D, 0, internalFmt,
			desc.Width, desc.Height, 0, glFmt, glType, arr)
	} else {
		d.gl.Call("texImage2D", tex2D, 0, internalFmt,
			desc.Width, desc.Height, 0, glFmt, glType, js.Null())
	}

	d.gl.Call("bindTexture", tex2D, js.Null())

	return &Texture{
		gl:     d.gl,
		handle: glTex,
		w:      desc.Width,
		h:      desc.Height,
		format: desc.Format,
	}, nil
}

// NewBuffer creates a WebGL2 buffer.
func (d *Device) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	size := desc.Size
	if len(desc.Data) > 0 {
		size = len(desc.Data)
	}
	if size <= 0 {
		return nil, fmt.Errorf("webgl: invalid buffer size %d", size)
	}

	glBuf := d.gl.Call("createBuffer")
	if glBuf.IsNull() || glBuf.IsUndefined() {
		return nil, fmt.Errorf("webgl: failed to create buffer")
	}

	target := glBufferTarget(d.gl, desc.Usage)
	d.gl.Call("bindBuffer", target, glBuf)

	if len(desc.Data) > 0 {
		arr := js.Global().Get("Uint8Array").New(len(desc.Data))
		js.CopyBytesToJS(arr, desc.Data)
		d.gl.Call("bufferData", target, arr, d.gl.Get("DYNAMIC_DRAW").Int())
	} else {
		d.gl.Call("bufferData", target, size, d.gl.Get("DYNAMIC_DRAW").Int())
	}

	d.gl.Call("bindBuffer", target, js.Null())

	return &Buffer{
		gl:     d.gl,
		handle: glBuf,
		size:   size,
		usage:  desc.Usage,
	}, nil
}

// NewShader creates a WebGL2 shader program.
func (d *Device) NewShader(desc backend.ShaderDescriptor) (backend.Shader, error) {
	return &Shader{
		gl:             d.gl,
		vertexSource:   translateGLSLES(desc.VertexSource),
		fragmentSource: translateGLSLES(desc.FragmentSource),
		attributes:     desc.Attributes,
		uniforms:       make(map[string]interface{}),
	}, nil
}

// NewRenderTarget creates a WebGL2 framebuffer object.
func (d *Device) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("webgl: invalid render target dimensions %dx%d", desc.Width, desc.Height)
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
		return nil, fmt.Errorf("webgl: render target color: %w", err)
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
			return nil, fmt.Errorf("webgl: render target depth: %w", err)
		}
		depthTex = dt
	}

	fbo := d.gl.Call("createFramebuffer")
	d.gl.Call("bindFramebuffer", d.gl.Get("FRAMEBUFFER").Int(), fbo)

	colorAttachment := d.gl.Get("COLOR_ATTACHMENT0").Int()
	tex2D := d.gl.Get("TEXTURE_2D").Int()
	d.gl.Call("framebufferTexture2D",
		d.gl.Get("FRAMEBUFFER").Int(), colorAttachment, tex2D,
		colorTex.(*Texture).handle, 0)

	if depthTex != nil {
		depthAttachment := d.gl.Get("DEPTH_ATTACHMENT").Int()
		d.gl.Call("framebufferTexture2D",
			d.gl.Get("FRAMEBUFFER").Int(), depthAttachment, tex2D,
			depthTex.(*Texture).handle, 0)
	}

	d.gl.Call("bindFramebuffer", d.gl.Get("FRAMEBUFFER").Int(), js.Null())

	return &RenderTarget{
		gl:       d.gl,
		fbo:      fbo,
		colorTex: colorTex.(*Texture),
		depthTex: depthTex,
		w:        desc.Width,
		h:        desc.Height,
	}, nil
}

// NewPipeline creates a WebGL2 pipeline state.
func (d *Device) NewPipeline(desc backend.PipelineDescriptor) (backend.Pipeline, error) {
	return &Pipeline{
		gl:   d.gl,
		desc: desc,
	}, nil
}

// Capabilities returns WebGL2 device capabilities.
func (d *Device) Capabilities() backend.DeviceCapabilities {
	return backend.DeviceCapabilities{
		MaxTextureSize:    4096,
		MaxRenderTargets:  4,
		SupportsInstanced: true,
		SupportsCompute:   false,
		SupportsMSAA:      true,
		MaxMSAASamples:    4,
		SupportsFloat16:   false,
	}
}

// Encoder returns the command encoder.
func (d *Device) Encoder() backend.CommandEncoder {
	return &Encoder{
		gl:     d.gl,
		width:  d.width,
		height: d.height,
	}
}

// glInternalFormat returns the WebGL2 internal format for a texture format.
func glInternalFormat(gl js.Value, f backend.TextureFormat) int {
	switch f {
	case backend.TextureFormatRGBA8:
		return gl.Get("RGBA8").Int()
	case backend.TextureFormatRGB8:
		return gl.Get("RGB8").Int()
	case backend.TextureFormatR8:
		return gl.Get("R8").Int()
	case backend.TextureFormatRGBA16F:
		return gl.Get("RGBA16F").Int()
	case backend.TextureFormatRGBA32F:
		return gl.Get("RGBA32F").Int()
	case backend.TextureFormatDepth24:
		return gl.Get("DEPTH_COMPONENT24").Int()
	case backend.TextureFormatDepth32F:
		return gl.Get("DEPTH_COMPONENT32F").Int()
	default:
		return gl.Get("RGBA8").Int()
	}
}

// glBaseFormat returns the WebGL2 base format for a texture format.
func glBaseFormat(gl js.Value, f backend.TextureFormat) int {
	switch f {
	case backend.TextureFormatRGBA8, backend.TextureFormatRGBA16F, backend.TextureFormatRGBA32F:
		return gl.Get("RGBA").Int()
	case backend.TextureFormatRGB8:
		return gl.Get("RGB").Int()
	case backend.TextureFormatR8:
		return gl.Get("RED").Int()
	case backend.TextureFormatDepth24, backend.TextureFormatDepth32F:
		return gl.Get("DEPTH_COMPONENT").Int()
	default:
		return gl.Get("RGBA").Int()
	}
}

// glPixelType returns the WebGL2 pixel type for a texture format.
func glPixelType(gl js.Value, f backend.TextureFormat) int {
	switch f {
	case backend.TextureFormatRGBA16F:
		return gl.Get("HALF_FLOAT").Int()
	case backend.TextureFormatRGBA32F, backend.TextureFormatDepth32F:
		return gl.Get("FLOAT").Int()
	case backend.TextureFormatDepth24:
		return gl.Get("UNSIGNED_INT").Int()
	default:
		return gl.Get("UNSIGNED_BYTE").Int()
	}
}

// glBufferTarget returns the WebGL2 buffer target for a buffer usage.
func glBufferTarget(gl js.Value, u backend.BufferUsage) int {
	switch u {
	case backend.BufferUsageIndex:
		return gl.Get("ELEMENT_ARRAY_BUFFER").Int()
	case backend.BufferUsageUniform:
		return gl.Get("UNIFORM_BUFFER").Int()
	default:
		return gl.Get("ARRAY_BUFFER").Int()
	}
}
