//go:build js && !soft

package webgpu

import (
	"syscall/js"

	"github.com/michaelraines/future-core/internal/backend"
)

// Encoder implements backend.CommandEncoder for WebGPU via the browser JS API.
type Encoder struct {
	dev    *Device
	width  int
	height int

	inRenderPass    bool
	currentPipeline *Pipeline
	passEncoder     js.Value
	cmdEncoder      js.Value

	// Format of the current render target (set in BeginRenderPass).
	targetFormat string

	// Current sampler filter per slot.
	slotFilters map[int]string

	// Keep temporary uniform buffers and bind groups alive until
	// EndRenderPass submits the command buffer. Without this, the Go GC
	// may release JS references before the GPU executes the draws.
	tempRefs []js.Value
}

// BeginRenderPass begins a WebGPU render pass.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	e.cmdEncoder = e.dev.device.Call("createCommandEncoder")

	view := e.dev.currentColorView()
	w, h := e.width, e.height
	var depthView js.Value

	// Determine the target format: canvas preferred format or rgba8unorm for offscreen.
	e.targetFormat = "rgba8unorm"
	if e.dev.hasContext && e.dev.preferredFormat != "" {
		e.targetFormat = e.dev.preferredFormat
	}

	if desc.Target != nil {
		if rt, ok := desc.Target.(*RenderTarget); ok {
			view = rt.colorTex.view
			e.targetFormat = jsTextureFormat(rt.colorTex.format)
			w = rt.w
			h = rt.h
			if rt.depthTex != nil {
				if dt, ok := rt.depthTex.(*Texture); ok {
					depthView = dt.view
				}
			}
		}
	}

	loadOp := "load"
	if desc.LoadAction == backend.LoadActionClear {
		loadOp = "clear"
	}

	colorAttach := js.Global().Get("Object").New()
	colorAttach.Set("view", view)
	colorAttach.Set("loadOp", loadOp)
	colorAttach.Set("storeOp", "store")

	if loadOp == "clear" {
		clearColor := js.Global().Get("Object").New()
		clearColor.Set("r", float64(desc.ClearColor[0]))
		clearColor.Set("g", float64(desc.ClearColor[1]))
		clearColor.Set("b", float64(desc.ClearColor[2]))
		clearColor.Set("a", float64(desc.ClearColor[3]))
		colorAttach.Set("clearValue", clearColor)
	}

	colorAttachments := js.Global().Get("Array").New(colorAttach)

	rpDesc := js.Global().Get("Object").New()
	rpDesc.Set("colorAttachments", colorAttachments)

	if !depthView.IsUndefined() && !depthView.IsNull() {
		depthAttach := js.Global().Get("Object").New()
		depthAttach.Set("view", depthView)
		depthAttach.Set("depthLoadOp", "clear")
		depthAttach.Set("depthStoreOp", "store")
		depthAttach.Set("depthClearValue", 1.0)
		rpDesc.Set("depthStencilAttachment", depthAttach)
	}

	e.passEncoder = e.cmdEncoder.Call("beginRenderPass", rpDesc)
	e.inRenderPass = true

	// Set default viewport.
	e.passEncoder.Call("setViewport", 0, 0, float64(w), float64(h), 0, 1)
}

// EndRenderPass ends the current render pass.
func (e *Encoder) EndRenderPass() {
	if e.inRenderPass {
		e.passEncoder.Call("end")
		e.passEncoder = js.Undefined()
		e.inRenderPass = false

		cmdBuf := e.cmdEncoder.Call("finish")
		e.dev.queue.Call("submit", js.Global().Get("Array").New(cmdBuf))
		e.cmdEncoder = js.Undefined()

		// Release temporary references now that the GPU has the commands.
		e.tempRefs = e.tempRefs[:0]
	}
}

// SetPipeline binds a render pipeline.
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	p, ok := pipeline.(*Pipeline)
	if !ok {
		return
	}
	e.currentPipeline = p

	// Lazily create (or recreate) the pipeline for the current target format.
	p.ensurePipelineForFormat(e.targetFormat)

	pipelineOK := !p.handle.IsUndefined() && !p.handle.IsNull()
	if pipelineOK && e.inRenderPass {
		e.passEncoder.Call("setPipeline", p.handle)
	}

	e.bindUniforms()

	// Bind default texture to group 1 so that draw calls without an
	// explicit SetTexture don't fail with "No bind group set at group index 1".
	// Any subsequent SetTexture call will override this.
	e.bindDefaultTexture()
}

// bindUniforms uploads shader uniforms and binds them to group 0.
func (e *Encoder) bindUniforms() {
	if e.currentPipeline == nil || !e.inRenderPass {
		return
	}
	shader, ok := e.currentPipeline.desc.Shader.(*Shader)
	if !ok || shader == nil {
		return
	}

	// Use the combined uniform layout (vertex + fragment fields at
	// non-overlapping offsets) so the buffer satisfies both stages.
	var data []byte
	if len(shader.combinedUniformLayout) > 0 {
		data = shader.packUniforms(shader.combinedUniformLayout)
	}
	if len(data) == 0 {
		return
	}

	if e.currentPipeline.uniformBGL.IsUndefined() || e.currentPipeline.uniformBGL.IsNull() {
		return
	}

	// Create a uniform buffer and upload data via queue.writeBuffer.
	// Avoids mappedAtCreation which can fail when the browser tab is
	// backgrounded and the GPU device has constrained resources.
	alignedSize := (len(data) + 3) &^ 3
	bufDesc := js.Global().Get("Object").New()
	bufDesc.Set("size", alignedSize)
	bufDesc.Set("usage", jsGPUBufferUsage(e.dev.device, "UNIFORM")|jsGPUBufferUsage(e.dev.device, "COPY_DST"))
	uboBuf := e.dev.device.Call("createBuffer", bufDesc)

	// Upload data via queue.writeBuffer — works reliably regardless of
	// GPU device state (tab visibility, resource pressure).
	srcArr := js.Global().Get("Uint8Array").New(alignedSize)
	js.CopyBytesToJS(srcArr, data)
	e.dev.queue.Call("writeBuffer", uboBuf, 0, srcArr)

	// Create bind group.
	entry := js.Global().Get("Object").New()
	entry.Set("binding", 0)
	bufBinding := js.Global().Get("Object").New()
	bufBinding.Set("buffer", uboBuf)
	bufBinding.Set("offset", 0)
	bufBinding.Set("size", alignedSize)
	entry.Set("resource", bufBinding)

	bgDesc := js.Global().Get("Object").New()
	bgDesc.Set("layout", e.currentPipeline.uniformBGL)
	bgDesc.Set("entries", js.Global().Get("Array").New(entry))
	bg := e.dev.device.Call("createBindGroup", bgDesc)

	e.passEncoder.Call("setBindGroup", 0, bg)

	// Keep the buffer and bind group alive until the command buffer is submitted.
	e.tempRefs = append(e.tempRefs, uboBuf, bg)
}

// bindDefaultTexture binds a 1x1 white placeholder texture to group 1,
// ensuring that draw calls without an explicit SetTexture don't trigger
// "No bind group set at group index 1" validation errors.
func (e *Encoder) bindDefaultTexture() {
	if e.currentPipeline == nil || !e.inRenderPass {
		return
	}
	t := e.dev.defaultWhiteTex
	if t == nil || t.view.IsUndefined() || t.view.IsNull() {
		return
	}
	bgl := e.currentPipeline.textureBGL
	if bgl.IsUndefined() || bgl.IsNull() {
		return
	}
	sampler := e.dev.getSampler("nearest")

	texEntry := js.Global().Get("Object").New()
	texEntry.Set("binding", 0)
	texEntry.Set("resource", t.view)

	sampEntry := js.Global().Get("Object").New()
	sampEntry.Set("binding", 1)
	sampEntry.Set("resource", sampler)

	bgDesc := js.Global().Get("Object").New()
	bgDesc.Set("layout", bgl)
	bgDesc.Set("entries", js.Global().Get("Array").New(texEntry, sampEntry))
	bg := e.dev.device.Call("createBindGroup", bgDesc)

	e.passEncoder.Call("setBindGroup", 1, bg)
	e.tempRefs = append(e.tempRefs, bg)
}

// SetVertexBuffer binds a vertex buffer.
func (e *Encoder) SetVertexBuffer(buf backend.Buffer, slot int) {
	if b, ok := buf.(*Buffer); ok {
		e.passEncoder.Call("setVertexBuffer", slot, b.handle)
	}
}

// SetIndexBuffer binds an index buffer.
func (e *Encoder) SetIndexBuffer(buf backend.Buffer, format backend.IndexFormat) {
	if b, ok := buf.(*Buffer); ok {
		idxFmt := "uint16"
		if format == backend.IndexUint32 {
			idxFmt = "uint32"
		}
		e.passEncoder.Call("setIndexBuffer", b.handle, idxFmt)
	}
}

// SetTexture binds a texture to a slot via bind groups.
func (e *Encoder) SetTexture(tex backend.Texture, slot int) {
	t, ok := tex.(*Texture)
	if !ok || !e.inRenderPass {
		return
	}
	if t.view.IsUndefined() || t.view.IsNull() {
		return
	}

	filter := "nearest"
	if e.slotFilters != nil {
		if f, ok := e.slotFilters[slot]; ok {
			filter = f
		}
	}
	sampler := e.dev.getSampler(filter)

	var bgl js.Value
	if e.currentPipeline != nil {
		bgl = e.currentPipeline.textureBGL
	}
	if bgl.IsUndefined() || bgl.IsNull() {
		return
	}

	texEntry := js.Global().Get("Object").New()
	texEntry.Set("binding", 0)
	texEntry.Set("resource", t.view)

	sampEntry := js.Global().Get("Object").New()
	sampEntry.Set("binding", 1)
	sampEntry.Set("resource", sampler)

	bgDesc := js.Global().Get("Object").New()
	bgDesc.Set("layout", bgl)
	bgDesc.Set("entries", js.Global().Get("Array").New(texEntry, sampEntry))
	bg := e.dev.device.Call("createBindGroup", bgDesc)

	e.passEncoder.Call("setBindGroup", 1, bg)
	e.tempRefs = append(e.tempRefs, bg)
}

// SetTextureFilter overrides the texture filter for a slot.
func (e *Encoder) SetTextureFilter(slot int, filter backend.TextureFilter) {
	if e.slotFilters == nil {
		e.slotFilters = make(map[int]string)
	}
	if filter == backend.FilterLinear {
		e.slotFilters[slot] = "linear"
	} else {
		e.slotFilters[slot] = "nearest"
	}
}

// SetStencil configures stencil test state (baked into pipeline in WebGPU).
func (e *Encoder) SetStencil(_ bool, _ backend.StencilDescriptor) {}

// SetColorWrite enables or disables color writing (baked into pipeline in WebGPU).
func (e *Encoder) SetColorWrite(_ bool) {}

// SetViewport sets the rendering viewport.
func (e *Encoder) SetViewport(vp backend.Viewport) {
	if e.inRenderPass {
		e.passEncoder.Call("setViewport",
			float64(vp.X), float64(vp.Y),
			float64(vp.Width), float64(vp.Height), 0, 1)
	}
}

// SetScissor sets the scissor rectangle.
func (e *Encoder) SetScissor(rect *backend.ScissorRect) {
	if !e.inRenderPass {
		return
	}
	if rect == nil {
		e.passEncoder.Call("setScissorRect", 0, 0, e.width, e.height)
		return
	}
	e.passEncoder.Call("setScissorRect", rect.X, rect.Y, rect.Width, rect.Height)
}

// Draw issues a non-indexed draw call.
func (e *Encoder) Draw(vertexCount, instanceCount, firstVertex int) {
	if e.inRenderPass {
		e.passEncoder.Call("draw", vertexCount, instanceCount, firstVertex, 0)
	}
}

// DrawIndexed issues an indexed draw call.
// Uniforms are re-bound before each draw to pick up per-batch changes
// (e.g., color matrix) that were set on the shader after SetPipeline.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	if e.inRenderPass {
		e.bindUniforms()
		e.passEncoder.Call("drawIndexed", indexCount, instanceCount, firstIndex, 0, 0)
	}
}

// Flush is a no-op — submission happens in EndRenderPass.
func (e *Encoder) Flush() {}
