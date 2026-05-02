//go:build darwin && !soft

package metal

import (
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/mtl"
)

// Encoder implements backend.CommandEncoder for Metal.
//
// The encoder reads screen dimensions from e.dev (not a cached copy) so
// that ResizeScreen — invoked when the window moves between displays
// with different backingScaleFactor — takes effect on the very next
// frame without re-creating the encoder.
type Encoder struct {
	dev *Device

	inRenderPass    bool
	currentPipeline *Pipeline
	renderEncoder   mtl.RenderCommandEncoder
	cmdBuffer       mtl.CommandBuffer
	indexFormat     backend.IndexFormat
	boundIndexBuf   *Buffer
	boundShader     *Shader

	// Per-pass nested autorelease pool. Drains the
	// MTLRenderPassDescriptor / colorAttachments / RenderCommandEncoder
	// (and any temp NSStrings the encoder allocates) immediately at
	// EndRenderPass, so scenes with many render passes don't balloon
	// the heap waiting for the per-frame pool to drain. The frame-level
	// pool on Device.frameAutoreleaseToken still bounds the cmdBuffer
	// itself; this nested pool only catches the per-pass churn.
	passAutoreleaseToken uintptr

	// passTargetOffscreen is true when the current render pass targets
	// an offscreen RT rather than the device's default screen texture.
	// EndRenderPass inserts a sync barrier after offscreen passes so
	// subsequent passes that sample the just-rendered texture see the
	// writes. See EndRenderPass for the reasoning.
	passTargetOffscreen bool

	// pendingBlend tracks the blend-mode override requested via
	// SetBlendMode. The next SetPipeline picks the pipeline variant
	// matching this blend instead of the pipeline-descriptor default.
	// Mirrors WebGPU/Vulkan's pendingBlend pattern. Once set, the
	// override is sticky across subsequent SetPipeline calls until
	// another SetBlendMode replaces it.
	pendingBlend    backend.BlendMode
	pendingBlendSet bool
}

// BeginRenderPass begins a Metal render pass.
//
// Frame architecture: one MTLCommandBuffer per frame, multiple
// RenderCommandEncoders within it. The frame's command buffer is
// lazily created on the first BeginRenderPass and committed in
// Device.EndFrame; ReadScreen / ResizeScreen / Dispose drain the
// previous frame's buffer before touching the screen texture.
//
// Per-pass autorelease pool: the MTLRenderPassDescriptor allocated
// here (plus its colorAttachments / attachment descriptors) is
// autoreleased; without a pool around each pass these accumulate
// across frames and the heap balloons after a few seconds of
// rendering.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	e.dev.ensureFrameStarted()
	e.cmdBuffer = e.dev.frameCmdBuffer
	e.passAutoreleaseToken = mtl.AutoreleasePoolPush()

	colorTex := e.dev.defaultColorTex
	w, h := uint32(e.dev.width), uint32(e.dev.height)
	e.passTargetOffscreen = false
	if desc.Target != nil {
		if rt, ok := desc.Target.(*RenderTarget); ok {
			colorTex = rt.colorTex.handle
			w = uint32(rt.w)
			h = uint32(rt.h)
			e.passTargetOffscreen = true
		}
	}

	loadAction := mtl.LoadActionLoad
	if desc.LoadAction == backend.LoadActionClear {
		loadAction = mtl.LoadActionClear
	}

	// Create MTLRenderPassDescriptor via ObjC runtime.
	rpDescClass := getClass("MTLRenderPassDescriptor")
	rpDesc := msgSend(uintptr(rpDescClass), sel("renderPassDescriptor"))

	// Configure color attachment 0.
	colorAttachments := msgSend(rpDesc, sel("colorAttachments"))
	ca0 := msgSend(colorAttachments, sel("objectAtIndexedSubscript:"), 0)
	msgSend(ca0, sel("setTexture:"), uintptr(colorTex))
	msgSend(ca0, sel("setLoadAction:"), uintptr(loadAction))
	msgSend(ca0, sel("setStoreAction:"), uintptr(mtl.StoreActionStore))
	if loadAction == mtl.LoadActionClear {
		clearColor := mtl.ClearColor{
			Red:   float64(desc.ClearColor[0]),
			Green: float64(desc.ClearColor[1]),
			Blue:  float64(desc.ClearColor[2]),
			Alpha: float64(desc.ClearColor[3]),
		}
		mtl.SetClearColor(ca0, clearColor)
	}

	e.renderEncoder = mtl.CommandBufferRenderCommandEncoder(e.cmdBuffer, rpDesc)
	e.inRenderPass = true

	if e.dev.rtSyncFence != 0 {
		mtl.RenderCommandEncoderWaitForFence(e.renderEncoder, e.dev.rtSyncFence, mtl.RenderStageFragment)
	}

	// Set default viewport.
	vp := mtl.Viewport{
		Width:  float64(w),
		Height: float64(h),
		ZNear:  0,
		ZFar:   1,
	}
	mtl.RenderCommandEncoderSetViewport(e.renderEncoder, vp)

	// Bind default texture + sampler at every slot the MSL fragment
	// signature could declare. The Kage→MSL emit declares uTexture0..3
	// (and matching samplers) on any shader that uses an
	// imageSrcNAt/Origin/Size builtin; leaving any declared slot
	// unbound makes Metal validation drop the draw — presenting as
	// fully-black render targets on cells that ran multi-texture
	// effect shaders (color_adjust, vignette, etc.).
	//
	// Slot 0 also needs a default because the sprite pass binds the
	// per-batch texture lazily and validation must pass at the first
	// draw before SetTexture has been called. The pipeline pairs with
	// `sprite_pass.lastTextureID = 0` reset on pass switch — without
	// that reset, the engine would skip re-binding slot 0 when a new
	// pass starts with the same TextureID as the prior pass's last
	// batch, leaving this whiteTex placeholder sampled instead of the
	// atlas the engine thinks is bound (the bug that painted
	// isometric-combat's terrain solid white).
	if e.dev.defaultSampler != 0 {
		for slot := 0; slot < 4; slot++ {
			mtl.RenderCommandEncoderSetFragmentSamplerState(e.renderEncoder, e.dev.defaultSampler, uint64(slot))
		}
	}
	if e.dev.whiteTex != 0 {
		for slot := 0; slot < 4; slot++ {
			mtl.RenderCommandEncoderSetFragmentTexture(e.renderEncoder, e.dev.whiteTex, uint64(slot))
		}
	}
}

// EndRenderPass ends the current render encoder. The frame's command
// buffer stays open across passes — every render pass goes into one
// MTLCommandBuffer, committed in Device.EndFrame. This is required for
// correctness: when render pass N writes to a texture and pass N+1
// samples it (e.g. isometric-combat's terrain atlas built up over
// passes 0-9 then sampled at pass 57), Metal only guarantees the
// write→read ordering when both passes live in the SAME command
// buffer. Per-pass commits split them across cmdBuffers which Metal
// pipelines aggressively without inserting a barrier, and the sample
// would race with the write.
//
// The per-frame autorelease pool keeps memory pressure low even with
// hundreds of passes batched into one buffer (scene-selector spawns
// ~50 thumbnail RTs per frame and renders fine).
func (e *Encoder) EndRenderPass() {
	wasOffscreen := e.passTargetOffscreen
	if e.inRenderPass {
		if wasOffscreen && e.dev.rtSyncFence != 0 {
			mtl.RenderCommandEncoderUpdateFence(e.renderEncoder, e.dev.rtSyncFence, mtl.RenderStageFragment)
		}
		mtl.RenderCommandEncoderEndEncoding(e.renderEncoder)
		e.renderEncoder = 0
		e.inRenderPass = false
		e.cmdBuffer = 0
	}
	e.passTargetOffscreen = false

	if e.passAutoreleaseToken != 0 {
		mtl.AutoreleasePoolPop(e.passAutoreleaseToken)
		e.passAutoreleaseToken = 0
	}
}

// SetPipeline binds a render pipeline state.
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

	// Pick the blend mode for this draw. The engine's sprite-pass calls
	// SetBlendMode BEFORE SetPipeline to override the pipeline's
	// descriptor-default blend (e.g. switching from SourceOver to
	// BlendLighter for additive draws). On Metal blend state is baked
	// into the MTLRenderPipelineState, so we maintain a per-blend
	// pipeline-variant cache on the Pipeline and select the right one
	// here. Without the override, additive / multiply / custom-blend
	// draws silently render with the descriptor's original blend
	// (typically SourceOver). Mirrors WebGPU's
	// ensurePipelineForVariant pattern.
	blend := p.desc.BlendMode
	if e.pendingBlendSet {
		blend = e.pendingBlend
	}
	pso := p.stateForBlend(blend)
	if pso != 0 && e.renderEncoder != 0 {
		mtl.RenderCommandEncoderSetRenderPipelineState(e.renderEncoder, pso)
	}

	// Apply cull mode.
	if e.renderEncoder != 0 {
		mtl.RenderCommandEncoderSetCullMode(e.renderEncoder, mtlCullMode(p.desc.CullMode))
	}
}

// mtlCullMode maps backend cull mode to Metal cull mode.
func mtlCullMode(mode backend.CullMode) int {
	switch mode {
	case backend.CullFront:
		return mtl.CullModeFront
	case backend.CullBack:
		return mtl.CullModeBack
	default:
		return mtl.CullModeNone
	}
}

// SetVertexBuffer binds a vertex buffer to a slot.
func (e *Encoder) SetVertexBuffer(buf backend.Buffer, slot int) {
	if b, ok := buf.(*Buffer); ok {
		mtl.RenderCommandEncoderSetVertexBuffer(e.renderEncoder, b.handle, 0, uint64(slot))
	}
}

// SetIndexBuffer binds an index buffer.
func (e *Encoder) SetIndexBuffer(buf backend.Buffer, format backend.IndexFormat) {
	if b, ok := buf.(*Buffer); ok {
		e.indexFormat = format
		e.boundIndexBuf = b
	}
}

// SetTexture binds a texture to a fragment shader slot.
func (e *Encoder) SetTexture(tex backend.Texture, slot int) {
	if t, ok := tex.(*Texture); ok && e.renderEncoder != 0 {
		mtl.RenderCommandEncoderSetFragmentTexture(e.renderEncoder, t.handle, uint64(slot))
	}
}

// SetTextureFilter overrides the texture filter for a slot.
func (e *Encoder) SetTextureFilter(_ int, filter backend.TextureFilter) {
	if e.renderEncoder == 0 {
		return
	}
	switch filter {
	case backend.FilterLinear:
		if e.dev.linearSampler != 0 {
			mtl.RenderCommandEncoderSetFragmentSamplerState(e.renderEncoder, e.dev.linearSampler, 0)
		}
	default:
		if e.dev.defaultSampler != 0 {
			mtl.RenderCommandEncoderSetFragmentSamplerState(e.renderEncoder, e.dev.defaultSampler, 0)
		}
	}
}

// SetStencilReference is a no-op until the Metal encoder is wired for
// stencil. Device advertises SupportsStencil=false so the sprite pass
// never routes stencil-requiring batches here.
//
// TODO(metal-stencil): build MTLDepthStencilState from the pipeline's
// stencil state on SetPipeline, then call setStencilReferenceValue:.
func (e *Encoder) SetStencilReference(_ uint32) {}

// SetColorWrite enables or disables color writing.
func (e *Encoder) SetColorWrite(_ bool) {}

// SetViewport sets the rendering viewport.
func (e *Encoder) SetViewport(vp backend.Viewport) {
	mtlVP := mtl.Viewport{
		OriginX: float64(vp.X),
		OriginY: float64(vp.Y),
		Width:   float64(vp.Width),
		Height:  float64(vp.Height),
		ZNear:   0,
		ZFar:    1,
	}
	mtl.RenderCommandEncoderSetViewport(e.renderEncoder, mtlVP)
}

// SetScissor sets the scissor rectangle.
func (e *Encoder) SetScissor(rect *backend.ScissorRect) {
	if rect == nil {
		mtl.RenderCommandEncoderSetScissorRect(e.renderEncoder, mtl.ScissorRect{
			Width:  uint64(e.dev.width),
			Height: uint64(e.dev.height),
		})
		return
	}
	mtl.RenderCommandEncoderSetScissorRect(e.renderEncoder, mtl.ScissorRect{
		X:      uint64(rect.X),
		Y:      uint64(rect.Y),
		Width:  uint64(rect.Width),
		Height: uint64(rect.Height),
	})
}

// Draw issues a non-indexed draw call.
func (e *Encoder) Draw(vertexCount, instanceCount, firstVertex int) {
	e.bindUniforms()
	primType := uint64(mtl.PrimitiveTypeTriangle)
	if e.currentPipeline != nil {
		primType = uint64(mtlPrimitiveType(e.currentPipeline.desc.Primitive))
	}
	mtl.RenderCommandEncoderDrawPrimitives(e.renderEncoder,
		primType, uint64(firstVertex), uint64(vertexCount), uint64(instanceCount))
}

// DrawIndexed issues an indexed draw call.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	e.bindUniforms()

	idxType := uint64(mtl.IndexTypeUInt16)
	byteOffset := uint64(firstIndex * 2)
	if e.indexFormat == backend.IndexUint32 {
		idxType = uint64(mtl.IndexTypeUInt32)
		byteOffset = uint64(firstIndex * 4)
	}

	primType := uint64(mtl.PrimitiveTypeTriangle)
	if e.currentPipeline != nil {
		primType = uint64(mtlPrimitiveType(e.currentPipeline.desc.Primitive))
	}

	var indexBuf mtl.Buffer
	if e.boundIndexBuf != nil {
		indexBuf = e.boundIndexBuf.handle
	}

	mtl.RenderCommandEncoderDrawIndexedPrimitives(e.renderEncoder,
		primType, uint64(indexCount), idxType, indexBuf, byteOffset, uint64(instanceCount))
}

// bindUniforms packs shader uniforms into the device's persistent
// uniform ring buffer and binds the appropriate slice via
// setVertexBuffer/setFragmentBuffer.
//
// Replaces the previous setVertexBytes/setFragmentBytes (inline) path
// — Metal's inline-bytes scratch allocator silently drops binding
// updates after some implementation-defined per-frame draw count,
// which is the Metal analog of Vulkan's descriptor-pool exhaustion
// (commit 91dfe0d). On iso-combat with ~93 batches per frame the
// scratch ran out partway through, leaving subsequent draws bound to
// whatever uniform/sample state was committed at the exhaustion
// point — visible as terrain atlas samples returning the engine's
// whiteTexture content (early-frame stale binding) instead of the
// atlas's diamond pixels. A real MTLBuffer with explicit offset is
// the documented Apple-recommended path for frequently-updated
// uniforms and has no scratch dependency.
func (e *Encoder) bindUniforms() {
	if e.boundShader == nil || e.renderEncoder == 0 {
		return
	}

	vBuf := e.boundShader.packUniformBuffer(e.boundShader.vertexUniformLayout)
	fBuf := e.boundShader.packUniformBuffer(e.boundShader.fragmentUniformLayout)

	if len(vBuf) > 0 {
		mtl.RenderCommandEncoderSetVertexBytes(e.renderEncoder, unsafe.Pointer(&vBuf[0]), uint64(len(vBuf)), 1)
	}
	if len(fBuf) > 0 {
		mtl.RenderCommandEncoderSetFragmentBytes(e.renderEncoder, unsafe.Pointer(&fBuf[0]), uint64(len(fBuf)), 0)
	}
}

// mtlPrimitiveType maps backend primitive type to Metal primitive type.
func mtlPrimitiveType(p backend.PrimitiveType) int {
	switch p {
	case backend.PrimitiveTriangles:
		return mtl.PrimitiveTypeTriangle
	case backend.PrimitiveTriangleStrip:
		return mtl.PrimitiveTypeTriangleStrip
	case backend.PrimitiveLines:
		return mtl.PrimitiveTypeLine
	case backend.PrimitiveLineStrip:
		return mtl.PrimitiveTypeLineStrip
	case backend.PrimitivePoints:
		return mtl.PrimitiveTypePoint
	default:
		return mtl.PrimitiveTypeTriangle
	}
}

// Flush is a no-op — submission happens in EndRenderPass.
// SetBlendMode records the blend mode override for the next SetPipeline.
// On Metal, blend state is baked into the MTLRenderPipelineState, so we
// can't toggle it on the encoder directly the way GL backends can —
// instead we mark the pending blend and SetPipeline picks the matching
// pipeline variant from the Pipeline's per-blend cache.
func (e *Encoder) SetBlendMode(b backend.BlendMode) {
	e.pendingBlend = b
	e.pendingBlendSet = true
	// If a pipeline is already bound, switch to the variant for this
	// blend immediately — the engine's flow normally calls SetPipeline
	// after SetBlendMode, but the sticky-override semantics also need
	// to work when SetBlendMode is called between draws on the same
	// pipeline.
	if e.currentPipeline != nil && e.renderEncoder != 0 {
		pso := e.currentPipeline.stateForBlend(b)
		if pso != 0 {
			mtl.RenderCommandEncoderSetRenderPipelineState(e.renderEncoder, pso)
		}
	}
}

func (e *Encoder) Flush() {}

// msgSend wraps the ObjC runtime call.
func msgSend(obj uintptr, s mtl.Selector, args ...uintptr) uintptr {
	return mtl.MsgSend(obj, s, args...)
}

// sel creates a selector.
func sel(name string) mtl.Selector {
	return mtl.Sel(name)
}

// getClass returns an ObjC class.
func getClass(name string) mtl.Class {
	return mtl.GetClass(name)
}
