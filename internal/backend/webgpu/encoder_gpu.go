//go:build (darwin || linux || freebsd || windows) && !soft

package webgpu

import (
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/wgpu"
)

// Encoder implements backend.CommandEncoder for WebGPU via wgpu-native.
type Encoder struct {
	dev    *Device
	width  int
	height int

	inRenderPass    bool
	currentPipeline *Pipeline
	passEncoder     wgpu.RenderPassEncoder
	cmdEncoder      wgpu.CommandEncoder
	boundTexture    *Texture

	// Current sampler filter per slot (default: nearest).
	slotFilters map[int]wgpu.FilterMode
}

// BeginRenderPass begins a WebGPU render pass.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	e.cmdEncoder = wgpu.DeviceCreateCommandEncoder(e.dev.device)

	// Use surface texture if presenting, otherwise offscreen default.
	view := e.dev.defaultColorView
	if e.dev.hasSurface && e.dev.currentTexView != 0 {
		view = e.dev.currentTexView
	}
	w, h := uint32(e.width), uint32(e.height)
	var depthView wgpu.TextureView
	if desc.Target != nil {
		if rt, ok := desc.Target.(*RenderTarget); ok {
			view = rt.colorTex.view
			w = uint32(rt.w)
			h = uint32(rt.h)
			if rt.depthTex != nil {
				if dt, ok := rt.depthTex.(*Texture); ok {
					depthView = dt.view
				}
			}
		}
	}

	loadOp := wgpu.LoadOpLoad
	if desc.LoadAction == backend.LoadActionClear {
		loadOp = wgpu.LoadOpClear
	}

	colorAttachment := wgpu.RenderPassColorAttachment{
		View:     view,
		LoadOp_:  loadOp,
		StoreOp_: wgpu.StoreOpStore,
		ClearValue: wgpu.Color{
			R: float64(desc.ClearColor[0]),
			G: float64(desc.ClearColor[1]),
			B: float64(desc.ClearColor[2]),
			A: float64(desc.ClearColor[3]),
		},
	}

	rpDesc := wgpu.RenderPassDescriptor{
		ColorAttachmentCount: 1,
		ColorAttachments:     ptrOf(&colorAttachment),
	}

	// Attach depth/stencil if available.
	var depthAttach wgpu.RenderPassDepthStencilAttachment
	if depthView != 0 {
		depthAttach = wgpu.RenderPassDepthStencilAttachment{
			View:            depthView,
			DepthLoadOp:     wgpu.LoadOpClear,
			DepthStoreOp:    wgpu.StoreOpStore,
			DepthClearValue: 1.0,
			StencilLoadOp:   wgpu.LoadOpClear,
			StencilStoreOp:  wgpu.StoreOpStore,
		}
		rpDesc.DepthStencilAttachment = uintptr(unsafe.Pointer(&depthAttach))
	}

	e.passEncoder = wgpu.CommandEncoderBeginRenderPass(e.cmdEncoder, &rpDesc)
	runtime.KeepAlive(colorAttachment)
	runtime.KeepAlive(depthAttach)
	e.inRenderPass = true

	// Set default viewport.
	wgpu.RenderPassSetViewport(e.passEncoder, 0, 0, float32(w), float32(h), 0, 1)
}

// EndRenderPass ends the current render pass.
func (e *Encoder) EndRenderPass() {
	if e.inRenderPass {
		wgpu.RenderPassEnd(e.passEncoder)
		wgpu.RenderPassRelease(e.passEncoder)
		e.passEncoder = 0
		e.inRenderPass = false

		// Finish and submit the command buffer.
		cmdBuf := wgpu.CommandEncoderFinish(e.cmdEncoder)
		wgpu.QueueSubmit(e.dev.queue, []wgpu.CommandBuffer{cmdBuf})
		wgpu.CommandBufferRelease(cmdBuf)
		wgpu.CommandEncoderRelease(e.cmdEncoder)
		e.cmdEncoder = 0
	}
}

// SetPipeline binds a render pipeline and uploads uniforms.
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	p, ok := pipeline.(*Pipeline)
	if !ok {
		return
	}
	e.currentPipeline = p

	// Lazily create the WGPURenderPipeline.
	if p.handle == 0 {
		p.createPipeline()
	}

	if p.handle != 0 && e.passEncoder != 0 {
		wgpu.RenderPassSetPipeline(e.passEncoder, p.handle)
	}

	// Bind uniform buffer (group 0) if the shader has uniforms.
	e.bindUniforms()
}

// bindUniforms writes the shader's recorded uniforms into the ring buffer
// and binds the region to group 0.
func (e *Encoder) bindUniforms() {
	if e.currentPipeline == nil || e.passEncoder == 0 || e.dev.device == 0 {
		return
	}
	shader, ok := e.currentPipeline.desc.Shader.(*Shader)
	if !ok || shader == nil {
		return
	}

	// Use the vertex uniform layout (most common for projection matrices).
	// If the fragment shader also has uniforms, pack those too.
	var data []byte
	if len(shader.vertexUniformLayout) > 0 {
		data = shader.packUniforms(shader.vertexUniformLayout)
	} else if len(shader.fragmentUniformLayout) > 0 {
		data = shader.packUniforms(shader.fragmentUniformLayout)
	}
	if len(data) == 0 {
		return
	}

	bgl := e.currentPipeline.uniformBGL
	if bgl == 0 {
		return
	}

	// Write into the ring buffer at the current cursor.
	offset, size := e.dev.writeUniformRing(data)
	if size == 0 {
		return
	}

	// Create bind group referencing the ring buffer region.
	bgEntries := []wgpu.BindGroupEntry{
		{
			Binding: 0,
			Buffer_: e.dev.uniformBuf,
			Offset:  uint64(offset),
			Size:    uint64(size),
		},
	}
	bgDesc := wgpu.BindGroupDescriptor{
		Layout:     bgl,
		EntryCount: 1,
		Entries:    uintptr(unsafe.Pointer(&bgEntries[0])),
	}
	bg := wgpu.DeviceCreateBindGroup(e.dev.device, &bgDesc)
	runtime.KeepAlive(bgEntries)
	if bg != 0 {
		wgpu.RenderPassSetBindGroup(e.passEncoder, 0, bg)
		wgpu.BindGroupRelease(bg)
	}
}

// SetVertexBuffer binds a vertex buffer to a slot.
func (e *Encoder) SetVertexBuffer(buf backend.Buffer, slot int) {
	if b, ok := buf.(*Buffer); ok {
		wgpu.RenderPassSetVertexBuffer(e.passEncoder, uint32(slot),
			b.handle, 0, uint64(b.size))
	}
}

// SetIndexBuffer binds an index buffer.
func (e *Encoder) SetIndexBuffer(buf backend.Buffer, format backend.IndexFormat) {
	if b, ok := buf.(*Buffer); ok {
		idxFmt := wgpu.IndexFormatUint16
		if format == backend.IndexUint32 {
			idxFmt = wgpu.IndexFormatUint32
		}
		wgpu.RenderPassSetIndexBuffer(e.passEncoder, b.handle, idxFmt, 0, uint64(b.size))
	}
}

// SetTexture binds a texture to a slot via bind groups.
func (e *Encoder) SetTexture(tex backend.Texture, slot int) {
	t, ok := tex.(*Texture)
	if !ok || e.dev.device == 0 || e.passEncoder == 0 {
		return
	}
	e.boundTexture = t
	if t.view == 0 {
		return
	}

	// Determine sampler filter for this slot.
	filter := wgpu.FilterModeNearest
	if e.slotFilters != nil {
		if f, ok := e.slotFilters[slot]; ok {
			filter = f
		}
	}
	sampler := e.dev.getSampler(filter)
	if sampler == 0 {
		return
	}

	// Use cached bind group layout from the current pipeline.
	var bgl wgpu.BindGroupLayout
	if e.currentPipeline != nil && e.currentPipeline.textureBGL != 0 {
		bgl = e.currentPipeline.textureBGL
	}
	if bgl == 0 {
		return
	}

	bgEntries := []wgpu.BindGroupEntry{
		{
			Binding:      0,
			TextureView_: t.view,
		},
		{
			Binding:  1,
			Sampler_: sampler,
		},
	}

	bgDesc := wgpu.BindGroupDescriptor{
		Layout:     bgl,
		EntryCount: uint32(len(bgEntries)),
		Entries:    uintptr(unsafe.Pointer(&bgEntries[0])),
	}
	bg := wgpu.DeviceCreateBindGroup(e.dev.device, &bgDesc)
	runtime.KeepAlive(bgEntries)
	if bg != 0 {
		wgpu.RenderPassSetBindGroup(e.passEncoder, 1, bg)
		wgpu.BindGroupRelease(bg)
	}
}

// SetTextureFilter overrides the texture filter for a slot.
func (e *Encoder) SetTextureFilter(slot int, filter backend.TextureFilter) {
	if e.slotFilters == nil {
		e.slotFilters = make(map[int]wgpu.FilterMode)
	}
	switch filter {
	case backend.FilterLinear:
		e.slotFilters[slot] = wgpu.FilterModeLinear
	default:
		e.slotFilters[slot] = wgpu.FilterModeNearest
	}
}

// SetStencil configures stencil test state.
// In WebGPU, stencil state is baked into the pipeline at creation time.
func (e *Encoder) SetStencil(_ bool, _ backend.StencilDescriptor) {}

// SetColorWrite enables or disables color writing.
// In WebGPU, the color write mask is baked into the pipeline at creation time.
func (e *Encoder) SetColorWrite(_ bool) {}

// SetViewport sets the rendering viewport.
func (e *Encoder) SetViewport(vp backend.Viewport) {
	wgpu.RenderPassSetViewport(e.passEncoder,
		float32(vp.X), float32(vp.Y),
		float32(vp.Width), float32(vp.Height),
		0, 1)
}

// SetScissor sets the scissor rectangle.
func (e *Encoder) SetScissor(rect *backend.ScissorRect) {
	if rect == nil {
		wgpu.RenderPassSetScissorRect(e.passEncoder,
			0, 0, uint32(e.width), uint32(e.height))
		return
	}
	wgpu.RenderPassSetScissorRect(e.passEncoder,
		uint32(rect.X), uint32(rect.Y),
		uint32(rect.Width), uint32(rect.Height))
}

// Draw issues a non-indexed draw call.
func (e *Encoder) Draw(vertexCount, instanceCount, firstVertex int) {
	wgpu.RenderPassDraw(e.passEncoder,
		uint32(vertexCount), uint32(instanceCount), uint32(firstVertex), 0)
}

// DrawIndexed issues an indexed draw call.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	wgpu.RenderPassDrawIndexed(e.passEncoder,
		uint32(indexCount), uint32(instanceCount), uint32(firstIndex), 0, 0)
}

// Flush is a no-op — submission happens in EndRenderPass.
func (e *Encoder) Flush() {}

// ptrOf returns the uintptr of a pointer.
func ptrOf[T any](p *T) uintptr {
	return uintptr(unsafePointer(p))
}

// unsafePointer converts a typed pointer to unsafe.Pointer.
//
//go:nosplit
func unsafePointer[T any](p *T) unsafePtr { //nolint:unused
	return unsafePtr(p)
}

// unsafePtr is an alias for unsafe.Pointer used to avoid import in every file.
type unsafePtr = unsafe.Pointer
