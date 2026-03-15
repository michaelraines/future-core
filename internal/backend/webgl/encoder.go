package webgl

import "github.com/michaelraines/future-render/internal/backend"

// Encoder implements backend.CommandEncoder for WebGL2.
// Delegates all commands to the soft rasterizer's encoder. A real
// implementation would translate these to WebGL2 gl.bindBuffer,
// gl.drawElements, etc. calls via syscall/js.
type Encoder struct {
	inner backend.CommandEncoder
}

// BeginRenderPass begins a render pass.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	// Unwrap webgl.RenderTarget to its inner soft.RenderTarget for delegation.
	unwrapped := desc
	if rt, ok := desc.Target.(*RenderTarget); ok {
		unwrapped.Target = rt.inner
	}
	e.inner.BeginRenderPass(unwrapped)
}

// EndRenderPass ends the current render pass.
func (e *Encoder) EndRenderPass() {
	e.inner.EndRenderPass()
}

// SetPipeline binds a render pipeline.
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	if p, ok := pipeline.(*Pipeline); ok {
		e.inner.SetPipeline(p.inner)
		return
	}
	e.inner.SetPipeline(pipeline)
}

// SetVertexBuffer binds a vertex buffer.
func (e *Encoder) SetVertexBuffer(buf backend.Buffer, slot int) {
	if b, ok := buf.(*Buffer); ok {
		e.inner.SetVertexBuffer(b.inner, slot)
		return
	}
	e.inner.SetVertexBuffer(buf, slot)
}

// SetIndexBuffer binds an index buffer.
func (e *Encoder) SetIndexBuffer(buf backend.Buffer, format backend.IndexFormat) {
	if b, ok := buf.(*Buffer); ok {
		e.inner.SetIndexBuffer(b.inner, format)
		return
	}
	e.inner.SetIndexBuffer(buf, format)
}

// SetTexture binds a texture to a slot.
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

// SetViewport sets the rendering viewport.
func (e *Encoder) SetViewport(vp backend.Viewport) {
	e.inner.SetViewport(vp)
}

// SetScissor sets the scissor rectangle.
func (e *Encoder) SetScissor(rect *backend.ScissorRect) {
	e.inner.SetScissor(rect)
}

// Draw issues a non-indexed draw call.
func (e *Encoder) Draw(vertexCount, instanceCount, firstVertex int) {
	e.inner.Draw(vertexCount, instanceCount, firstVertex)
}

// DrawIndexed issues an indexed draw call.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	e.inner.DrawIndexed(indexCount, instanceCount, firstIndex)
}

// Flush submits all recorded commands.
func (e *Encoder) Flush() {
	e.inner.Flush()
}
