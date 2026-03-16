//go:build dx12native

package dx12

import (
	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/d3d12"
)

// Encoder implements backend.CommandEncoder for DirectX 12.
type Encoder struct {
	dev    *Device
	width  int
	height int

	inRenderPass    bool
	currentPipeline *Pipeline
	indexFormat     backend.IndexFormat
}

// BeginRenderPass begins a DX12 render pass.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	rtvHandle := e.dev.defaultRTVHandle
	w, h := int32(e.width), int32(e.height)

	if desc.Target != nil {
		if rt, ok := desc.Target.(*RenderTarget); ok {
			w = int32(rt.w)
			h = int32(rt.h)
			if rt.rtvHandle.Ptr != 0 {
				rtvHandle = rt.rtvHandle
			}
		}
	}

	// Set render target.
	d3d12.CmdOMSetRenderTargets(e.dev.commandList, 1, rtvHandle)

	// Clear if requested.
	if desc.LoadAction == backend.LoadActionClear {
		clearColor := d3d12.ClearColor{
			R: desc.ClearColor[0],
			G: desc.ClearColor[1],
			B: desc.ClearColor[2],
			A: desc.ClearColor[3],
		}
		d3d12.CmdClearRenderTargetView(e.dev.commandList, rtvHandle, clearColor)
	}

	// Set default viewport and scissor.
	vp := d3d12.Viewport{
		Width:    float32(w),
		Height:   float32(h),
		MaxDepth: 1,
	}
	d3d12.CmdSetViewports(e.dev.commandList, vp)

	scissor := d3d12.Rect{Right: w, Bottom: h}
	d3d12.CmdSetScissorRects(e.dev.commandList, scissor)

	e.inRenderPass = true
}

// EndRenderPass ends the current render pass.
func (e *Encoder) EndRenderPass() {
	if e.inRenderPass {
		e.inRenderPass = false
	}
}

// SetPipeline binds a pipeline state object.
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	if p, ok := pipeline.(*Pipeline); ok {
		e.currentPipeline = p
		// In a full implementation, we'd call SetPipelineState and SetGraphicsRootSignature.
	}
}

// SetVertexBuffer binds a vertex buffer.
func (e *Encoder) SetVertexBuffer(buf backend.Buffer, slot int) {
	if b, ok := buf.(*Buffer); ok {
		d3d12.CmdSetVertexBuffers(e.dev.commandList, uint32(slot),
			b.gpuAddr, uint32(b.size), 0) // stride set per-pipeline
	}
}

// SetIndexBuffer binds an index buffer.
func (e *Encoder) SetIndexBuffer(buf backend.Buffer, format backend.IndexFormat) {
	if b, ok := buf.(*Buffer); ok {
		e.indexFormat = format
		dxFmt := int32(d3d12.FormatR16UInt)
		if format == backend.IndexUint32 {
			dxFmt = int32(d3d12.FormatR32UInt)
		}
		d3d12.CmdSetIndexBuffer(e.dev.commandList, b.gpuAddr, uint32(b.size), dxFmt)
	}
}

// SetTexture binds a texture to a slot.
func (e *Encoder) SetTexture(_ backend.Texture, _ int) {
	// In a full implementation, this would create/update an SRV in a descriptor table.
}

// SetTextureFilter overrides the texture filter for a slot.
func (e *Encoder) SetTextureFilter(_ int, _ backend.TextureFilter) {
	// Would configure a sampler in the root signature.
}

// SetStencil configures stencil test state.
func (e *Encoder) SetStencil(_ bool, _ backend.StencilDescriptor) {
	// Stencil state is baked into the PSO.
}

// SetColorWrite enables or disables color writing.
func (e *Encoder) SetColorWrite(_ bool) {
	// Color write mask is baked into the PSO.
}

// SetViewport sets the rendering viewport.
func (e *Encoder) SetViewport(vp backend.Viewport) {
	d3d12.CmdSetViewports(e.dev.commandList, d3d12.Viewport{
		TopLeftX: float32(vp.X),
		TopLeftY: float32(vp.Y),
		Width:    float32(vp.Width),
		Height:   float32(vp.Height),
		MinDepth: 0,
		MaxDepth: 1,
	})
}

// SetScissor sets the scissor rectangle.
func (e *Encoder) SetScissor(rect *backend.ScissorRect) {
	if rect == nil {
		d3d12.CmdSetScissorRects(e.dev.commandList, d3d12.Rect{
			Right:  int32(e.width),
			Bottom: int32(e.height),
		})
		return
	}
	d3d12.CmdSetScissorRects(e.dev.commandList, d3d12.Rect{
		Left:   int32(rect.X),
		Top:    int32(rect.Y),
		Right:  int32(rect.X + rect.Width),
		Bottom: int32(rect.Y + rect.Height),
	})
}

// Draw issues a non-indexed draw call.
func (e *Encoder) Draw(vertexCount, instanceCount, firstVertex int) {
	d3d12.CmdDrawInstanced(e.dev.commandList,
		uint32(vertexCount), uint32(instanceCount), uint32(firstVertex), 0)
}

// DrawIndexed issues an indexed draw call.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	d3d12.CmdDrawIndexedInstanced(e.dev.commandList,
		uint32(indexCount), uint32(instanceCount), uint32(firstIndex), 0, 0)
}

// Flush is a no-op — submission happens in EndFrame.
func (e *Encoder) Flush() {}
