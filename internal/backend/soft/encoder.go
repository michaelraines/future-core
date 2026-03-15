package soft

import (
	"math"

	"github.com/michaelraines/future-render/internal/backend"
)

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
// It tracks bound state and rasterizes triangles into the render target.
type Encoder struct {
	inPass        bool
	passDesc      backend.RenderPassDescriptor
	draws         []DrawRecord
	pipelineBound bool
	viewport      backend.Viewport
	scissor       *backend.ScissorRect
	stencil       bool
	colorWrite    bool

	// Depth buffer persists across draw calls within a render pass.
	depthBuf []float32

	// Bound state for rasterization.
	boundVertexBuf *Buffer
	boundIndexBuf  *Buffer
	boundIndexFmt  backend.IndexFormat
	boundTexture   *Texture
	boundFilter    backend.TextureFilter
	boundPipeline  *Pipeline
	boundShader    *Shader
}

// BeginRenderPass begins a render pass. For the software backend, this
// clears the target texture if the load action is LoadActionClear.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	e.inPass = true
	e.passDesc = desc
	e.colorWrite = true

	if desc.Target != nil {
		rt := desc.Target.(*RenderTarget)

		if desc.LoadAction == backend.LoadActionClear {
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

		// Allocate or reset the depth buffer if the render target has depth.
		if rt.depth != nil {
			size := rt.rtWidth * rt.rtHeight
			if cap(e.depthBuf) >= size {
				e.depthBuf = e.depthBuf[:size]
			} else {
				e.depthBuf = make([]float32, size)
			}
			if desc.LoadAction == backend.LoadActionClear {
				clearVal := float32(math.MaxFloat32)
				if desc.ClearDepth != 0 {
					clearVal = desc.ClearDepth
				}
				for i := range e.depthBuf {
					e.depthBuf[i] = clearVal
				}
			} else {
				for i := range e.depthBuf {
					e.depthBuf[i] = float32(math.MaxFloat32)
				}
			}
		}
	}
}

// EndRenderPass ends the current render pass.
func (e *Encoder) EndRenderPass() {
	e.inPass = false
	e.depthBuf = nil
}

// SetPipeline binds a render pipeline and its shader.
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	e.pipelineBound = true
	if p, ok := pipeline.(*Pipeline); ok {
		e.boundPipeline = p
		if p.desc.Shader != nil {
			if s, ok := p.desc.Shader.(*Shader); ok {
				e.boundShader = s
			}
		}
	}
}

// SetVertexBuffer binds a vertex buffer.
func (e *Encoder) SetVertexBuffer(buf backend.Buffer, _ int) {
	if b, ok := buf.(*Buffer); ok {
		e.boundVertexBuf = b
	}
}

// SetIndexBuffer binds an index buffer.
func (e *Encoder) SetIndexBuffer(buf backend.Buffer, format backend.IndexFormat) {
	if b, ok := buf.(*Buffer); ok {
		e.boundIndexBuf = b
		e.boundIndexFmt = format
	}
}

// SetTexture binds a texture.
func (e *Encoder) SetTexture(tex backend.Texture, _ int) {
	if t, ok := tex.(*Texture); ok {
		e.boundTexture = t
	}
}

// SetTextureFilter sets the texture filter for sampling.
func (e *Encoder) SetTextureFilter(_ int, filter backend.TextureFilter) {
	e.boundFilter = filter
}

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

// Draw issues a non-indexed draw call with rasterization.
func (e *Encoder) Draw(vertexCount, instanceCount, firstVertex int) {
	e.draws = append(e.draws, DrawRecord{
		Indexed:       false,
		VertexCount:   vertexCount,
		InstanceCount: instanceCount,
		FirstVertex:   firstVertex,
	})
	e.rasterizeNonIndexed(vertexCount, firstVertex)
}

// DrawIndexed issues an indexed draw call with rasterization.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	e.draws = append(e.draws, DrawRecord{
		Indexed:       true,
		IndexCount:    indexCount,
		InstanceCount: instanceCount,
		FirstIndex:    firstIndex,
	})
	e.rasterizeIndexed(indexCount, firstIndex)
}

// Flush submits all recorded commands (no-op for software backend).
func (e *Encoder) Flush() {}

// Draws returns all recorded draw calls. For testing only.
func (e *Encoder) Draws() []DrawRecord { return e.draws }

// ResetDraws clears the draw record list. For testing only.
func (e *Encoder) ResetDraws() { e.draws = nil }

// InPass reports whether a render pass is currently active.
func (e *Encoder) InPass() bool { return e.inPass }

// --- Rasterization ---

// rasterizeIndexed performs CPU rasterization for an indexed draw call.
func (e *Encoder) rasterizeIndexed(indexCount, firstIndex int) {
	rt := e.renderTarget()
	if rt == nil || e.boundVertexBuf == nil || e.boundIndexBuf == nil {
		return
	}

	verts := unpackVertices(e.boundVertexBuf.data)

	r := e.buildRasterizer(rt)
	proj := e.projectionMatrix()
	colorBody, colorTrans := e.colorMatrix()
	sampler := e.textureSampler()

	// Process triangles (3 indices per triangle).
	// Unpack indices according to the bound index format.
	if e.boundIndexFmt == backend.IndexUint32 {
		indices := unpackIndicesU32(e.boundIndexBuf.data)
		end := firstIndex + indexCount
		if end > len(indices) {
			end = len(indices)
		}
		for i := firstIndex; i+2 < end; i += 3 {
			i0, i1, i2 := int(indices[i]), int(indices[i+1]), int(indices[i+2])
			if i0 >= len(verts) || i1 >= len(verts) || i2 >= len(verts) {
				continue
			}
			r.rasterizeTriangle(verts[i0], verts[i1], verts[i2], proj, sampler, colorBody, colorTrans)
		}
	} else {
		indices := unpackIndicesU16(e.boundIndexBuf.data)
		end := firstIndex + indexCount
		if end > len(indices) {
			end = len(indices)
		}
		for i := firstIndex; i+2 < end; i += 3 {
			i0, i1, i2 := int(indices[i]), int(indices[i+1]), int(indices[i+2])
			if i0 >= len(verts) || i1 >= len(verts) || i2 >= len(verts) {
				continue
			}
			r.rasterizeTriangle(verts[i0], verts[i1], verts[i2], proj, sampler, colorBody, colorTrans)
		}
	}
}

// rasterizeNonIndexed performs CPU rasterization for a non-indexed draw call.
func (e *Encoder) rasterizeNonIndexed(vertexCount, firstVertex int) {
	rt := e.renderTarget()
	if rt == nil || e.boundVertexBuf == nil {
		return
	}

	verts := unpackVertices(e.boundVertexBuf.data)

	r := e.buildRasterizer(rt)
	proj := e.projectionMatrix()
	colorBody, colorTrans := e.colorMatrix()
	sampler := e.textureSampler()

	end := firstVertex + vertexCount
	if end > len(verts) {
		end = len(verts)
	}
	for i := firstVertex; i+2 < end; i += 3 {
		r.rasterizeTriangle(verts[i], verts[i+1], verts[i+2], proj, sampler, colorBody, colorTrans)
	}
}

// buildRasterizer creates a rasterizer configured with current encoder state.
func (e *Encoder) buildRasterizer(rt *RenderTarget) *rasterizer {
	r := &rasterizer{
		colorBuf:   rt.color.pixels,
		width:      rt.rtWidth,
		height:     rt.rtHeight,
		bpp:        rt.color.bpp,
		colorWrite: e.colorWrite,
		viewport:   viewportRect{x: e.viewport.X, y: e.viewport.Y, w: e.viewport.Width, h: e.viewport.Height},
	}

	// Default viewport to render target size if not set.
	if r.viewport.w == 0 || r.viewport.h == 0 {
		r.viewport = viewportRect{x: 0, y: 0, w: rt.rtWidth, h: rt.rtHeight}
	}

	// Scissor.
	if e.scissor != nil {
		r.scissor = &scissorRect{
			x: e.scissor.X, y: e.scissor.Y,
			w: e.scissor.Width, h: e.scissor.Height,
		}
	}

	// Depth state from pipeline.
	if e.boundPipeline != nil {
		r.depthTest = e.boundPipeline.desc.DepthTest
		r.depthWrite = e.boundPipeline.desc.DepthWrite
	}
	if r.depthTest && e.depthBuf != nil {
		r.depthBuf = e.depthBuf
	}

	// Blend mode from pipeline.
	r.blend = e.resolveBlendFunc()

	return r
}

// renderTarget returns the current render target, or nil if none.
func (e *Encoder) renderTarget() *RenderTarget {
	if e.passDesc.Target == nil {
		return nil
	}
	rt, ok := e.passDesc.Target.(*RenderTarget)
	if !ok {
		return nil
	}
	return rt
}

// projectionMatrix returns the projection matrix from the bound shader.
func (e *Encoder) projectionMatrix() [16]float32 {
	if e.boundShader == nil {
		return identityMatrix()
	}
	v, ok := e.boundShader.Uniform("uProjection")
	if !ok {
		return identityMatrix()
	}
	if mat, ok := v.([16]float32); ok {
		return mat
	}
	return identityMatrix()
}

// colorMatrix returns the color body matrix and translation from the bound shader.
func (e *Encoder) colorMatrix() (body [16]float32, trans [4]float32) {
	body = identityMatrix()

	if e.boundShader == nil {
		return body, trans
	}
	if v, ok := e.boundShader.Uniform("uColorBody"); ok {
		if m, ok := v.([16]float32); ok {
			body = m
		}
	}
	if v, ok := e.boundShader.Uniform("uColorTranslation"); ok {
		if t, ok := v.([4]float32); ok {
			trans = t
		}
	}
	return body, trans
}

// textureSampler returns a texture sampling function using the bound texture.
func (e *Encoder) textureSampler() func(u, v float32) (float32, float32, float32, float32) {
	if e.boundTexture == nil || e.boundTexture.pixels == nil {
		return func(_, _ float32) (float32, float32, float32, float32) {
			return 1, 1, 1, 1 // white if no texture
		}
	}
	t := e.boundTexture
	filter := e.boundFilter
	return func(u, v float32) (float32, float32, float32, float32) {
		if filter == backend.FilterLinear {
			return sampleLinear(t.pixels, t.w, t.h, t.bpp, u, v)
		}
		return sampleNearest(t.pixels, t.w, t.h, t.bpp, u, v)
	}
}

// resolveBlendFunc returns the blend function for the current pipeline blend mode.
func (e *Encoder) resolveBlendFunc() blendFunc {
	if e.boundPipeline == nil {
		return blendSourceOver
	}
	switch e.boundPipeline.desc.BlendMode {
	case backend.BlendNone:
		return blendNone
	case backend.BlendAdditive:
		return blendAdditive
	case backend.BlendMultiplicative:
		return blendMultiplicative
	case backend.BlendPremultiplied:
		return blendPremultiplied
	default:
		return blendSourceOver
	}
}

func identityMatrix() [16]float32 {
	return [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
}

// clampByte converts a float [0,1] to a byte [0,255].
func clampByte(f float32) byte {
	if f <= 0 {
		return 0
	}
	if f >= 1 {
		return 255
	}
	return byte(f*255 + 0.5)
}
