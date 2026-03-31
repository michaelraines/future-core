//go:build js && !soft

package webgpu

import (
	"fmt"
	"syscall/js"

	"github.com/michaelraines/future-core/internal/backend"
)

// Device implements backend.Device for WebGPU via the browser's navigator.gpu API.
type Device struct {
	gpu     js.Value // navigator.gpu
	adapter js.Value // GPUAdapter
	device  js.Value // GPUDevice
	queue   js.Value // GPUQueue

	canvas  js.Value // Canvas element
	context js.Value // GPUCanvasContext

	width  int
	height int

	// Default offscreen render target (when no canvas context).
	defaultColorTex  js.Value
	defaultColorView js.Value

	// Presentation state.
	hasContext bool

	// Sampler cache keyed by filter string ("nearest" or "linear").
	samplers map[string]js.Value
}

// New creates a new WebGPU device.
func New() *Device {
	return &Device{
		samplers: make(map[string]js.Value),
	}
}

// Init initializes the WebGPU device by requesting adapter and device from navigator.gpu.
func (d *Device) Init(cfg backend.DeviceConfig) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("webgpu: invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
	d.width = cfg.Width
	d.height = cfg.Height

	navigator := js.Global().Get("navigator")
	if navigator.IsUndefined() || navigator.IsNull() {
		return fmt.Errorf("webgpu: navigator not available")
	}

	d.gpu = navigator.Get("gpu")
	if d.gpu.IsUndefined() || d.gpu.IsNull() {
		return fmt.Errorf("webgpu: navigator.gpu not available (WebGPU not supported)")
	}

	// Request adapter (async).
	adapter, err := awaitPromise(d.gpu.Call("requestAdapter"))
	if err != nil {
		return fmt.Errorf("webgpu: requestAdapter: %w", err)
	}
	if adapter.IsNull() || adapter.IsUndefined() {
		return fmt.Errorf("webgpu: no suitable adapter found")
	}
	d.adapter = adapter

	// Request device (async).
	device, err := awaitPromise(d.adapter.Call("requestDevice"))
	if err != nil {
		return fmt.Errorf("webgpu: requestDevice: %w", err)
	}
	if device.IsNull() || device.IsUndefined() {
		return fmt.Errorf("webgpu: failed to create device")
	}
	d.device = device
	d.queue = d.device.Get("queue")

	// Set up canvas and GPUCanvasContext if DOM is available.
	doc := js.Global().Get("document")
	if !doc.IsUndefined() && !doc.IsNull() {
		d.canvas = doc.Call("createElement", "canvas")
		d.canvas.Set("width", d.width)
		d.canvas.Set("height", d.height)

		d.context = d.canvas.Call("getContext", "webgpu")
		if !d.context.IsUndefined() && !d.context.IsNull() {
			preferredFormat := d.gpu.Call("getPreferredCanvasFormat")
			configObj := js.Global().Get("Object").New()
			configObj.Set("device", d.device)
			configObj.Set("format", preferredFormat)
			configObj.Set("alphaMode", "opaque")
			d.context.Call("configure", configObj)
			d.hasContext = true
		}
	}

	if !d.hasContext {
		// Offscreen: create default color texture.
		d.createDefaultTexture()
	}

	return nil
}

// createDefaultTexture creates an offscreen color texture for headless rendering.
func (d *Device) createDefaultTexture() {
	desc := js.Global().Get("Object").New()
	sizeArr := js.Global().Get("Array").New(d.width, d.height, 1)
	desc.Set("size", sizeArr)
	desc.Set("format", "rgba8unorm")
	desc.Set("usage",
		jsGPUTextureUsage(d.device, "RENDER_ATTACHMENT")|
			jsGPUTextureUsage(d.device, "COPY_SRC")|
			jsGPUTextureUsage(d.device, "COPY_DST")|
			jsGPUTextureUsage(d.device, "TEXTURE_BINDING"))
	d.defaultColorTex = d.device.Call("createTexture", desc)
	d.defaultColorView = d.defaultColorTex.Call("createView")
}

// Dispose releases all WebGPU resources.
func (d *Device) Dispose() {
	// GPU objects are garbage-collected by the browser.
	d.samplers = nil
}

// ReadScreen copies the default color texture pixels into dst.
func (d *Device) ReadScreen(dst []byte) bool {
	if d.hasContext || d.defaultColorTex.IsUndefined() || d.defaultColorTex.IsNull() {
		return false
	}

	bpp := 4
	bytesPerRow := d.width * bpp
	alignedBytesPerRow := (bytesPerRow + 255) &^ 255
	totalSize := alignedBytesPerRow * d.height

	// Create staging buffer.
	bufDesc := js.Global().Get("Object").New()
	bufDesc.Set("size", totalSize)
	bufDesc.Set("usage",
		jsGPUBufferUsage(d.device, "COPY_DST")|jsGPUBufferUsage(d.device, "MAP_READ"))
	stagingBuf := d.device.Call("createBuffer", bufDesc)

	// Encode copy texture → buffer.
	enc := d.device.Call("createCommandEncoder")

	srcObj := js.Global().Get("Object").New()
	srcObj.Set("texture", d.defaultColorTex)

	dstObj := js.Global().Get("Object").New()
	dstObj.Set("buffer", stagingBuf)
	dstObj.Set("bytesPerRow", alignedBytesPerRow)
	dstObj.Set("rowsPerImage", d.height)

	sizeObj := js.Global().Get("Object").New()
	sizeObj.Set("width", d.width)
	sizeObj.Set("height", d.height)

	enc.Call("copyTextureToBuffer", srcObj, dstObj, sizeObj)
	cmdBuf := enc.Call("finish")
	d.queue.Call("submit", js.Global().Get("Array").New(cmdBuf))

	// Map and read.
	mapPromise := stagingBuf.Call("mapAsync", jsGPUMapMode(d.device, "READ"))
	if _, err := awaitPromise(mapPromise); err != nil {
		return false
	}

	mapped := stagingBuf.Call("getMappedRange")
	srcArr := js.Global().Get("Uint8Array").New(mapped)

	// Copy row by row to handle alignment padding.
	dstOffset := 0
	for row := 0; row < d.height; row++ {
		srcStart := alignedBytesPerRow * row
		n := bytesPerRow
		if dstOffset+n > len(dst) {
			n = len(dst) - dstOffset
		}
		if n <= 0 {
			break
		}
		rowSlice := srcArr.Call("slice", srcStart, srcStart+n)
		js.CopyBytesToGo(dst[dstOffset:dstOffset+n], rowSlice)
		dstOffset += n
	}

	stagingBuf.Call("unmap")
	stagingBuf.Call("destroy")
	return true
}

// BeginFrame prepares for a new frame.
func (d *Device) BeginFrame() {}

// EndFrame finalizes the frame.
func (d *Device) EndFrame() {}

// NewTexture creates a WebGPU texture.
func (d *Device) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("webgpu: invalid texture dimensions %dx%d", desc.Width, desc.Height)
	}

	texDesc := js.Global().Get("Object").New()
	sizeArr := js.Global().Get("Array").New(desc.Width, desc.Height, 1)
	texDesc.Set("size", sizeArr)
	texDesc.Set("format", jsTextureFormat(desc.Format))
	texDesc.Set("usage",
		jsGPUTextureUsage(d.device, "TEXTURE_BINDING")|
			jsGPUTextureUsage(d.device, "COPY_DST")|
			jsGPUTextureUsage(d.device, "COPY_SRC"))

	handle := d.device.Call("createTexture", texDesc)
	if handle.IsNull() || handle.IsUndefined() {
		return nil, fmt.Errorf("webgpu: failed to create texture")
	}

	view := handle.Call("createView")

	tex := &Texture{
		dev:    d,
		handle: handle,
		view:   view,
		w:      desc.Width,
		h:      desc.Height,
		format: desc.Format,
	}

	if len(desc.Data) > 0 {
		tex.Upload(desc.Data, 0)
	}

	return tex, nil
}

// NewBuffer creates a WebGPU buffer.
func (d *Device) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	size := desc.Size
	if len(desc.Data) > 0 {
		size = len(desc.Data)
	}
	if size <= 0 {
		return nil, fmt.Errorf("webgpu: invalid buffer size %d", size)
	}

	// Align to 4 bytes.
	alignedSize := (size + 3) &^ 3

	bufDesc := js.Global().Get("Object").New()
	bufDesc.Set("size", alignedSize)
	bufDesc.Set("usage", jsBufferUsage(d.device, desc.Usage)|jsGPUBufferUsage(d.device, "COPY_DST"))

	handle := d.device.Call("createBuffer", bufDesc)
	if handle.IsNull() || handle.IsUndefined() {
		return nil, fmt.Errorf("webgpu: failed to create buffer")
	}

	buf := &Buffer{
		dev:    d,
		handle: handle,
		size:   size,
	}

	if len(desc.Data) > 0 {
		buf.Upload(desc.Data)
	}

	return buf, nil
}

// NewShader creates a WebGPU shader module.
func (d *Device) NewShader(desc backend.ShaderDescriptor) (backend.Shader, error) {
	return &Shader{
		dev:            d,
		vertexSource:   desc.VertexSource,
		fragmentSource: desc.FragmentSource,
		attributes:     desc.Attributes,
		uniforms:       make(map[string]interface{}),
	}, nil
}

// NewRenderTarget creates a WebGPU render target.
func (d *Device) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("webgpu: invalid render target dimensions %dx%d", desc.Width, desc.Height)
	}

	colorFmt := desc.ColorFormat
	if colorFmt == 0 {
		colorFmt = backend.TextureFormatRGBA8
	}

	colorTex, err := d.NewTexture(backend.TextureDescriptor{
		Width: desc.Width, Height: desc.Height, Format: colorFmt,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: render target color: %w", err)
	}

	var depthTex backend.Texture
	if desc.HasDepth {
		depthFmt := desc.DepthFormat
		if depthFmt == 0 {
			depthFmt = backend.TextureFormatDepth24
		}
		dt, err := d.NewTexture(backend.TextureDescriptor{
			Width: desc.Width, Height: desc.Height, Format: depthFmt,
		})
		if err != nil {
			colorTex.Dispose()
			return nil, fmt.Errorf("webgpu: render target depth: %w", err)
		}
		depthTex = dt
	}

	return &RenderTarget{
		colorTex: colorTex.(*Texture),
		depthTex: depthTex,
		w:        desc.Width,
		h:        desc.Height,
	}, nil
}

// NewPipeline creates a WebGPU render pipeline.
func (d *Device) NewPipeline(desc backend.PipelineDescriptor) (backend.Pipeline, error) {
	return &Pipeline{
		dev:  d,
		desc: desc,
	}, nil
}

// Capabilities returns WebGPU device capabilities.
func (d *Device) Capabilities() backend.DeviceCapabilities {
	return backend.DeviceCapabilities{
		MaxTextureSize:    8192,
		MaxRenderTargets:  8,
		SupportsInstanced: true,
		SupportsCompute:   true,
		SupportsMSAA:      true,
		MaxMSAASamples:    4,
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

// getSampler returns a cached sampler for the given filter mode.
func (d *Device) getSampler(filter string) js.Value {
	if s, ok := d.samplers[filter]; ok {
		return s
	}
	desc := js.Global().Get("Object").New()
	desc.Set("magFilter", filter)
	desc.Set("minFilter", filter)
	desc.Set("addressModeU", "clamp-to-edge")
	desc.Set("addressModeV", "clamp-to-edge")
	s := d.device.Call("createSampler", desc)
	d.samplers[filter] = s
	return s
}

// currentColorView returns the active color view for rendering.
func (d *Device) currentColorView() js.Value {
	if d.hasContext {
		tex := d.context.Call("getCurrentTexture")
		return tex.Call("createView")
	}
	return d.defaultColorView
}

// --- JS helper functions ---

// awaitPromise synchronously waits for a JS Promise to resolve.
func awaitPromise(promise js.Value) (js.Value, error) {
	ch := make(chan js.Value, 1)
	errCh := make(chan error, 1)

	thenFn := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			ch <- args[0]
		} else {
			ch <- js.Undefined()
		}
		return nil
	})
	catchFn := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		msg := "unknown error"
		if len(args) > 0 {
			msg = args[0].Call("toString").String()
		}
		errCh <- fmt.Errorf("%s", msg)
		return nil
	})
	defer thenFn.Release()
	defer catchFn.Release()

	promise.Call("then", thenFn).Call("catch", catchFn)

	select {
	case result := <-ch:
		return result, nil
	case err := <-errCh:
		return js.Undefined(), err
	}
}

// jsGPUTextureUsage returns the GPUTextureUsage flag value.
func jsGPUTextureUsage(device js.Value, name string) int {
	return js.Global().Get("GPUTextureUsage").Get(name).Int()
}

// jsGPUBufferUsage returns the GPUBufferUsage flag value.
func jsGPUBufferUsage(device js.Value, name string) int {
	return js.Global().Get("GPUBufferUsage").Get(name).Int()
}

// jsGPUMapMode returns the GPUMapMode flag value.
func jsGPUMapMode(device js.Value, name string) int {
	return js.Global().Get("GPUMapMode").Get(name).Int()
}

// jsTextureFormat maps backend texture format to WebGPU JS format string.
func jsTextureFormat(f backend.TextureFormat) string {
	switch f {
	case backend.TextureFormatRGBA8:
		return "rgba8unorm"
	case backend.TextureFormatRGB8:
		return "rgba8unorm" // No RGB8 in WebGPU
	case backend.TextureFormatR8:
		return "r8unorm"
	case backend.TextureFormatRGBA16F:
		return "rgba16float"
	case backend.TextureFormatRGBA32F:
		return "rgba32float"
	case backend.TextureFormatDepth24:
		return "depth24plus"
	case backend.TextureFormatDepth32F:
		return "depth32float"
	default:
		return "rgba8unorm"
	}
}

// jsBufferUsage maps backend buffer usage to WebGPU JS usage flags.
func jsBufferUsage(device js.Value, u backend.BufferUsage) int {
	switch u {
	case backend.BufferUsageVertex:
		return jsGPUBufferUsage(device, "VERTEX")
	case backend.BufferUsageIndex:
		return jsGPUBufferUsage(device, "INDEX")
	case backend.BufferUsageUniform:
		return jsGPUBufferUsage(device, "UNIFORM")
	default:
		return jsGPUBufferUsage(device, "VERTEX")
	}
}
