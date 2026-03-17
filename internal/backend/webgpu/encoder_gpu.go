//go:build wgpunative

package webgpu

import (
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/wgpu"
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
}

// BeginRenderPass begins a WebGPU render pass.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	e.cmdEncoder = wgpu.DeviceCreateCommandEncoder(e.dev.device)

	view := e.dev.defaultColorView
	w, h := uint32(e.width), uint32(e.height)
	if desc.Target != nil {
		if rt, ok := desc.Target.(*RenderTarget); ok {
			view = rt.colorTex.view
			w = uint32(rt.w)
			h = uint32(rt.h)
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

	e.passEncoder = wgpu.CommandEncoderBeginRenderPass(e.cmdEncoder, &rpDesc)
	runtime.KeepAlive(colorAttachment)
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

// SetPipeline binds a render pipeline.
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

	// Create a bind group with the texture view and a default sampler.
	// This requires a bind group layout matching the pipeline's expectations.
	// For now, create a simple texture-only bind group.
	if t.view == 0 {
		return
	}

	// Create sampler.
	sampler := e.dev.ensureDefaultSampler()
	if sampler == 0 {
		return
	}

	// Create bind group layout for texture + sampler.
	entries := []wgpu.BindGroupLayoutEntry{
		{
			Binding:    0,
			Visibility: 2, // Fragment
			Sampler_: wgpu.BindGroupLayoutEntrySampler{
				Type: 1, // Filtering
			},
		},
		{
			Binding:    1,
			Visibility: 2, // Fragment
			Texture_: wgpu.BindGroupLayoutEntryTexture{
				SampleType:    1, // Float
				ViewDimension: 2, // 2D
			},
		},
	}

	bglDesc := wgpu.BindGroupLayoutDescriptor{
		EntryCount: uint32(len(entries)),
		Entries:    uintptr(unsafe.Pointer(&entries[0])),
	}
	bgl := wgpu.DeviceCreateBindGroupLayout(e.dev.device, &bglDesc)
	runtime.KeepAlive(entries)
	if bgl == 0 {
		return
	}

	bgEntries := []wgpu.BindGroupEntry{
		{
			Binding:  0,
			Sampler_: sampler,
		},
		{
			Binding:      1,
			TextureView_: t.view,
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
		wgpu.RenderPassSetBindGroup(e.passEncoder, uint32(slot), bg)
		wgpu.BindGroupRelease(bg)
	}
	wgpu.BindGroupLayoutRelease(bgl)
}

// SetTextureFilter overrides the texture filter for a slot.
func (e *Encoder) SetTextureFilter(_ int, _ backend.TextureFilter) {
	// Would create/update a sampler in the bind group.
}

// SetStencil configures stencil test state.
func (e *Encoder) SetStencil(_ bool, _ backend.StencilDescriptor) {
	// Stencil state is baked into the pipeline in WebGPU.
}

// SetColorWrite enables or disables color writing.
func (e *Encoder) SetColorWrite(_ bool) {
	// Color write mask is baked into the pipeline in WebGPU.
}

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
