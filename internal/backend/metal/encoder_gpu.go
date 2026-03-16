//go:build metal

package metal

import (
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/mtl"
)

// Encoder implements backend.CommandEncoder for Metal.
type Encoder struct {
	dev    *Device
	width  int
	height int

	inRenderPass    bool
	currentPipeline *Pipeline
	renderEncoder   mtl.RenderCommandEncoder
	cmdBuffer       mtl.CommandBuffer
	indexFormat     backend.IndexFormat
}

// BeginRenderPass begins a Metal render pass.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	e.cmdBuffer = mtl.CommandQueueCommandBuffer(e.dev.commandQueue)

	colorTex := e.dev.defaultColorTex
	w, h := uint32(e.width), uint32(e.height)
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
		// Set clear color — pass as MTLClearColor struct.
		clearColor := mtl.ClearColor{
			Red:   float64(desc.ClearColor[0]),
			Green: float64(desc.ClearColor[1]),
			Blue:  float64(desc.ClearColor[2]),
			Alpha: float64(desc.ClearColor[3]),
		}
		msgSend(ca0, sel("setClearColor:"), *(*uintptr)(unsafe.Pointer(&clearColor)))
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
	if p, ok := pipeline.(*Pipeline); ok {
		e.currentPipeline = p
		// In a full implementation, we'd bind the MTLRenderPipelineState:
		// msgSend(uintptr(e.renderEncoder), sel("setRenderPipelineState:"), uintptr(p.pipelineState))
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
	if _, ok := buf.(*Buffer); ok {
		e.indexFormat = format
		// Metal doesn't have a separate "bind index buffer" — the index buffer
		// is passed directly to drawIndexedPrimitives.
	}
}

// SetTexture binds a texture to a slot.
func (e *Encoder) SetTexture(_ backend.Texture, _ int) {
	// In a full implementation, this would call setFragmentTexture:atIndex:.
}

// SetTextureFilter overrides the texture filter for a slot.
func (e *Encoder) SetTextureFilter(_ int, _ backend.TextureFilter) {
	// Would create/bind an MTLSamplerState.
}

// SetStencil configures stencil test state.
func (e *Encoder) SetStencil(_ bool, _ backend.StencilDescriptor) {
	// Stencil state is baked into the MTLDepthStencilState.
}

// SetColorWrite enables or disables color writing.
func (e *Encoder) SetColorWrite(_ bool) {
	// Color write mask is baked into the MTLRenderPipelineState.
}

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
			Width:  uint64(e.width),
			Height: uint64(e.height),
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
	mtl.RenderCommandEncoderDrawPrimitives(e.renderEncoder,
		3, // MTLPrimitiveTypeTriangle
		uint64(firstVertex), uint64(vertexCount), uint64(instanceCount))
}

// DrawIndexed issues an indexed draw call.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	idxType := uint64(mtl.IndexTypeUInt16)
	byteOffset := uint64(firstIndex * 2)
	if e.indexFormat == backend.IndexUint32 {
		idxType = uint64(mtl.IndexTypeUInt32)
		byteOffset = uint64(firstIndex * 4)
	}
	// Note: in Metal, the index buffer must be passed here.
	// In a full implementation, we'd track the bound index buffer.
	mtl.RenderCommandEncoderDrawIndexedPrimitives(e.renderEncoder,
		3, // MTLPrimitiveTypeTriangle
		uint64(indexCount), idxType, 0, byteOffset, uint64(instanceCount))
}

// Flush is a no-op — submission happens in EndRenderPass.
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
