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

	"github.com/michaelraines/future-core/internal/backend"
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
	// NeedsStencil requests a stencil attachment on the scene's render
	// target. Scenes with NeedsStencil=true are skipped on devices that
	// report Capabilities.SupportsStencil=false; otherwise the scene
	// would either allocate an attachment the backend cannot satisfy or
	// silently miss its stencil ops.
	NeedsStencil bool
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
		sceneFillRuleNonZero(),
		sceneFillRuleEvenOdd(),
	}
}

// RunAll runs all conformance test scenes against the given device.
func RunAll(t *testing.T, dev backend.Device, enc backend.CommandEncoder) {
	RunAllExcept(t, dev, enc, nil)
}

// RunAllExcept is like RunAll but skips scenes listed in the `skip` map,
// logging the reason for each skipped scene. Used by backends that
// pass the bulk of the suite but diverge on specific scenes for
// documented convention reasons (e.g. WebGPU native's nearest-neighbor
// texture sampling doesn't match soft's at cell boundaries).
func RunAllExcept(t *testing.T, dev backend.Device, enc backend.CommandEncoder, skip map[string]string) {
	t.Helper()
	for _, scene := range Scenes() {
		t.Run(scene.Name, func(t *testing.T) {
			if reason, skipped := skip[scene.Name]; skipped {
				t.Skip(reason)
			}
			RunScene(t, dev, enc, scene)
		})
	}
}

// RunScene renders a single scene and compares against the golden reference.
func RunScene(t *testing.T, dev backend.Device, enc backend.CommandEncoder, scene Scene) {
	t.Helper()

	if scene.NeedsStencil && !dev.Capabilities().SupportsStencil {
		t.Skipf("scene %q requires stencil support; device reports SupportsStencil=false", scene.Name)
	}

	// Create render target.
	rt, err := dev.NewRenderTarget(backend.RenderTargetDescriptor{
		Width:       SceneSize,
		Height:      SceneSize,
		ColorFormat: backend.TextureFormatRGBA8,
		HasStencil:  scene.NeedsStencil,
	})
	require.NoError(t, err)
	t.Cleanup(rt.Dispose)

	ctx := &RenderContext{
		Device:  dev,
		Target:  rt,
		Encoder: enc,
		Width:   SceneSize,
		Height:  SceneSize,
	}

	// Render the scene.
	scene.Render(t, ctx)

	// Flush before readback: backends that record into a deferred
	// command buffer (WebGPU native, Vulkan) need an explicit submit
	// before their render-pass writes are visible to ReadPixels. Soft
	// and GL-style backends make Flush a no-op, so this is free for
	// them.
	ctx.Encoder.Flush()

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

	minLen := min(len(actual), len(expected))

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
// testdata/golden/ relative to the running test's working directory
// (which `go test` sets to the package under test). Can be overridden
// via the FUTURE_CORE_GOLDEN_DIR environment variable — used by the
// lavapipe Docker harness to point the Vulkan suite at a separate
// golden tree so its results don't collide with MoltenVK-generated
// goldens captured on the Mac dev loop.
//
// When FUTURE_CORE_GOLDEN_DIR is set, callers MUST supply an absolute
// path. `go test` runs each package with a different working
// directory, so a relative override (e.g. "internal/backend/vulkan/…")
// would resolve differently when the conformance package test runs
// (cwd=internal/backend/conformance) vs the Vulkan package test
// (cwd=internal/backend/vulkan), writing goldens into two separate
// trees. See docker-compose.yml for the absolute-path usage.
func GoldenDir() string {
	if d := os.Getenv("FUTURE_CORE_GOLDEN_DIR"); d != "" {
		return d
	}
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

			_, pipeline := newBasicPipeline(t, ctx.Device, backend.BlendNone)

			verts := packVertices(
				vtx(-0.5, -0.5, 0, 0, 1, 0, 0, 1),
				vtx(0.5, -0.5, 0, 0, 1, 0, 0, 1),
				vtx(0, 0.5, 0, 0, 1, 0, 0, 1),
			)
			indices := packIndices(0, 1, 2)

			vbuf := newBuffer(t, ctx.Device, verts)
			ibuf := newBuffer(t, ctx.Device, indices)

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

			_, pipeline := newBasicPipeline(t, ctx.Device, backend.BlendNone)

			verts := packVertices(
				vtx(-0.8, -0.8, 0, 0, 1, 0, 0, 1), // red
				vtx(0.8, -0.8, 0, 0, 0, 1, 0, 1),  // green
				vtx(0, 0.8, 0, 0, 0, 0, 1, 1),     // blue
			)
			indices := packIndices(0, 1, 2)

			vbuf := newBuffer(t, ctx.Device, verts)
			ibuf := newBuffer(t, ctx.Device, indices)

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

			_, pipeline := newBasicPipeline(t, ctx.Device, backend.BlendNone)

			// Two triangles forming a quad.
			verts := packVertices(
				vtx(-0.8, -0.8, 0, 0, 1, 1, 1, 1),
				vtx(0.8, -0.8, 1, 0, 1, 1, 1, 1),
				vtx(0.8, 0.8, 1, 1, 1, 1, 1, 1),
				vtx(-0.8, 0.8, 0, 1, 1, 1, 1, 1),
			)
			indices := packIndices(0, 1, 2, 0, 2, 3)

			vbuf := newBuffer(t, ctx.Device, verts)
			ibuf := newBuffer(t, ctx.Device, indices)

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

			_, pipeline := newBasicPipeline(t, ctx.Device, backend.BlendSourceOver)

			// Red background quad.
			bg := packVertices(
				vtx(-1, -1, 0, 0, 1, 0, 0, 1),
				vtx(1, -1, 0, 0, 1, 0, 0, 1),
				vtx(1, 1, 0, 0, 1, 0, 0, 1),
				vtx(-1, 1, 0, 0, 1, 0, 0, 1),
			)
			bgIdx := packIndices(0, 1, 2, 0, 2, 3)
			bgVbuf := newBuffer(t, ctx.Device, bg)
			bgIbuf := newBuffer(t, ctx.Device, bgIdx)

			// Semi-transparent green triangle.
			fg := packVertices(
				vtx(-0.5, -0.5, 0, 0, 0, 1, 0, 0.5),
				vtx(0.5, -0.5, 0, 0, 0, 1, 0, 0.5),
				vtx(0, 0.5, 0, 0, 0, 1, 0, 0.5),
			)
			fgIdx := packIndices(0, 1, 2)
			fgVbuf := newBuffer(t, ctx.Device, fg)
			fgIbuf := newBuffer(t, ctx.Device, fgIdx)

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

			// Red background with BlendNone.
			_, pipelineBG := newBasicPipeline(t, ctx.Device, backend.BlendNone)

			// Additive foreground.
			_, pipelineFG := newBasicPipeline(t, ctx.Device, backend.BlendAdditive)

			bg := packVertices(
				vtx(-1, -1, 0, 0, 0.5, 0, 0, 1),
				vtx(1, -1, 0, 0, 0.5, 0, 0, 1),
				vtx(1, 1, 0, 0, 0.5, 0, 0, 1),
				vtx(-1, 1, 0, 0, 0.5, 0, 0, 1),
			)
			bgIdx := packIndices(0, 1, 2, 0, 2, 3)
			bgVbuf := newBuffer(t, ctx.Device, bg)
			bgIbuf := newBuffer(t, ctx.Device, bgIdx)

			fg := packVertices(
				vtx(-0.5, -0.5, 0, 0, 0, 0, 0.5, 1),
				vtx(0.5, -0.5, 0, 0, 0, 0, 0.5, 1),
				vtx(0, 0.5, 0, 0, 0, 0, 0.5, 1),
			)
			fgIdx := packIndices(0, 1, 2)
			fgVbuf := newBuffer(t, ctx.Device, fg)
			fgIbuf := newBuffer(t, ctx.Device, fgIdx)

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

			_, pipeline := newBasicPipeline(t, ctx.Device, backend.BlendNone)

			verts := packVertices(
				vtx(-1, -1, 0, 0, 1, 1, 1, 1),
				vtx(1, -1, 0, 0, 1, 1, 1, 1),
				vtx(1, 1, 0, 0, 1, 1, 1, 1),
				vtx(-1, 1, 0, 0, 1, 1, 1, 1),
			)
			indices := packIndices(0, 1, 2, 0, 2, 3)

			vbuf := newBuffer(t, ctx.Device, verts)
			ibuf := newBuffer(t, ctx.Device, indices)

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

			shader, pipeline := newBasicPipeline(t, ctx.Device, backend.BlendNone)

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
			ibuf := newBuffer(t, ctx.Device, indices)

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

			_, pipeline := newBasicPipeline(t, ctx.Device, backend.BlendNone)

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
			ibuf := newBuffer(t, ctx.Device, indices)

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

// The scene helpers below register disposal via t.Cleanup rather than
// returning naked objects for the caller to `defer Dispose()`. Cleanup
// functions run AFTER the test (including the RunScene readback and
// comparison) completes, so buffers/textures/pipelines stay alive
// across the deferred submit that backends like WebGPU native and
// Vulkan require between "record draw" and "texture read" — deferring
// in the scene itself (the old pattern) disposed them before Flush,
// tripping wgpu's "buffer has been destroyed" validation.
func newWhiteTexture(t *testing.T, dev backend.Device) backend.Texture {
	t.Helper()
	tex, err := dev.NewTexture(backend.TextureDescriptor{
		Width: 1, Height: 1, Format: backend.TextureFormatRGBA8,
		Data: []byte{255, 255, 255, 255},
	})
	require.NoError(t, err)
	t.Cleanup(tex.Dispose)
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
	t.Cleanup(tex.Dispose)
	return tex
}

// conformanceVertexFormat matches packVertices's 32-byte per-vertex layout
// (pos/uv/rgba). Declaring it on the pipeline is a no-op for most
// backends but WebGPU native's wgpu_create_render_pipeline requires a
// non-empty vertex buffer layout to accept subsequent SetVertexBuffer
// calls — an empty descriptor compiles a pipeline whose first draw
// trips "Render pipeline must be set / Pipeline must be set".
func conformanceVertexFormat() backend.VertexFormat {
	return backend.VertexFormat{
		Stride: 32,
		Attributes: []backend.VertexAttribute{
			{Name: "position", Format: backend.AttributeFloat2, Offset: 0},
			{Name: "texcoord", Format: backend.AttributeFloat2, Offset: 8},
			{Name: "color", Format: backend.AttributeFloat4, Offset: 16},
		},
	}
}

// conformanceVertexGLSL / conformanceFragmentGLSL define a minimal
// textured-quad shader the conformance scenes run against. The soft
// backend ignores the source and uses its built-in rasterizer; WebGPU
// native and other real-GPU backends need actual GLSL to translate
// into their native shader language. An empty ShaderDescriptor compiles
// to zero shader modules on those backends, leaving the pipeline handle
// null and every subsequent draw tripping "Render pipeline must be set".
//
// Soft's rasterizer now uses Y-down viewport space (ndcY=+1 → screen top,
// ndcY=-1 → screen bottom), matching Vulkan/WebGPU/Metal, so no Y-flip
// is needed in this vertex shader. An earlier revision flipped
// gl_Position.y to compensate for soft's legacy Y-up mapping; removing
// that flip was part of the same change that brought scene-selector
// parity between soft and GPU backends from ~48% to ~3.5% pixel diff.
const conformanceVertexGLSL = `#version 330 core
layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;
uniform mat4 uProjection;
out vec2 vTexCoord;
out vec4 vColor;
void main() {
    vTexCoord = aTexCoord;
    vColor = aColor;
    gl_Position = uProjection * vec4(aPosition, 0.0, 1.0);
}
`

// The fragment shader premultiplies alpha because backend.BlendSourceOver
// uses SrcFactor=One, DstFactor=OneMinusSrcAlpha — the classic
// pre-multiplied-alpha formula. Without the premultiply, semi-transparent
// draws blend as if fully opaque (SrcFactor=One means "use src.rgb as-is",
// which for vColor=(0,1,0,0.5) paints full-intensity green over the
// background). Soft's built-in rasterizer handles this inline, so its
// output — and therefore the golden images — is premultiplied.
const conformanceFragmentGLSL = `#version 330 core
in vec2 vTexCoord;
in vec4 vColor;
uniform sampler2D uTexture;
uniform mat4 uColorBody;
uniform vec4 uColorTranslation;
out vec4 fragColor;
void main() {
    vec4 c = texture(uTexture, vTexCoord) * vColor;
    c = uColorBody * c + uColorTranslation;
    fragColor = vec4(c.rgb * c.a, c.a);
}
`

func newBasicPipeline(t *testing.T, dev backend.Device, blend backend.BlendMode) (backend.Shader, backend.Pipeline) {
	t.Helper()
	shader, err := dev.NewShader(backend.ShaderDescriptor{
		VertexSource:   conformanceVertexGLSL,
		FragmentSource: conformanceFragmentGLSL,
		Attributes:     conformanceVertexFormat().Attributes,
	})
	require.NoError(t, err)
	t.Cleanup(shader.Dispose)

	// Default uniforms: identity projection (the shader itself handles
	// the NDC Y-flip needed to match soft's conventions on GPU
	// backends). Identity color body + zero translation pass
	// texture×vColor through untouched.
	shader.SetUniformMat4("uProjection", [16]float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	})
	shader.SetUniformMat4("uColorBody", [16]float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	})
	shader.SetUniformVec4("uColorTranslation", [4]float32{0, 0, 0, 0})

	pipeline, err := dev.NewPipeline(backend.PipelineDescriptor{
		Shader:       shader,
		BlendMode:    blend,
		VertexFormat: conformanceVertexFormat(),
	})
	require.NoError(t, err)
	t.Cleanup(pipeline.Dispose)
	return shader, pipeline
}

func newBuffer(t *testing.T, dev backend.Device, data []byte) backend.Buffer {
	t.Helper()
	buf, err := dev.NewBuffer(backend.BufferDescriptor{Data: data})
	require.NoError(t, err)
	t.Cleanup(buf.Dispose)
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

// sceneFillRuleNonZero renders two overlapping opposite-winding triangles
// through the NonZero stencil path. The winding counters cancel in the
// overlap region, producing a hole. The golden image is generated from
// the soft rasterizer and all other stencil-capable backends must match
// it within the ±3 tolerance.
func sceneFillRuleNonZero() Scene {
	return Scene{
		Name:         "fill_rule_nonzero",
		Description:  "Two opposite-winding triangles composited under NonZero (hole in overlap)",
		NeedsStencil: true,
		Render: func(t *testing.T, ctx *RenderContext) {
			t.Helper()
			renderFillRuleScene(t, ctx, fillRuleVariantNonZero)
		},
	}
}

// sceneFillRuleEvenOdd renders two overlapping same-winding triangles
// through the EvenOdd stencil path. Overlap bits are inverted, producing
// a hole. The same golden-image gate as NonZero applies.
func sceneFillRuleEvenOdd() Scene {
	return Scene{
		Name:         "fill_rule_evenodd",
		Description:  "Two same-winding triangles composited under EvenOdd (hole in overlap)",
		NeedsStencil: true,
		Render: func(t *testing.T, ctx *RenderContext) {
			t.Helper()
			renderFillRuleScene(t, ctx, fillRuleVariantEvenOdd)
		},
	}
}

type fillRuleVariant int

const (
	fillRuleVariantNonZero fillRuleVariant = iota
	fillRuleVariantEvenOdd
)

// renderFillRuleScene runs the two-pass stencil draw sequence used by
// the sprite pass in production, but against hand-built geometry here so
// conformance is independent of the sprite-pass / batch layer. Two
// overlapping triangles composite into a single fill-rule batch; the
// stencil test in the color pass selects "covered by the winding sign"
// pixels and leaves the overlap unfilled for both NonZero (opposite
// winding cancels) and EvenOdd (two Invert ops cancel).
func renderFillRuleScene(t *testing.T, ctx *RenderContext, variant fillRuleVariant) {
	t.Helper()
	whiteTex := newWhiteTexture(t, ctx.Device)

	dev := ctx.Device
	shader, err := dev.NewShader(backend.ShaderDescriptor{})
	require.NoError(t, err)

	// Write pipeline — ops differ between NonZero (Incr/Decr-wrap,
	// two-sided) and EvenOdd (Invert, single-face).
	var writeStencil backend.StencilDescriptor
	switch variant {
	case fillRuleVariantNonZero:
		writeStencil = backend.StencilDescriptor{
			Func:      backend.CompareAlways,
			Mask:      0xFF,
			WriteMask: 0xFF,
			TwoSided:  true,
			Front: backend.StencilFaceOps{
				SFail:  backend.StencilKeep,
				DPFail: backend.StencilKeep,
				DPPass: backend.StencilIncrWrap,
			},
			Back: backend.StencilFaceOps{
				SFail:  backend.StencilKeep,
				DPFail: backend.StencilKeep,
				DPPass: backend.StencilDecrWrap,
			},
		}
	case fillRuleVariantEvenOdd:
		writeStencil = backend.StencilDescriptor{
			Func:      backend.CompareAlways,
			Mask:      0xFF,
			WriteMask: 0xFF,
			Front: backend.StencilFaceOps{
				SFail:  backend.StencilKeep,
				DPFail: backend.StencilKeep,
				DPPass: backend.StencilInvert,
			},
		}
	}
	writePipe, err := dev.NewPipeline(backend.PipelineDescriptor{
		Shader:             shader,
		BlendMode:          backend.BlendSourceOver,
		CullMode:           backend.CullNone,
		Primitive:          backend.PrimitiveTriangles,
		StencilEnable:      true,
		Stencil:            writeStencil,
		DepthStencilFormat: backend.TextureFormatDepth24Stencil8,
		// Color writes disabled — the write pass populates stencil only.
		// Critical on WebGPU/Vulkan/Metal/DX12 where encoder.SetColorWrite
		// is a no-op (color write is baked into pipeline state there).
		ColorWriteDisabled: true,
	})
	require.NoError(t, err)

	// Color pipeline — NotEqual ref=0, Zero-on-pass so the stencil
	// buffer is cleared as we draw.
	colorPipe, err := dev.NewPipeline(backend.PipelineDescriptor{
		Shader:        shader,
		BlendMode:     backend.BlendSourceOver,
		CullMode:      backend.CullNone,
		Primitive:     backend.PrimitiveTriangles,
		StencilEnable: true,
		Stencil: backend.StencilDescriptor{
			Func:      backend.CompareNotEqual,
			Mask:      0xFF,
			WriteMask: 0xFF,
			Front: backend.StencilFaceOps{
				SFail:  backend.StencilKeep,
				DPFail: backend.StencilKeep,
				DPPass: backend.StencilZero,
			},
			Back: backend.StencilFaceOps{
				SFail:  backend.StencilKeep,
				DPFail: backend.StencilKeep,
				DPPass: backend.StencilZero,
			},
		},
		DepthStencilFormat: backend.TextureFormatDepth24Stencil8,
	})
	require.NoError(t, err)

	// Geometry: two overlapping triangles, yellow. For NonZero we use
	// opposite windings so the counter cancels; for EvenOdd the winding
	// is irrelevant (both CCW) because Invert is self-canceling.
	var verts []byte
	if variant == fillRuleVariantNonZero {
		verts = packVertices(
			// Outer triangle — CCW (front)
			vtx(-0.7, -0.7, 0, 0, 1, 1, 0, 1),
			vtx(0.7, -0.7, 0, 0, 1, 1, 0, 1),
			vtx(0.0, 0.7, 0, 0, 1, 1, 0, 1),
			// Inner triangle — CW (back), flipped vertex order
			vtx(-0.3, -0.3, 0, 0, 1, 1, 0, 1),
			vtx(0.0, 0.3, 0, 0, 1, 1, 0, 1),
			vtx(0.3, -0.3, 0, 0, 1, 1, 0, 1),
		)
	} else {
		verts = packVertices(
			// Outer triangle — CCW
			vtx(-0.7, -0.7, 0, 0, 1, 1, 0, 1),
			vtx(0.7, -0.7, 0, 0, 1, 1, 0, 1),
			vtx(0.0, 0.7, 0, 0, 1, 1, 0, 1),
			// Inner triangle — CCW too; Invert is self-canceling
			// under EvenOdd so winding doesn't matter.
			vtx(-0.3, -0.3, 0, 0, 1, 1, 0, 1),
			vtx(0.3, -0.3, 0, 0, 1, 1, 0, 1),
			vtx(0.0, 0.3, 0, 0, 1, 1, 0, 1),
		)
	}
	indices := packIndices(0, 1, 2, 3, 4, 5)

	vbuf := newBuffer(t, dev, verts)
	ibuf := newBuffer(t, dev, indices)

	ctx.Encoder.BeginRenderPass(backend.RenderPassDescriptor{
		Target:             ctx.Target,
		LoadAction:         backend.LoadActionClear,
		ClearColor:         [4]float32{0, 0, 0, 1},
		ClearStencil:       0,
		StencilLoadAction:  backend.LoadActionClear,
		StencilStoreAction: backend.StoreActionStore,
	})
	ctx.Encoder.SetVertexBuffer(vbuf, 0)
	ctx.Encoder.SetIndexBuffer(ibuf, backend.IndexUint16)
	ctx.Encoder.SetTexture(whiteTex, 0)

	// Stencil-write pass: color off (baked into writePipe), winding
	// counters accumulate.
	ctx.Encoder.SetPipeline(writePipe)
	ctx.Encoder.SetStencilReference(0)
	ctx.Encoder.DrawIndexed(6, 1, 0)

	// Color pass: writes yellow where stencil != 0, zeroing the buffer.
	ctx.Encoder.SetPipeline(colorPipe)
	ctx.Encoder.SetStencilReference(0)
	ctx.Encoder.DrawIndexed(6, 1, 0)

	ctx.Encoder.EndRenderPass()

	// Independent semantic check: the overlap region must be unfilled
	// (color=black), and the outer-only region must be yellow. Without
	// this assertion a backend that produces "solid yellow everywhere"
	// would match any golden regenerated from the same-wrong source,
	// hiding a broken fill-rule implementation. The checks use 0-index
	// SceneSize/64 coords and account for the ±Tolerance slack that
	// ComparePixels otherwise applies.
	pixels := make([]byte, ctx.Width*ctx.Height*4)
	ctx.Target.ColorTexture().ReadPixels(pixels)

	// Center of the geometry (0,0 in NDC → middle of the RT). Both
	// variants place opposite/overlapping triangles such that the
	// exact center falls inside the overlap — NonZero cancels to zero
	// winding, EvenOdd inverts twice to zero. In both cases the color
	// pass's NotEqual ref=0 rejects the pixel, leaving the cleared
	// background (black, opaque).
	cx := ctx.Width / 2
	cy := ctx.Height / 2
	centerOff := (cy*ctx.Width + cx) * 4
	require.LessOrEqualf(t, int(pixels[centerOff]), int(Tolerance),
		"fill-rule overlap should be black; center R=%d", pixels[centerOff])
	require.LessOrEqualf(t, int(pixels[centerOff+1]), int(Tolerance),
		"fill-rule overlap should be black; center G=%d", pixels[centerOff+1])

	// Inside the outer triangle but outside the inner — below the
	// centroid, both variants cover with yellow (R=255, G=255, B=0).
	// Pick a pixel ~1/4 of the height from the bottom, which is
	// outside the inner triangle's bounding box by construction.
	sx := ctx.Width / 2
	sy := ctx.Height*3/4 + 2
	outerOff := (sy*ctx.Width + sx) * 4
	require.GreaterOrEqualf(t, int(pixels[outerOff]), 255-int(Tolerance),
		"outer fill should be yellow; outer R=%d", pixels[outerOff])
	require.GreaterOrEqualf(t, int(pixels[outerOff+1]), 255-int(Tolerance),
		"outer fill should be yellow; outer G=%d", pixels[outerOff+1])
	require.LessOrEqualf(t, int(pixels[outerOff+2]), int(Tolerance),
		"outer fill should be yellow; outer B=%d", pixels[outerOff+2])
}
