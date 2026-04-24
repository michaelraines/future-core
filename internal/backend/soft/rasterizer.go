package soft

import (
	"encoding/binary"
	"math"

	"github.com/michaelraines/future-core/internal/backend"
)

// rasterizer performs CPU-based triangle rasterization into a framebuffer.
type rasterizer struct {
	colorBuf   []byte
	depthBuf   []float32
	width      int
	height     int
	bpp        int
	blend      blendFunc
	depthTest  bool
	depthWrite bool
	colorWrite bool
	scissor    *scissorRect
	viewport   viewportRect

	// Stencil state (flat in the hot path — copied from the bound
	// pipeline + encoder in buildRasterizer so the loop never touches
	// backend types). When stencilEnable is false the stencil test is
	// skipped entirely; there is no per-pixel cost.
	stencilBuf       []uint8
	stencilEnable    bool
	stencilFunc      backend.CompareFunc
	stencilRef       uint8
	stencilMask      uint8
	stencilWriteMask uint8
	stencilFront     backend.StencilFaceOps
	stencilBack      backend.StencilFaceOps
}

type scissorRect struct {
	x, y, w, h int
}

type viewportRect struct {
	x, y, w, h int
}

// blendFunc blends a source RGBA onto a destination RGBA.
type blendFunc func(sr, sg, sb, sa, dr, dg, db, da float32) (or, og, ob, oa float32)

// vertex2D is the unpacked form of a Vertex2D from the vertex buffer.
type vertex2D struct {
	px, py     float32 // position
	tu, tv     float32 // texcoord
	r, g, b, a float32 // color
}

// unpackVertices reads Vertex2D structs from raw bytes.
// Each vertex is 32 bytes: [PosX, PosY, TexU, TexV, R, G, B, A] as float32.
func unpackVertices(data []byte) []vertex2D {
	count := len(data) / 32
	verts := make([]vertex2D, count)
	for i := range count {
		off := i * 32
		verts[i] = vertex2D{
			px: math.Float32frombits(binary.LittleEndian.Uint32(data[off:])),
			py: math.Float32frombits(binary.LittleEndian.Uint32(data[off+4:])),
			tu: math.Float32frombits(binary.LittleEndian.Uint32(data[off+8:])),
			tv: math.Float32frombits(binary.LittleEndian.Uint32(data[off+12:])),
			r:  math.Float32frombits(binary.LittleEndian.Uint32(data[off+16:])),
			g:  math.Float32frombits(binary.LittleEndian.Uint32(data[off+20:])),
			b:  math.Float32frombits(binary.LittleEndian.Uint32(data[off+24:])),
			a:  math.Float32frombits(binary.LittleEndian.Uint32(data[off+28:])),
		}
	}
	return verts
}

// unpackIndicesU16 reads uint16 indices from raw bytes.
func unpackIndicesU16(data []byte) []uint16 {
	count := len(data) / 2
	idx := make([]uint16, count)
	for i := range count {
		idx[i] = binary.LittleEndian.Uint16(data[i*2:])
	}
	return idx
}

// unpackIndicesU32 reads uint32 indices from raw bytes.
func unpackIndicesU32(data []byte) []uint32 {
	count := len(data) / 4
	idx := make([]uint32, count)
	for i := range count {
		idx[i] = binary.LittleEndian.Uint32(data[i*4:])
	}
	return idx
}

// transformVertex applies a 4x4 projection matrix to a vertex position.
// The matrix is column-major [16]float32.
// Returns clip-space position (x, y, z, w).
func transformVertex(v vertex2D, proj [16]float32) (x, y, z, w float32) {
	px, py := v.px, v.py
	// Multiply: proj * [px, py, 0, 1]
	x = proj[0]*px + proj[4]*py + proj[12]
	y = proj[1]*px + proj[5]*py + proj[13]
	z = proj[2]*px + proj[6]*py + proj[14]
	w = proj[3]*px + proj[7]*py + proj[15]
	return
}

// ndcToScreen converts NDC coordinates to screen pixel coordinates.
//
// The engine's orthographic projection (fmath.Mat4Ortho with bottom=H
// and top=0) places world Y=0 at NDC Y=+1 and world Y=H at NDC Y=-1.
// We then need to land world Y=0 at screen row 0 (the top) and world
// Y=H at screen row H-1 (the bottom) — i.e. ndcY=+1 → top, ndcY=-1 →
// bottom. That matches Vulkan/WebGPU's Y-down screen convention and
// is what the rest of the stack (sprite pass, scene-selector layout,
// debug HUD positioning) assumes.
//
// The earlier form — (ndcY+1)*0.5*H — produced OpenGL's Y-up screen
// space (ndcY=+1 → bottom), which rendered the entire composite
// vertically mirrored vs. GPU backends. Visible symptom: scene-
// selector's debug HUD appeared in the top-right corner with text
// upside-down instead of the bottom-right with upright text.
func ndcToScreen(ndcX, ndcY float32, vp viewportRect) (sx, sy float32) {
	sx = float32(vp.x) + (ndcX+1)*0.5*float32(vp.w)
	sy = float32(vp.y) + (1-ndcY)*0.5*float32(vp.h)
	return sx, sy
}

// rasterizeTriangle rasterizes a single triangle using the half-space method.
// It calls emit for each fragment that passes the depth test.
//
// All vertex transform, screen projection, barycentric weight, and UV
// interpolation is computed in float64 with explicit math.FMA to guarantee
// cross-platform determinism. ARM64 auto-FMAs float32/float64 multiply-add
// chains while x86_64 (GOAMD64=v1) does not, causing different rounding at
// texel boundaries. math.FMA forces identical correctly-rounded results on
// both platforms (hardware FMA on ARM64, software on x86_64).
func (r *rasterizer) rasterizeTriangle(
	v0, v1, v2 vertex2D,
	proj [16]float32,
	texSampler func(u, v float32) (float32, float32, float32, float32),
	colorBody [16]float32,
	colorTranslation [4]float32,
	colorIsIdentity bool,
) {
	// Transform vertices to clip space in float64 with explicit FMA.
	p := [16]float64{}
	for i, v := range proj {
		p[i] = float64(v)
	}
	// colorIsIdentity is a caller-provided hoist of the identity check.
	// applyColorMatrix uses it to skip matrix/translation work — the
	// common case — without a per-pixel [16]float32 compare. Callers
	// compute it once per draw via isIdentityMatrix + zero-vector check.
	px0, py0 := float64(v0.px), float64(v0.py)
	px1, py1 := float64(v1.px), float64(v1.py)
	px2, py2 := float64(v2.px), float64(v2.py)

	// proj * [px, py, 0, 1] using math.FMA for determinism.
	cx0 := math.FMA(p[0], px0, math.FMA(p[4], py0, p[12]))
	cy0 := math.FMA(p[1], px0, math.FMA(p[5], py0, p[13]))
	cz0 := math.FMA(p[2], px0, math.FMA(p[6], py0, p[14]))
	cw0 := math.FMA(p[3], px0, math.FMA(p[7], py0, p[15]))

	cx1 := math.FMA(p[0], px1, math.FMA(p[4], py1, p[12]))
	cy1 := math.FMA(p[1], px1, math.FMA(p[5], py1, p[13]))
	cz1 := math.FMA(p[2], px1, math.FMA(p[6], py1, p[14]))
	cw1 := math.FMA(p[3], px1, math.FMA(p[7], py1, p[15]))

	cx2 := math.FMA(p[0], px2, math.FMA(p[4], py2, p[12]))
	cy2 := math.FMA(p[1], px2, math.FMA(p[5], py2, p[13]))
	cz2 := math.FMA(p[2], px2, math.FMA(p[6], py2, p[14]))
	cw2 := math.FMA(p[3], px2, math.FMA(p[7], py2, p[15]))

	// Perspective divide → NDC.
	if cw0 == 0 || cw1 == 0 || cw2 == 0 {
		return
	}
	nx0, ny0, nz0 := cx0/cw0, cy0/cw0, cz0/cw0
	nx1, ny1, nz1 := cx1/cw1, cy1/cw1, cz1/cw1
	nx2, ny2, nz2 := cx2/cw2, cy2/cw2, cz2/cw2

	// NDC → screen in float64 with math.FMA. Y-down convention
	// (ndcY=+1 → screen top, ndcY=-1 → screen bottom) to match
	// Vulkan/WebGPU and the engine's ortho projection. See the
	// commentary on ndcToScreen for why we don't use OpenGL's Y-up
	// mapping here.
	vpx, vpy := float64(r.viewport.x), float64(r.viewport.y)
	halfW := 0.5 * float64(r.viewport.w)
	halfH := 0.5 * float64(r.viewport.h)

	sx0 := math.FMA(nx0+1, halfW, vpx)
	sy0 := math.FMA(1-ny0, halfH, vpy)
	sx1 := math.FMA(nx1+1, halfW, vpx)
	sy1 := math.FMA(1-ny1, halfH, vpy)
	sx2 := math.FMA(nx2+1, halfW, vpx)
	sy2 := math.FMA(1-ny2, halfH, vpy)

	// Bounding box (clamped to framebuffer).
	minX := int(math.Floor(min3(sx0, sx1, sx2)))
	maxX := int(math.Ceil(max3(sx0, sx1, sx2)))
	minY := int(math.Floor(min3(sy0, sy1, sy2)))
	maxY := int(math.Ceil(max3(sy0, sy1, sy2)))

	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > r.width {
		maxX = r.width
	}
	if maxY > r.height {
		maxY = r.height
	}

	// Apply scissor.
	if r.scissor != nil {
		if minX < r.scissor.x {
			minX = r.scissor.x
		}
		if minY < r.scissor.y {
			minY = r.scissor.y
		}
		if maxX > r.scissor.x+r.scissor.w {
			maxX = r.scissor.x + r.scissor.w
		}
		if maxY > r.scissor.y+r.scissor.h {
			maxY = r.scissor.y + r.scissor.h
		}
	}

	// Edge function denominator for barycentric coordinates.
	denom := edgeFuncFMA(sx0, sy0, sx1, sy1, sx2, sy2)
	if denom == 0 {
		return // degenerate triangle
	}
	invDenom := 1.0 / denom

	// Triangle winding — positive denom is CCW (front) under this
	// rasterizer's convention (matches WebGPU/Vulkan FrontFaceCCW). Picked
	// once per triangle so the inner loop indexes the face's ops directly.
	faceOps := r.stencilFront
	if denom < 0 {
		faceOps = r.stencilBack
	}

	// Precompute float64 UV coordinates.
	tu0, tv0 := float64(v0.tu), float64(v0.tv)
	tu1, tv1 := float64(v1.tu), float64(v1.tv)
	tu2, tv2 := float64(v2.tu), float64(v2.tv)

	// Rasterize: iterate over bounding box pixels.
	for py := minY; py < maxY; py++ {
		for px := minX; px < maxX; px++ {
			// Sample at pixel center.
			pcx := float64(px) + 0.5
			pcy := float64(py) + 0.5

			// Barycentric coordinates via FMA edge function.
			w0 := edgeFuncFMA(sx1, sy1, sx2, sy2, pcx, pcy) * invDenom
			w1 := edgeFuncFMA(sx2, sy2, sx0, sy0, pcx, pcy) * invDenom
			w2 := edgeFuncFMA(sx0, sy0, sx1, sy1, pcx, pcy) * invDenom

			// Inside triangle test.
			if w0 < 0 || w1 < 0 || w2 < 0 {
				continue
			}

			// Interpolate depth (all terms via FMA for consistent rounding).
			depth := math.FMA(w0, nz0, math.FMA(w1, nz1, math.FMA(w2, nz2, 0)))

			pixelIdx := py*r.width + px

			// Stencil test runs before depth per GL semantics. When the
			// stencil test fails we apply the SFail op and skip both
			// color write and depth write. A pixel outside the stencil
			// buffer (defensive guard — shouldn't happen in practice
			// because the buffer is sized to the full RT) is treated as
			// a test failure: we skip color output since we can't
			// apply ops either, and writing a colored pixel without a
			// matching stencil update would diverge from GL/WebGPU.
			if r.stencilEnable {
				if pixelIdx >= len(r.stencilBuf) {
					continue
				}
				current := r.stencilBuf[pixelIdx]
				if !stencilCompare(r.stencilFunc, r.stencilRef&r.stencilMask, current&r.stencilMask) {
					next := stencilApplyOp(faceOps.SFail, r.stencilRef, current)
					r.stencilBuf[pixelIdx] = (current &^ r.stencilWriteMask) |
						(next & r.stencilWriteMask)
					continue
				}
			}

			// Depth test.
			depthPassed := true
			if r.depthTest {
				depthF32 := float32(depth)
				if pixelIdx < len(r.depthBuf) && depthF32 > r.depthBuf[pixelIdx] {
					depthPassed = false
				} else if r.depthWrite && pixelIdx < len(r.depthBuf) {
					r.depthBuf[pixelIdx] = depthF32
				}
			}

			// Stencil op after depth: DPPass / DPFail mirror GL.
			if r.stencilEnable && pixelIdx < len(r.stencilBuf) {
				current := r.stencilBuf[pixelIdx]
				op := faceOps.DPPass
				if !depthPassed {
					op = faceOps.DPFail
				}
				next := stencilApplyOp(op, r.stencilRef, current)
				r.stencilBuf[pixelIdx] = (current &^ r.stencilWriteMask) |
					(next & r.stencilWriteMask)
			}

			if !depthPassed {
				continue
			}

			// Interpolate texcoords with FMA for determinism (all terms via FMA).
			u := float32(math.FMA(w0, tu0, math.FMA(w1, tu1, math.FMA(w2, tu2, 0))))
			v := float32(math.FMA(w0, tv0, math.FMA(w1, tv1, math.FMA(w2, tv2, 0))))

			// Interpolate vertex color (float32 — ±3 tolerance absorbs any FMA diff).
			w0f, w1f, w2f := float32(w0), float32(w1), float32(w2)
			cr := w0f*v0.r + w1f*v1.r + w2f*v2.r
			cg := w0f*v0.g + w1f*v1.g + w2f*v2.g
			cb := w0f*v0.b + w1f*v1.b + w2f*v2.b
			ca := w0f*v0.a + w1f*v1.a + w2f*v2.a

			// Sample texture.
			tr, tg, tb, ta := texSampler(u, v)

			// Combine vertex color with texture (multiply).
			fr := cr * tr
			fg := cg * tg
			fb := cb * tb
			fa := ca * ta

			// Apply color matrix transform.
			fr, fg, fb, fa = applyColorMatrix(fr, fg, fb, fa, colorBody, colorTranslation, colorIsIdentity)

			// Write fragment.
			if r.colorWrite {
				r.writePixel(px, py, fr, fg, fb, fa)
			}
		}
	}
}

// stencilCompare applies a stencil compare function. Both operands are
// already pre-masked by the caller (ref & readMask, buf & readMask).
func stencilCompare(fn backend.CompareFunc, ref, buf uint8) bool {
	switch fn {
	case backend.CompareNever:
		return false
	case backend.CompareLess:
		return ref < buf
	case backend.CompareLessEqual:
		return ref <= buf
	case backend.CompareEqual:
		return ref == buf
	case backend.CompareGreaterEqual:
		return ref >= buf
	case backend.CompareGreater:
		return ref > buf
	case backend.CompareNotEqual:
		return ref != buf
	default: // CompareAlways
		return true
	}
}

// stencilApplyOp computes the new stencil value for a single op. Does not
// apply the write mask — callers merge via (current & ^writeMask) | (new &
// writeMask). Wrap variants rely on uint8 wrap-around arithmetic.
func stencilApplyOp(op backend.StencilOp, ref, current uint8) uint8 {
	switch op {
	case backend.StencilZero:
		return 0
	case backend.StencilReplace:
		return ref
	case backend.StencilIncr:
		if current < 0xFF {
			return current + 1
		}
		return current
	case backend.StencilDecr:
		if current > 0 {
			return current - 1
		}
		return current
	case backend.StencilInvert:
		return ^current
	case backend.StencilIncrWrap:
		return current + 1
	case backend.StencilDecrWrap:
		return current - 1
	default: // StencilKeep
		return current
	}
}

// applyColorMatrix applies the 4x4 color body matrix and translation vector.
//
// Callers MUST pass isIdentity=true when body is the identity matrix and
// trans is the zero vector. The identity check itself is cheap at call
// sites (once per triangle/draw) but pathological per-pixel — profiling
// scene-selector found this function consuming ~20% of CPU with the
// whole-array [16]float32 compare dominating, despite identity being the
// overwhelmingly common case. Hoisting the check out of the inner
// rasterization loop cut the overhead to near zero.
func applyColorMatrix(r, g, b, a float32, body [16]float32, trans [4]float32, isIdentity bool) (or, og, ob, oa float32) {
	if isIdentity {
		return r, g, b, a
	}
	// Column-major: body[col*4+row]
	or = body[0]*r + body[4]*g + body[8]*b + body[12]*a + trans[0]
	og = body[1]*r + body[5]*g + body[9]*b + body[13]*a + trans[1]
	ob = body[2]*r + body[6]*g + body[10]*b + body[14]*a + trans[2]
	oa = body[3]*r + body[7]*g + body[11]*b + body[15]*a + trans[3]
	return clampf(or), clampf(og), clampf(ob), clampf(oa)
}

// isIdentityMatrix checks if a [16]float32 is the identity matrix.
// Call this ONCE per draw/triangle at the outermost sensible layer and
// thread the result through as a bool.
//
// Implemented as explicit element compares rather than `m == [16]float32{...}`
// because Go generates a runtime-called `type:.eq.[16]float32` helper for
// the latter that can't short-circuit, and it dominated the profile
// even once per-pixel calls were eliminated. With short-circuiting,
// non-identity inputs (mid-scale + translate is common for transforms)
// return after the first failing diagonal check.
func isIdentityMatrix(m [16]float32) bool {
	return m[0] == 1 && m[5] == 1 && m[10] == 1 && m[15] == 1 &&
		m[1] == 0 && m[2] == 0 && m[3] == 0 &&
		m[4] == 0 && m[6] == 0 && m[7] == 0 &&
		m[8] == 0 && m[9] == 0 && m[11] == 0 &&
		m[12] == 0 && m[13] == 0 && m[14] == 0
}

// writePixel blends and writes a fragment to the color buffer.
func (r *rasterizer) writePixel(x, y int, sr, sg, sb, sa float32) {
	idx := (y*r.width + x) * r.bpp
	if idx+3 >= len(r.colorBuf) {
		return
	}

	if r.blend != nil {
		// Read existing pixel.
		dr := float32(r.colorBuf[idx]) / 255.0
		dg := float32(r.colorBuf[idx+1]) / 255.0
		db := float32(r.colorBuf[idx+2]) / 255.0
		da := float32(r.colorBuf[idx+3]) / 255.0

		sr, sg, sb, sa = r.blend(sr, sg, sb, sa, dr, dg, db, da)
	}

	r.colorBuf[idx] = floatToByte(sr)
	r.colorBuf[idx+1] = floatToByte(sg)
	r.colorBuf[idx+2] = floatToByte(sb)
	r.colorBuf[idx+3] = floatToByte(sa)
}

// --- Blend functions ---

func blendNone(sr, sg, sb, sa, _, _, _, _ float32) (or, og, ob, oa float32) {
	return sr, sg, sb, sa
}

func blendSourceOver(sr, sg, sb, sa, dr, dg, db, da float32) (or, og, ob, oa float32) {
	oa = sa + da*(1-sa)
	if oa == 0 {
		return 0, 0, 0, 0
	}
	or = (sr*sa + dr*da*(1-sa)) / oa
	og = (sg*sa + dg*da*(1-sa)) / oa
	ob = (sb*sa + db*da*(1-sa)) / oa
	return or, og, ob, oa
}

func blendAdditive(sr, sg, sb, sa, dr, dg, db, da float32) (or, og, ob, oa float32) {
	return clampf(dr + sr*sa), clampf(dg + sg*sa), clampf(db + sb*sa), clampf(da + sa)
}

func blendMultiplicative(sr, sg, sb, _, dr, dg, db, da float32) (or, og, ob, oa float32) {
	return dr * sr, dg * sg, db * sb, da
}

func blendPremultiplied(sr, sg, sb, sa, dr, dg, db, da float32) (or, og, ob, oa float32) {
	return clampf(sr + dr*(1-sa)), clampf(sg + dg*(1-sa)), clampf(sb + db*(1-sa)), clampf(sa + da*(1-sa))
}

// --- Texture sampling ---

// sampleNearest returns the texel at the nearest integer coordinate.
func sampleNearest(pixels []byte, w, h, bpp int, u, v float32) (cr, cg, cb, ca float32) {
	if w <= 0 || h <= 0 || len(pixels) < bpp {
		return 0, 0, 0, 0
	}
	// Clamp to [0, 1].
	u = clampf(u)
	v = clampf(v)

	x := int(math.Round(float64(u) * float64(w-1)))
	y := int(math.Round(float64(v) * float64(h-1)))
	if x >= w {
		x = w - 1
	}
	if y >= h {
		y = h - 1
	}

	idx := (y*w + x) * bpp
	if idx+3 >= len(pixels) {
		return 0, 0, 0, 0
	}
	return float32(pixels[idx]) / 255, float32(pixels[idx+1]) / 255,
		float32(pixels[idx+2]) / 255, float32(pixels[idx+3]) / 255
}

// sampleLinear returns bilinearly interpolated texel.
func sampleLinear(pixels []byte, w, h, bpp int, u, v float32) (cr, cg, cb, ca float32) {
	if w <= 0 || h <= 0 || len(pixels) < bpp {
		return 0, 0, 0, 0
	}
	u = clampf(u)
	v = clampf(v)

	fx := u * float32(w-1)
	fy := v * float32(h-1)

	x0 := int(fx)
	y0 := int(fy)
	x1 := x0 + 1
	y1 := y0 + 1

	if x1 >= w {
		x1 = w - 1
	}
	if y1 >= h {
		y1 = h - 1
	}

	dx := fx - float32(x0)
	dy := fy - float32(y0)

	r00, g00, b00, a00 := texel(pixels, w, bpp, x0, y0)
	r10, g10, b10, a10 := texel(pixels, w, bpp, x1, y0)
	r01, g01, b01, a01 := texel(pixels, w, bpp, x0, y1)
	r11, g11, b11, a11 := texel(pixels, w, bpp, x1, y1)

	r := bilerp(r00, r10, r01, r11, dx, dy)
	g := bilerp(g00, g10, g01, g11, dx, dy)
	b := bilerp(b00, b10, b01, b11, dx, dy)
	a := bilerp(a00, a10, a01, a11, dx, dy)

	return r, g, b, a
}

func texel(pixels []byte, w, bpp, x, y int) (cr, cg, cb, ca float32) {
	idx := (y*w + x) * bpp
	if idx+3 >= len(pixels) {
		return 0, 0, 0, 0
	}
	return float32(pixels[idx]) / 255, float32(pixels[idx+1]) / 255,
		float32(pixels[idx+2]) / 255, float32(pixels[idx+3]) / 255
}

func bilerp(v00, v10, v01, v11, dx, dy float32) float32 {
	top := v00*(1-dx) + v10*dx
	bot := v01*(1-dx) + v11*dx
	return top*(1-dy) + bot*dy
}

// --- Helpers ---

// edgeFuncFMA computes the edge function using math.FMA for cross-platform
// determinism. The result is (bx-ax)*(cy-ay) - (by-ay)*(cx-ax).
// Both terms use FMA to ensure symmetric rounding at shared triangle edges,
// preventing gaps or double-draws between adjacent triangles.
func edgeFuncFMA(ax, ay, bx, by, cx, cy float64) float64 {
	dx1, dy1 := bx-ax, cy-ay
	dx2, dy2 := by-ay, cx-ax
	// Compute dx2*dy2 via FMA (with 0 addend) for symmetric rounding,
	// then negate and fuse with dx1*dy1.
	return math.FMA(dx1, dy1, -math.FMA(dx2, dy2, 0))
}

func edgeFunc(ax, ay, bx, by, cx, cy float32) float32 {
	return (bx-ax)*(cy-ay) - (by-ay)*(cx-ax)
}

func min3(a, b, c float64) float64 {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func max3(a, b, c float64) float64 {
	if b > a {
		a = b
	}
	if c > a {
		a = c
	}
	return a
}

func min3f(a, b, c float32) float32 {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func max3f(a, b, c float32) float32 {
	if b > a {
		a = b
	}
	if c > a {
		a = c
	}
	return a
}

func clampf(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func floatToByte(f float32) byte {
	if f <= 0 {
		return 0
	}
	if f >= 1 {
		return 255
	}
	return byte(f*255 + 0.5)
}
