package pipeline

import (
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
)

// Per-frame rendering tracer, controlled by environment variables. Useful
// for understanding what the sprite pass is doing on a given backend when
// debugging visual regressions. Kept in a separate file to make it obvious
// that this is instrumentation, not core logic.
//
// Environment variables (all cap frame count to keep logs small):
//
//	FUTURE_CORE_TRACE_BATCHES=N
//	    Dump the batcher's per-frame batch metadata (target ID, texture ID,
//	    shader ID, filter, blend mode, vertex count, index count) for the
//	    first N frames. Useful for seeing the sequence of target switches
//	    and confirming what the batcher handed to the sprite pass.
//
//	FUTURE_CORE_TRACE_PASSES=N
//	    Dump every render pass boundary (BeginRenderPass / EndRenderPass)
//	    with its target ID, load action, viewport, and framebuffer size,
//	    for the first N frames. Useful for seeing exactly how the sprite
//	    pass decomposes the batch stream into GPU submissions — e.g. to
//	    catch "each screen re-entry clears the framebuffer" bugs.
//
// Both counters use a single atomic int for thread safety, though in
// practice the render loop is single-goroutine. Both default to 0
// (disabled) when the env var is unset or cannot be parsed as a positive
// integer.
//
// Example session — capturing batch metadata for the first 3 frames of a
// program that renders two tiles onto the screen:
//
//	FUTURE_CORE_TRACE_BATCHES=3 \
//	  FUTURE_CORE_HEADLESS=3 \
//	  FUTURE_CORE_HEADLESS_OUTPUT=/tmp/foo.png \
//	  ./myprogram
//
// Output on stderr looks like:
//
//	=== frame 1: 6 batches ===
//	  batch[0] target=2 texture=1 shader=0 filter=0 blend=1 verts=4 indices=6
//	  batch[1] target=2 texture=0 shader=0 filter=0 blend=1 verts=20 indices=30
//	  ...
//
// Combine with FUTURE_CORE_TRACE_PASSES to cross-reference batch ranges
// against the actual BeginRenderPass / EndRenderPass calls.

// traceBatchesLimit is the maximum number of frames to trace via
// FUTURE_CORE_TRACE_BATCHES. Zero disables tracing. Resolved once at
// package init so the hot path reads a single integer.
var traceBatchesLimit = parseEnvInt("FUTURE_CORE_TRACE_BATCHES")

// traceBatchesCount counts the number of frames already traced via
// FUTURE_CORE_TRACE_BATCHES.
var traceBatchesCount atomic.Int32

// tracePassesLimit is the maximum number of frames to trace via
// FUTURE_CORE_TRACE_PASSES. Zero disables tracing.
var tracePassesLimit = parseEnvInt("FUTURE_CORE_TRACE_PASSES")

// tracePassesCount counts the number of pass-boundary events already
// traced. Unlike traceBatchesCount this advances on every pass begin, not
// on every frame — the limit is interpreted as "frames", and each
// Execute() records up to tracePassesLimit "generations" using
// tracePassesFrameCount.
var tracePassesFrameCount atomic.Int32

// parseEnvInt reads an int from the given environment variable. Returns 0
// if unset, empty, not parseable, or negative.
func parseEnvInt(name string) int {
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

// tracef writes to stderr when the named tracer is active. Callers should
// check the appropriate "enabled" predicate first to avoid constructing
// the format arguments when tracing is off.
func tracef(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
}

// traceBatchesActive returns true when the FUTURE_CORE_TRACE_BATCHES
// tracer is enabled and the frame budget is not yet exhausted. The caller
// should advance the counter (via traceBatchesBeginFrame) AFTER deciding
// to emit output; this predicate is cheap and intended to be checked in
// the hot path.
func traceBatchesActive() bool {
	return traceBatchesLimit > 0 && int(traceBatchesCount.Load()) < traceBatchesLimit
}

// traceBatchesBeginFrame increments the frame counter and returns the new
// value. Callers that have already checked traceBatchesActive() can
// simply emit output until the returned value exceeds traceBatchesLimit.
func traceBatchesBeginFrame() int {
	return int(traceBatchesCount.Add(1))
}

// tracePassesActive reports whether pass-boundary tracing is enabled and
// the frame budget hasn't been used.
func tracePassesActive() bool {
	return tracePassesLimit > 0 && int(tracePassesFrameCount.Load()) < tracePassesLimit
}

// tracePassesBeginFrame increments the pass-boundary frame counter.
func tracePassesBeginFrame() int {
	return int(tracePassesFrameCount.Add(1))
}
