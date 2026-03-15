// Package conformance provides a golden-image integration testing framework
// for backend.Device implementations. It renders a set of canonical test
// scenes through any backend and compares the resulting pixel buffers against
// reference images produced by the software rasterizer.
//
// Usage in backend tests:
//
//	func TestConformance(t *testing.T) {
//	    dev := mybackend.New()
//	    conformance.RunAll(t, dev)
//	}
//
// Each test scene renders geometry into a render target using the backend's
// full pipeline (Device → Buffer → Texture → Shader → Pipeline → Encoder →
// DrawIndexed), then reads back pixels and compares against the golden
// reference.
package conformance

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/backend"
)

// Tolerance is the maximum per-channel difference allowed between actual
// and expected pixel values (0–255 scale). Accounts for floating-point
// rounding differences between CPU and GPU rasterizers.
const Tolerance = 3

// SceneSize is the width and height of conformance test render targets.
const SceneSize = 64

// Scene describes a test scene that can be rendered by any backend.
type Scene struct {
	Name        string
	Description string
	Render      func(t *testing.T, ctx *RenderContext)
}

// RenderContext provides the resources needed to render a scene.
type RenderContext struct {
	Device  backend.Device
	Target  backend.RenderTarget
	Encoder backend.CommandEncoder
	Width   int
	Height  int
}

// Result holds the pixel output of a rendered scene.
type Result struct {
	Pixels []byte
	Width  int
	Height int
}

// CompareResult describes the outcome of comparing two pixel buffers.
type CompareResult struct {
	Match         bool
	MaxDiff       int
	MismatchCount int
	TotalPixels   int
}

// Scenes returns the canonical set of conformance test scenes.
func Scenes() []Scene {
	return []Scene{
		sceneClearRed(),
		sceneClearGreen(),
		sceneTriangleRed(),
		sceneTriangleVertexColors(),
		sceneTexturedQuad(),
		sceneBlendSourceOver(),
		sceneBlendAdditive(),
		sceneScissorRect(),
		sceneOrthoProjection(),
		sceneMultipleTriangles(),
	}
}

// RunAll runs all conformance test scenes against the given device.
func RunAll(t *testing.T, dev backend.Device, enc backend.CommandEncoder) {
	t.Helper()
	for _, scene := range Scenes() {
		t.Run(scene.Name, func(t *testing.T) {
			RunScene(t, dev, enc, scene)
		})
	}
}

// RunScene renders a single scene and compares against the golden reference.
func RunScene(t *testing.T, dev backend.Device, enc backend.CommandEncoder, scene Scene) {
	t.Helper()

	// Create render target.
	rt, err := dev.NewRenderTarget(backend.RenderTargetDescriptor{
		Width:       SceneSize,
		Height:      SceneSize,
		ColorFormat: backend.TextureFormatRGBA8,
	})
	require.NoError(t, err)
	defer rt.Dispose()

	ctx := &RenderContext{
		Device:  dev,
		Target:  rt,
		Encoder: enc,
		Width:   SceneSize,
		Height:  SceneSize,
	}

	// Render the scene.
	scene.Render(t, ctx)

	// Read back pixels.
	actual := readPixels(t, rt)

	// Load or generate golden reference.
	golden := loadOrGenerateGolden(t, scene.Name, actual)

	// Compare.
	result := ComparePixels(actual.Pixels, golden.Pixels, actual.Width, actual.Height, Tolerance)
	if !result.Match {
		// Save actual and diff images for debugging.
		saveDiffArtifacts(t, scene.Name, actual, golden)
		require.Failf(t, "pixel mismatch", "scene %q: max diff %d, %d/%d pixels differ (tolerance %d)",
			scene.Name, result.MaxDiff, result.MismatchCount, result.TotalPixels, Tolerance)
	}
}

// ComparePixels compares two RGBA pixel buffers with a per-channel tolerance.
func ComparePixels(actual, expected []byte, width, height, tolerance int) CompareResult {
	total := width * height
	result := CompareResult{
		Match:       true,
		TotalPixels: total,
	}

	minLen := len(actual)
	if len(expected) < minLen {
		minLen = len(expected)
	}

	for i := 0; i+3 < minLen; i += 4 {
		for c := range 4 {
			diff := absDiff(actual[i+c], expected[i+c])
			if diff > result.MaxDiff {
				result.MaxDiff = diff
			}
			if diff > tolerance {
				result.Match = false
				result.MismatchCount++
				break // count pixel once
			}
		}
	}

	// Check for size mismatch.
	if len(actual) != len(expected) {
		result.Match = false
	}

	return result
}

// --- Golden image management ---

// GoldenDir returns the directory for golden images. Defaults to
// testdata/golden/ relative to the conformance package.
func GoldenDir() string {
	return filepath.Join("testdata", "golden")
}

func goldenPath(sceneName string) string {
	return filepath.Join(GoldenDir(), sceneName+".png")
}

func loadOrGenerateGolden(t *testing.T, sceneName string, actual *Result) *Result {
	t.Helper()

	path := goldenPath(sceneName)
	data, err := os.ReadFile(path)
	if err == nil {
		return decodeGoldenPNG(t, data)
	}

	// Golden doesn't exist — generate it from actual (first run).
	if os.Getenv("CONFORMANCE_UPDATE_GOLDEN") == "1" {
		saveGoldenPNG(t, path, actual)
		t.Logf("generated golden image: %s", path)
		return actual
	}

	// Auto-generate on first run.
	if os.IsNotExist(err) {
		err = os.MkdirAll(filepath.Dir(path), 0o755)
		require.NoError(t, err, "creating golden dir")
		saveGoldenPNG(t, path, actual)
		t.Logf("generated golden image: %s (first run)", path)
		return actual
	}

	require.NoError(t, err, "reading golden image %s", path)
	return nil
}

func decodeGoldenPNG(t *testing.T, data []byte) *Result {
	t.Helper()
	f, err := png.Decode(bytesReader(data))
	require.NoError(t, err)

	bounds := f.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	pixels := make([]byte, w*h*4)
	for y := range h {
		for x := range w {
			r, g, b, a := f.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			off := (y*w + x) * 4
			pixels[off] = byte(r >> 8)
			pixels[off+1] = byte(g >> 8)
			pixels[off+2] = byte(b >> 8)
			pixels[off+3] = byte(a >> 8)
		}
	}
	return &Result{Pixels: pixels, Width: w, Height: h}
}

func saveGoldenPNG(t *testing.T, path string, r *Result) {
	t.Helper()
	img := resultToImage(r)
	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	require.NoError(t, png.Encode(f, img))
}

func saveDiffArtifacts(t *testing.T, sceneName string, actual, golden *Result) {
	t.Helper()
	dir := filepath.Join(GoldenDir(), "diff")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("warning: cannot create diff dir: %v", err)
		return
	}

	// Save actual.
	actualPath := filepath.Join(dir, sceneName+"_actual.png")
	saveGoldenPNG(t, actualPath, actual)

	// Save diff visualization.
	diffPath := filepath.Join(dir, sceneName+"_diff.png")
	diffImg := createDiffImage(actual, golden)
	f, err := os.Create(diffPath)
	if err != nil {
		t.Logf("warning: cannot create diff image: %v", err)
		return
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			t.Logf("warning: cannot close diff image: %v", cerr)
		}
	}()
	if err := png.Encode(f, diffImg); err != nil {
		t.Logf("warning: cannot encode diff image: %v", err)
	}
	t.Logf("diff artifacts saved to %s", dir)
}

func createDiffImage(actual, golden *Result) *image.RGBA {
	w, h := actual.Width, actual.Height
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			off := (y*w + x) * 4
			if off+3 >= len(actual.Pixels) || off+3 >= len(golden.Pixels) {
				continue
			}
			dr := absDiff(actual.Pixels[off], golden.Pixels[off])
			dg := absDiff(actual.Pixels[off+1], golden.Pixels[off+1])
			db := absDiff(actual.Pixels[off+2], golden.Pixels[off+2])
			// Scale diff for visibility.
			scale := 4
			img.SetRGBA(x, y, color.RGBA{
				R: clampByte255(dr * scale),
				G: clampByte255(dg * scale),
				B: clampByte255(db * scale),
				A: 255,
			})
		}
	}
	return img
}

// --- Scene definitions ---

func sceneClearRed() Scene {
	return Scene{
		Name:        "clear_red",
		Description: "Clear render target to solid red",
		Render: func(t *testing.T, ctx *RenderContext) {
			t.Helper()
			ctx.Encoder.BeginRenderPass(backend.RenderPassDescriptor{
				Target:     ctx.Target,
				LoadAction: backend.LoadActionClear,
				ClearColor: [4]float32{1, 0, 0, 1},
			})
			ctx.Encoder.EndRenderPass()
		},
	}
}

func sceneClearGreen() Scene {
	return Scene{
		Name:        "clear_green",
		Description: "Clear render target to solid green",
		Render: func(t *testing.T, ctx *RenderContext) {
			t.Helper()
			ctx.Encoder.BeginRenderPass(backend.RenderPassDescriptor{
				Target:     ctx.Target,
				LoadAction: backend.LoadActionClear,
				ClearColor: [4]float32{0, 1, 0, 1},
			})
			ctx.Encoder.EndRenderPass()
		},
	}
}

func sceneTriangleRed() Scene {
	return Scene{
		Name:        "triangle_red",
		Description: "Solid red triangle on black background",
		Render: func(t *testing.T, ctx *RenderContext) {
			t.Helper()
			whiteTex := newWhiteTexture(t, ctx.Device)
			defer whiteTex.Dispose()

			shader, pipeline := newBasicPipeline(t, ctx.Device, backend.BlendNone)
			defer shader.Dispose()
			defer pipeline.Dispose()

			verts := packVertices(
				vtx(-0.5, -0.5, 0, 0, 1, 0, 0, 1),
				vtx(0.5, -0.5, 0, 0, 1, 0, 0, 1),
				vtx(0, 0.5, 0, 0, 1, 0, 0, 1),
			)
			indices := packIndices(0, 1, 2)

			vbuf := newBuffer(t, ctx.Device, verts)
			defer vbuf.Dispose()
			ibuf := newBuffer(t, ctx.Device, indices)
			defer ibuf.Dispose()

			ctx.Encoder.BeginRenderPass(backend.RenderPassDescriptor{
				Target:     ctx.Target,
				LoadAction: backend.LoadActionClear,
				ClearColor: [4]float32{0, 0, 0, 1},
			})
			ctx.Encoder.SetPipeline(pipeline)
			ctx.Encoder.SetVertexBuffer(vbuf, 0)
			ctx.Encoder.SetIndexBuffer(ibuf, backend.IndexUint16)
			ctx.Encoder.SetTexture(whiteTex, 0)
			ctx.Encoder.DrawIndexed(3, 1, 0)
			ctx.Encoder.EndRenderPass()
		},
	}
}

func sceneTriangleVertexColors() Scene {
	return Scene{
		Name:        "triangle_vertex_colors",
		Description: "Triangle with red/green/blue vertex colors",
		Render: func(t *testing.T, ctx *RenderContext) {
			t.Helper()
			whiteTex := newWhiteTexture(t, ctx.Device)
			defer whiteTex.Dispose()

			shader, pipeline := newBasicPipeline(t, ctx.Device, backend.BlendNone)
			defer shader.Dispose()
			defer pipeline.Dispose()

			verts := packVertices(
				vtx(-0.8, -0.8, 0, 0, 1, 0, 0, 1), // red
				vtx(0.8, -0.8, 0, 0, 0, 1, 0, 1),  // green
				vtx(0, 0.8, 0, 0, 0, 0, 1, 1),     // blue
			)
			indices := packIndices(0, 1, 2)

			vbuf := newBuffer(t, ctx.Device, verts)
			defer vbuf.Dispose()
			ibuf := newBuffer(t, ctx.Device, indices)
			defer ibuf.Dispose()

			ctx.Encoder.BeginRenderPass(backend.RenderPassDescriptor{
				Target:     ctx.Target,
				LoadAction: backend.LoadActionClear,
				ClearColor: [4]float32{0, 0, 0, 1},
			})
			ctx.Encoder.SetPipeline(pipeline)
			ctx.Encoder.SetVertexBuffer(vbuf, 0)
			ctx.Encoder.SetIndexBuffer(ibuf, backend.IndexUint16)
			ctx.Encoder.SetTexture(whiteTex, 0)
			ctx.Encoder.DrawIndexed(3, 1, 0)
			ctx.Encoder.EndRenderPass()
		},
	}
}

func sceneTexturedQuad() Scene {
	return Scene{
		Name:        "textured_quad",
		Description: "Quad with 4x4 checkerboard texture",
		Render: func(t *testing.T, ctx *RenderContext) {
			t.Helper()
			checker := newCheckerTexture(t, ctx.Device, 4, 4)
			defer checker.Dispose()

			shader, pipeline := newBasicPipeline(t, ctx.Device, backend.BlendNone)
			defer shader.Dispose()
			defer pipeline.Dispose()

			// Two triangles forming a quad.
			verts := packVertices(
				vtx(-0.8, -0.8, 0, 0, 1, 1, 1, 1),
				vtx(0.8, -0.8, 1, 0, 1, 1, 1, 1),
				vtx(0.8, 0.8, 1, 1, 1, 1, 1, 1),
				vtx(-0.8, 0.8, 0, 1, 1, 1, 1, 1),
			)
			indices := packIndices(0, 1, 2, 0, 2, 3)

			vbuf := newBuffer(t, ctx.Device, verts)
			defer vbuf.Dispose()
			ibuf := newBuffer(t, ctx.Device, indices)
			defer ibuf.Dispose()

			ctx.Encoder.BeginRenderPass(backend.RenderPassDescriptor{
				Target:     ctx.Target,
				LoadAction: backend.LoadActionClear,
				ClearColor: [4]float32{0, 0, 0, 1},
			})
			ctx.Encoder.SetPipeline(pipeline)
			ctx.Encoder.SetVertexBuffer(vbuf, 0)
			ctx.Encoder.SetIndexBuffer(ibuf, backend.IndexUint16)
			ctx.Encoder.SetTexture(checker, 0)
			ctx.Encoder.DrawIndexed(6, 1, 0)
			ctx.Encoder.EndRenderPass()
		},
	}
}

func sceneBlendSourceOver() Scene {
	return Scene{
		Name:        "blend_source_over",
		Description: "Semi-transparent green triangle over red background",
		Render: func(t *testing.T, ctx *RenderContext) {
			t.Helper()
			whiteTex := newWhiteTexture(t, ctx.Device)
			defer whiteTex.Dispose()

			shader, pipeline := newBasicPipeline(t, ctx.Device, backend.BlendSourceOver)
			defer shader.Dispose()
			defer pipeline.Dispose()

			// Red background quad.
			bg := packVertices(
				vtx(-1, -1, 0, 0, 1, 0, 0, 1),
				vtx(1, -1, 0, 0, 1, 0, 0, 1),
				vtx(1, 1, 0, 0, 1, 0, 0, 1),
				vtx(-1, 1, 0, 0, 1, 0, 0, 1),
			)
			bgIdx := packIndices(0, 1, 2, 0, 2, 3)
			bgVbuf := newBuffer(t, ctx.Device, bg)
			defer bgVbuf.Dispose()
			bgIbuf := newBuffer(t, ctx.Device, bgIdx)
			defer bgIbuf.Dispose()

			// Semi-transparent green triangle.
			fg := packVertices(
				vtx(-0.5, -0.5, 0, 0, 0, 1, 0, 0.5),
				vtx(0.5, -0.5, 0, 0, 0, 1, 0, 0.5),
				vtx(0, 0.5, 0, 0, 0, 1, 0, 0.5),
			)
			fgIdx := packIndices(0, 1, 2)
			fgVbuf := newBuffer(t, ctx.Device, fg)
			defer fgVbuf.Dispose()
			fgIbuf := newBuffer(t, ctx.Device, fgIdx)
			defer fgIbuf.Dispose()

			ctx.Encoder.BeginRenderPass(backend.RenderPassDescriptor{
				Target:     ctx.Target,
				LoadAction: backend.LoadActionClear,
				ClearColor: [4]float32{0, 0, 0, 0},
			})
			ctx.Encoder.SetPipeline(pipeline)

			// Draw red background.
			ctx.Encoder.SetVertexBuffer(bgVbuf, 0)
			ctx.Encoder.SetIndexBuffer(bgIbuf, backend.IndexUint16)
			ctx.Encoder.SetTexture(whiteTex, 0)
			ctx.Encoder.DrawIndexed(6, 1, 0)

			// Draw semi-transparent green.
			ctx.Encoder.SetVertexBuffer(fgVbuf, 0)
			ctx.Encoder.SetIndexBuffer(fgIbuf, backend.IndexUint16)
			ctx.Encoder.DrawIndexed(3, 1, 0)

			ctx.Encoder.EndRenderPass()
		},
	}
}

func sceneBlendAdditive() Scene {
	return Scene{
		Name:        "blend_additive",
		Description: "Additive blend: blue triangle over red background",
		Render: func(t *testing.T, ctx *RenderContext) {
			t.Helper()
			whiteTex := newWhiteTexture(t, ctx.Device)
			defer whiteTex.Dispose()

			// Red background with BlendNone.
			shaderBG, pipelineBG := newBasicPipeline(t, ctx.Device, backend.BlendNone)
			defer shaderBG.Dispose()
			defer pipelineBG.Dispose()

			// Additive foreground.
			shaderFG, pipelineFG := newBasicPipeline(t, ctx.Device, backend.BlendAdditive)
			defer shaderFG.Dispose()
			defer pipelineFG.Dispose()

			bg := packVertices(
				vtx(-1, -1, 0, 0, 0.5, 0, 0, 1),
				vtx(1, -1, 0, 0, 0.5, 0, 0, 1),
				vtx(1, 1, 0, 0, 0.5, 0, 0, 1),
				vtx(-1, 1, 0, 0, 0.5, 0, 0, 1),
			)
			bgIdx := packIndices(0, 1, 2, 0, 2, 3)
			bgVbuf := newBuffer(t, ctx.Device, bg)
			defer bgVbuf.Dispose()
			bgIbuf := newBuffer(t, ctx.Device, bgIdx)
			defer bgIbuf.Dispose()

			fg := packVertices(
				vtx(-0.5, -0.5, 0, 0, 0, 0, 0.5, 1),
				vtx(0.5, -0.5, 0, 0, 0, 0, 0.5, 1),
				vtx(0, 0.5, 0, 0, 0, 0, 0.5, 1),
			)
			fgIdx := packIndices(0, 1, 2)
			fgVbuf := newBuffer(t, ctx.Device, fg)
			defer fgVbuf.Dispose()
			fgIbuf := newBuffer(t, ctx.Device, fgIdx)
			defer fgIbuf.Dispose()

			ctx.Encoder.BeginRenderPass(backend.RenderPassDescriptor{
				Target:     ctx.Target,
				LoadAction: backend.LoadActionClear,
				ClearColor: [4]float32{0, 0, 0, 1},
			})

			ctx.Encoder.SetPipeline(pipelineBG)
			ctx.Encoder.SetVertexBuffer(bgVbuf, 0)
			ctx.Encoder.SetIndexBuffer(bgIbuf, backend.IndexUint16)
			ctx.Encoder.SetTexture(whiteTex, 0)
			ctx.Encoder.DrawIndexed(6, 1, 0)

			ctx.Encoder.SetPipeline(pipelineFG)
			ctx.Encoder.SetVertexBuffer(fgVbuf, 0)
			ctx.Encoder.SetIndexBuffer(fgIbuf, backend.IndexUint16)
			ctx.Encoder.DrawIndexed(3, 1, 0)

			ctx.Encoder.EndRenderPass()
		},
	}
}

func sceneScissorRect() Scene {
	return Scene{
		Name:        "scissor_rect",
		Description: "Full-screen white quad scissored to center quadrant",
		Render: func(t *testing.T, ctx *RenderContext) {
			t.Helper()
			whiteTex := newWhiteTexture(t, ctx.Device)
			defer whiteTex.Dispose()

			shader, pipeline := newBasicPipeline(t, ctx.Device, backend.BlendNone)
			defer shader.Dispose()
			defer pipeline.Dispose()

			verts := packVertices(
				vtx(-1, -1, 0, 0, 1, 1, 1, 1),
				vtx(1, -1, 0, 0, 1, 1, 1, 1),
				vtx(1, 1, 0, 0, 1, 1, 1, 1),
				vtx(-1, 1, 0, 0, 1, 1, 1, 1),
			)
			indices := packIndices(0, 1, 2, 0, 2, 3)

			vbuf := newBuffer(t, ctx.Device, verts)
			defer vbuf.Dispose()
			ibuf := newBuffer(t, ctx.Device, indices)
			defer ibuf.Dispose()

			q := SceneSize / 4
			ctx.Encoder.BeginRenderPass(backend.RenderPassDescriptor{
				Target:     ctx.Target,
				LoadAction: backend.LoadActionClear,
				ClearColor: [4]float32{0, 0, 0, 1},
			})
			ctx.Encoder.SetPipeline(pipeline)
			ctx.Encoder.SetVertexBuffer(vbuf, 0)
			ctx.Encoder.SetIndexBuffer(ibuf, backend.IndexUint16)
			ctx.Encoder.SetTexture(whiteTex, 0)
			ctx.Encoder.SetScissor(&backend.ScissorRect{X: q, Y: q, Width: q * 2, Height: q * 2})
			ctx.Encoder.DrawIndexed(6, 1, 0)
			ctx.Encoder.SetScissor(nil)
			ctx.Encoder.EndRenderPass()
		},
	}
}

func sceneOrthoProjection() Scene {
	return Scene{
		Name:        "ortho_projection",
		Description: "Triangle with orthographic projection (pixel coordinates)",
		Render: func(t *testing.T, ctx *RenderContext) {
			t.Helper()
			whiteTex := newWhiteTexture(t, ctx.Device)
			defer whiteTex.Dispose()

			shader, pipeline := newBasicPipeline(t, ctx.Device, backend.BlendNone)
			defer shader.Dispose()
			defer pipeline.Dispose()

			// Set ortho projection: [0, SceneSize] → NDC
			s := float32(SceneSize)
			shader.SetUniformMat4("uProjection", orthoMatrix(0, s, 0, s))

			// Triangle in pixel coordinates.
			verts := packVertices(
				vtx(16, 16, 0, 0, 1, 1, 0, 1),
				vtx(48, 16, 0, 0, 1, 1, 0, 1),
				vtx(32, 48, 0, 0, 1, 1, 0, 1),
			)
			indices := packIndices(0, 1, 2)

			vbuf := newBuffer(t, ctx.Device, verts)
			defer vbuf.Dispose()
			ibuf := newBuffer(t, ctx.Device, indices)
			defer ibuf.Dispose()

			ctx.Encoder.BeginRenderPass(backend.RenderPassDescriptor{
				Target:     ctx.Target,
				LoadAction: backend.LoadActionClear,
				ClearColor: [4]float32{0, 0, 0, 1},
			})
			ctx.Encoder.SetPipeline(pipeline)
			ctx.Encoder.SetVertexBuffer(vbuf, 0)
			ctx.Encoder.SetIndexBuffer(ibuf, backend.IndexUint16)
			ctx.Encoder.SetTexture(whiteTex, 0)
			ctx.Encoder.DrawIndexed(3, 1, 0)
			ctx.Encoder.EndRenderPass()
		},
	}
}

func sceneMultipleTriangles() Scene {
	return Scene{
		Name:        "multiple_triangles",
		Description: "Four colored triangles in each quadrant",
		Render: func(t *testing.T, ctx *RenderContext) {
			t.Helper()
			whiteTex := newWhiteTexture(t, ctx.Device)
			defer whiteTex.Dispose()

			shader, pipeline := newBasicPipeline(t, ctx.Device, backend.BlendNone)
			defer shader.Dispose()
			defer pipeline.Dispose()

			// Four small triangles in each quadrant.
			verts := packVertices(
				// Top-left (red)
				vtx(-0.9, 0.1, 0, 0, 1, 0, 0, 1),
				vtx(-0.1, 0.1, 0, 0, 1, 0, 0, 1),
				vtx(-0.5, 0.9, 0, 0, 1, 0, 0, 1),
				// Top-right (green)
				vtx(0.1, 0.1, 0, 0, 0, 1, 0, 1),
				vtx(0.9, 0.1, 0, 0, 0, 1, 0, 1),
				vtx(0.5, 0.9, 0, 0, 0, 1, 0, 1),
				// Bottom-left (blue)
				vtx(-0.9, -0.9, 0, 0, 0, 0, 1, 1),
				vtx(-0.1, -0.9, 0, 0, 0, 0, 1, 1),
				vtx(-0.5, -0.1, 0, 0, 0, 0, 1, 1),
				// Bottom-right (yellow)
				vtx(0.1, -0.9, 0, 0, 1, 1, 0, 1),
				vtx(0.9, -0.9, 0, 0, 1, 1, 0, 1),
				vtx(0.5, -0.1, 0, 0, 1, 1, 0, 1),
			)
			indices := packIndices(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11)

			vbuf := newBuffer(t, ctx.Device, verts)
			defer vbuf.Dispose()
			ibuf := newBuffer(t, ctx.Device, indices)
			defer ibuf.Dispose()

			ctx.Encoder.BeginRenderPass(backend.RenderPassDescriptor{
				Target:     ctx.Target,
				LoadAction: backend.LoadActionClear,
				ClearColor: [4]float32{0, 0, 0, 1},
			})
			ctx.Encoder.SetPipeline(pipeline)
			ctx.Encoder.SetVertexBuffer(vbuf, 0)
			ctx.Encoder.SetIndexBuffer(ibuf, backend.IndexUint16)
			ctx.Encoder.SetTexture(whiteTex, 0)
			ctx.Encoder.DrawIndexed(12, 1, 0)
			ctx.Encoder.EndRenderPass()
		},
	}
}

// --- Helpers ---

func readPixels(t *testing.T, rt backend.RenderTarget) *Result {
	t.Helper()
	w, h := rt.Width(), rt.Height()
	pixels := make([]byte, w*h*4)
	rt.ColorTexture().ReadPixels(pixels)
	return &Result{Pixels: pixels, Width: w, Height: h}
}

func resultToImage(r *Result) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, r.Width, r.Height))
	copy(img.Pix, r.Pixels)
	return img
}

type vtxData struct {
	px, py, tu, tv, r, g, b, a float32
}

func vtx(px, py, tu, tv, r, g, b, a float32) vtxData {
	return vtxData{px, py, tu, tv, r, g, b, a}
}

func packVertices(verts ...vtxData) []byte {
	data := make([]byte, len(verts)*32)
	for i, v := range verts {
		off := i * 32
		binary.LittleEndian.PutUint32(data[off:], math.Float32bits(v.px))
		binary.LittleEndian.PutUint32(data[off+4:], math.Float32bits(v.py))
		binary.LittleEndian.PutUint32(data[off+8:], math.Float32bits(v.tu))
		binary.LittleEndian.PutUint32(data[off+12:], math.Float32bits(v.tv))
		binary.LittleEndian.PutUint32(data[off+16:], math.Float32bits(v.r))
		binary.LittleEndian.PutUint32(data[off+20:], math.Float32bits(v.g))
		binary.LittleEndian.PutUint32(data[off+24:], math.Float32bits(v.b))
		binary.LittleEndian.PutUint32(data[off+28:], math.Float32bits(v.a))
	}
	return data
}

func packIndices(indices ...uint16) []byte {
	data := make([]byte, len(indices)*2)
	for i, idx := range indices {
		binary.LittleEndian.PutUint16(data[i*2:], idx)
	}
	return data
}

func newWhiteTexture(t *testing.T, dev backend.Device) backend.Texture {
	t.Helper()
	tex, err := dev.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1, Format: backend.TextureFormatRGBA8,
		Data: []byte{255, 255, 255, 255},
	})
	require.NoError(t, err)
	return tex
}

func newCheckerTexture(t *testing.T, dev backend.Device, w, h int) backend.Texture {
	t.Helper()
	pixels := make([]byte, w*h*4)
	for y := range h {
		for x := range w {
			off := (y*w + x) * 4
			if (x+y)%2 == 0 {
				pixels[off] = 255
				pixels[off+1] = 255
				pixels[off+2] = 255
			}
			pixels[off+3] = 255
		}
	}
	tex, err := dev.NewTexture(backend.TextureDescriptor{
		Width: w, Height: h, Format: backend.TextureFormatRGBA8,
		Data: pixels,
	})
	require.NoError(t, err)
	return tex
}

func newBasicPipeline(t *testing.T, dev backend.Device, blend backend.BlendMode) (backend.Shader, backend.Pipeline) {
	t.Helper()
	shader, err := dev.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)

	pipeline, err := dev.NewPipeline(backend.PipelineDescriptor{
		Shader:    shader,
		BlendMode: blend,
	})
	require.NoError(t, err)
	return shader, pipeline
}

func newBuffer(t *testing.T, dev backend.Device, data []byte) backend.Buffer {
	t.Helper()
	buf, err := dev.NewBuffer(backend.BufferDescriptor{Data: data})
	require.NoError(t, err)
	return buf
}

func orthoMatrix(left, right, bottom, top float32) [16]float32 {
	w := right - left
	h := top - bottom
	return [16]float32{
		2 / w, 0, 0, 0,
		0, 2 / h, 0, 0,
		0, 0, -1, 0,
		-(right + left) / w, -(top + bottom) / h, 0, 1,
	}
}

func absDiff(a, b byte) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func clampByte255(v int) uint8 {
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func bytesReader(data []byte) *bytes.Reader {
	return bytes.NewReader(data)
}
