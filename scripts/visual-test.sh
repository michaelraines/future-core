#!/usr/bin/env bash
#
# visual-test.sh — Headless visual testing for any backend and example.
#
# Renders an example program through the full engine pipeline (including
# window creation) and captures a screenshot. Works on machines with or
# without a physical display by using a virtual framebuffer (Xvfb) when
# no display is available.
#
# Two modes:
#   soft  — Uses the software rasterizer. Works everywhere, no GPU needed.
#   gpu   — Uses the platform's preferred GPU backend (auto-detected).
#           Requires GPU hardware + drivers (real or virtual display).
#
# Usage:
#   ./scripts/visual-test.sh [options]
#
# Options:
#   -m MODE      Mode: soft or gpu (default: soft)
#   -b BACKEND   Override backend: auto, opengl, metal, vulkan, etc.
#                (default: "soft" in soft mode, "auto" in gpu mode)
#   -e EXAMPLE   Example to run: sprite, clear, triangles, etc. (default: sprite)
#   -f FRAMES    Number of frames to render before capture (default: 10)
#   -o FILE      Output screenshot path (default: testdata/visual/<mode>_<example>.png)
#   -r FILE      Reference image to compare against
#   -h           Show help
#
# Examples:
#   ./scripts/visual-test.sh                          # soft sprite test
#   ./scripts/visual-test.sh -m gpu                   # gpu sprite test (auto backend)
#   ./scripts/visual-test.sh -m gpu -b vulkan         # gpu sprite test (force vulkan)
#   ./scripts/visual-test.sh -m soft -e triangles     # soft triangles test
#   ./scripts/visual-test.sh -m gpu -f 60             # gpu, capture at frame 60
#
# Environment:
#   Set DISPLAY to use an existing X display. If DISPLAY is unset and no
#   display is detected, Xvfb is started automatically (Linux only).
#   On macOS, the native display is always available when a session exists.

set -euo pipefail

MODE="soft"
BACKEND=""
EXAMPLE="sprite"
FRAMES=60
OUTPUT=""
REFERENCE=""

while getopts "m:b:e:f:o:r:h" opt; do
  case $opt in
    m) MODE="$OPTARG" ;;
    b) BACKEND="$OPTARG" ;;
    e) EXAMPLE="$OPTARG" ;;
    f) FRAMES="$OPTARG" ;;
    o) OUTPUT="$OPTARG" ;;
    r) REFERENCE="$OPTARG" ;;
    h)
      sed -n '2,/^$/p' "$0" | sed 's/^#//' | sed 's/^ //'
      exit 0
      ;;
    *)
      echo "Unknown option: -$OPTARG" >&2
      exit 1
      ;;
  esac
done

# Validate mode.
case "$MODE" in
  soft|gpu) ;;
  *) echo "Error: mode must be 'soft' or 'gpu', got '$MODE'" >&2; exit 1 ;;
esac

# Resolve backend from mode if not explicitly set.
if [ -z "$BACKEND" ]; then
  case "$MODE" in
    soft) BACKEND="soft" ;;
    gpu)  BACKEND="auto" ;;
  esac
fi

# Resolve paths.
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CMD_DIR="$ROOT/cmd/$EXAMPLE"
if [ ! -d "$CMD_DIR" ]; then
  echo "Error: example '$EXAMPLE' not found at $CMD_DIR" >&2
  exit 1
fi

OUTDIR="$ROOT/testdata/visual"
mkdir -p "$OUTDIR"
if [ -z "$OUTPUT" ]; then
  OUTPUT="$OUTDIR/${MODE}_${EXAMPLE}.png"
fi

# ---------------------------------------------------------------------------
# Virtual display setup (Linux headless environments)
# ---------------------------------------------------------------------------
XVFB_PID=""

setup_virtual_display() {
  # macOS: display is always available via Quartz, no Xvfb needed.
  if [ "$(uname)" = "Darwin" ]; then
    return 0
  fi

  # If DISPLAY is set and valid, use it.
  if [ -n "${DISPLAY:-}" ]; then
    return 0
  fi

  # No display — try to start Xvfb.
  if ! command -v Xvfb &>/dev/null; then
    echo "Error: no display available and Xvfb not found." >&2
    echo "  Install with: sudo apt-get install xvfb" >&2
    echo "  Or set DISPLAY to an existing X server." >&2
    exit 1
  fi

  # Find a free display number.
  DISPLAY_NUM=99
  while [ -e "/tmp/.X${DISPLAY_NUM}-lock" ]; do
    DISPLAY_NUM=$((DISPLAY_NUM + 1))
  done

  echo "==> Starting Xvfb on :${DISPLAY_NUM}..."
  Xvfb ":${DISPLAY_NUM}" -screen 0 1280x1024x24 &>/dev/null &
  XVFB_PID=$!
  export DISPLAY=":${DISPLAY_NUM}"

  # Wait briefly for Xvfb to start.
  sleep 0.5
  if ! kill -0 "$XVFB_PID" 2>/dev/null; then
    echo "Error: Xvfb failed to start" >&2
    exit 1
  fi
}

cleanup() {
  if [ -n "$XVFB_PID" ]; then
    kill "$XVFB_PID" 2>/dev/null || true
    wait "$XVFB_PID" 2>/dev/null || true
  fi
  rm -f "${BINARY:-}"
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Build and run
# ---------------------------------------------------------------------------

# Build to a temp binary.
BINARY=$(mktemp /tmp/visual-test-XXXXXX)
echo "==> Building cmd/$EXAMPLE..."
(cd "$ROOT" && go build -o "$BINARY" "./cmd/$EXAMPLE/")

# Set up virtual display if needed.
setup_virtual_display

# Run the binary with headless capture enabled.
echo "==> Running $EXAMPLE (mode=$MODE, backend=$BACKEND, frames=$FRAMES)..."
FUTURE_CORE_BACKEND="$BACKEND" \
  FUTURE_CORE_HEADLESS="$FRAMES" \
  FUTURE_CORE_HEADLESS_OUTPUT="$OUTPUT" \
  "$BINARY" 2>&1

# Verify output was created.
if [ ! -f "$OUTPUT" ]; then
  echo "Error: screenshot was not created at $OUTPUT" >&2
  exit 1
fi

echo "==> Screenshot: $OUTPUT"

# Compare if reference provided.
if [ -n "$REFERENCE" ]; then
  if [ ! -f "$REFERENCE" ]; then
    echo "Warning: reference image not found: $REFERENCE"
    exit 0
  fi
  echo "==> Comparing against $REFERENCE..."
  if command -v sips &>/dev/null; then
    OUT_SIZE=$(sips -g pixelWidth -g pixelHeight "$OUTPUT" 2>/dev/null | tail -2 | awk '{print $2}' | tr '\n' 'x')
    REF_SIZE=$(sips -g pixelWidth -g pixelHeight "$REFERENCE" 2>/dev/null | tail -2 | awk '{print $2}' | tr '\n' 'x')
    if [ "$OUT_SIZE" = "$REF_SIZE" ]; then
      echo "  Dimensions match: $OUT_SIZE"
    else
      echo "  Dimension mismatch: output=$OUT_SIZE reference=$REF_SIZE"
    fi
  fi
  echo "  Visual comparison: open $OUTPUT $REFERENCE"
fi

echo "==> Done."
