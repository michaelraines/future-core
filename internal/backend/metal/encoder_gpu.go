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
}

// BeginRenderPass begins a Metal render pass.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	e.cmdBuffer = mtl.CommandQueueCommandBuffer(e.dev.commandQueue)

	colorTex := e.dev.defaultColorTex
	w, h := uint32(e.dev.width), uint32(e.dev.height)
	if desc.Target != nil {
		if rt, ok := desc.Target.(*RenderTarget); ok {
			colorTex = rt.colorTex.handle
			w = uint32(rt.w)
			h = uint32(rt.h)
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

	// Set default viewport.
	vp := mtl.Viewport{
		Width:  float64(w),
		Height: float64(h),
		ZNear:  0,
		ZFar:   1,
	}
	mtl.RenderCommandEncoderSetViewport(e.renderEncoder, vp)

	// Bind default sampler.
	if e.dev.defaultSampler != 0 {
		mtl.RenderCommandEncoderSetFragmentSamplerState(e.renderEncoder, e.dev.defaultSampler, 0)
	}
}

// EndRenderPass ends the current render pass.
func (e *Encoder) EndRenderPass() {
	if e.inRenderPass {
		mtl.RenderCommandEncoderEndEncoding(e.renderEncoder)
		e.renderEncoder = 0
		e.inRenderPass = false

		// Commit and wait.
		mtl.CommandBufferCommit(e.cmdBuffer)
		mtl.CommandBufferWaitUntilCompleted(e.cmdBuffer)
		e.cmdBuffer = 0
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

	// Lazily create the MTLRenderPipelineState.
	if p.pipelineState == 0 {
		_ = p.createPipelineState()
	}

	if p.pipelineState != 0 && e.renderEncoder != 0 {
		mtl.RenderCommandEncoderSetRenderPipelineState(e.renderEncoder, p.pipelineState)
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

// bindUniforms packs shader uniforms into Metal buffers and binds them.
func (e *Encoder) bindUniforms() {
	if e.boundShader == nil || e.renderEncoder == 0 {
		return
	}

	// Vertex uniforms → buffer slot 1.
	if vBuf := e.boundShader.packUniformBuffer(e.boundShader.vertexUniformLayout); len(vBuf) > 0 {
		mtl.RenderCommandEncoderSetVertexBytes(e.renderEncoder, unsafe.Pointer(&vBuf[0]), uint64(len(vBuf)), 1)
	}

	// Fragment uniforms → buffer slot 0.
	if fBuf := e.boundShader.packUniformBuffer(e.boundShader.fragmentUniformLayout); len(fBuf) > 0 {
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
// SetBlendMode is a no-op for this backend.
func (e *Encoder) SetBlendMode(_ backend.BlendMode) {}

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
