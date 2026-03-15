package vulkan

import "github.com/michaelraines/future-render/internal/backend"

// Encoder implements backend.CommandEncoder for Vulkan.
// Models a VkCommandBuffer recording. In a real implementation each method
// would call the corresponding vkCmd* function.
type Encoder struct {
	inner backend.CommandEncoder
}

// BeginRenderPass begins a render pass (vkCmdBeginRenderPass).
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	unwrapped := desc
	if rt, ok := desc.Target.(*RenderTarget); ok {
		unwrapped.Target = rt.inner
	}
	e.inner.BeginRenderPass(unwrapped)
}

// EndRenderPass ends the current render pass (vkCmdEndRenderPass).
func (e *Encoder) EndRenderPass() {
	e.inner.EndRenderPass()
}

// SetPipeline binds a graphics pipeline (vkCmdBindPipeline).
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	if p, ok := pipeline.(*Pipeline); ok {
		e.inner.SetPipeline(p.inner)
		return
	}
	e.inner.SetPipeline(pipeline)
}

// SetVertexBuffer binds a vertex buffer (vkCmdBindVertexBuffers).
func (e *Encoder) SetVertexBuffer(buf backend.Buffer, slot int) {
	if b, ok := buf.(*Buffer); ok {
		e.inner.SetVertexBuffer(b.inner, slot)
		return
	}
	e.inner.SetVertexBuffer(buf, slot)
}

// SetIndexBuffer binds an index buffer (vkCmdBindIndexBuffer).
func (e *Encoder) SetIndexBuffer(buf backend.Buffer, format backend.IndexFormat) {
	if b, ok := buf.(*Buffer); ok {
		e.inner.SetIndexBuffer(b.inner, format)
		return
	}
	e.inner.SetIndexBuffer(buf, format)
}

// SetTexture binds a texture via descriptor set (vkCmdBindDescriptorSets).
func (e *Encoder) SetTexture(tex backend.Texture, slot int) {
	if t, ok := tex.(*Texture); ok {
		e.inner.SetTexture(t.inner, slot)
		return
	}
	e.inner.SetTexture(tex, slot)
}

// SetTextureFilter overrides the texture filter for a slot.
func (e *Encoder) SetTextureFilter(slot int, filter backend.TextureFilter) {
	e.inner.SetTextureFilter(slot, filter)
}

// SetStencil configures stencil test state.
func (e *Encoder) SetStencil(enabled bool, desc backend.StencilDescriptor) {
	e.inner.SetStencil(enabled, desc)
}

// SetColorWrite enables or disables color writing.
func (e *Encoder) SetColorWrite(enabled bool) {
	e.inner.SetColorWrite(enabled)
}

// SetViewport sets the rendering viewport (vkCmdSetViewport).
func (e *Encoder) SetViewport(vp backend.Viewport) {
	e.inner.SetViewport(vp)
}

// SetScissor sets the scissor rectangle (vkCmdSetScissor).
func (e *Encoder) SetScissor(rect *backend.ScissorRect) {
	e.inner.SetScissor(rect)
}

// Draw issues a non-indexed draw call (vkCmdDraw).
func (e *Encoder) Draw(vertexCount, instanceCount, firstVertex int) {
	e.inner.Draw(vertexCount, instanceCount, firstVertex)
}

// DrawIndexed issues an indexed draw call (vkCmdDrawIndexed).
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	e.inner.DrawIndexed(indexCount, instanceCount, firstIndex)
}

// Flush submits all recorded commands (vkQueueSubmit).
func (e *Encoder) Flush() {
	e.inner.Flush()
}
