#!/usr/bin/env bash
#
# profile-compare.sh — Compare two profile captures side by side.
#
# Takes two directories produced by profile.sh (each containing cpu.pb.gz
# and allocs.pb.gz) and prints a structured comparison showing which
# functions got faster/slower and which allocations grew/shrank.
#
# Usage:
#   ./scripts/profile-compare.sh <baseline-dir> <optimized-dir>
#
# Example:
#   ./scripts/profile.sh -n baseline
#   # ... make changes ...
#   ./scripts/profile.sh -n optimized
#   ./scripts/profile-compare.sh testdata/profiles/baseline testdata/profiles/optimized
#
# Output:
#   - Side-by-side CPU top functions
#   - Side-by-side allocation top functions
#   - Delta summary table for key hotspots (bindUniforms, packUniforms, etc.)
#   - Total allocation comparison
#
set -euo pipefail

if [ $# -lt 2 ]; then
    echo "Usage: $0 <baseline-dir> <optimized-dir>" >&2
    echo "" >&2
    echo "Both directories should contain cpu.pb.gz and allocs.pb.gz" >&2
    echo "as produced by scripts/profile.sh." >&2
    exit 1
fi

BASE="$1"
OPT="$2"

# Validate inputs.
for f in cpu.pb.gz allocs.pb.gz; do
    if [ ! -f "${BASE}/${f}" ]; then
        echo "ERROR: ${BASE}/${f} not found" >&2
        exit 1
    fi
    if [ ! -f "${OPT}/${f}" ]; then
        echo "ERROR: ${OPT}/${f} not found" >&2
        exit 1
    fi
done

BASE_NAME="$(basename "$BASE")"
OPT_NAME="$(basename "$OPT")"

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║  Profile Comparison: ${BASE_NAME} → ${OPT_NAME}"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

# --- CPU comparison ---
echo "┌──────────────────────────────────────────────────────────────┐"
echo "│  CPU Profile (top by cumulative time)                        │"
echo "└──────────────────────────────────────────────────────────────┘"
echo ""

echo "--- ${BASE_NAME} (baseline) ---"
go tool pprof -top -cum "${BASE}/cpu.pb.gz" 2>&1 | head -20
echo ""
echo "--- ${OPT_NAME} (optimized) ---"
go tool pprof -top -cum "${OPT}/cpu.pb.gz" 2>&1 | head -20
echo ""

# --- Alloc comparison ---
echo "┌──────────────────────────────────────────────────────────────┐"
echo "│  Allocation Profile (top by flat alloc_space)                │"
echo "└──────────────────────────────────────────────────────────────┘"
echo ""

echo "--- ${BASE_NAME} (baseline) ---"
go tool pprof -top "${BASE}/allocs.pb.gz" 2>&1 | head -20
echo ""
echo "--- ${OPT_NAME} (optimized) ---"
go tool pprof -top "${OPT}/allocs.pb.gz" 2>&1 | head -20
echo ""

# --- Key hotspot delta table ---
echo "┌──────────────────────────────────────────────────────────────┐"
echo "│  Key Hotspot Delta                                           │"
echo "└──────────────────────────────────────────────────────────────┘"
echo ""

# Helper: extract a metric for a function from a profile.
# $1 = profile path, $2 = function substring, $3 = column (flat=1, cum=5 for -top -cum)
extract_cpu_cum() {
    go tool pprof -top -cum "$1" 2>&1 | grep "$2" | head -1 | awk '{print $5}' || echo "—"
}

extract_alloc_flat() {
    go tool pprof -top "$1" 2>&1 | grep "$2" | head -1 | awk '{print $1}' || echo "—"
}

extract_alloc_cum() {
    go tool pprof -top "$1" 2>&1 | grep "$2" | head -1 | awk '{print $5}' || echo "—"
}

extract_total() {
    go tool pprof -top "$1" 2>&1 | grep "total" | head -1 | sed 's/.*of //' | awk '{print $1}' || echo "—"
}

printf "%-35s  %12s  %12s\n" "Function" "${BASE_NAME}" "${OPT_NAME}"
printf "%-35s  %12s  %12s\n" "---" "---" "---"

# CPU cumulative for key functions.
for func in "bindUniforms" "SpritePass.*Execute" "DrawIndexed" "QueueWriteBuffer" "SetPipeline" "writeUniformRing"; do
    base_val=$(extract_cpu_cum "${BASE}/cpu.pb.gz" "$func")
    opt_val=$(extract_cpu_cum "${OPT}/cpu.pb.gz" "$func")
    printf "%-35s  %12s  %12s  (CPU cum)\n" "$func" "$base_val" "$opt_val"
done

echo ""

# Alloc flat for key functions.
for func in "packUniforms" "SetUniformMat4" "bindUniforms" "Batcher.*Flush" "DeviceCreateBindGroup"; do
    base_val=$(extract_alloc_flat "${BASE}/allocs.pb.gz" "$func")
    opt_val=$(extract_alloc_flat "${OPT}/allocs.pb.gz" "$func")
    printf "%-35s  %12s  %12s  (alloc flat)\n" "$func" "$base_val" "$opt_val"
done

echo ""

# Alloc cumulative for bindUniforms.
base_cum=$(extract_alloc_cum "${BASE}/allocs.pb.gz" "bindUniforms")
opt_cum=$(extract_alloc_cum "${OPT}/allocs.pb.gz" "bindUniforms")
printf "%-35s  %12s  %12s  (alloc cum)\n" "bindUniforms" "$base_cum" "$opt_cum"

echo ""

# Total allocation.
base_total=$(extract_total "${BASE}/allocs.pb.gz")
opt_total=$(extract_total "${OPT}/allocs.pb.gz")
printf "%-35s  %12s  %12s\n" "TOTAL alloc_space" "$base_total" "$opt_total"

echo ""
echo "For interactive exploration:"
echo "  go tool pprof -diff_base=${BASE}/cpu.pb.gz ${OPT}/cpu.pb.gz"
echo "  go tool pprof -diff_base=${BASE}/allocs.pb.gz ${OPT}/allocs.pb.gz"
