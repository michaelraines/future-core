#!/usr/bin/env bash
#
# dx12-capture.sh — run a single headless capture of the future demo
# binary inside the dx12 Wine + vkd3d-proton + lavapipe container.
# Mirrors parity-capture.sh's interface.
#
# Used by the docker-compose `dx12-parity` service. Not intended for
# direct host use — the corresponding host wrapper is
# future-meta/scripts/capture-dx12.sh.
#
# Pipeline:
#
#   GOOS=windows GOARCH=amd64 go build → future.exe
#       → wine future.exe (loads vkd3d-proton's d3d12.dll)
#       → vkd3d-proton translates DX12 → Vulkan
#       → Mesa lavapipe runs Vulkan on CPU
#       → headless capture writes PNG to /output/
#
# The Wine prefix has vkd3d-proton's DLLs registered as overrides
# (see Dockerfile.dx12's setup_vkd3d_proton.sh install step), so the
# binary's d3d12 / dxgi imports resolve to the translation layer
# instead of stub Wine DLLs.
#
# Inputs (all read from environment, set by `docker compose run`):
#
#   FUTURE_SCENE                 scene id (required)
#   FUTURE_CORE_HEADLESS         frame count for headless capture (required)
#   FUTURE_CORE_HEADLESS_OUTPUT  output PNG path (defaults to /output/capture.png)
#   FUTURE_WINDOW_WIDTH          window width (default 1920)
#   FUTURE_WINDOW_HEIGHT         window height (default 1080)
#   FUTURE_SEED                  optional global RNG seed
#   FUTURE_PARITY_REBUILD        if "1", rebuild the binary even when cached

set -euo pipefail

: "${FUTURE_SCENE:?FUTURE_SCENE is required}"
: "${FUTURE_CORE_HEADLESS:?FUTURE_CORE_HEADLESS is required (frame count)}"

OUTPUT="${FUTURE_CORE_HEADLESS_OUTPUT:-/output/capture.png}"
WIDTH="${FUTURE_WINDOW_WIDTH:-1920}"
HEIGHT="${FUTURE_WINDOW_HEIGHT:-1080}"
REBUILD="${FUTURE_PARITY_REBUILD:-0}"

BIN_PATH="/cached-bin/future.exe"

# Cross-compile to Windows. go.work in /workspace/meta resolves
# future-core to the bind-mounted directory, so any uncommitted
# future-core change on the host is exercised here.
#
# Tags pin the active backend + native shader pack. Same set as the
# host build, but locked to dx12 explicitly so a stray
# FUTURE_CORE_BACKEND env var doesn't force a different code path.
SHADER_TAGS="lang_msl lang_wgsl lang_glsl lang_glsles lang_hlsl lang_spirv"

if [ "$REBUILD" = "1" ] || [ ! -f "$BIN_PATH" ]; then
    echo "==> Building future.exe (Windows / DX12)"
    cd /workspace/meta/future
    GOOS=windows GOARCH=amd64 go build \
        -tags "$SHADER_TAGS" \
        -o "$BIN_PATH" \
        ./cmd/driver
    echo "==> Build complete: $(ls -la "$BIN_PATH" | awk '{print $5}') bytes"
fi

# Headless Xvfb display. Wine + GLFW need an X server even for
# offscreen swapchain rendering; the framebuffer is captured via
# the engine's FUTURE_CORE_HEADLESS path, not via X11 readback.
Xvfb :99 -screen 0 "${WIDTH}x${HEIGHT}x24" &
XVFB_PID=$!
trap "kill $XVFB_PID 2>/dev/null || true" EXIT
export DISPLAY=:99
sleep 0.5

echo "==> Running future.exe under Wine + vkd3d-proton"
echo "    scene=$FUTURE_SCENE frames=$FUTURE_CORE_HEADLESS res=${WIDTH}x${HEIGHT}"
echo "    output=$OUTPUT"

# DXVK_HUD=1 prints a corner overlay confirming vkd3d-proton ran (in
# headless capture this just lands in stderr as a "vkd3d-proton: ..."
# init line). VKD3D_CONFIG=dxr_disable avoids ray-tracing path
# incompatibilities on lavapipe.
export DXVK_LOG_LEVEL=warn
export VKD3D_DEBUG=warn
export VKD3D_CONFIG=dxr_disable

env \
    FUTURE_CORE_BACKEND=dx12 \
    FUTURE_SCENE="$FUTURE_SCENE" \
    FUTURE_CORE_HEADLESS="$FUTURE_CORE_HEADLESS" \
    FUTURE_CORE_HEADLESS_OUTPUT="$(echo "$OUTPUT" | sed 's|/output|Z:\\\\output|; s|/|\\\\|g')" \
    FUTURE_WINDOW_WIDTH="$WIDTH" \
    FUTURE_WINDOW_HEIGHT="$HEIGHT" \
    ${FUTURE_SEED:+FUTURE_SEED="$FUTURE_SEED"} \
    wine "$BIN_PATH"

# Wine writes to a Z: drive that maps to / inside the container.
# Symlink from the actual output path back into /output if Wine
# wrote to its prefix instead.
if [ ! -f "$OUTPUT" ]; then
    WINE_OUTPUT="${WINEPREFIX:-/root/.wine}/drive_c/users/root/$(basename "$OUTPUT")"
    if [ -f "$WINE_OUTPUT" ]; then
        cp "$WINE_OUTPUT" "$OUTPUT"
    fi
fi

if [ ! -f "$OUTPUT" ]; then
    echo "==> ERROR: output file not produced at $OUTPUT" >&2
    exit 1
fi

echo "==> Capture complete: $(ls -la "$OUTPUT" | awk '{print $5}') bytes"
