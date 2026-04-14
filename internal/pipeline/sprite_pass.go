package pipeline

import (
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/batch"
)

// TextureResolver maps a batcher texture ID to a backend.Texture.
type TextureResolver func(textureID uint32) backend.Texture

// ShaderInfo holds a custom shader's backend resources for rendering.
type ShaderInfo struct {
	Shader   backend.Shader
	Pipeline backend.Pipeline
}

// ShaderResolver maps a batcher shader ID to a ShaderInfo.
// Returns nil if the shader ID is not registered (use default).
type ShaderResolver func(shaderID uint32) *ShaderInfo

// RenderTargetResolver maps a target ID to a backend.RenderTarget.
// Returns nil for the screen target (ID 0).
type RenderTargetResolver func(targetID uint32) backend.RenderTarget

// SpritePass renders 2D sprite batches. It flushes the batcher, uploads
// vertex/index data to dynamic GPU buffers, and issues indexed draw calls.
type SpritePass struct {
	batcher  *batch.Batcher
	pipeline backend.Pipeline
	shader   backend.Shader

	// Dynamic GPU buffers for per-frame vertex/index uploads.
	vertexBuf backend.Buffer
	indexBuf  backend.Buffer

	// screenCleared tracks whether the screen (target=0) has already been
	// cleared this frame. When batches force the sprite pass to leave the
	// screen target for an offscreen and then return, the second
	// BeginRenderPass(screen) MUST use LoadActionLoad — otherwise it wipes
	// the draws the first screen pass already produced. Reset to false at
	// the top of each Execute call.
	screenCleared bool

	// traceFrame is the current frame number for pass-boundary tracing.
	// Set when FUTURE_CORE_TRACE_PASSES is active; zero otherwise.
	traceFrame int

	// Per-frame state tracking to avoid redundant uniform/texture/filter
	// updates when consecutive batches share the same values. Reset at the
	// top of Execute. Each of these eliminates a SetUniform* map write +
	// dirty-flag set + downstream pack/upload/bind when unchanged.
	lastColorBody  [16]float32
	lastColorTrans [4]float32
	lastTextureID  uint32
	lastFilter     backend.TextureFilter
	colorBodySet   bool // false until the first batch sets it this frame

	// ResolveTexture maps batch texture IDs to backend textures.
	ResolveTexture TextureResolver

	// ResolveShader maps batch shader IDs to custom shader info.
	ResolveShader ShaderResolver

	// ResolveRenderTarget maps batch target IDs to render targets.
	ResolveRenderTarget RenderTargetResolver

	// ConsumePendingClear checks whether a target has a pending Clear()
	// and returns true if so, atomically clearing the flag. The sprite
	// pass uses this in beginTargetPass to emit LoadActionClear on the
	// target's first render pass instead of LoadActionLoad. Returns false
	// when nil (no clear tracking) or when the target has no pending clear.
	ConsumePendingClear func(targetID uint32) bool

	// ApplyUniforms applies a snapshot of user-provided uniforms to a
	// backend shader. Called per-draw for custom shader batches that have
	// per-draw uniforms (e.g., each light has different LightPos/Color).
	// The engine sets this to Shader.applyUniforms during initialization.
	ApplyUniforms func(shader backend.Shader, uniforms map[string]any)

	// Projection is the orthographic projection matrix, set each frame.
	Projection [16]float32

	// Reusable per-frame slices to avoid heap allocations each frame.
	// In WASM, per-frame allocations accumulate faster than GC can
	// collect them, eventually causing OOM.
	tmpVertices []batch.Vertex2D
	tmpIndices  []uint16
}

// SpritePassConfig holds configuration for creating a SpritePass.
type SpritePassConfig struct {
	Device   backend.Device
	Batcher  *batch.Batcher
	Pipeline backend.Pipeline
	Shader   backend.Shader

	// MaxVertices is the capacity of the dynamic vertex buffer.
	MaxVertices int

	// MaxIndices is the capacity of the dynamic index buffer.
	MaxIndices int
}

// NewSpritePass creates a new sprite pass with pre-allocated GPU buffers.
func NewSpritePass(cfg SpritePassConfig) (*SpritePass, error) {
	vbuf, err := cfg.Device.NewBuffer(backend.BufferDescriptor{
		Size:    cfg.MaxVertices * batch.Vertex2DSize,
		Usage:   backend.BufferUsageVertex,
		Dynamic: true,
	})
	if err != nil {
		return nil, err
	}

	ibuf, err := cfg.Device.NewBuffer(backend.BufferDescriptor{
		Size:    cfg.MaxIndices * 2, // uint16 indices
		Usage:   backend.BufferUsageIndex,
		Dynamic: true,
	})
	if err != nil {
		vbuf.Dispose()
		return nil, err
	}

	return &SpritePass{
		batcher:   cfg.Batcher,
		pipeline:  cfg.Pipeline,
		shader:    cfg.Shader,
		vertexBuf: vbuf,
		indexBuf:  ibuf,
	}, nil
}

// Name returns the pass name.
func (sp *SpritePass) Name() string { return "sprite" }

// Execute flushes the batcher and renders all batches.
// Batches are grouped by render target. For each target group, a render pass
// is begun, all batches are drawn, and the pass is ended.
// If no batches exist, the screen target is still cleared (when enabled).
func (sp *SpritePass) Execute(enc backend.CommandEncoder, ctx *PassContext) {
	// Reset per-frame state before flushing.
	sp.screenCleared = false
	sp.colorBodySet = false
	sp.lastTextureID = 0
	sp.lastFilter = 0

	batches := sp.batcher.Flush()

	// Diagnostic: dump per-frame batch metadata when FUTURE_CORE_TRACE_BATCHES
	// is set. See internal/pipeline/trace.go for details.
	if traceBatchesActive() {
		n := traceBatchesBeginFrame()
		tracef("=== frame %d: %d batches ===\n", n, len(batches))
		for i := range batches {
			b := &batches[i]
			tracef("  batch[%d] target=%d texture=%d shader=%d filter=%d blend=%v verts=%d indices=%d\n",
				i, b.TargetID, b.TextureID, b.ShaderID, b.Filter, b.BlendMode,
				len(b.Vertices), len(b.Indices))
		}
	}

	// Diagnostic: count the frame for pass-boundary tracing too, so that
	// pass events can be attributed to a specific frame number in the
	// trace output. sp.traceFrame is read by beginTargetPass.
	sp.traceFrame = 0
	if tracePassesActive() {
		sp.traceFrame = tracePassesBeginFrame()
	}

	if len(batches) == 0 {
		// Even with no draws, clear the screen target so the previous
		// frame's content doesn't persist.
		sp.beginTargetPass(enc, ctx, 0)
		enc.EndRenderPass()
		enc.Flush()
		return
	}

	// Pre-upload all vertex and index data before recording draw commands.
	// This is required for deferred-execution backends (WebGPU, Vulkan) where
	// buffer writes via queue.writeBuffer happen immediately but draw commands
	// execute later at queue.submit. If we upload per-batch, each upload
	// overwrites the buffer at offset 0, and all draws see only the last
	// batch's geometry. By uploading everything at contiguous offsets first,
	// each draw can reference its own region via firstIndex.
	type batchRegion struct {
		firstIndex int // starting index in the combined index buffer
		indexCount int
	}
	regions := make([]batchRegion, len(batches))

	// Accumulate all vertex data contiguously. Adjust index values to
	// account for the vertex offset so each batch's indices reference
	// the correct vertices in the combined buffer.
	// Reuse slices between frames to avoid per-frame heap allocations
	// that can cause OOM in memory-constrained environments (WASM).
	sp.tmpVertices = sp.tmpVertices[:0]
	sp.tmpIndices = sp.tmpIndices[:0]
	for i := range batches {
		b := &batches[i]
		baseVertex := uint16(len(sp.tmpVertices))
		regions[i].firstIndex = len(sp.tmpIndices)
		regions[i].indexCount = len(b.Indices)
		sp.tmpVertices = append(sp.tmpVertices, b.Vertices...)
		for _, idx := range b.Indices {
			sp.tmpIndices = append(sp.tmpIndices, idx+baseVertex)
		}
	}

	// Upload all vertex/index data at once. Pad the index buffer to a
	// 4-byte boundary: WebGPU's writeBuffer requires the byte count to be
	// a multiple of 4, and an odd number of uint16 indices produces a
	// 2-byte-aligned but not 4-byte-aligned size.
	if len(sp.tmpVertices) > 0 {
		sp.vertexBuf.Upload(vertexSliceToBytes(sp.tmpVertices))
	}
	if len(sp.tmpIndices) > 0 {
		if len(sp.tmpIndices)%2 != 0 {
			sp.tmpIndices = append(sp.tmpIndices, 0) // pad to even count → 4-byte aligned
		}
		sp.indexBuf.Upload(indexSliceToBytes(sp.tmpIndices))
	}

	// Track current render target and shader to minimize state changes.
	currentTargetID := batches[0].TargetID
	currentShaderID := uint32(0)

	// Begin the first render pass.
	sp.beginTargetPass(enc, ctx, currentTargetID)
	sp.bindDefaultShader(enc)
	sp.setProjectionForTarget(enc, ctx, currentTargetID)

	// Bind vertex/index buffers (must be within a render pass).
	enc.SetVertexBuffer(sp.vertexBuf, 0)
	enc.SetIndexBuffer(sp.indexBuf, backend.IndexUint16)

	for i := range batches {
		b := &batches[i]

		// Switch render target if needed.
		if b.TargetID != currentTargetID {
			enc.EndRenderPass()
			currentTargetID = b.TargetID
			currentShaderID = 0
			sp.beginTargetPass(enc, ctx, currentTargetID)
			sp.bindDefaultShader(enc)
			sp.setProjectionForTarget(enc, ctx, currentTargetID)
			// Re-bind buffers after render pass switch.
			enc.SetVertexBuffer(sp.vertexBuf, 0)
			enc.SetIndexBuffer(sp.indexBuf, backend.IndexUint16)
		}

		// Switch shader if this batch uses a different one.
		var resolvedInfo *ShaderInfo
		if b.ShaderID != 0 && sp.ResolveShader != nil {
			resolvedInfo = sp.ResolveShader(b.ShaderID)
		}

		if b.ShaderID != currentShaderID {
			switch {
			case b.ShaderID == 0:
				sp.bindDefaultShader(enc)
			case resolvedInfo != nil:
				// Set blend mode before pipeline so WebGPU can
				// recreate the pipeline if the blend differs.
				enc.SetBlendMode(b.BlendMode)
				enc.SetPipeline(resolvedInfo.Pipeline)
				// Custom shaders read uProjection too. Use the
				// per-target ortho — sp.Projection is screen-space
				// only; a lightmap RT with a different size (or even
				// the same size with a different orientation) would
				// produce off-screen quads otherwise.
				resolvedInfo.Shader.SetUniformMat4("uProjection", sp.projectionForTarget(currentTargetID))
			default:
				sp.bindDefaultShader(enc)
			}
			currentShaderID = b.ShaderID
		} else if b.ShaderID != 0 && resolvedInfo != nil {
			// Same shader but potentially different blend mode.
			enc.SetBlendMode(b.BlendMode)
			enc.SetPipeline(resolvedInfo.Pipeline)
		}

		// For custom shader batches, re-apply the per-draw user uniforms
		// before drawing. Each light (or other custom draw) has its own
		// uniform values that were snapshotted at draw time.
		activeShader := sp.shader
		if resolvedInfo != nil {
			activeShader = resolvedInfo.Shader
			if sp.ApplyUniforms != nil && len(b.Uniforms) > 0 {
				sp.ApplyUniforms(resolvedInfo.Shader, b.Uniforms)
			}
		}

		// Set color matrix uniforms only when they differ from the
		// previous batch. Most batches use the identity matrix, so this
		// skips the SetUniform* → dirty-flag → pack → upload → bind
		// chain for the majority of draws.
		if !sp.colorBodySet || b.ColorBody != sp.lastColorBody || b.ColorTranslation != sp.lastColorTrans {
			activeShader.SetUniformMat4("uColorBody", b.ColorBody)
			activeShader.SetUniformVec4("uColorTranslation", b.ColorTranslation)
			sp.lastColorBody = b.ColorBody
			sp.lastColorTrans = b.ColorTranslation
			sp.colorBodySet = true
		}

		// Set filter BEFORE texture: the WebGPU encoder reads the current
		// filter when creating the texture bind group in SetTexture. If
		// filter is set after, the bind group uses the stale filter value.
		if b.Filter != sp.lastFilter {
			enc.SetTextureFilter(0, b.Filter)
			sp.lastFilter = b.Filter
		}
		if sp.ResolveTexture != nil && b.TextureID != sp.lastTextureID {
			tex := sp.ResolveTexture(b.TextureID)
			if tex != nil {
				enc.SetTexture(tex, 0)
			}
			sp.lastTextureID = b.TextureID
		}

		// Bind extra textures for custom shader draws (slots 1-3).
		if b.ShaderID != 0 && sp.ResolveTexture != nil {
			for slot, texID := range b.ExtraTextureIDs {
				if texID != 0 {
					tex := sp.ResolveTexture(texID)
					if tex != nil {
						enc.SetTexture(tex, slot+1)
					}
				}
			}
		}

		// Draw using the pre-computed index region.
		if b.FillRule == backend.FillRuleEvenOdd {
			sp.drawEvenOdd(enc, b)
		} else {
			enc.DrawIndexed(regions[i].indexCount, 1, regions[i].firstIndex)
		}
	}

	enc.EndRenderPass()

	// Submit all accumulated render passes as a single GPU command buffer.
	// For deferred-execution backends (WebGPU, Vulkan) this batches all
	// render passes into one queue.submit(), dramatically reducing the
	// number of Go→JS/Go→C boundary crossings per frame.
	enc.Flush()
}

// beginTargetPass starts a render pass for the given target ID.
func (sp *SpritePass) beginTargetPass(enc backend.CommandEncoder, ctx *PassContext, targetID uint32) {
	var rt backend.RenderTarget
	if targetID != 0 && sp.ResolveRenderTarget != nil {
		rt = sp.ResolveRenderTarget(targetID)
	}

	loadAction := backend.LoadActionLoad
	clearColor := [4]float32{0, 0, 0, 0}
	if targetID == 0 && ctx.ScreenClearEnabled && !sp.screenCleared {
		// Screen target clears to opaque black on the FIRST screen pass of
		// the frame. Subsequent screen passes (re-entries after a detour to
		// an offscreen target) must use LoadActionLoad; otherwise they
		// wipe the content the first screen pass just produced.
		loadAction = backend.LoadActionClear
		clearColor = [4]float32{0, 0, 0, 1}
	}
	if targetID == 0 {
		sp.screenCleared = true
	}
	// Offscreen targets with a pending Clear() use LoadActionClear with
	// transparent black. This is a GPU-native clear — no CPU data transfer.
	if targetID != 0 && sp.ConsumePendingClear != nil && sp.ConsumePendingClear(targetID) {
		loadAction = backend.LoadActionClear
		// clearColor is already {0,0,0,0} (transparent black).
	}

	enc.BeginRenderPass(backend.RenderPassDescriptor{
		Target:      rt,
		ClearColor:  clearColor,
		ClearDepth:  1.0,
		LoadAction:  loadAction,
		StoreAction: backend.StoreActionStore,
	})

	// Set viewport based on target dimensions.
	w, h := ctx.FramebufferWidth, ctx.FramebufferHeight
	if rt != nil {
		w, h = rt.Width(), rt.Height()
	}
	enc.SetViewport(backend.Viewport{
		X: 0, Y: 0,
		Width:  w,
		Height: h,
	})

	if sp.traceFrame > 0 {
		loadStr := "load"
		if loadAction == backend.LoadActionClear {
			loadStr = "clear"
		}
		tracef("[frame %d] BeginRenderPass target=%d load=%s viewport=%dx%d clear=[%.2f %.2f %.2f %.2f]\n",
			sp.traceFrame, targetID, loadStr, w, h,
			clearColor[0], clearColor[1], clearColor[2], clearColor[3])
	}
}

// setProjectionForTarget sets the projection matrix appropriate for the
// current render target. Screen targets use sp.Projection (set by the
// engine from Layout dimensions). Off-screen targets use a per-target
// ortho projection so draws map 1:1 to the target's pixels.
func (sp *SpritePass) setProjectionForTarget(_ backend.CommandEncoder, _ *PassContext, targetID uint32) {
	sp.shader.SetUniformMat4("uProjection", sp.projectionForTarget(targetID))
}

// projectionForTarget returns the ortho projection matrix that maps the
// target's pixel space to NDC. Screen targets reuse sp.Projection (set by
// the engine); offscreen targets need their own ortho derived from RT size,
// otherwise custom shaders that bind sp.Projection would draw quads at
// wrong NDC coordinates whenever the offscreen RT differs from the screen
// dimensions.
func (sp *SpritePass) projectionForTarget(targetID uint32) [16]float32 {
	if targetID == 0 {
		return sp.Projection
	}
	if sp.ResolveRenderTarget == nil {
		return sp.Projection
	}
	rt := sp.ResolveRenderTarget(targetID)
	if rt == nil {
		return sp.Projection
	}
	w, h := float32(rt.Width()), float32(rt.Height())
	return [16]float32{
		2 / w, 0, 0, 0,
		0, -2 / h, 0, 0,
		0, 0, -1, 0,
		-1, 1, 0, 1,
	}
}

// bindDefaultShader sets the default sprite pipeline and projection.
func (sp *SpritePass) bindDefaultShader(enc backend.CommandEncoder) {
	enc.SetPipeline(sp.pipeline)
	sp.shader.SetUniformMat4("uProjection", sp.Projection)
	sp.shader.SetUniformInt("uTexture", 0)
}

// drawEvenOdd renders a batch using the even-odd fill rule via stencil.
// Pass 1: draw triangles to stencil only (INVERT), color writes disabled.
// Pass 2: redraw with stencil test NOTEQUAL 0, then reset stencil state.
func (sp *SpritePass) drawEvenOdd(enc backend.CommandEncoder, b *batch.Batch) {
	// Pass 1: write to stencil, no color output.
	enc.SetColorWrite(false)
	enc.SetStencil(true, backend.StencilDescriptor{
		Func:      backend.CompareAlways,
		Ref:       0,
		Mask:      0xFF,
		SFail:     backend.StencilKeep,
		DPFail:    backend.StencilKeep,
		DPPass:    backend.StencilInvert,
		WriteMask: 0xFF,
	})
	enc.DrawIndexed(len(b.Indices), 1, 0)

	// Pass 2: draw where stencil != 0.
	enc.SetColorWrite(true)
	enc.SetStencil(true, backend.StencilDescriptor{
		Func:      backend.CompareNotEqual,
		Ref:       0,
		Mask:      0xFF,
		SFail:     backend.StencilKeep,
		DPFail:    backend.StencilKeep,
		DPPass:    backend.StencilZero, // clear stencil as we draw
		WriteMask: 0xFF,
	})
	enc.DrawIndexed(len(b.Indices), 1, 0)

	// Disable stencil for subsequent batches.
	enc.SetStencil(false, backend.StencilDescriptor{})
}

// Dispose releases the pass's GPU buffers.
func (sp *SpritePass) Dispose() {
	if sp.vertexBuf != nil {
		sp.vertexBuf.Dispose()
	}
	if sp.indexBuf != nil {
		sp.indexBuf.Dispose()
	}
}

// vertexSliceToBytes reinterprets a []Vertex2D as a []byte without copying.
func vertexSliceToBytes(verts []batch.Vertex2D) []byte {
	if len(verts) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&verts[0])), len(verts)*batch.Vertex2DSize)
}

// indexSliceToBytes reinterprets a []uint16 as a []byte without copying.
func indexSliceToBytes(indices []uint16) []byte {
	if len(indices) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&indices[0])), len(indices)*2)
}
