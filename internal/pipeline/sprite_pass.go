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

	// ResolveTexture maps batch texture IDs to backend textures.
	ResolveTexture TextureResolver

	// ResolveShader maps batch shader IDs to custom shader info.
	ResolveShader ShaderResolver

	// ResolveRenderTarget maps batch target IDs to render targets.
	ResolveRenderTarget RenderTargetResolver

	// Projection is the orthographic projection matrix, set each frame.
	Projection [16]float32
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
	batches := sp.batcher.Flush()
	if len(batches) == 0 {
		// Even with no draws, clear the screen target so the previous
		// frame's content doesn't persist.
		sp.beginTargetPass(enc, ctx, 0)
		enc.EndRenderPass()
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
	var allVertices []batch.Vertex2D
	var allIndices []uint16
	for i := range batches {
		b := &batches[i]
		baseVertex := uint16(len(allVertices))
		regions[i].firstIndex = len(allIndices)
		regions[i].indexCount = len(b.Indices)
		allVertices = append(allVertices, b.Vertices...)
		for _, idx := range b.Indices {
			allIndices = append(allIndices, idx+baseVertex)
		}
	}

	// Upload all vertex/index data at once.
	if len(allVertices) > 0 {
		sp.vertexBuf.Upload(vertexSliceToBytes(allVertices))
	}
	if len(allIndices) > 0 {
		sp.indexBuf.Upload(indexSliceToBytes(allIndices))
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
				enc.SetPipeline(resolvedInfo.Pipeline)
				resolvedInfo.Shader.SetUniformMat4("uProjection", sp.Projection)
			default:
				sp.bindDefaultShader(enc)
			}
			currentShaderID = b.ShaderID
		}

		// Set color matrix uniforms on the active shader for this batch.
		activeShader := sp.shader
		if resolvedInfo != nil {
			activeShader = resolvedInfo.Shader
		}
		activeShader.SetUniformMat4("uColorBody", b.ColorBody)
		activeShader.SetUniformVec4("uColorTranslation", b.ColorTranslation)

		// Bind texture and set per-draw filter via sampler object.
		if sp.ResolveTexture != nil {
			tex := sp.ResolveTexture(b.TextureID)
			if tex != nil {
				enc.SetTexture(tex, 0)
			}
		}
		enc.SetTextureFilter(0, b.Filter)

		// Draw using the pre-computed index region.
		if b.FillRule == backend.FillRuleEvenOdd {
			sp.drawEvenOdd(enc, b)
		} else {
			enc.DrawIndexed(regions[i].indexCount, 1, regions[i].firstIndex)
		}
	}

	enc.EndRenderPass()
}

// beginTargetPass starts a render pass for the given target ID.
func (sp *SpritePass) beginTargetPass(enc backend.CommandEncoder, ctx *PassContext, targetID uint32) {
	var rt backend.RenderTarget
	if targetID != 0 && sp.ResolveRenderTarget != nil {
		rt = sp.ResolveRenderTarget(targetID)
	}

	loadAction := backend.LoadActionLoad
	clearColor := [4]float32{0, 0, 0, 0}
	if targetID == 0 && ctx.ScreenClearEnabled {
		// Screen target clears each frame to opaque black.
		loadAction = backend.LoadActionClear
		clearColor = [4]float32{0, 0, 0, 1}
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
}

// setProjectionForTarget sets the projection matrix appropriate for the
// current render target. Screen targets use sp.Projection (set by the
// engine from Layout dimensions). Off-screen targets use a per-target
// ortho projection so draws map 1:1 to the target's pixels.
func (sp *SpritePass) setProjectionForTarget(enc backend.CommandEncoder, ctx *PassContext, targetID uint32) {
	if targetID == 0 {
		// Screen: use the engine-provided projection (logical dimensions).
		sp.shader.SetUniformMat4("uProjection", sp.Projection)
	} else if sp.ResolveRenderTarget != nil {
		if rt := sp.ResolveRenderTarget(targetID); rt != nil {
			w, h := float32(rt.Width()), float32(rt.Height())
			sp.shader.SetUniformMat4("uProjection", [16]float32{
				2 / w, 0, 0, 0,
				0, -2 / h, 0, 0,
				0, 0, -1, 0,
				-1, 1, 0, 1,
			})
		}
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
