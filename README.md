# Sim2Real Bundle: Quartic Per-Band Dispatch Ceiling (Parameter-Free)

Simulation-evolved dispatch ceiling policy that reduces critical-band P99 TTFT by **37-98%** under load, with ≤10% throughput penalty universally. Transferable to llm-d as a `UsageLimitPolicy` plugin.

## File Structure

```
quartic/
  algorithms/
    quartic_ceiling.go          — Treatment plugin (Go, llm-d compatible)
    constant_ceiling_control.go — Control plugin (ceiling=1.0, same interface)
  workloads/                    — Workload YAMLs (balanced shape × 4 load levels)
  scripts/
    run.sh                      — Clone BLIS, build, run baseline + treatment
    compare.sh                  — Parse results, print comparison table
    treatment.patch             — One-line code patch for BLIS
  results/                      — Simulation output (populated by run.sh)
  config.md                     — Deployment config (vLLM, llm-d, BLIS flags)
  README.md                     — This file
```

## Algorithm: Quartic Ceiling (Treatment)

**Source**: [`algorithms/quartic_ceiling.go`](algorithms/quartic_ceiling.go)

```
ceiling[i] = 1.0 - i/(N-1) * sat^4
```

- `N` = number of active priority bands
- `sat` = pool-wide saturation (0.0 to 1.0)
- `i` = band position (0 = highest priority, N-1 = lowest)

| Saturation | Critical (i=0) | Sheddable (i=1, N=2) |
|------------|---------------|---------------------|
| 0.0 | 1.0 | 1.0 |
| 0.5 | 1.0 | 0.9375 |
| 0.8 | 1.0 | 0.5904 |
| 0.9 | 1.0 | 0.3439 |
| 1.0 | 1.0 | 0.0 |

Parameter-free: uses only saturation and band count, both already in the dispatch path. No `math` import needed — sat^4 is inline multiplication.

## Algorithm: Constant Ceiling Control

**Source**: [`algorithms/constant_ceiling_control.go`](algorithms/constant_ceiling_control.go)

Returns ceiling=1.0 for all bands via the same plugin interface. Deploy alongside treatment to confirm plugin framework introduces no behavioral difference vs default llm-d.

## How to Transfer to llm-d

### Signal Mapping

| Signal | llm-d accessor |
|--------|--------------|
| Saturation | `SaturationDetector.Saturation(ctx, pool)` — `avg(max(QD/5, KV/0.8))` |
| Priority | `InferenceObjective.Spec.Priority` |
| Band count | `shard.AllOrderedPriorityLevels()` — dynamic, descending order |

### Priority Tiers (InferenceObjective CRs)

| Tier | Priority | Role |
|------|----------|------|
| critical | 100 | Protected — ceiling stays at 1.0 under load |
| sheddable | -50 | Low-priority — gated earliest, held in queue |

```yaml
apiVersion: llm-d.ai/v1alpha2
kind: InferenceObjective
metadata:
  name: critical
spec:
  priority: 100
---
apiVersion: llm-d.ai/v1alpha2
kind: InferenceObjective
metadata:
  name: sheddable
spec:
  priority: -50
```

### Building llm-d with the Plugin

Build from llm-d-inference-scheduler (`llm-d-router`) at commit `2072f90a` or later. The plugin requires no new dependencies — no imports beyond stdlib and the llm-d framework. After adding the file and registration line below, build the EPP binary as usual (`make build` or `go build ./cmd/epp/`).

### Transfer: Treatment (Quartic Ceiling)

**Step 1.** Create `pkg/epp/framework/plugins/flowcontrol/usagelimits/quarticceiling/policy.go`:

```go
package quarticceiling

import (
	"context"
	"encoding/json"

	"github.com/llm-d/llm-d-router/pkg/epp/framework/interface/plugin"
	"github.com/llm-d/llm-d-router/pkg/epp/framework/plugins/flowcontrol/usagelimits"
)

const PolicyType = "quartic-ceiling-policy"

func Factory(name string, _ json.RawMessage, _ plugin.Handle) (plugin.Plugin, error) {
	return usagelimits.NewPolicyFunc(name, computeQuarticCeilings), nil
}

func computeQuarticCeilings(_ context.Context, saturation float64, priorities []int) []float64 {
	n := len(priorities)
	ceilings := make([]float64, n)
	if n <= 1 {
		if n == 1 {
			ceilings[0] = 1.0
		}
		return ceilings
	}
	sat4 := saturation * saturation * saturation * saturation
	for i := range priorities {
		ceilings[i] = 1.0 - float64(i)/float64(n-1)*sat4
	}
	return ceilings
}
```

**Step 2.** Register in `cmd/epp/runner/runner.go` (near line 509, next to existing registrations):

```go
import "github.com/llm-d/llm-d-router/pkg/epp/framework/plugins/flowcontrol/usagelimits/quarticceiling"

fwkplugin.Register(quarticceiling.PolicyType, quarticceiling.Factory)
```

**Step 3.** EndpointPickerConfig YAML:

```yaml
apiVersion: llm-d.ai/v1alpha1
kind: EndpointPickerConfig
featureGates:
  - flowControl
plugins:
  - type: quartic-ceiling-policy
    name: quartic-ceiling
  - type: queue-scorer
  - type: kv-cache-utilization-scorer
  - type: prefix-cache-scorer
schedulingProfiles:
  - name: default
    plugins:
      - pluginRef: queue-scorer
        weight: 2.0
      - pluginRef: kv-cache-utilization-scorer
        weight: 2.0
      - pluginRef: prefix-cache-scorer
        weight: 3.0
flowControl:
  usageLimitPolicyPluginRef: "quartic-ceiling"
```

### Transfer: Baseline (Default llm-d)

No custom plugin. Flow control enabled with default constant ceiling (1.0 for all bands):

```yaml
apiVersion: llm-d.ai/v1alpha1
kind: EndpointPickerConfig
featureGates:
  - flowControl
plugins:
  - type: queue-scorer
  - type: kv-cache-utilization-scorer
  - type: prefix-cache-scorer
schedulingProfiles:
  - name: default
    plugins:
      - pluginRef: queue-scorer
        weight: 2.0
      - pluginRef: kv-cache-utilization-scorer
        weight: 2.0
      - pluginRef: prefix-cache-scorer
        weight: 3.0
flowControl: {}
```

### Transfer: Control (Constant Ceiling via Plugin)

Same 1.0 ceiling delivered through custom plugin framework. Should match baseline exactly — if not, framework bug.

```yaml
apiVersion: llm-d.ai/v1alpha1
kind: EndpointPickerConfig
featureGates:
  - flowControl
plugins:
  - type: constant-ceiling-control
    name: constant-control
  - type: queue-scorer
  - type: kv-cache-utilization-scorer
  - type: prefix-cache-scorer
schedulingProfiles:
  - name: default
    plugins:
      - pluginRef: queue-scorer
        weight: 2.0
      - pluginRef: kv-cache-utilization-scorer
        weight: 2.0
      - pluginRef: prefix-cache-scorer
        weight: 3.0
flowControl:
  usageLimitPolicyPluginRef: "constant-control"
```

### Load Generator (blis observe)

```bash
blis observe \
  --server-url http://<epp-endpoint>:8000 \
  --model qwen/qwen3-14b \
  --workload-spec workloads/<workload>.yaml \
  --max-concurrency 10000 \
  --warmup-requests 200 \
  --timeout 900 \
  --trace-header trace.yaml \
  --trace-data trace.csv
```

- **`--max-concurrency 10000`**: True open-loop arrival. Default 256 causes client-side queueing that masks overload.
- **`--warmup-requests 200`**: Discard initial requests to avoid cold-start artifacts.
- **`--timeout 900`**: Per-request HTTP timeout (15 min). Prevents premature timeouts under overload.

### Deployment Plan

Three variants to deploy and compare:

1. **Default llm-d** — `flowControl: {}` (constant ceiling 1.0, no custom plugin)
2. **Constant Control** — same 1.0 ceiling via `constant-ceiling-control` plugin
3. **Quartic** — `1.0 - i/(N-1) * sat^4` via `quartic-ceiling-policy` plugin

Comparing (1) vs (2) isolates plugin framework overhead. Comparing (2) vs (3) isolates the algorithm improvement.

All three use default llm-d routing (queue-scorer + kv-cache-utilization-scorer + prefix-cache-scorer with weights 2/2/3). The ceiling policy is the only variable.

## Config

See [`config.md`](config.md) for vLLM pod arguments, llm-d configuration, and BLIS simulation flags.


## Workloads

One workload shape (balanced) at four load levels, targeting different saturation regimes:

| File | Rate | Requests | Expected Saturation | Purpose |
|------|------|----------|--------------------:|---------|
| `balanced_20.yaml` | 20 QPS | 10,000 | ~0.2 (dormant) | Below capacity — quartic inactive |
| `balanced_30.yaml` | 30 QPS | 15,000 | ~0.3-0.4 | Around capacity — quartic engages on bursts |
| `balanced_40.yaml` | 40 QPS | 20,000 | ~0.5-0.7 | Overloaded — quartic actively gating |
| `balanced_50.yaml` | 50 QPS | 25,000 | ~0.8+ | Heavy overload — aggressive gating |

All workloads: 50% critical / 50% sheddable, Gaussian input (mean=1024, std=256), Gaussian output (mean=256, std=64), Poisson arrivals, streaming=true, seed=42.

Binary tiers: critical (priority 100, protected) and sheddable (priority -50, gated).

Instance count: 4 pods (Qwen3-14B on H100 TP=1).

## Code References (llm-d @ `2072f90a`)

| Component | File | Lines |
|-----------|------|-------|
| UsageLimitPolicy interface | `pkg/epp/framework/interface/flowcontrol/plugins.go` | 139-178 |
| Dispatch cycle + ComputeLimit call | `pkg/epp/flowcontrol/controller/internal/processor.go` | 322-375 |
| HoL blocking check | Same file | 340-347 |
| Dispatch ticker (1ms) | Same file | 179 |
| NewConstPolicy (default) | `pkg/epp/framework/plugins/flowcontrol/usagelimits/usagelimitpolicy.go` | 60-68 |
| NewPolicyFunc helper | Same file | 71 |
| Plugin registration | `cmd/epp/runner/runner.go` | 509 |
| Saturation detector | `pkg/epp/framework/plugins/flowcontrol/saturationdetector/utilization/detector.go` | 115-137 |
| Default thresholds (QD=5, KV=0.8) | Same package, `config.go` | 29-33 |
| Band ordering (descending) | `pkg/epp/flowcontrol/registry/shard.go` | 157-160 |
| Policy selection | `pkg/epp/flowcontrol/config.go` | 62-76 |
| FeatureGate constant | `pkg/epp/flowcontrol/config.go` | 29 |
| EndpointPickerConfig API | `apix/config/v1alpha1/endpointpickerconfig_types.go` | FlowControlConfig struct |

## Evolution History

Discovered via 8 iterations of Nous campaign (`flow-control-fixed`):

1. **Linear** (iter 1): `1 - i/(N-1) * sat` — too aggressive, 14-19% throughput hit
2. **Quadratic** (iter 2): `1 - i/(N-1) * sat²` — fails at >50% sheddable fraction
3. **Quartic** (iter 3): `1 - i/(N-1) * sat⁴` — minimum integer exponent passing ≤10% universally
4. **Multi-band** (iter 4): Extended to 4 bands with `i/(N-1)` normalization, zero new parameters
5. **Final validation** (iter 5): 6 workloads × 2 seeds, all pass
6. **Detector invariance** (iter 6): queue-depth-threshold variation produces ≤0.66pp difference
7. **Burst recovery** (iter 7): Bursty workloads pass; stateless formula handles transitions trivially
8. **Full regression** (iter 8): 8 overloaded + 4 under-capacity controls, all pass — campaign complete
