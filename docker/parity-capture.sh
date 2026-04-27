#!/usr/bin/env bash
#
# parity-capture.sh — run a single headless capture of the future demo
# binary inside the lavapipe Docker container, with the same env-var
# interface as scripts/capture.sh on the host.
#
# Used by the docker-compose `vulkan-parity` service. Not intended for
# host use — the corresponding host wrapper is
# future-meta/scripts/capture-lavapipe.sh.
#
# Inputs (all read from environment, set by `docker compose run`):
#
#   FUTURE_SCENE                 scene id (required)
#   FUTURE_CORE_HEADLESS         frame count for headless capture (required)
#   FUTURE_CORE_HEADLESS_OUTPUT  output PNG path (defaults to /output/capture.png)
#   FUTURE_WINDOW_WIDTH          window width (default 1920)
#   FUTURE_WINDOW_HEIGHT         window height (default 1080)
#   FUTURE_SEED                  optional global RNG seed (forwarded as-is)
#   FUTURE_PARITY_REBUILD        if "1", rebuild the binary even when cached
#
# The binary is built once per container start and cached at
# /tmp/future-bin. Subsequent `docker compose run` calls reuse the
# cached binary because Docker volumes don't persist /tmp across
# restarts but `go build` is fast on warm caches anyway.

set -euo pipefail

: "${FUTURE_SCENE:?FUTURE_SCENE is required}"
: "${FUTURE_CORE_HEADLESS:?FUTURE_CORE_HEADLESS is required (frame count)}"

OUTPUT="${FUTURE_CORE_HEADLESS_OUTPUT:-/output/capture.png}"
WIDTH="${FUTURE_WINDOW_WIDTH:-1920}"
HEIGHT="${FUTURE_WINDOW_HEIGHT:-1080}"
REBUILD="${FUTURE_PARITY_REBUILD:-0}"

# Build the future binary against the local workspace future-core.
# go.work in /workspace/meta resolves future-core to the bind-mounted
# directory, so any uncommitted future-core change on the host is
# exercised here.
#
# /cached-bin is a persistent named volume (see docker-compose.yml's
# parity-bin-cache). Without it, /tmp gets thrown away by
# `docker compose run --rm` and every scene of a sweep rebuilds the
# binary — a 10-20s cost per scene that adds up fast.
#
# Cache invalidation: rebuild whenever the future binary is missing,
# OR when any source file under /workspace/meta/{future,future-core}
# is newer than the cached binary. This is coarse but correct — we
# never serve a stale binary, even when the user iterates on engine
# code mid-sweep.
BIN=/cached-bin/future-bin
need_build=0
if [ "$REBUILD" = "1" ] || [ ! -x "$BIN" ]; then
  need_build=1
else
  # Find any source file newer than the binary; if so, rebuild.
  # Watch *.go (regular sources) AND embedded asset types:
  #   .kage    — kage shaders (//go:embed'd into shader.go)
  #   .json    — component configs and demo resources
  #   .png     — textures and util/text.png debug font
  #   .ttf     — fonts under future/libs/font/resources/fonts
  #   .md      — embedded via wildcard patterns in examples/embed.go
  # Without these, an edit to a covered asset leaves the cached binary
  # stale and parity-diff measures the OLD asset's output against the
  # freshly-rebuilt host binary — silent divergence.
  newer_count=$(find /workspace/meta/future /workspace/meta/future-core \
                  -type f \( -name "*.go" -o -name "*.kage" -o -name "*.json" \
                          -o -name "*.png" -o -name "*.ttf" -o -name "*.md" \) \
                  -newer "$BIN" 2>/dev/null | head -1 | wc -l)
  if [ "$newer_count" -gt 0 ]; then
    echo "==> Source files newer than cached binary; rebuilding"
    need_build=1
  fi
fi
if [ "$need_build" = "1" ]; then
  echo "==> Building future binary (lavapipe container) → $BIN"
  mkdir -p "$(dirname "$BIN")"
  cd /workspace/meta/future
  go build -tags futurecore -o "$BIN" ./cmd/driver/
else
  echo "==> Reusing cached future binary at $BIN"
fi

# Sanity-print the resolved Vulkan ICD so capture logs make it obvious
# which driver landed in this container. `vulkaninfo --summary` is a
# few hundred ms; cheap to keep.
echo "==> Vulkan ICD:"
vulkaninfo --summary 2>/dev/null | grep -A1 "Devices:" | tail -2 | sed 's/^/    /'

# Ensure the output directory exists and is writable. The compose
# service mounts /output to a host directory; if the host dir is
# missing, the bind-mount creates an empty one we can write to.
mkdir -p "$(dirname "$OUTPUT")"
rm -f "$OUTPUT"

# Assemble the env. Most vars are already in the environment from
# `docker compose run -e ...`; we just set defaults for the unset
# ones and pass FUTURE_CORE_BACKEND=vulkan explicitly.
echo "==> Running headless capture: scene=$FUTURE_SCENE frames=$FUTURE_CORE_HEADLESS size=${WIDTH}x${HEIGHT}"
echo "    seed=${FUTURE_SEED:-<unset>}"

# xvfb-run gives the binary an X display so GLFW window creation works.
# `-a` auto-picks a free :NN to avoid races when the service is run
# repeatedly back-to-back.
xvfb-run -a env \
  FUTURE_SCENE="$FUTURE_SCENE" \
  FUTURE_CORE_BACKEND=vulkan \
  FUTURE_CORE_HEADLESS="$FUTURE_CORE_HEADLESS" \
  FUTURE_CORE_HEADLESS_OUTPUT="$OUTPUT" \
  FUTURE_WINDOW_WIDTH="$WIDTH" \
  FUTURE_WINDOW_HEIGHT="$HEIGHT" \
  ${FUTURE_SEED:+FUTURE_SEED="$FUTURE_SEED"} \
  "$BIN"

if [ ! -f "$OUTPUT" ]; then
  echo "FATAL: capture did not produce $OUTPUT" >&2
  exit 1
fi

echo "==> Captured $(stat -c%s "$OUTPUT" 2>/dev/null || stat -f%z "$OUTPUT") bytes at $OUTPUT"
