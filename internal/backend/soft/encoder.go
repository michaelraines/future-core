package soft

import (
	"math"

	"github.com/michaelraines/future-core/internal/backend"
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
	dev           *Device
	inPass        bool
	passDesc      backend.RenderPassDescriptor
	draws         []DrawRecord
	pipelineBound bool
	viewport      backend.Viewport
	scissor       *backend.ScissorRect
	stencilRef    uint32
	colorWrite    bool

	// Depth buffer persists across draw calls within a render pass.
	depthBuf []float32

	// Stencil buffer (one byte per pixel) when the current render pass's
	// target was created with HasStencil=true. Lives on the encoder for
	// the same reason depthBuf does — render-pass lifetime, reused
	// allocation across passes.
	stencilBuf []uint8

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

		// Allocate or reset the stencil buffer when the target carries a
		// stencil attachment. Unlike depth, stencil is not a depth-buffer
		// derivative — it's an independent uint8-per-pixel plane written
		// by stencil ops and compared against a dynamic reference value.
		if rt.hasStencil {
			size := rt.rtWidth * rt.rtHeight
			if cap(e.stencilBuf) >= size {
				e.stencilBuf = e.stencilBuf[:size]
			} else {
				e.stencilBuf = make([]uint8, size)
			}
			if desc.StencilLoadAction == backend.LoadActionClear {
				clearVal := uint8(desc.ClearStencil & 0xFF)
				for i := range e.stencilBuf {
					e.stencilBuf[i] = clearVal
				}
			}
		} else {
			e.stencilBuf = nil
		}
	}
}

// EndRenderPass ends the current render pass.
func (e *Encoder) EndRenderPass() {
	e.inPass = false
	e.depthBuf = nil
	e.stencilBuf = nil
}

// SetPipeline binds a render pipeline and its shader. Pipeline-baked color
// write state applies eagerly: ColorWriteDisabled overrides any prior
// SetColorWrite call for the duration this pipeline is bound. Consumers
// that still want runtime color-write toggling can call SetColorWrite
// after SetPipeline.
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	e.pipelineBound = true
	if p, ok := pipeline.(*Pipeline); ok {
		e.boundPipeline = p
		e.colorWrite = !p.desc.ColorWriteDisabled
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

// SetStencilReference updates the dynamic stencil reference value. The
// enabled flag, ops, compare func, and masks are baked into the pipeline
// and read from sp.boundStencil on SetPipeline. The reference value is
// used on every stencil test against the bound pipeline's Func.
func (e *Encoder) SetStencilReference(ref uint32) {
	e.stencilRef = ref
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
// SetBlendMode is a no-op for this backend.
func (e *Encoder) SetBlendMode(_ backend.BlendMode) {}

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
	// Hoist the color-matrix identity check out of the per-triangle loop.
	// Scene-selector issues tens of thousands of triangles per frame with
	// the same (identity) colorBody; the [16]float32 compare that powers
	// the inner fast-path dominated CPU in soft-backend profiling.
	colorIsIdentity := isIdentityMatrix(colorBody) && colorTrans == [4]float32{}
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
			r.rasterizeTriangle(verts[i0], verts[i1], verts[i2], proj, sampler, colorBody, colorTrans, colorIsIdentity)
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
			r.rasterizeTriangle(verts[i0], verts[i1], verts[i2], proj, sampler, colorBody, colorTrans, colorIsIdentity)
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
	colorIsIdentity := isIdentityMatrix(colorBody) && colorTrans == [4]float32{}
	sampler := e.textureSampler()

	end := firstVertex + vertexCount
	if end > len(verts) {
		end = len(verts)
	}
	for i := firstVertex; i+2 < end; i += 3 {
		r.rasterizeTriangle(verts[i], verts[i+1], verts[i+2], proj, sampler, colorBody, colorTrans, colorIsIdentity)
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

	// Stencil state from pipeline. Ops, compare func, and masks are baked
	// into the pipeline; the reference value is dynamic on the encoder.
	// The rasterizer reads these copies; the hot loop short-circuits when
	// stencilEnable is false so the zero-stencil case is almost free.
	//
	// A stencil-enabled pipeline bound against an RT without a stencil
	// attachment (stencilBuf == nil) is a caller bug — the draw would
	// silently produce no pixels (write pass has ColorWriteDisabled,
	// color pass's NotEqual ref=0 test reads zeros from the missing
	// buffer and rejects everything). Panic with a precise message so
	// the misconfiguration is visible rather than invisibly broken.
	if e.boundPipeline != nil && e.boundPipeline.desc.StencilEnable {
		if e.stencilBuf == nil {
			panic("soft: stencil pipeline bound to render target without HasStencil; " +
				"set RenderTargetDescriptor.HasStencil=true or gate via DeviceCapabilities.SupportsStencil")
		}
		sd := e.boundPipeline.desc.Stencil
		r.stencilBuf = e.stencilBuf
		r.stencilEnable = true
		r.stencilFunc = sd.Func
		r.stencilMask = uint8(sd.Mask & 0xFF)
		r.stencilWriteMask = uint8(sd.WriteMask & 0xFF)
		r.stencilRef = uint8(e.stencilRef & 0xFF)
		r.stencilFront = sd.Front
		if sd.TwoSided {
			r.stencilBack = sd.Back
		} else {
			r.stencilBack = sd.Front
		}
	}

	// Blend mode from pipeline.
	r.blend = e.resolveBlendFunc()

	return r
}

// renderTarget returns the current render target. When the pass target is nil
// (screen rendering), the device's internal screen render target is used so
// the software rasterizer produces actual pixels for presentation.
func (e *Encoder) renderTarget() *RenderTarget {
	if e.passDesc.Target == nil {
		if e.dev != nil {
			return e.dev.screenRT
		}
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
// Preset modes map to hand-tuned functions for clarity; any other factor/op
// combination (e.g. alpha-masking or shadow-modulated additive blends used
// by the lighting system) falls through to a generic factor-based blender
// that honors the pipeline descriptor's BlendMode struct exactly.
func (e *Encoder) resolveBlendFunc() blendFunc {
	if e.boundPipeline == nil {
		return blendSourceOver
	}
	mode := e.boundPipeline.desc.BlendMode
	switch mode {
	case backend.BlendNone:
		return blendNone
	case backend.BlendSourceOver, backend.BlendPremultiplied:
		return blendSourceOver
	case backend.BlendAdditive:
		return blendAdditive
	case backend.BlendMultiplicative:
		return blendMultiplicative
	}
	if !mode.Enabled {
		return blendNone
	}
	return makeGenericBlender(mode)
}

// makeGenericBlender returns a blendFunc that evaluates
//
//	out = op(src * srcFactor, dst * dstFactor)
//
// per channel, using the factors and operations from mode. This is the
// correctness path for arbitrary blend modes (e.g. lighting's
// blendAddModAlpha) and is why the software rasterizer can act as the
// reference implementation for every GPU backend.
func makeGenericBlender(mode backend.BlendMode) blendFunc {
	return func(sr, sg, sb, sa, dr, dg, db, da float32) (or, og, ob, oa float32) {
		srcFR, srcFG, srcFB := blendFactorRGB(mode.SrcFactorRGB, sr, sg, sb, sa, dr, dg, db, da)
		dstFR, dstFG, dstFB := blendFactorRGB(mode.DstFactorRGB, sr, sg, sb, sa, dr, dg, db, da)
		srcFA := blendFactorAlpha(mode.SrcFactorAlpha, sa, da)
		dstFA := blendFactorAlpha(mode.DstFactorAlpha, sa, da)

		or = combineChannel(mode.OpRGB, sr*srcFR, dr*dstFR, sr, dr)
		og = combineChannel(mode.OpRGB, sg*srcFG, dg*dstFG, sg, dg)
		ob = combineChannel(mode.OpRGB, sb*srcFB, db*dstFB, sb, db)
		oa = combineChannel(mode.OpAlpha, sa*srcFA, da*dstFA, sa, da)

		return clampf(or), clampf(og), clampf(ob), clampf(oa)
	}
}

// blendFactorRGB returns the per-channel multiplier for an RGB factor. The
// last two unnamed return values correspond to channels r, g, b; when a
// factor is a scalar (like src-alpha) all three channels receive the same
// multiplier.
func blendFactorRGB(f backend.BlendFactor, sr, sg, sb, sa, dr, dg, db, da float32) (fr, fg, fb float32) {
	switch f {
	case backend.BlendFactorZero:
		return 0, 0, 0
	case backend.BlendFactorOne:
		return 1, 1, 1
	case backend.BlendFactorSrcAlpha:
		return sa, sa, sa
	case backend.BlendFactorOneMinusSrcAlpha:
		return 1 - sa, 1 - sa, 1 - sa
	case backend.BlendFactorDstAlpha:
		return da, da, da
	case backend.BlendFactorOneMinusDstAlpha:
		return 1 - da, 1 - da, 1 - da
	case backend.BlendFactorSrcColor:
		return sr, sg, sb
	case backend.BlendFactorOneMinusSrcColor:
		return 1 - sr, 1 - sg, 1 - sb
	case backend.BlendFactorDstColor:
		return dr, dg, db
	case backend.BlendFactorOneMinusDstColor:
		return 1 - dr, 1 - dg, 1 - db
	default:
		return 1, 1, 1
	}
}

// blendFactorAlpha returns the alpha-channel multiplier for a factor.
// Color-based factors degrade to their alpha equivalents here because the
// alpha channel has no separate RGB components to sample.
func blendFactorAlpha(f backend.BlendFactor, sa, da float32) float32 {
	switch f {
	case backend.BlendFactorZero:
		return 0
	case backend.BlendFactorOne:
		return 1
	case backend.BlendFactorSrcAlpha, backend.BlendFactorSrcColor:
		return sa
	case backend.BlendFactorOneMinusSrcAlpha, backend.BlendFactorOneMinusSrcColor:
		return 1 - sa
	case backend.BlendFactorDstAlpha, backend.BlendFactorDstColor:
		return da
	case backend.BlendFactorOneMinusDstAlpha, backend.BlendFactorOneMinusDstColor:
		return 1 - da
	default:
		return 1
	}
}

// combineChannel applies a BlendOperation to already-weighted source and
// destination values. For min/max the weighted values are ignored per the
// GPU spec — factors are inputs only to Add/Subtract/ReverseSubtract. The
// raw (unweighted) values are passed in via rawS/rawD for Min/Max.
func combineChannel(op backend.BlendOperation, weightedS, weightedD, rawS, rawD float32) float32 {
	switch op {
	case backend.BlendOpAdd:
		return weightedS + weightedD
	case backend.BlendOpSubtract:
		return weightedS - weightedD
	case backend.BlendOpReverseSubtract:
		return weightedD - weightedS
	case backend.BlendOpMin:
		if rawS < rawD {
			return rawS
		}
		return rawD
	case backend.BlendOpMax:
		if rawS > rawD {
			return rawS
		}
		return rawD
	default:
		return weightedS + weightedD
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
