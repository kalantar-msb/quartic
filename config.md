# Configuration

## llm-d Real Deployment

To reproduce the experiment on a real llm-d cluster, deploy with this setup.

### vLLM Pod Configuration

| Parameter | Value | Notes |
|---|---|---|
| Model | `Qwen/Qwen3-14B` | |
| GPU | H100-SXM-80GB | |
| `tensor_parallel_size` | 1 | |
| `max_num_seqs` | 256 | Max concurrent requests per pod |
| `max_num_batched_tokens` | 2048 | Chunked prefill budget |
| `block_size` | 16 | KV cache block size in tokens |
| `gpu_memory_utilization` | 0.9 | |
| `max_model_len` | 40960 | |
| `enable_chunked_prefill` | True | |
| Number of pods | 1 | |

### llm-d EPP Configuration (Baseline)

Flow control enabled with default constant ceiling. Default llm-d routing (load-aware scoring):

```yaml
apiVersion: inference.networking.x-k8s.io/v1alpha1
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

### llm-d EPP Configuration (Treatment)

Add the quartic ceiling plugin (see README for full Go code):

```yaml
apiVersion: inference.networking.x-k8s.io/v1alpha1
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

### Priority Bands (InferenceObjective CRs)

Two tiers used across all workloads:

| Tier | Priority value | Role |
|---|---|---|
| critical | 100 | Protected — ceiling stays at 1.0 under load |
| sheddable | -50 | Low-priority — gated earliest, held in queue |

```yaml
apiVersion: inference.networking.x-k8s.io/v1alpha2
kind: InferenceObjective
metadata:
  name: critical
spec:
  priority: 100
---
apiVersion: inference.networking.x-k8s.io/v1alpha2
kind: InferenceObjective
metadata:
  name: sheddable
spec:
  priority: -50
```

### Real-Cluster Load Generator

Use `blis observe` to send workload to the real cluster:

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

| Flag | Value | Notes |
|---|---|---|
| `--max-concurrency` | 10000 | True open-loop; default 256 masks overload |
| `--warmup-requests` | 200 | Discard cold-start requests |
| `--timeout` | 900 | 15 min per-request timeout |
| `--workload-spec` | `workloads/<name>.yaml` | Same workload files used in simulation |

---

## BLIS Simulation

The scripts (`scripts/run.sh`) use these BLIS flags:

| Flag | Value | Notes |
|---|---|---|
| `--model` | qwen/qwen3-14b | |
| `--latency-model` | trained-physics | Physics-informed with learned corrections |
| `--max-model-len` | 40960 | |
| `--flow-control` | (enabled) | |
| `--saturation-detector` | utilization | |
| `--queue-depth-threshold` | 5 | |
| `--kv-cache-util-threshold` | 0.8 | |
| `--dispatch-order` | priority | |
| `--num-instances` | 4 | |

BLIS defaults used (not overridden, auto-derived from model/hardware):

| Parameter | Value | Corresponds to vLLM |
|---|---|---|
| `--max-num-running-reqs` | 256 | `max_num_seqs=256` |
| `--max-num-scheduled-tokens` | 2048 | `max_num_batched_tokens=2048` |
| `--block-size-in-tokens` | 16 | `block_size=16` |
| `--gpu-memory-utilization` | 0.9 | `gpu_memory_utilization=0.9` |
| `--total-kv-blocks` | auto-calculated | Derived from model size, GPU memory, gpu_memory_utilization |

Treatment is applied via `scripts/treatment.patch` (one-line formula change in `DequeueGated()`).

## Commits

| Component | Commit | Notes |
|---|---|---|
| BLIS (scripts) | `0195abf` | Latest main |
| llm-d (code proofs) | `2072f90a` | Source references in README |
| GAIE | `9d2dfe9b` | Latest main |
| Nous (campaign) | `flow-control-fixed` | 8 iterations, all confirmed |
