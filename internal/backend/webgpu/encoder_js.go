//go:build js && !soft

package webgpu

import (
	"fmt"
	"syscall/js"

	"github.com/michaelraines/future-core/internal/backend"
)

// Diagnostic console.log budget — enabled by FUTURE_CORE_DIAG_LOGS=N,
// disabled by default (limit=0). When enabled, the browser backend emits
// console.log lines from BeginRenderPass, Flush, and createRenderPipeline
// up to a combined total of N entries across the session.
//
// This is a coarser tool than FUTURE_CORE_TRACE_WEBGPU: that tracer is
// frame-scoped (logs every call for the first N frames) and writes to
// stderr. This one is per-call-site and writes to console.log so entries
// show up naturally in the browser's devtools while probing from Go.
//
// When a subtle WebGPU-browser rendering regression is suspected
// (mysterious blank canvas, wrong attachment sizes, silent pipeline
// creation failures), start here: set FUTURE_CORE_DIAG_LOGS=200 on
// go.env in wasm_exec.js and watch the console stream.
var (
	diagLogLimit      = parseTraceEnvInt("FUTURE_CORE_DIAG_LOGS")
	diagLogCount      int
	beginPassLogCount int
	flushLogCount     int
)

// diagLogAllow reports whether a diagnostic console.log line should fire
// for a given bucket counter. Increments the counter on allow.
func diagLogAllow(counter *int) bool {
	if diagLogLimit == 0 {
		return false
	}
	if *counter >= diagLogLimit {
		return false
	}
	*counter++
	diagLogCount++
	return true
}

// Encoder implements backend.CommandEncoder for WebGPU via the browser JS API.
type Encoder struct {
	dev    *Device
	width  int
	height int

	// Dimensions of the current render target (updated each BeginRenderPass).
	// SetScissor(nil) defaults to these, matching the active attachment;
	// using e.width/e.height would leak the device's default (canvas) size
	// into passes rendering to smaller offscreen RTs and trip WebGPU's
	// "Scissor rect is not contained in the render target" validation
	// (observed in the browser orb-drop demo: canvas 2048×1536 with RT
	// 1024×768).
	currentW int
	currentH int

	inRenderPass    bool
	currentPipeline *Pipeline
	passEncoder     js.Value
	cmdEncoder      js.Value

	// Format of the current render target (set in BeginRenderPass).
	targetFormat string

	// Current sampler filter per slot.
	slotFilters map[int]string

	// Cached uniform bind group for the current pipeline, referencing the
	// ring buffer with hasDynamicOffset. Recreated only on pipeline change.
	uniformBG         js.Value
	uniformBGPipeline *Pipeline // pipeline the cached BG was created for

	// Pre-allocated Uint32Array(1) reused for dynamic-offset setBindGroup
	// calls, avoiding a JS typed-array allocation per draw.
	dynOffsets js.Value

	// Keep temporary bind groups alive until EndRenderPass submits the
	// command buffer. Without this, the Go GC may release JS references
	// before the GPU executes the draws.
	tempRefs []js.Value

	// Pending blend mode set by SetBlendMode, applied on next SetPipeline.
	pendingBlend    backend.BlendMode
	pendingBlendSet bool
}

// BeginRenderPass begins a WebGPU render pass.
// The command encoder is created lazily on the first call per frame and
// reused across all render passes until Flush submits the accumulated
// command buffer. This batches all passes into a single queue.submit,
// dramatically reducing Go→JS boundary crossings.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	if e.cmdEncoder.IsUndefined() || e.cmdEncoder.IsNull() {
		e.cmdEncoder = e.dev.device.Call("createCommandEncoder")
	}

	// Canvas may resize between frames (scenes that request different
	// viewport sizes) — re-allocate the screen depth-stencil if so to keep
	// color+depth attachments the same size.
	e.dev.ensureScreenDepthStencilForCanvas()

	view := e.dev.currentColorView()
	w, h := e.width, e.height
	depthView := e.dev.screenDepthView
	hasStencil := true

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
			depthView = js.Undefined()
			hasStencil = rt.hasStencil
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
		if hasStencil {
			depthAttach.Set("stencilLoadOp", "clear")
			depthAttach.Set("stencilStoreOp", "store")
			depthAttach.Set("stencilClearValue", int(desc.ClearStencil))
		}
		rpDesc.Set("depthStencilAttachment", depthAttach)
	}

	e.passEncoder = e.cmdEncoder.Call("beginRenderPass", rpDesc)
	e.inRenderPass = true
	e.currentW = w
	e.currentH = h
	if diagLogAllow(&beginPassLogCount) {
		js.Global().Get("console").Call("log",
			fmt.Sprintf("[webgpu] BeginRenderPass target=%v format=%s w=%d h=%d depthView=%v hasStencil=%v loadOp=%s",
				desc.Target != nil, e.targetFormat, w, h,
				!depthView.IsUndefined() && !depthView.IsNull(),
				hasStencil, loadOp))
	}

	// Set default viewport.
	e.passEncoder.Call("setViewport", 0, 0, float64(w), float64(h), 0, 1)

	if traceWebGPUActive() {
		rtTag := "screen"
		if rt, ok := desc.Target.(*RenderTarget); ok && rt != nil {
			rtTag = fmt.Sprintf("rt@%p(%dx%d)", rt, rt.w, rt.h)
		}
		traceWebGPUf("[wgpu frame %d] BeginRenderPass target=%s format=%s load=%s viewport=%dx%d clear=[%.2f %.2f %.2f %.2f]\n",
			traceWebGPUFrame.Load()+1, rtTag, e.targetFormat, loadOp, w, h,
			desc.ClearColor[0], desc.ClearColor[1], desc.ClearColor[2], desc.ClearColor[3])
	}
}

// EndRenderPass ends the current render pass but does NOT submit the
// command buffer. Call Flush() after all render passes to submit the
// accumulated work as a single queue.submit(). This batching reduces
// Go→JS boundary crossings from O(passes) to O(1) per frame.
func (e *Encoder) EndRenderPass() {
	if e.inRenderPass {
		e.passEncoder.Call("end")
		e.passEncoder = js.Undefined()
		e.inRenderPass = false
		e.uniformBGPipeline = nil
		if traceWebGPUActive() {
			traceWebGPUf("[wgpu frame %d] EndRenderPass\n", traceWebGPUFrame.Load()+1)
		}
	}
}

// SetPipeline binds a render pipeline.
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	p, ok := pipeline.(*Pipeline)
	if !ok {
		return
	}
	e.currentPipeline = p

	// Lazily create (or recreate) the pipeline for the current target
	// format and blend mode. Custom shader draws use SetBlendMode to
	// request per-draw blend (e.g., additive for lights).
	if e.pendingBlendSet {
		p.ensurePipeline(e.targetFormat, e.pendingBlend)
		e.pendingBlendSet = false
	} else {
		p.ensurePipelineForFormat(e.targetFormat)
	}

	pipelineOK := !p.handle.IsUndefined() && !p.handle.IsNull()
	if pipelineOK && e.inRenderPass {
		e.passEncoder.Call("setPipeline", p.handle)
	}
	if traceWebGPUActive() {
		traceWebGPUf("[wgpu frame %d] SetPipeline p@%p format=%s blend=%+v pipelineOK=%v\n",
			traceWebGPUFrame.Load()+1, p, p.createdFormat, p.createdBlend, pipelineOK)
	}

	e.bindUniforms()

	// Bind default texture to group 1 so that draw calls without an
	// explicit SetTexture don't fail with "No bind group set at group index 1".
	// Any subsequent SetTexture call will override this.
	e.bindDefaultTexture()
}

// bindUniforms uploads shader uniforms into the ring buffer and binds
// group 0 with a dynamic offset. Skips the entire pack+upload+bind cycle
// when no uniform values have changed since the last bind, avoiding
// per-draw heap allocations and Go→JS round-trips.
func (e *Encoder) bindUniforms() {
	if e.currentPipeline == nil || !e.inRenderPass {
		return
	}
	shader, ok := e.currentPipeline.desc.Shader.(*Shader)
	if !ok || shader == nil {
		return
	}

	// Fast path: if no SetUniform* calls have happened since the last
	// bindUniforms, the ring buffer already has the right data and the
	// bind group is still bound. Skip everything.
	if !shader.uniformsDirty {
		if traceWebGPUActive() {
			traceWebGPUf("[wgpu frame %d]   bindUniforms SKIP (not dirty)\n", traceWebGPUFrame.Load()+1)
		}
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
	if bgl.IsUndefined() || bgl.IsNull() {
		return
	}

	// Write uniform data into the ring buffer at a 256-byte-aligned offset.
	offset, alignedSize := e.dev.writeUniformRing(data)
	if alignedSize == 0 {
		return
	}

	// Create (or reuse) one bind group per pipeline that references the
	// entire ring buffer. Dynamic offsets select the per-draw sub-region.
	if e.uniformBGPipeline != e.currentPipeline {
		entry := js.Global().Get("Object").New()
		entry.Set("binding", 0)
		bufBinding := js.Global().Get("Object").New()
		bufBinding.Set("buffer", e.dev.uniformBuf)
		bufBinding.Set("offset", 0)
		bufBinding.Set("size", uniformAlignOffset) // min binding size
		entry.Set("resource", bufBinding)

		bgDesc := js.Global().Get("Object").New()
		bgDesc.Set("layout", bgl)
		bgDesc.Set("entries", js.Global().Get("Array").New(entry))
		e.uniformBG = e.dev.device.Call("createBindGroup", bgDesc)
		e.uniformBGPipeline = e.currentPipeline
		e.tempRefs = append(e.tempRefs, e.uniformBG)
	}

	// Bind with dynamic offset pointing to this draw's data.
	e.dynOffsets.SetIndex(0, offset)
	e.passEncoder.Call("setBindGroup", 0, e.uniformBG, e.dynOffsets)

	if traceWebGPUActive() {
		// First 8 bytes of the payload — enough to distinguish per-light
		// Center values when correlating with DrawIndexed below.
		summary := ""
		if len(data) >= 8 {
			summary = fmt.Sprintf("first8bytes=[%02x %02x %02x %02x %02x %02x %02x %02x]",
				data[0], data[1], data[2], data[3], data[4], data[5], data[6], data[7])
		}
		traceWebGPUf("[wgpu frame %d]   bindUniforms offset=%d size=%d aligned=%d %s\n",
			traceWebGPUFrame.Load()+1, offset, len(data), alignedSize, summary)
	}
	shader.uniformsDirty = false
}

// bindDefaultTexture binds a 1x1 white placeholder texture to group 1,
// ensuring that draw calls without an explicit SetTexture don't trigger
// "No bind group set at group index 1" validation errors.
// Uses the texture bind group cache so the bind group is created only once.
func (e *Encoder) bindDefaultTexture() {
	if e.currentPipeline == nil || !e.inRenderPass {
		return
	}
	t := e.dev.defaultWhiteTex
	if t == nil {
		return
	}
	e.SetTexture(t, 0)
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
// Bind groups are cached by (textureID, filter) since they are immutable.
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

	var bgl js.Value
	if e.currentPipeline != nil {
		bgl = e.currentPipeline.textureBGL
	}
	if bgl.IsUndefined() || bgl.IsNull() {
		return
	}

	// Check cache.
	key := texBindGroupKey{texID: t.id, filter: filter}
	if bg, ok := e.dev.textureBindGroups[key]; ok {
		e.passEncoder.Call("setBindGroup", 1, bg)
		if traceWebGPUActive() {
			traceWebGPUf("[wgpu frame %d]   SetTexture slot=%d texID=%d filter=%s (cached)\n",
				traceWebGPUFrame.Load()+1, slot, t.id, filter)
		}
		return
	}

	// Cache miss — create and store.
	sampler := e.dev.getSampler(filter)

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

	e.dev.textureBindGroups[key] = bg
	e.passEncoder.Call("setBindGroup", 1, bg)
	if traceWebGPUActive() {
		traceWebGPUf("[wgpu frame %d]   SetTexture slot=%d texID=%d filter=%s (NEW bind group)\n",
			traceWebGPUFrame.Load()+1, slot, t.id, filter)
	}
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

// SetStencilReference updates the dynamic stencil reference value.
// Stencil ops/compare/masks are baked into the pipeline; only ref is dynamic.
// No-op when no render pass is active.
func (e *Encoder) SetStencilReference(ref uint32) {
	if e.inRenderPass {
		e.passEncoder.Call("setStencilReference", int(ref))
	}
}

// SetColorWrite enables or disables color writing (baked into pipeline in WebGPU).
// SetBlendMode records the desired blend mode. On the next SetPipeline
// call, the pipeline will be recreated if the blend differs.
func (e *Encoder) SetBlendMode(mode backend.BlendMode) {
	e.pendingBlend = mode
	e.pendingBlendSet = true
}

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
// A nil rect defaults to the current render target's bounds (tracked in
// BeginRenderPass). Using e.width/e.height here would exceed smaller
// offscreen RT dimensions and trip WebGPU's "Scissor rect not contained
// in the render target" validation on multi-RT frames.
func (e *Encoder) SetScissor(rect *backend.ScissorRect) {
	if !e.inRenderPass {
		return
	}
	if rect == nil {
		e.passEncoder.Call("setScissorRect", 0, 0, e.currentW, e.currentH)
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
		if traceWebGPUActive() {
			traceWebGPUf("[wgpu frame %d]   DrawIndexed indexCount=%d firstIndex=%d\n",
				traceWebGPUFrame.Load()+1, indexCount, firstIndex)
		}
	}
}

// Flush is a no-op — submission happens in EndRenderPass.
// Flush submits all render passes accumulated since the command encoder
// was created (on the first BeginRenderPass of this frame). After Flush,
// the command encoder is released and a new one will be created on the
// next BeginRenderPass. This is the single queue.submit() call for the
// entire frame's GPU work.
func (e *Encoder) Flush() {
	if e.cmdEncoder.IsUndefined() || e.cmdEncoder.IsNull() {
		return
	}
	cmdBuf := e.cmdEncoder.Call("finish")
	if diagLogAllow(&flushLogCount) {
		js.Global().Get("console").Call("log", "[webgpu] Flush: submitting command buffer")
	}
	e.dev.queue.Call("submit", js.Global().Get("Array").New(cmdBuf))
	e.cmdEncoder = js.Undefined()

	// Release temporary references now that the GPU has the commands.
	e.tempRefs = e.tempRefs[:0]
	if traceWebGPUActive() {
		frame := traceWebGPUFrame.Load() + 1
		traceWebGPUf("[wgpu frame %d] Flush (queue.submit) ==== END FRAME ====\n", frame)
		traceWebGPUAdvanceFrame()
	}
}
