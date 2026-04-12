#!/usr/bin/env bash
#
# profile.sh — Capture CPU and allocation profiles from a future-core program.
#
# Builds the specified cmd/ program, runs it with pprof enabled on the
# native WebGPU backend, captures a CPU profile and an allocation profile,
# then saves both to a named output directory for later comparison.
#
# Requires:
#   - go toolchain
#   - WGPU_NATIVE_LIB_PATH set (or wgpu-native installed in a standard path)
#   - The target program must call futurerender.RunGame (which starts pprof
#     via FUTURE_CORE_PPROF)
#
# Usage:
#   ./scripts/profile.sh [options]
#
# Options:
#   -p PROGRAM   cmd/ program to profile (default: rttest)
#   -n NAME      Name for this capture (default: "baseline")
#   -d SECONDS   CPU profile duration in seconds (default: 10)
#   -o DIR       Output directory (default: testdata/profiles/<name>)
#   -b BACKEND   Backend to use (default: webgpu)
#   -P PORT      Pprof port (default: 6060)
#   -f FRAMES    Headless frame count; must exceed profile duration at ~60fps
#                (default: duration * 200, generous margin)
#   -h           Show help
#
# Examples:
#   # Capture a "baseline" profile from cmd/rttest on WebGPU (10s CPU):
#   ./scripts/profile.sh -n baseline
#
#   # Capture an "optimized" profile for comparison:
#   ./scripts/profile.sh -n optimized
#
#   # Profile cmd/sprite for 30 seconds on the soft backend:
#   ./scripts/profile.sh -p sprite -b soft -d 30 -n sprite-soft
#
#   # Compare two captures:
#   ./scripts/profile-compare.sh testdata/profiles/baseline testdata/profiles/optimized
#
set -euo pipefail

PROGRAM="rttest"
NAME="baseline"
DURATION=10
OUTDIR=""
BACKEND="webgpu"
PORT=6060
FRAMES=""

usage() {
    sed -n '2,/^set -/p' "$0" | grep '^#' | sed 's/^# \?//'
    exit 0
}

while getopts "p:n:d:o:b:P:f:h" opt; do
    case $opt in
        p) PROGRAM="$OPTARG" ;;
        n) NAME="$OPTARG" ;;
        d) DURATION="$OPTARG" ;;
        o) OUTDIR="$OPTARG" ;;
        b) BACKEND="$OPTARG" ;;
        P) PORT="$OPTARG" ;;
        f) FRAMES="$OPTARG" ;;
        h) usage ;;
        *) usage ;;
    esac
done

# Default output directory.
if [ -z "$OUTDIR" ]; then
    OUTDIR="testdata/profiles/${NAME}"
fi

# Default frame count: enough headless frames to outlast the profile duration.
# Native GPU backends can render well over 1000fps headless, so we use a
# generous multiplier to ensure the program stays alive for the full
# profiling window. Better to overshoot than have it exit mid-capture.
if [ -z "$FRAMES" ]; then
    FRAMES=$(( DURATION * 5000 ))
fi

# Resolve paths relative to repo root.
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

mkdir -p "$OUTDIR"

echo "=== Profile Capture: ${NAME} ==="
echo "  Program:  cmd/${PROGRAM}"
echo "  Backend:  ${BACKEND}"
echo "  Duration: ${DURATION}s CPU"
echo "  Output:   ${OUTDIR}/"
echo ""

# Build.
echo "Building cmd/${PROGRAM}..."
BINARY="${OUTDIR}/${PROGRAM}"
go build -o "$BINARY" "./cmd/${PROGRAM}"

# Determine backend-specific env vars.
EXTRA_ENV=""
if [ "$BACKEND" = "webgpu" ]; then
    WGPU_PATH="${WGPU_NATIVE_LIB_PATH:-/opt/homebrew/lib}"
    EXTRA_ENV="WGPU_NATIVE_LIB_PATH=${WGPU_PATH}"
fi

# Start the program with pprof.
echo "Starting ${PROGRAM} (pprof on :${PORT}, backend=${BACKEND}, ${FRAMES} frames)..."
# shellcheck disable=SC2086 # intentional word-splitting on EXTRA_ENV
env ${EXTRA_ENV} \
    FUTURE_CORE_PPROF=":${PORT}" \
    FUTURE_CORE_BACKEND="${BACKEND}" \
    FUTURE_CORE_HEADLESS="${FRAMES}" \
    FUTURE_CORE_HEADLESS_OUTPUT="${OUTDIR}/screenshot.png" \
    "$BINARY" &
PID=$!

# Wait for pprof to be ready.
for _ in $(seq 1 20); do
    if curl -sf "http://localhost:${PORT}/debug/pprof/" > /dev/null 2>&1; then
        break
    fi
    sleep 0.25
done

if ! curl -sf "http://localhost:${PORT}/debug/pprof/" > /dev/null 2>&1; then
    echo "ERROR: pprof server didn't start on :${PORT}" >&2
    kill "$PID" 2>/dev/null || true
    exit 1
fi

# Capture CPU profile.
echo "Capturing CPU profile (${DURATION}s)..."
go tool pprof -proto \
    -output "${OUTDIR}/cpu.pb.gz" \
    "http://localhost:${PORT}/debug/pprof/profile?seconds=${DURATION}" 2>&1 | tail -1

# Capture allocation profile.
echo "Capturing allocs profile..."
go tool pprof -proto \
    -output "${OUTDIR}/allocs.pb.gz" \
    "http://localhost:${PORT}/debug/pprof/allocs" 2>&1 | tail -1

# Capture heap profile.
echo "Capturing heap profile..."
go tool pprof -proto \
    -output "${OUTDIR}/heap.pb.gz" \
    "http://localhost:${PORT}/debug/pprof/heap" 2>&1 | tail -1

# Stop the program.
kill "$PID" 2>/dev/null || true
wait "$PID" 2>/dev/null || true

# Generate text summaries for quick inspection.
echo "Generating text summaries..."
go tool pprof -top -cum "${OUTDIR}/cpu.pb.gz" > "${OUTDIR}/cpu_top.txt" 2>&1
go tool pprof -top "${OUTDIR}/allocs.pb.gz" > "${OUTDIR}/allocs_top.txt" 2>&1

echo ""
echo "=== Capture complete: ${OUTDIR}/ ==="
ls -lh "${OUTDIR}/"*.pb.gz "${OUTDIR}/"*.txt 2>/dev/null
echo ""
echo "Quick inspect:"
echo "  go tool pprof ${OUTDIR}/cpu.pb.gz"
echo "  go tool pprof ${OUTDIR}/allocs.pb.gz"
echo ""
echo "Compare with another capture:"
echo "  ./scripts/profile-compare.sh ${OUTDIR} testdata/profiles/<other>"
