package soft

import (
	"encoding/binary"
	"math"
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
func ndcToScreen(ndcX, ndcY float32, vp viewportRect) (sx, sy float32) {
	sx = float32(vp.x) + (ndcX+1)*0.5*float32(vp.w)
	sy = float32(vp.y) + (ndcY+1)*0.5*float32(vp.h)
	return sx, sy
}

// rasterizeTriangle rasterizes a single triangle using the half-space method.
// It calls emit for each fragment that passes the depth test.
func (r *rasterizer) rasterizeTriangle(
	v0, v1, v2 vertex2D,
	proj [16]float32,
	texSampler func(u, v float32) (float32, float32, float32, float32),
	colorBody [16]float32,
	colorTranslation [4]float32,
) {
	// Transform vertices to clip space.
	x0, y0, z0, w0 := transformVertex(v0, proj)
	x1, y1, z1, w1 := transformVertex(v1, proj)
	x2, y2, z2, w2 := transformVertex(v2, proj)

	// Perspective divide → NDC.
	if w0 == 0 || w1 == 0 || w2 == 0 {
		return
	}
	nx0, ny0, nz0 := x0/w0, y0/w0, z0/w0
	nx1, ny1, nz1 := x1/w1, y1/w1, z1/w1
	nx2, ny2, nz2 := x2/w2, y2/w2, z2/w2

	// NDC → screen.
	sx0, sy0 := ndcToScreen(nx0, ny0, r.viewport)
	sx1, sy1 := ndcToScreen(nx1, ny1, r.viewport)
	sx2, sy2 := ndcToScreen(nx2, ny2, r.viewport)

	// Bounding box (clamped to framebuffer).
	minX := int(math.Floor(float64(min3f(sx0, sx1, sx2))))
	maxX := int(math.Ceil(float64(max3f(sx0, sx1, sx2))))
	minY := int(math.Floor(float64(min3f(sy0, sy1, sy2))))
	maxY := int(math.Ceil(float64(max3f(sy0, sy1, sy2))))

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
	denom := edgeFunc(sx0, sy0, sx1, sy1, sx2, sy2)
	if denom == 0 {
		return // degenerate triangle
	}
	invDenom := 1.0 / denom

	// Rasterize: iterate over bounding box pixels.
	for py := minY; py < maxY; py++ {
		for px := minX; px < maxX; px++ {
			// Sample at pixel center.
			cx := float32(px) + 0.5
			cy := float32(py) + 0.5

			// Barycentric coordinates.
			w0f := edgeFunc(sx1, sy1, sx2, sy2, cx, cy) * invDenom
			w1f := edgeFunc(sx2, sy2, sx0, sy0, cx, cy) * invDenom
			w2f := edgeFunc(sx0, sy0, sx1, sy1, cx, cy) * invDenom

			// Inside triangle test.
			if w0f < 0 || w1f < 0 || w2f < 0 {
				continue
			}

			// Interpolate depth.
			depth := w0f*nz0 + w1f*nz1 + w2f*nz2

			// Depth test.
			if r.depthTest {
				idx := py*r.width + px
				if idx < len(r.depthBuf) && depth > r.depthBuf[idx] {
					continue
				}
				if r.depthWrite && idx < len(r.depthBuf) {
					r.depthBuf[idx] = depth
				}
			}

			// Interpolate texcoords.
			u := w0f*v0.tu + w1f*v1.tu + w2f*v2.tu
			v := w0f*v0.tv + w1f*v1.tv + w2f*v2.tv

			// Interpolate vertex color.
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
			fr, fg, fb, fa = applyColorMatrix(fr, fg, fb, fa, colorBody, colorTranslation)

			// Write fragment.
			if r.colorWrite {
				r.writePixel(px, py, fr, fg, fb, fa)
			}
		}
	}
}

// applyColorMatrix applies the 4x4 color body matrix and translation vector.
func applyColorMatrix(r, g, b, a float32, body [16]float32, trans [4]float32) (or, og, ob, oa float32) {
	// Check if identity (optimization for common case).
	if isIdentityMatrix(body) && trans == [4]float32{} {
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
func isIdentityMatrix(m [16]float32) bool {
	return m == [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
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

	x := int(u * float32(w-1))
	y := int(v * float32(h-1))
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

func edgeFunc(ax, ay, bx, by, cx, cy float32) float32 {
	return (bx-ax)*(cy-ay) - (by-ay)*(cx-ax)
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
