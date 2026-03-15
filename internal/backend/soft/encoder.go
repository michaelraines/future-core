package soft

import "github.com/michaelraines/future-render/internal/backend"

// DrawRecord captures the parameters of a single draw call. For testing and
// conformance verification.
type DrawRecord struct {
	Indexed       bool
	VertexCount   int
	IndexCount    int
	InstanceCount int
	FirstVertex   int
	FirstIndex    int
}

// Encoder implements backend.CommandEncoder for the software backend.
// It records commands for inspection rather than executing GPU operations.
type Encoder struct {
	inPass        bool
	passDesc      backend.RenderPassDescriptor
	draws         []DrawRecord
	pipelineBound bool
	viewport      backend.Viewport
	scissor       *backend.ScissorRect
	stencil       bool
	colorWrite    bool
}

// BeginRenderPass begins a render pass. For the software backend, this
// clears the target texture if the load action is LoadActionClear.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	e.inPass = true
	e.passDesc = desc
	e.colorWrite = true

	if desc.LoadAction == backend.LoadActionClear && desc.Target != nil {
		rt := desc.Target.(*RenderTarget)
		pixels := rt.color.pixels
		c := desc.ClearColor
		r := clampByte(c[0])
		g := clampByte(c[1])
		b := clampByte(c[2])
		a := clampByte(c[3])
		for i := 0; i+3 < len(pixels); i += 4 {
			pixels[i] = r
			pixels[i+1] = g
			pixels[i+2] = b
			pixels[i+3] = a
		}
	}
}

// EndRenderPass ends the current render pass.
func (e *Encoder) EndRenderPass() {
	e.inPass = false
}

// SetPipeline binds a render pipeline.
func (e *Encoder) SetPipeline(_ backend.Pipeline) {
	e.pipelineBound = true
}

// SetVertexBuffer binds a vertex buffer (no-op for software backend).
func (e *Encoder) SetVertexBuffer(_ backend.Buffer, _ int) {}

// SetIndexBuffer binds an index buffer (no-op for software backend).
func (e *Encoder) SetIndexBuffer(_ backend.Buffer, _ backend.IndexFormat) {}

// SetTexture binds a texture (no-op for software backend).
func (e *Encoder) SetTexture(_ backend.Texture, _ int) {}

// SetTextureFilter sets the texture filter (no-op for software backend).
func (e *Encoder) SetTextureFilter(_ int, _ backend.TextureFilter) {}

// SetStencil configures stencil test state.
func (e *Encoder) SetStencil(enabled bool, _ backend.StencilDescriptor) {
	e.stencil = enabled
}

// SetColorWrite enables or disables color writing.
func (e *Encoder) SetColorWrite(enabled bool) {
	e.colorWrite = enabled
}

// SetViewport sets the rendering viewport.
func (e *Encoder) SetViewport(vp backend.Viewport) {
	e.viewport = vp
}

// SetScissor sets the scissor rectangle.
func (e *Encoder) SetScissor(rect *backend.ScissorRect) {
	e.scissor = rect
}

// Draw issues a non-indexed draw call.
func (e *Encoder) Draw(vertexCount, instanceCount, firstVertex int) {
	e.draws = append(e.draws, DrawRecord{
		Indexed:       false,
		VertexCount:   vertexCount,
		InstanceCount: instanceCount,
		FirstVertex:   firstVertex,
	})
}

// DrawIndexed issues an indexed draw call.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	e.draws = append(e.draws, DrawRecord{
		Indexed:       true,
		IndexCount:    indexCount,
		InstanceCount: instanceCount,
		FirstIndex:    firstIndex,
	})
}

// Flush submits all recorded commands (no-op for software backend).
func (e *Encoder) Flush() {}

// Draws returns all recorded draw calls. For testing only.
func (e *Encoder) Draws() []DrawRecord { return e.draws }

// ResetDraws clears the draw record list. For testing only.
func (e *Encoder) ResetDraws() { e.draws = nil }

// InPass reports whether a render pass is currently active.
func (e *Encoder) InPass() bool { return e.inPass }

// clampByte converts a float [0,1] to a byte [0,255].
func clampByte(f float32) byte {
	if f <= 0 {
		return 0
	}
	if f >= 1 {
		return 255
	}
	return byte(f * 255)
}
