//go:build js && !soft

package webgpu

import (
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
)

// DIAGNOSTIC TOOL — FUTURE_CORE_TRACE_WEBGPU=N enables detailed per-call
// logging of every browser-WebGPU encoder event (BeginRenderPass,
// SetPipeline, setBindGroup with dynamic offsets, SetTexture, DrawIndexed,
// Flush) for the first N frames. Frame boundaries are delimited by Flush()
// calls — each Flush corresponds to one queue.submit, i.e. the end of a
// frame's recorded work.
//
// Companion diagnostics (see those files for details):
//
//	internal/pipeline/trace.go                         — FUTURE_CORE_TRACE_BATCHES / _PASSES
//	internal/shadertranslate/tooling_dump_wgsl_test.go — TestDumpPointLightWGSL
//
// Use this tracer when the batcher / sprite-pass tracers show the right
// batch sequence but pixels still look wrong. This one dumps the actual
// WebGPU calls the encoder produces, so you can diff against a known-good
// hand-written WebGPU program (e.g. parity-tests/wgsl-lighting-demo/).
//
// The hot-path cost is a single atomic int load and comparison per call
// site; zero output is emitted when the env var is unset or the frame
// budget has been exhausted.

var traceWebGPULimit = parseTraceEnvInt("FUTURE_CORE_TRACE_WEBGPU")

var traceWebGPUFrame atomic.Int32

func parseTraceEnvInt(name string) int {
	s := os.Getenv(name)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// traceWebGPUActive reports whether the tracer is on for the current frame.
func traceWebGPUActive() bool {
	return traceWebGPULimit > 0 && int(traceWebGPUFrame.Load()) < traceWebGPULimit
}

// traceWebGPUf writes to stderr (routed to the browser console in WASM).
func traceWebGPUf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
}

// traceWebGPUAdvanceFrame increments the frame counter. Called from
// Flush() so that each queue.submit boundary ends the current frame.
func traceWebGPUAdvanceFrame() int {
	return int(traceWebGPUFrame.Add(1))
}
