// Command rttest is a diagnostic program for a WebGPU-only bug in future-core:
// when a single frame contains two or more "allocate a render target, write
// into it, then sample it from the screen" sequences interleaved, only the
// LAST such sequence actually appears on the screen. Earlier sequences are
// silently dropped.
//
// This bug is observable with both the native WebGPU backend (wgpu-native
// via purego) and the browser WebGPU backend (navigator.gpu via syscall/js).
// The soft rasterizer backend handles the same command stream correctly.
//
// Why this matters: anti-aliasing via a bigOffscreenBuffer-style 2x
// supersample requires this exact pattern — write into an AA buffer, then
// downsample-composite the AA buffer back into its parent target, all in
// the same frame. Until this primitive works on WebGPU, the AA feature
// cannot be built on top of it.
//
// Curious observation: the scene-selector in `future/examples/scene-selector/`
// renders 20 tiles per frame through what looks like the same pattern
// (each tile has its own persistent Canvas, drawn into each frame, then
// composited onto the screen via DrawImage), and it renders correctly in
// the parity-test. So the bug is not a generic "alternating target switches
// fail" issue — something about the scene-selector's call chain avoids it.
// The next investigation step is to trace the scene-selector's exact
// batcher command stream and compare it to this program's.
//
// To run:
//
//	go build ./cmd/rttest
//
//	# Soft backend — baseline correctness, both squares render.
//	FUTURE_CORE_BACKEND=soft \
//	  FUTURE_CORE_HEADLESS=3 \
//	  FUTURE_CORE_HEADLESS_OUTPUT=/tmp/rttest_soft.png \
//	  ./rttest
//
//	# Native WebGPU — reproduces the bug; only the green (second) square.
//	WGPU_NATIVE_LIB_PATH=/opt/homebrew/lib \
//	  FUTURE_CORE_BACKEND=webgpu \
//	  FUTURE_CORE_HEADLESS=3 \
//	  FUTURE_CORE_HEADLESS_OUTPUT=/tmp/rttest_webgpu.png \
//	  ./rttest
//
//	# Browser WebGPU — also reproduces. Build as wasm and load in a
//	# WebGPU-enabled browser; an example harness lives at
//	# /tmp/rttest_harness/ if that earlier session's files are still there.
//	GOOS=js GOARCH=wasm go build -o /tmp/rttest.wasm ./cmd/rttest
//
// Expected output (soft backend): a red 48x48 square on the left and a
// green 48x48 square on the right, both at y=104 on a 256x256 screen.
//
// Observed output (WebGPU backends): only the green square. The red square
// is dropped.
package main

import (
	"image/color"
	"log"

	futurerender "github.com/michaelraines/future-core"
	"github.com/michaelraines/future-core/vector"
)

const (
	screenW = 256
	screenH = 256
)

type rttestGame struct{}

func (g *rttestGame) Update() error {
	if futurerender.IsKeyPressed(futurerender.KeyEscape) {
		return futurerender.ErrTermination
	}
	return nil
}

func (g *rttestGame) Draw(screen *futurerender.Image) {
	// Canonical scene:
	//
	//   Tile 1 — red fill, white border, composited at (40, 104).
	//   Tile 2 — green fill, white border, composited at (168, 104).
	//
	// Each tile allocates its own render target mid-frame, draws into
	// it, and composites onto the screen. Between the two tiles the
	// sprite pass must leave target=0 (screen), write into the second
	// offscreen RT, then return to target=0. Without the sprite_pass
	// screenCleared guard (only clear on first entry per frame), the
	// second screen re-entry clears the screen and wipes tile 1's
	// composite — so this test regresses if that guard is ever
	// removed or broken. It also regresses if drawTrianglesAA's
	// deferred disposal path is broken (the vector DrawFilledRect/
	// StrokeRect calls route through drawTrianglesAA when antialias
	// is true, and vector's stroke tessellation produces a different
	// bbox region than the filled rect, forcing a region-change
	// flush + buffer deferral).
	//
	// Expected output: red square on the left half, green square on
	// the right half, both with white 1px borders, on a black screen.

	rt1 := futurerender.NewImage(48, 48)
	if rt1 != nil {
		rt1.Clear()
		vector.DrawFilledRect(rt1, 0, 0, 48, 48,
			color.RGBA{R: 255, G: 0, B: 0, A: 255}, true)
		vector.StrokeRect(rt1, 0, 0, 48, 48, 1,
			color.RGBA{R: 255, G: 255, B: 255, A: 255}, true)
		op := &futurerender.DrawImageOptions{}
		op.GeoM.Translate(40, 104)
		screen.DrawImage(rt1, op)
	}

	rt2 := futurerender.NewImage(48, 48)
	if rt2 != nil {
		rt2.Clear()
		vector.DrawFilledRect(rt2, 0, 0, 48, 48,
			color.RGBA{R: 0, G: 255, B: 0, A: 255}, true)
		vector.StrokeRect(rt2, 0, 0, 48, 48, 1,
			color.RGBA{R: 255, G: 255, B: 255, A: 255}, true)
		op := &futurerender.DrawImageOptions{}
		op.GeoM.Translate(168, 104)
		screen.DrawImage(rt2, op)
	}
}

func (g *rttestGame) Layout(_, _ int) (int, int) {
	return screenW, screenH
}

func main() {
	futurerender.SetWindowSize(screenW, screenH)
	futurerender.SetWindowTitle("Future Render — RT Sampling Repro")
	if err := futurerender.RunGame(&rttestGame{}); err != nil {
		log.Fatal(err)
	}
}
