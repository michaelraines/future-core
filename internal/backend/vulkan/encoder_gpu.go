//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/vk"
)

// Encoder implements backend.CommandEncoder for Vulkan by recording into
// a VkCommandBuffer.
type Encoder struct {
	dev *Device
	cmd vk.CommandBuffer

	// Current render pass state.
	inRenderPass      bool
	currentRenderPass vk.RenderPass
	currentRenderTarget *RenderTarget // nil for the default/screen target
	currentPipeline   *Pipeline
	boundTexture      *Texture
	boundShader       *Shader
	// boundFilter drives samplerFor() in bindUniforms. Defaults to
	// FilterNearest (which is also backend.TextureFilter's zero value),
	// matching the prior always-nearest behaviour. SetTextureFilter
	// overrides per draw.
	boundFilter    backend.TextureFilter
	descriptorPool vk.DescriptorPool
	descriptorSet  vk.DescriptorSet
	colorWriteOn   bool

	// Command-buffer lifecycle tracking. The encoder's `cmd` is shared
	// with the device's main command buffer. Two usage modes exist:
	//
	//  1. App flow: Device.BeginFrame does BeginCommandBuffer, the
	//     encoder records many passes, Device.EndFrame submits. In this
	//     path Encoder.Flush is a no-op — submission is handled by
	//     EndFrame and a mid-frame Flush (e.g. the sprite pass calling
	//     Flush at pass boundaries) must NOT submit.
	//
	//  2. Standalone flow: conformance tests + any other caller that
	//     drives the encoder without BeginFrame/EndFrame. Here commands
	//     get recorded into a cmd buffer that was never begun — Vulkan
	//     silently drops them on MoltenVK (validation layers would
	//     catch it) and ReadPixels returns uninitialized memory. For
	//     this path the encoder lazily begins recording on the first
	//     command and Flush ends + submits + waits + resets so
	//     subsequent passes start clean.
	//
	// `recording` tracks whether BeginCommandBuffer is currently open
	// on e.cmd. `standalone` tracks whether the encoder itself opened
	// it (and so should close it on Flush) vs whether BeginFrame did.
	recording  bool
	standalone bool

	// Per-draw blend state. SetBlendMode stores the caller's intent;
	// SetPipeline reads it back and picks (or creates) the matching
	// pipeline variant. The sprite pass drives this: e.g.
	// `enc.SetBlendMode(b.BlendMode); enc.SetPipeline(p)` — the
	// pattern we inherited from WebGPU / Metal, which both key their
	// pipelines on (shader, blend) pairs. On Vulkan this used to be
	// dropped silently because SetBlendMode was a no-op and the
	// pipeline shipped with BlendSourceOver baked in. `blendModeSet`
	// distinguishes "caller explicitly requested a blend" from "use
	// the pipeline-descriptor default" so a bare SetPipeline without
	// SetBlendMode keeps the legacy behavior instead of silently
	// defaulting to the previous frame's leftover blend.
	currentBlendMode backend.BlendMode
	blendModeSet     bool

	// Per-draw bindUniforms scratch. bindUniforms runs on every
	// DrawIndexed and used to allocate a fresh []vk.WriteDescriptorSet
	// and []vk.DescriptorSet per call; at ~1000 draws/frame (the
	// lighting demo with many lights spawned) those two-alloc-per-draw
	// patterns generated 120k+ GC-tracked allocations per second and
	// showed up as a visible FPS cliff. Fixed-size arrays on the
	// encoder let us reuse the backing memory across draws.
	//
	// LIFETIME: the PImageInfo / PBufferInfo fields inside
	// writeScratch[i] are uintptrs into *stack locals* in bindUniforms
	// (imgInfo, fragBufInfo, vtxBufInfo). Those pointers are valid
	// only until bindUniforms returns — vk.UpdateDescriptorSets
	// consumes them before that point, and runtime.KeepAlive guards
	// the stack escape. Do NOT read writeScratch outside of
	// bindUniforms; the embedded pointers will dangle.
	writeScratch [3]vk.WriteDescriptorSet
	setScratch   [1]vk.DescriptorSet
}

// ensureRecording lazily begins the command buffer if nothing has done
// so yet. Called from any entry point that records commands, so that
// a standalone-mode caller (conformance tests, direct device tests)
// doesn't record into a cmd buffer that was never begun. The flip side
// — BeginFrame-driven app flow — sets `recording` from outside and
// this is a no-op.
func (e *Encoder) ensureRecording() {
	if e.recording || e.cmd == 0 {
		return
	}
	_ = vk.ResetCommandBuffer(e.cmd)
	_ = vk.BeginCommandBuffer(e.cmd, vk.CommandBufferUsageOneTimeSubmit)
	e.recording = true
	e.standalone = true
}

// markRecording tells the encoder that BeginFrame has begun the shared
// command buffer. Called from Device.BeginFrame after BeginCommandBuffer
// so the encoder knows it doesn't own the lifecycle — Flush should stay
// a no-op and EndFrame will handle submission.
func (e *Encoder) markRecording() {
	e.recording = true
	e.standalone = false
}

// markNotRecording tells the encoder that EndFrame has submitted the
// shared command buffer, so the next recording must start fresh.
func (e *Encoder) markNotRecording() {
	e.recording = false
	e.standalone = false
}

// BeginRenderPass begins a Vulkan render pass.
//
// Note on load ops: Vulkan bakes AttachmentLoadOp into the VkRenderPass
// object at creation, so desc.StencilLoadAction is ignored on this
// backend — the shared depth-stencil attachment always clears on load
// (see createSwapchain / createDefaultRenderTarget). The sprite pass's
// color-pass pipeline self-clears the stencil buffer via DPPass=Zero so
// successive fill-rule batches within a pass still start clean, and
// today no caller sets StencilLoadAction=LoadActionLoad on a Vulkan
// target. When that changes, switch to render-pass variants keyed on
// (stencilLoadOp, stencilStoreOp) instead of the single shared object.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	e.ensureRecording()

	// Vulkan render passes on this backend always declare color +
	// depth-stencil attachments, so every vkCmdBeginRenderPass needs
	// two matching clear values. The second slot reuses ClearValue's
	// 16-byte union: first 4 bytes = depth float32, next 4 = stencil
	// uint32.
	clearValues := [2]vk.ClearValue{
		{Color: desc.ClearColor},
		makeDepthStencilClearValue(1.0, desc.ClearStencil),
	}

	// Default offscreen target (used when no swapchain exists — desktop
	// builds use GL presenter + ReadScreen from the default color image
	// rather than a VkSwapchain). Pick Clear vs Load variant so screen
	// re-entries preserve prior composites.
	rp := e.dev.defaultRenderPass
	if desc.LoadAction == backend.LoadActionLoad && e.dev.defaultRenderPassLoad != 0 {
		rp = e.dev.defaultRenderPassLoad
	}
	fb := e.dev.defaultFramebuffer
	w := uint32(e.dev.width)
	h := uint32(e.dev.height)

	if desc.Target != nil {
		if rt, ok := desc.Target.(*RenderTarget); ok {
			e.currentRenderTarget = rt
			w = uint32(rt.w)
			h = uint32(rt.h)
			// Pick the render-pass variant matching the load action. Vulkan
			// bakes LoadOp into the VkRenderPass, so we carry a Clear and
			// Load pair on each RT and pick the right one here. The
			// framebuffer is render-pass-compatible with both.
			if desc.LoadAction == backend.LoadActionLoad && rt.renderPassLoad != 0 {
				rp = rt.renderPassLoad
			} else if rt.renderPass != 0 {
				rp = rt.renderPass
			}
			if rt.framebuffer != 0 {
				fb = rt.framebuffer
			}
		}
	} else if e.dev.hasSwapchain {
		// Rendering to the screen — use swapchain render pass and framebuffer.
		// Pick Clear vs Load variant to match the caller's LoadAction.
		// The sprite pass composites multiple offscreen RTs onto the
		// screen in back-to-back render passes within a single frame;
		// without a Load variant every re-entry clears the swapchain and
		// only the final composite survives. Both variants share the
		// same framebuffers (render-pass-compatible: same attachment
		// formats and counts, just different LoadOp/InitialLayout).
		if desc.LoadAction == backend.LoadActionLoad && e.dev.swapchainRenderPassLoad != 0 {
			rp = e.dev.swapchainRenderPassLoad
		} else {
			rp = e.dev.swapchainRenderPass
		}
		fb = e.dev.swapchainFBs[e.dev.currentImageIndex]
		w = e.dev.swapchainExtent[0]
		h = e.dev.swapchainExtent[1]
	}

	rpBegin := vk.RenderPassBeginInfo{
		SType:           vk.StructureTypeRenderPassBeginInfo,
		RenderPass_:     rp,
		Framebuffer_:    fb,
		RenderAreaW:     w,
		RenderAreaH:     h,
		ClearValueCount: 2,
		PClearValues:    uintptr(unsafe.Pointer(&clearValues[0])),
	}
	vk.CmdBeginRenderPass(e.cmd, &rpBegin)
	runtime.KeepAlive(clearValues)
	e.inRenderPass = true
	e.currentRenderPass = rp

	// Set default scissor/viewport matching the render area. Vulkan requires
	// both to be set before draws when using dynamic state.
	vk.CmdSetScissor(e.cmd, vk.Rect2D{ExtentW: w, ExtentH: h})
	vk.CmdSetViewport(e.cmd, vk.Viewport{
		Width: float32(w), Height: float32(h),
		MaxDepth: 1,
	})
	e.colorWriteOn = true
}

// makeDepthStencilClearValue packs a depth float32 and stencil uint32
// into a vk.ClearValue. Vulkan's VkClearValue is a union of
// [4]float32 color and {float32 depth, uint32 stencil}; both share the
// same 16-byte storage, and the Go binding models the color side as
// [4]float32. We overlay the first 8 bytes with a ClearValueDepthStencil
// struct — a single typed pointer conversion rather than raw uintptr
// arithmetic, so intent is explicit and tooling stays quiet.
func makeDepthStencilClearValue(depth float32, stencil uint32) vk.ClearValue {
	var cv vk.ClearValue
	*(*vk.ClearValueDepthStencil)(unsafe.Pointer(&cv)) = vk.ClearValueDepthStencil{
		Depth:   depth,
		Stencil: stencil,
	}
	return cv
}

// EndRenderPass ends the current render pass.
func (e *Encoder) EndRenderPass() {
	if !e.inRenderPass {
		return
	}
	vk.CmdEndRenderPass(e.cmd)
	e.inRenderPass = false
	e.currentRenderPass = 0

	// Explicit memory barrier on the just-rendered RT color image,
	// forcing color-attachment writes visible to subsequent
	// fragment-shader reads. The render pass's 0→EXTERNAL subpass
	// dependency already declares this chain (see
	// createOffscreenRenderPass), but MoltenVK on macOS intermittently
	// does not flush the implicit transition before the next render
	// pass samples the image — manifesting as scene-selector tiles
	// disappearing on alternating frames. Latency (e.g. stderr
	// tracing) masks the race, which is the giveaway that the
	// subpass-dep promise isn't always honoured on Metal.
	//
	// The image is already in ShaderReadOnlyOptimal via the render
	// pass FinalLayout, so oldLayout == newLayout — no layout
	// transition, just the memory-dependency side of the barrier.
	// Screen targets are presented, not sampled, and don't need this.
	if e.currentRenderTarget != nil && e.currentRenderTarget.colorTex != nil {
		barriers := []vk.ImageMemoryBarrier{{
			SType:               vk.StructureTypeImageMemoryBarrier,
			SrcAccessMask:       vk.AccessColorAttachmentWrite,
			DstAccessMask:       vk.AccessShaderRead,
			OldLayout:           vk.ImageLayoutShaderReadOnlyOptimal,
			NewLayout:           vk.ImageLayoutShaderReadOnlyOptimal,
			SrcQueueFamilyIndex: vk.QueueFamilyIgnored,
			DstQueueFamilyIndex: vk.QueueFamilyIgnored,
			Image_:              e.currentRenderTarget.colorTex.image,
			SubresAspectMask:    vk.ImageAspectColor,
			SubresLevelCount:    1,
			SubresLayerCount:    1,
		}}
		vk.CmdPipelineBarrier(e.cmd,
			vk.PipelineStageColorAttachmentOutput,
			vk.PipelineStageFragmentShader,
			barriers)
		runtime.KeepAlive(barriers)
	}
	e.currentRenderTarget = nil

	// NOTE: Do NOT destroy the descriptor pool here! The command buffer
	// hasn't been submitted yet. Destroying the pool would free the
	// descriptor sets while the GPU still references them. Cleanup
	// happens in resetFrame() after the fence signals.
}

// SetPipeline binds a VkPipeline.
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	p, ok := pipeline.(*Pipeline)
	if !ok {
		return
	}
	e.currentPipeline = p

	// Track the shader for uniform binding.
	if s, ok := p.desc.Shader.(*Shader); ok {
		e.boundShader = s
	}

	// Pipelines are baked against a specific (VkRenderPass, BlendMode)
	// pair at creation time. A single `Pipeline` abstract object serves
	// every render pass (offscreen RTs, screen swapchain) the sprite
	// pass walks through in a frame AND every blend mode a caller
	// switches through via SetBlendMode (multiply composite, additive
	// lights, per-blend shadow stamps). Cache one VkPipeline per
	// (renderPass, blend) and create on demand.
	rp := e.currentRenderPass
	if rp == 0 {
		rp = e.dev.defaultRenderPass
	}
	blend := p.desc.BlendMode
	if e.blendModeSet {
		blend = e.currentBlendMode
	}
	pip := p.pipelineFor(rp, blend)
	if pip == 0 {
		if err := p.createVkPipeline(rp, blend); err != nil {
			return
		}
		pip = p.pipelineFor(rp, blend)
	}
	if pip != 0 {
		vk.CmdBindPipeline(e.cmd, pip)
		if sh := e.boundShader; sh != nil {
			tracePipelineBind(sh.vertexSource, sh.fragmentSource, uint64(pip))
		}
	}
}

// SetVertexBuffer binds a vertex buffer at the most recently uploaded offset.
func (e *Encoder) SetVertexBuffer(buf backend.Buffer, slot int) {
	if b, ok := buf.(*Buffer); ok {
		vk.CmdBindVertexBuffer(e.cmd, uint32(slot), b.buffer, uint64(b.lastWriteOffset))
		traceVertexBind(slot, uint64(b.buffer), uint64(b.lastWriteOffset))
	}
}

// SetIndexBuffer binds an index buffer at the most recently uploaded offset.
func (e *Encoder) SetIndexBuffer(buf backend.Buffer, format backend.IndexFormat) {
	if b, ok := buf.(*Buffer); ok {
		idxType := uint32(vk.IndexTypeUint16)
		if format == backend.IndexUint32 {
			idxType = vk.IndexTypeUint32
		}
		vk.CmdBindIndexBuffer(e.cmd, b.buffer, uint64(b.lastWriteOffset), idxType)
	}
}

// SetTexture records the texture to bind at the given slot. The actual
// descriptor set update happens in bindUniforms before each draw call so
// that the sampler, fragment UBO, and vertex UBO are all written together.
func (e *Encoder) SetTexture(tex backend.Texture, slot int) {
	t, ok := tex.(*Texture)
	if !ok {
		return
	}
	e.boundTexture = t
}

// SetTextureFilter overrides the texture filter for a slot. Vulkan
// bakes filter state into the VkSampler rather than setting it dynamically,
// so this records the requested filter and the descriptor-binding path
// (bindUniforms) looks up the matching cached sampler via
// Device.samplerFor(). Slot is ignored because today every backend binds
// a single combined-image-sampler at binding 0 — when a multi-texture
// path arrives this extends to a per-slot filter array.
func (e *Encoder) SetTextureFilter(slot int, filter backend.TextureFilter) {
	_ = slot
	e.boundFilter = filter
}

// SetStencilReference updates the dynamic stencil reference value.
// Stencil ops/func/masks are baked into the VkPipeline; the pipeline must
// include VK_DYNAMIC_STATE_STENCIL_REFERENCE in its dynamic states for
// this command to take effect.
func (e *Encoder) SetStencilReference(ref uint32) {
	if e.cmd == 0 {
		return
	}
	vk.CmdSetStencilReference(e.cmd, vk.StencilFaceFrontAndBack, ref)
}

// SetColorWrite enables or disables writing to the color buffer.
func (e *Encoder) SetColorWrite(enabled bool) {
	// Color write mask is baked into the VkPipeline in Vulkan.
	// A full implementation would require pipeline variants.
	e.colorWriteOn = enabled
}

// SetViewport sets the rendering viewport.
func (e *Encoder) SetViewport(vp backend.Viewport) {
	vkVP := vk.Viewport{
		X: float32(vp.X), Y: float32(vp.Y),
		Width: float32(vp.Width), Height: float32(vp.Height),
		MinDepth: 0, MaxDepth: 1,
	}
	vk.CmdSetViewport(e.cmd, vkVP)
}

// SetScissor sets the scissor rectangle.
func (e *Encoder) SetScissor(rect *backend.ScissorRect) {
	if rect == nil {
		// Disable scissor by setting to full viewport size.
		vk.CmdSetScissor(e.cmd, vk.Rect2D{
			ExtentW: uint32(e.dev.width),
			ExtentH: uint32(e.dev.height),
		})
		return
	}
	vk.CmdSetScissor(e.cmd, vk.Rect2D{
		OffsetX: int32(rect.X),
		OffsetY: int32(rect.Y),
		ExtentW: uint32(rect.Width),
		ExtentH: uint32(rect.Height),
	})
}

// Draw issues a non-indexed draw call.
func (e *Encoder) Draw(vertexCount, instanceCount, firstVertex int) {
	e.bindUniforms()
	vk.CmdDraw(e.cmd, uint32(vertexCount), uint32(instanceCount), uint32(firstVertex), 0)
}

// DrawIndexed issues an indexed draw call.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	e.bindUniforms()
	vk.CmdDrawIndexed(e.cmd, uint32(indexCount), uint32(instanceCount), uint32(firstIndex), 0, 0)
}

// uniformAlignOffset is the minimum offset alignment for UBO descriptors.
// Vulkan spec requires minUniformBufferOffsetAlignment, typically 256 bytes.
const uniformAlignOffset = 256

// descriptorPoolMaxSets caps descriptor sets allocated per frame. Every
// DrawIndexed allocates one set (sampler + frag UBO + vertex UBO) via
// bindUniforms, and scene-selector runs ~100 draws per frame, with
// content-heavy scenes pushing that higher. An earlier 16-set pool
// exhausted mid-frame on any non-trivial scene, and
// vk.AllocateDescriptorSet returns an error without a log path here —
// the silent failure left the previous draw's descriptors bound, so
// everything downstream sampled from the wrong texture/UBO (visible as
// all-white scene-selector, all-black bubble-pop). The pool is reset
// per frame in resetFrame() so the cap is per-frame not forever; we
// bound descriptor-set counts and sizes generously because individual
// sets are cheap and bumping the cap here is strictly simpler than
// implementing pool-grow-on-exhaustion.
const (
	descriptorPoolMaxSets  = 2048
	descriptorPoolSamplers = 2048
	// UBOs: vertex + fragment per set.
	descriptorPoolUBOs = 2 * descriptorPoolMaxSets
)

// ensureDescriptorPool creates a descriptor pool supporting combined image
// samplers and uniform buffers if one does not already exist.
func (e *Encoder) ensureDescriptorPool() bool {
	if e.descriptorPool != 0 {
		return true
	}
	if e.currentPipeline == nil || e.currentPipeline.descSetLayout == 0 {
		return false
	}
	poolSizes := []vk.DescriptorPoolSize{
		{Type_: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: descriptorPoolSamplers},
		{Type_: vk.DescriptorTypeUniformBuffer, DescriptorCount: descriptorPoolUBOs},
	}
	poolCI := vk.DescriptorPoolCreateInfo{
		SType:         vk.StructureTypeDescriptorPoolCreateInfo,
		MaxSets:       descriptorPoolMaxSets,
		PoolSizeCount: uint32(len(poolSizes)),
		PPoolSizes:    uintptr(unsafe.Pointer(&poolSizes[0])),
	}
	pool, err := vk.CreateDescriptorPool(e.dev.device, &poolCI)
	runtime.KeepAlive(poolSizes)
	if err != nil {
		return false
	}
	e.descriptorPool = pool
	return true
}

// bindUniforms writes shader uniform data into the shared UBO buffer and
// binds a descriptor set with the sampler (binding 0), fragment UBO
// (binding 1), and vertex UBO (binding 2).
func (e *Encoder) bindUniforms() {
	if e.boundShader == nil || e.currentPipeline == nil || e.currentPipeline.pipelineLayout == 0 {
		return
	}
	if e.dev.uniformMapped == nil {
		return
	}
	if !e.ensureDescriptorPool() {
		return
	}

	// Pack vertex and fragment uniforms directly into the mapped UBO
	// ring buffer. A prior implementation allocated two []byte per
	// draw for packing and then copied — that was ~2 heap allocations
	// per draw, which at ~1000 draws/frame pushed 120k alloc/s into
	// the GC and showed up as a FPS cliff in the lighting demo when
	// users clicked to add more lights.
	vtxSize := uniformLayoutSize(e.boundShader.vertexUniformLayout)
	fragSize := uniformLayoutSize(e.boundShader.fragmentUniformLayout)

	// Each draw needs: vtxSize (aligned to 256) + fragSize (aligned to 256).
	vtxAligned := (vtxSize + uniformAlignOffset - 1) &^ (uniformAlignOffset - 1)
	if vtxAligned < uniformAlignOffset {
		vtxAligned = uniformAlignOffset
	}
	fragAligned := (fragSize + uniformAlignOffset - 1) &^ (uniformAlignOffset - 1)
	if fragAligned < uniformAlignOffset {
		fragAligned = uniformAlignOffset
	}
	needed := vtxAligned + fragAligned

	// Wrap if we'd overflow.
	if e.dev.uniformCursor+needed > e.dev.uniformBufSize {
		e.dev.uniformCursor = 0
	}

	vtxOffset := e.dev.uniformCursor
	fragOffset := vtxOffset + vtxAligned

	fullBuf := unsafe.Slice((*byte)(e.dev.uniformMapped), e.dev.uniformBufSize)
	if vtxSize > 0 {
		e.boundShader.packUniformBufferInto(e.boundShader.vertexUniformLayout,
			fullBuf[vtxOffset:vtxOffset+vtxSize])
	}
	if fragSize > 0 {
		e.boundShader.packUniformBufferInto(e.boundShader.fragmentUniformLayout,
			fullBuf[fragOffset:fragOffset+fragSize])
	}
	e.dev.uniformCursor += needed

	// Allocate a descriptor set from the pool.
	set, err := vk.AllocateDescriptorSet(e.dev.device, e.descriptorPool, e.currentPipeline.descSetLayout)
	if err != nil {
		return
	}
	e.descriptorSet = set

	// Binding 0: combined image sampler. Picks the sampler matching
	// the current filter state (Nearest / Linear); without this the
	// AA buffer downsample composite silently used Nearest even though
	// the caller asked for Linear.
	sampler := e.dev.samplerFor(e.boundFilter)
	tex := e.boundTexture
	if tex == nil {
		tex = e.dev.defaultTexture
	}
	imgInfo := vk.DescriptorImageInfo{
		Sampler:     sampler,
		ImageView:   tex.view,
		ImageLayout: vk.ImageLayoutShaderReadOnlyOptimal,
	}
	fragBufInfo := vk.DescriptorBufferInfo{
		Buffer_: e.dev.uniformBuffer,
		Offset:  uint64(fragOffset),
		Range_:  uint64(uniformAlignOffset),
	}
	vtxBufInfo := vk.DescriptorBufferInfo{
		Buffer_: e.dev.uniformBuffer,
		Offset:  uint64(vtxOffset),
		Range_:  uint64(uniformAlignOffset),
	}

	// Build descriptor writes for all 3 bindings. Reuse the encoder's
	// cached array (and backing slice) so each draw doesn't heap-grow
	// a fresh []vk.WriteDescriptorSet — the lighting demo with N
	// lights spends most of its per-frame time in bindUniforms and
	// every tiny alloc here multiplies through the GC.
	e.writeScratch[0] = vk.WriteDescriptorSet{
		SType:           vk.StructureTypeWriteDescriptorSet,
		DstSet:          set,
		DstBinding:      0,
		DescriptorCount: 1,
		DescriptorType:  vk.DescriptorTypeCombinedImageSampler,
		PImageInfo:      uintptr(unsafe.Pointer(&imgInfo)),
	}
	e.writeScratch[1] = vk.WriteDescriptorSet{
		SType:           vk.StructureTypeWriteDescriptorSet,
		DstSet:          set,
		DstBinding:      1,
		DescriptorCount: 1,
		DescriptorType:  vk.DescriptorTypeUniformBuffer,
		PBufferInfo:     uintptr(unsafe.Pointer(&fragBufInfo)),
	}
	e.writeScratch[2] = vk.WriteDescriptorSet{
		SType:           vk.StructureTypeWriteDescriptorSet,
		DstSet:          set,
		DstBinding:      2,
		DescriptorCount: 1,
		DescriptorType:  vk.DescriptorTypeUniformBuffer,
		PBufferInfo:     uintptr(unsafe.Pointer(&vtxBufInfo)),
	}

	vk.UpdateDescriptorSets(e.dev.device, e.writeScratch[:])
	runtime.KeepAlive(imgInfo)
	runtime.KeepAlive(fragBufInfo)
	runtime.KeepAlive(vtxBufInfo)

	// Bind the descriptor set. Reuse the cached one-element slice so we
	// don't allocate a new []vk.DescriptorSet per draw.
	e.setScratch[0] = set
	vk.CmdBindDescriptorSets(e.cmd, e.currentPipeline.pipelineLayout, 0, e.setScratch[:])
}

// SetBlendMode records the caller's desired blend state. The next
// SetPipeline call will pick (or create) the matching pipeline
// variant. Previously this was a no-op, which silently dropped any
// non-pipeline-descriptor blend the caller asked for — scenes that
// relied on mid-frame blend swaps (multiply lightmap composite,
// additive light stamps, shadow-mask blend pass) fell through to the
// pipeline's baked-in SourceOver. See Encoder.currentBlendMode for
// the full context.
func (e *Encoder) SetBlendMode(b backend.BlendMode) {
	e.currentBlendMode = b
	e.blendModeSet = true
}

// Flush submits pending command-buffer work — but only when the
// encoder owns the cmd-buffer lifecycle (standalone mode). In the
// app flow where Device.BeginFrame/EndFrame brackets recording, Flush
// is a no-op: the sprite pass calls it at pass boundaries purely as
// a backend hook, and an unconditional submit here would break the
// per-frame submission contract (one submit per frame, sync'd with
// the swapchain semaphores). Standalone callers — conformance tests
// and direct-device unit tests — need Flush to actually execute
// their recorded work before ReadPixels can see it.
func (e *Encoder) Flush() {
	if !e.standalone || !e.recording || e.cmd == 0 {
		return
	}
	_ = vk.EndCommandBuffer(e.cmd)
	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    uintptr(unsafe.Pointer(&e.cmd)),
	}
	cmd := e.cmd
	_ = vk.QueueSubmit(e.dev.graphicsQueue, &submitInfo, 0)
	runtime.KeepAlive(cmd)
	runtime.KeepAlive(submitInfo)
	// Wait synchronously — Flush is a fence-like call in the
	// conformance/standalone path; callers follow it immediately with
	// ReadPixels which must see the committed output.
	_ = vk.DeviceWaitIdle(e.dev.device)
	e.recording = false
	e.standalone = false

	// Standalone callers (conformance tests + direct device unit tests)
	// drive the encoder through BeginRenderPass → draws → EndRenderPass
	// → Flush repeatedly without Device.BeginFrame bracketing, so
	// resetFrame() never runs. Every bindUniforms allocation from the
	// 2048-set descriptor pool accumulates across Flush calls; once the
	// pool exhausts, AllocateDescriptorSet silently errors and every
	// subsequent draw renders nothing. Reset here so the pool gets
	// reused across standalone cycles too.
	e.resetFrame()
}

// resetFrame resets descriptor pool for the next frame.
// Called from Device.BeginFrame after the fence signals (GPU work complete).
// Also clears the sticky blend override so a SetBlendMode call from a
// prior frame doesn't leak into the new frame's first SetPipeline.
// Without this reset a scene that calls SetBlendMode(Additive) once for
// a light draw and then relies on the pipeline-descriptor's default blend
// on the next frame's first draw would silently re-use Additive.
func (e *Encoder) resetFrame() {
	if e.descriptorPool != 0 {
		// Reset is much cheaper than destroy+recreate — keeps the pool
		// allocated and just frees all sets.
		vk.ResetDescriptorPool(e.dev.device, e.descriptorPool)
		e.descriptorSet = 0
	}
	e.blendModeSet = false
	e.currentBlendMode = backend.BlendMode{}
}
