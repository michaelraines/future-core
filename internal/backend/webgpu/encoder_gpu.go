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

	// Dimensions of the current render target (updated each BeginRenderPass).
	// SetScissor(nil) defaults to these, matching the active attachment.
	// Distinct from width/height which track the device's default target.
	currentW uint32
	currentH uint32

	// Format of the current render target (set in BeginRenderPass).
	targetFormat wgpu.TextureFormat

	// Current sampler filter per slot (default: nearest).
	slotFilters map[int]wgpu.FilterMode
}

// BeginRenderPass begins a WebGPU render pass.
// The command encoder is created lazily on the first call per frame and
// reused across all render passes until Flush submits the accumulated
// command buffer.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	if e.cmdEncoder == 0 {
		e.cmdEncoder = wgpu.DeviceCreateCommandEncoder(e.dev.device)
	}

	// Use surface texture if presenting, otherwise offscreen default.
	view := e.dev.defaultColorView
	e.targetFormat = wgpu.TextureFormatRGBA8Unorm // offscreen default
	if e.dev.hasSurface && e.dev.currentTexView != 0 {
		view = e.dev.currentTexView
		e.targetFormat = e.dev.surfaceFormat
	}
	w, h := uint32(e.width), uint32(e.height)
	var depthView wgpu.TextureView
	if desc.Target != nil {
		if rt, ok := desc.Target.(*RenderTarget); ok {
			view = rt.colorTex.view
			e.targetFormat = wgpuTextureFormatEnum(rt.colorTex.format)
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
		View:       view,
		DepthSlice: 0xFFFFFFFF, // WGPU_DEPTH_SLICE_UNDEFINED
		LoadOp_:    loadOp,
		StoreOp_:   wgpu.StoreOpStore,
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
	e.currentW = w
	e.currentH = h

	// Set default viewport.
	wgpu.RenderPassSetViewport(e.passEncoder, 0, 0, float32(w), float32(h), 0, 1)
}

// EndRenderPass ends the current render pass but does NOT submit the
// command buffer. Call Flush() after all render passes to submit the
// accumulated work in a single queue submission.
func (e *Encoder) EndRenderPass() {
	if e.inRenderPass {
		wgpu.RenderPassEnd(e.passEncoder)
		wgpu.RenderPassRelease(e.passEncoder)
		e.passEncoder = 0
		e.inRenderPass = false
	}
}

// SetPipeline binds a render pipeline and uploads uniforms.
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	p, ok := pipeline.(*Pipeline)
	if !ok {
		return
	}
	e.currentPipeline = p

	// Lazily create (or recreate) the pipeline for the current target format.
	p.ensurePipelineForFormat(e.targetFormat)

	if p.handle != 0 && e.passEncoder != 0 {
		wgpu.RenderPassSetPipeline(e.passEncoder, p.handle)
	}

	// Bind uniform buffer (group 0) if the shader has uniforms.
	e.bindUniforms()

	// Bind default texture to group 1 so that draw calls without an
	// explicit SetTexture don't fail with "No bind group set at group index 1".
	// Any subsequent SetTexture call will override this.
	e.bindDefaultTexture()
}

// bindUniforms writes the shader's recorded uniforms into the ring buffer
// and binds the region to group 0. Skips the pack+upload+bind cycle when
// no uniform values have changed since the last bind.
func (e *Encoder) bindUniforms() {
	if e.currentPipeline == nil || e.passEncoder == 0 || e.dev.device == 0 {
		return
	}
	shader, ok := e.currentPipeline.desc.Shader.(*Shader)
	if !ok || shader == nil {
		return
	}

	// Fast path: skip if no SetUniform* calls since last bind.
	if !shader.uniformsDirty {
		return
	}

	// Use the combined uniform layout (vertex + fragment fields at
	// non-overlapping offsets) so the buffer satisfies both stages.
	var data []byte
	if len(shader.combinedUniformLayout) > 0 {
		data = shader.packUniforms(shader.combinedUniformLayout)
	}
	if len(data) == 0 {
		shader.uniformsDirty = false
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

	shader.uniformsDirty = false
}

// bindDefaultTexture binds a 1x1 white placeholder texture to group 1,
// ensuring that draw calls without an explicit SetTexture don't trigger
// "No bind group set at group index 1" validation errors.
func (e *Encoder) bindDefaultTexture() {
	if e.currentPipeline == nil || e.passEncoder == 0 || e.dev.device == 0 {
		return
	}
	t := e.dev.defaultWhiteTex
	if t == nil || t.view == 0 {
		return
	}
	bgl := e.currentPipeline.textureBGL
	if bgl == 0 {
		return
	}
	sampler := e.dev.getSampler(wgpu.FilterModeNearest)
	if sampler == 0 {
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
		EntryCount: uintptr(len(bgEntries)),
		Entries:    uintptr(unsafe.Pointer(&bgEntries[0])),
	}
	bg := wgpu.DeviceCreateBindGroup(e.dev.device, &bgDesc)
	runtime.KeepAlive(bgEntries)
	if bg != 0 {
		wgpu.RenderPassSetBindGroup(e.passEncoder, 1, bg)
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
		EntryCount: uintptr(len(bgEntries)),
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

// SetStencilReference is a no-op pending wgpu-native bindings for
// wgpuRenderPassEncoderSetStencilReference. Native GPU stencil is not yet
// wired up; Capabilities().SupportsStencil stays false so the sprite pass
// never routes through the stencil path on this backend.
func (e *Encoder) SetStencilReference(_ uint32) {}

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
// A nil rect defaults to the current render target's bounds (tracked in
// BeginRenderPass); using e.width/e.height here would leak the device's
// default target size into passes that target smaller offscreen RTs and
// trip wgpu's "scissor not contained in render target" validation.
func (e *Encoder) SetScissor(rect *backend.ScissorRect) {
	if rect == nil {
		wgpu.RenderPassSetScissorRect(e.passEncoder, 0, 0, e.currentW, e.currentH)
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
// Uniforms are re-bound before each draw to pick up per-batch changes
// (e.g., color matrix) that were set on the shader after SetPipeline.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	e.bindUniforms()
	wgpu.RenderPassDrawIndexed(e.passEncoder,
		uint32(indexCount), uint32(instanceCount), uint32(firstIndex), 0, 0)
}

// Flush is a no-op — submission happens in EndRenderPass.
// Flush submits all render passes accumulated in the current command
// encoder as a single queue submission. Called once per frame after
// the sprite pass has recorded all its render passes.
// SetBlendMode is a no-op for this backend.
func (e *Encoder) SetBlendMode(_ backend.BlendMode) {}

func (e *Encoder) Flush() {
	if e.cmdEncoder == 0 {
		return
	}
	cmdBuf := wgpu.CommandEncoderFinish(e.cmdEncoder)
	wgpu.QueueSubmit(e.dev.queue, []wgpu.CommandBuffer{cmdBuf})
	wgpu.CommandBufferRelease(cmdBuf)
	wgpu.CommandEncoderRelease(e.cmdEncoder)
	e.cmdEncoder = 0
}

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
