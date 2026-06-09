#!/bin/bash
# Run baseline (constant ceiling) vs treatment (quartic ceiling) comparison.
# Clones BLIS, runs baseline as-is, then applies the treatment patch and re-runs.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BUNDLE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
RESULTS="$BUNDLE_DIR/results"
WORKLOADS="$BUNDLE_DIR/workloads"
PATCH="$SCRIPT_DIR/treatment.patch"
BLIS_DIR="$BUNDLE_DIR/.blis"
BLIS_COMMIT="0195abf"
BLIS_REPO="https://github.com/inference-sim/inference-sim.git"

# Clean previous results
rm -rf "$RESULTS"
mkdir -p "$RESULTS"

# --- Clone and build BLIS ---
if [ ! -d "$BLIS_DIR" ]; then
  echo "Cloning BLIS at commit $BLIS_COMMIT..."
  git clone --quiet "$BLIS_REPO" "$BLIS_DIR"
  cd "$BLIS_DIR"
  git checkout --quiet "$BLIS_COMMIT"
else
  echo "Using existing BLIS clone at $BLIS_DIR"
  cd "$BLIS_DIR"
  git checkout --quiet -- .
  git checkout --quiet "$BLIS_COMMIT"
fi

echo "Building BLIS (baseline)..."
go build -o blis main.go
BLIS="$BLIS_DIR/blis"
echo "Build complete."

# --- Common flags matching llm-d parity (see config.md) ---
COMMON_FLAGS="\
  --model qwen/qwen3-14b \
  --latency-model trained-physics \
  --max-model-len 40960 \
  --flow-control \
  --saturation-detector utilization \
  --queue-depth-threshold 5 \
  --kv-cache-util-threshold 0.8 \
  --dispatch-order priority"

# --- Run function ---
TIMING_LOG="$RESULTS/timing.txt"
: > "$TIMING_LOG"

run_blis() {
  local workload=$1 output=$2 seed=$3 instances=$4
  local start=$(python3 -c "import time; print(time.time())")
  (cd "$BLIS_DIR" && ./blis run $COMMON_FLAGS \
    --num-instances "$instances" \
    --workload-spec "$workload" \
    --seed "$seed" \
    --metrics-path "$RESULTS/${output}.json" \
    > "$RESULTS/${output}.txt" 2>/dev/null)
  local end=$(python3 -c "import time; print(time.time())")
  local dur=$(python3 -c "print(f'{$end - $start:.2f}s')")
  echo "  $output: $dur"
  echo "$output $dur" >> "$TIMING_LOG"
}

# Instance count per workload
get_instances() {
  local wl_name=$1
  case $wl_name in
    *_under) echo 5 ;;
    *)       echo 4 ;;
  esac
}

# --- Phase 1: Run baseline (unpatched, constant ceiling) ---
echo ""
echo "════════════════════════════════════════════════════"
echo "  PHASE 1: BASELINE (constant ceiling = llm-d default)"
echo "════════════════════════════════════════════════════"

for workload_file in "$WORKLOADS"/*.yaml; do
  wl_name=$(basename "$workload_file" .yaml)
  instances=$(get_instances "$wl_name")
  echo ""
  echo "--- Workload: $wl_name ($instances instances) ---"
  for seed in ${SEEDS:-42 43 44}; do
    run_blis "$workload_file" "baseline_${wl_name}_s${seed}" "$seed" "$instances"
  done
done

# --- Phase 2: Apply treatment patch, rebuild, and run ---
echo ""
echo "════════════════════════════════════════════════════"
echo "  PHASE 2: TREATMENT (quartic ceiling patch)"
echo "════════════════════════════════════════════════════"

cd "$BLIS_DIR"
echo "Applying treatment patch..."
git checkout --quiet -- .
git apply "$PATCH"
echo "Rebuilding BLIS with quartic ceiling..."
go build -o blis main.go
echo "Build complete."

for workload_file in "$WORKLOADS"/*.yaml; do
  wl_name=$(basename "$workload_file" .yaml)
  instances=$(get_instances "$wl_name")
  echo ""
  echo "--- Workload: $wl_name ($instances instances) ---"
  for seed in ${SEEDS:-42 43 44}; do
    run_blis "$workload_file" "treatment_${wl_name}_s${seed}" "$seed" "$instances"
  done
done

# --- Reset BLIS to clean state ---
cd "$BLIS_DIR"
git checkout --quiet -- .

echo ""
echo "════════════════════════════════════════════════════"
echo "  All runs complete. Run: bash scripts/compare.sh"
echo "════════════════════════════════════════════════════"
echo ""
echo "  Run durations:"
column -t "$TIMING_LOG" | sed 's/^/    /'
