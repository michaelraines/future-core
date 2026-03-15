package softdelegate

import "github.com/michaelraines/future-render/internal/backend"

// Compile-time assertion that Encoder implements backend.CommandEncoder.
var _ backend.CommandEncoder = (*Encoder)(nil)

// PipelineUnwrapper provides access to the inner soft pipeline.
type PipelineUnwrapper interface {
	InnerPipeline() backend.Pipeline
}

// BufferUnwrapper provides access to the inner soft buffer.
type BufferUnwrapper interface {
	InnerBuffer() backend.Buffer
}

// TextureUnwrapper provides access to the inner soft texture.
type TextureUnwrapper interface {
	InnerTexture() backend.Texture
}

// RenderTargetUnwrapper provides access to the inner soft render target.
type RenderTargetUnwrapper interface {
	InnerRenderTarget() backend.RenderTarget
}

// Encoder implements backend.CommandEncoder by delegating to an inner
// encoder. Wrapper types are automatically unwrapped via the Unwrapper
// interfaces before delegation to the soft encoder.
type Encoder struct {
	Inner backend.CommandEncoder
}

// BeginRenderPass begins a render pass, unwrapping the target if needed.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	if rt, ok := desc.Target.(RenderTargetUnwrapper); ok {
		unwrapped := desc
		unwrapped.Target = rt.InnerRenderTarget()
		e.Inner.BeginRenderPass(unwrapped)
		return
	}
	e.Inner.BeginRenderPass(desc)
}

// EndRenderPass ends the current render pass.
func (e *Encoder) EndRenderPass() {
	e.Inner.EndRenderPass()
}

// SetPipeline binds a render pipeline, unwrapping if needed.
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	if p, ok := pipeline.(PipelineUnwrapper); ok {
		e.Inner.SetPipeline(p.InnerPipeline())
		return
	}
	e.Inner.SetPipeline(pipeline)
}

// SetVertexBuffer binds a vertex buffer, unwrapping if needed.
func (e *Encoder) SetVertexBuffer(buf backend.Buffer, slot int) {
	if b, ok := buf.(BufferUnwrapper); ok {
		e.Inner.SetVertexBuffer(b.InnerBuffer(), slot)
		return
	}
	e.Inner.SetVertexBuffer(buf, slot)
}

// SetIndexBuffer binds an index buffer, unwrapping if needed.
func (e *Encoder) SetIndexBuffer(buf backend.Buffer, format backend.IndexFormat) {
	if b, ok := buf.(BufferUnwrapper); ok {
		e.Inner.SetIndexBuffer(b.InnerBuffer(), format)
		return
	}
	e.Inner.SetIndexBuffer(buf, format)
}

// SetTexture binds a texture to a slot, unwrapping if needed.
func (e *Encoder) SetTexture(tex backend.Texture, slot int) {
	if t, ok := tex.(TextureUnwrapper); ok {
		e.Inner.SetTexture(t.InnerTexture(), slot)
		return
	}
	e.Inner.SetTexture(tex, slot)
}

// SetTextureFilter overrides the texture filter for a slot.
func (e *Encoder) SetTextureFilter(slot int, filter backend.TextureFilter) {
	e.Inner.SetTextureFilter(slot, filter)
}

// SetStencil configures stencil test state.
func (e *Encoder) SetStencil(enabled bool, desc backend.StencilDescriptor) {
	e.Inner.SetStencil(enabled, desc)
}

// SetColorWrite enables or disables color writing.
func (e *Encoder) SetColorWrite(enabled bool) {
	e.Inner.SetColorWrite(enabled)
}

// SetViewport sets the rendering viewport.
func (e *Encoder) SetViewport(vp backend.Viewport) {
	e.Inner.SetViewport(vp)
}

// SetScissor sets the scissor rectangle.
func (e *Encoder) SetScissor(rect *backend.ScissorRect) {
	e.Inner.SetScissor(rect)
}

// Draw issues a non-indexed draw call.
func (e *Encoder) Draw(vertexCount, instanceCount, firstVertex int) {
	e.Inner.Draw(vertexCount, instanceCount, firstVertex)
}

// DrawIndexed issues an indexed draw call.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	e.Inner.DrawIndexed(indexCount, instanceCount, firstIndex)
}

// Flush submits all recorded commands.
func (e *Encoder) Flush() {
	e.Inner.Flush()
}
