# Methodology & data-collection notes — for discussion

> Purpose: gather in one place the data, technique, and design improvements that came up
> while reviewing the `quartic-mk` results, plus the multi-pod issues we hit in
> `quartic-jc/results.{1..3}-prefix-caching`. Intended for circulation; not all of these
> need to be done, but each is a real lever and is worth a conversation.
>
> Organized into four sections:
>
> 1. **Data we should collect** (Section 1) — per-request, per-pod, time-series, cluster-state.
> 2. **Statistical techniques** (Section 2) — what we can do once the data exists.
> 3. **Experimental-design changes** (Section 3) — block design, calibration, screening.
> 4. **Prefix/KV-cache contamination** (Section 4) — why block design is more delicate
>    once we move to prefix-shared workloads, and what to do about it.

## Context — where we are today

`quartic-mk` results came from **single-pod-per-deployment** runs. The single-pod choice
was deliberate: an earlier multi-pod run (`quartic-jc/results.{1..3}-prefix-caching`,
4 pods/deployment) showed substantially more noise, and the per-pod analysis we did
afterwards confirmed the noise was driven by EPP routing imbalance creating pod-level
load asymmetry inside each deployment.

That works for this paper, but it leaves us with three problems for follow-up work:

- **The single-pod evidence doesn't generalize automatically to multi-pod.** Reviewers
  will ask whether the policy still works when the EPP picker is operating across pods,
  not just regulating one. We need to be able to answer this affirmatively, with data.
- **We can't fully attribute the few outliers we did see** (e.g., the rep3-treatment-balanced_8
  TPOT shift visible in `tpot_ecdf_by_replicate.png`) — there isn't enough per-pod or
  per-request server-side state to localize the cause.
- **Future workloads with shared prefixes** are very different beasts than our current
  `balanced_X` synthetic load — both because the prefix cache is then a first-class
  performance factor and because it changes what designs are valid (Section 4).

## 1. Data we should collect

### 1.1 Per-request pod attribution (the missing primitive)

Single most-impactful gap. The client-side `trace_data.csv` has per-request timing but no
pod identity. The EPP knows which pod served which request, but the join key isn't
preserved across the boundary: trace uses a small integer `request_id` (203, 214, …),
EPP uses a UUID `x-request-id`, and there's no bridge file emitted by the load generator.

Three viable paths (any one works; first is cheapest):

- **Bridge file from the load generator.** Add a logging line that records
  `(integer_request_id, x-request-id-uuid, send_time_us)` per request. Join to the EPP's
  routing log to attribute each request to a pod.
- **EPP routing-log captured at consistent verbosity by default.** We saw the EPP log has
  the routing decision at debug level (`scheduling/scheduler_profile.go:210` lines), but
  it gets dropped at info-level for the control arm in our quartic-jc data. Standardize
  the verbosity per run and capture into a known location.
- **EPP sidecar CSV.** Have the EPP write a structured CSV alongside `trace_data.csv`
  containing `(x-request-id, pod_name, target_address, decision_score_breakdown, ts_us)`.
  Cleanest long-term answer because the schema is stable; no JSON parsing required.

Without per-request pod attribution, every per-request metric in a multi-pod run is a
mixture across pods. With it, we can stratify everything in Section 2.

### 1.2 Per-pod calibration data (the most underrated)

Before each experiment, run a 60-second **calibration workload** that hits every pod with
identical, known load. Record per pod:

- TTFT and TPOT under the calibration workload (a per-pod baseline)
- GPU UUID, compute capability, observed clock during steady state
- Node name, kernel version, driver version
- Set of co-located pods on the same node (noisy-neighbor risk)
- KV cache state at end of warmup (was prefix-cache primed?)

This buys us three things:

1. **Pre-experiment slow-pod detection** — flag a deployment where one pod is more than
   2σ off its peers on the calibration workload, and redeploy before wasting hours of
   wall-clock on a known-bad cluster.
2. **A per-pod normalizer** — express every measurement as `observed / pod_baseline`,
   factoring out per-pod hardware variation before computing arm effects (Section 2.1).
3. **Quantification of cluster heterogeneity** — gives the prior on how much per-pod
   variance to expect, independent of the policy under test. Useful in the paper as a
   reported metric ("our pods varied by less than X% on calibration TPOT, supporting the
   assumption of homogeneous hardware").

### 1.3 Finer time-series granularity

vLLM's engine summary fires every ~10 seconds. That's coarse:

- Single bad 5-second stalls are invisible.
- Sub-step batch-size effects can't be separated from per-step timing.
- We can't correlate a TTFT spike at second 47 with a queue-depth event at second 49.

Two improvements:

- **1-second engine summaries.** vLLM has a config knob; the cost is more log volume.
- **Per-request server-side timing.** Time-to-prefill, time-per-decode-step,
  batch-position when each request entered/left. vLLM does not log this at any verbosity
  by default — would require either patching vLLM or running with a debug build that emits
  per-request lifecycle events. **This is the single biggest analysis unlock**: most of
  the per-pod questions become trivial with per-request server-side timing.

### 1.4 Cluster-state context

Things that are easy to capture once at the start of a run and currently aren't:

- Node-level metrics (Prometheus snapshot at start/end of run): CPU, memory pressure,
  network I/O, GPU utilization, NUMA topology.
- Pod creation timestamp through "ready" timestamp per pod (the pod-startup-state
  confound we hypothesized about for the rep3 outlier).
- `kubectl get pods -o wide --show-labels` snapshot pinned to the run — gives the
  pod-to-node map directly, without log mining (we ended up extracting node identity
  heuristically from pod IPs in `/24` subnets, which is fragile).

## 2. Statistical techniques

### 2.1 Per-pod baselining

If calibration data exists (Section 1.2), report all results as ratios
`observed / pod_baseline`. The slow pod and the fast pod report the same ratio when both
are equally loaded — which is what we want, because per-pod hardware speed is a pod
property, not a policy property.

This eliminates the dominant noise source we identified in the multi-pod analysis: that
"slow pods" by aggregate `gen_tps` are mostly under-loaded pods, not slow-per-token pods.
With per-pod baselining, the question becomes "did the policy change request placement
relative to baseline" rather than "did request latency change", which is more directly
the thing we want to measure.

### 2.2 Inverse-probability-of-routing weighting (IPW)

The EPP routes preferentially to the highest-scoring pod, so the requests that landed on
each pod aren't a random sample of the workload — they're selected by routing policy.
Fast pods see disproportionately more requests; "easy" requests (small `input_tokens`)
may get routed to whichever pod's KV cache is least full. There's selection bias even
within a single deployment.

If we have routing-decision data with the score breakdown, we can model
`p(this_pod | request_attributes)` and weight each request by `1 / p`. Aggregate metrics
under IPW are unbiased to the routing policy. Standard tool from observational studies;
adds about 50 lines of regression code.

### 2.3 Mixed-effects model with pod as a random effect

Once per-request pod attribution exists, the right framework for a multi-pod paper is:

```
latency_i ~ α + β · arm + γ · workload + (1 | pod) + (1 | replicate) + ε
```

with `pod` and `replicate` modeled as random effects. The fitted variance components
decompose total variance into:

- σ²_arm (the interesting part)
- σ²_workload
- σ²_pod (cluster heterogeneity)
- σ²_replicate (cross-deployment / cluster-transient noise)
- σ²_residual (within-pod request-level noise)

Reviewers like to see this decomposition because it makes uncertainty sources explicit.
It also gives us proper standard errors on `β` (the policy effect) controlled for pod
identity — which is exactly the thing we cannot get from the current marginal analysis.

Standard tooling: `lme4` in R, `statsmodels.MixedLM` or `pymer4` in Python.

### 2.4 Matched-pair / conditional analysis

For each treatment request, find the most-similar baseline request (same `slo_class`,
`input_tokens` within ±5%, `output_tokens` within ±5%, same `prefix_group` if applicable,
near-time arrival), then compute paired Δs. This eliminates workload-mix variance — a
thing the current aggregate analysis silently averages over. Roughly 80% of the columns
in `trace_data.csv` then become covariates rather than nuisance variance sources.

Cheap to add, no new data needed. Probably the highest-leverage statistical change for
the existing single-pod data.

### 2.5 Hierarchical bootstrap (request × pod × replicate)

The bootstrap CIs in the current paper are flat request-level resamples. For multi-pod,
that understates uncertainty: requests within a pod aren't independent (shared scheduler,
shared KV cache state). Resample **pods within deployment** first (with replacement),
then resample **requests within each pod**. This gives CIs that properly account for
both pod-level and request-level variance.

For the single-pod paper, flat bootstrap is fine. For the multi-pod follow-up, hierarchical
bootstrap is the right tool.

### 2.6 Permutation tests on stratified data

Shuffle arm labels within (pod, workload) strata, recompute the test statistic,
build the empirical null distribution from many random shuffles. The p-value drops out
empirically without parametric assumptions about the latency distribution. With 10,000
permutations this takes seconds. Useful as a robustness check alongside Cohen's d.

## 3. Experimental-design changes

### 3.1 Block design with shared pods (with caveats — see Section 4)

The current design uses a fresh deployment per (arm × workload × replicate) cell, which
means pod hardware is a *between-cell* variance source. Switching to a block design —
deploy once, run all three arms back-to-back on the same pod set, then redeploy — moves
pod hardware into *within-cell* (paired). The arm-effect estimator drops in variance
substantially.

This applies cleanly to **non-prefix-shared** workloads. For prefix-shared workloads,
block design introduces KV/prefix-cache contamination between arms, which can be a larger
problem than the per-pod variance it solves. **See Section 4 for the detailed analysis;
Sections 3.1–3.3 here assume non-prefix-shared workloads.**

### 3.2 Pre-experiment calibration and slow-pod screening

Add a calibration phase to every deployment:

1. Deploy the cluster.
2. Run a fixed 60s calibration workload across all pods.
3. Compute per-pod baseline metrics (Section 1.2).
4. If max-min per-pod gen_tps spread > X% (we'd choose X based on cluster history),
   redeploy. Otherwise proceed.

This costs one calibration workload per deployment (small) and removes the deployments
that would otherwise contribute outsized noise. We saw at least one such deployment in
`quartic-jc` (the results.3-control-balanced_30 deployment with the pod 23% slow); the
calibration screen would have caught it before measurement started.

### 3.3 Latin-square arm ordering when block-designing

If we're block-designing (running multiple arms back-to-back), the arm ordering itself
becomes a confound: arm 1 sees a cold pod, arms 2 and 3 see a warmer one. Latin-square
the order across replicates so the cold-cache penalty rotates through arms and averages
out. Standard counterbalancing in within-subject designs.

### 3.4 Vary the workload-generator seed

Currently every replicate uses `workload_seed: 42`. That means our H1 reproducibility
analysis tests *cluster* variance, not *workload* variance. State this explicitly in any
paper, and run a small set of replicates with varied seeds to characterize both.

## 4. Prefix/KV-cache contamination — why block design is delicate

**This is the part that needs colleague-level discussion**: it changes whether block
design (Section 3.1) is the right move for our planned shared-prefix experiments.

### 4.1 Why it matters now

The current `balanced_X` workload has **0.0% prefix cache hit rate** (visible in the EPP
metrics). For workloads where we expect significant common prefix in requests
(chat-system-prompt, RAG with shared retrieval results, agent workflows with repeated
tool descriptions), the prefix cache becomes a first-class performance factor:

| State | TTFT for a 1k-prefix + 100-suffix request |
|---|---|
| Cold prefix cache | ~prefill cost of full 1100 tokens |
| Warm prefix cache (hit) | ~prefill cost of 100-token suffix only |

That ratio is **10×+** on TTFT. If two arms are run back-to-back and the cache state at
their starting points differs, the difference in cache-warmth swamps any 50% policy
effect we were trying to measure.

### 4.2 Three failure modes

1. **First-arm penalty.** Arm 1 starts cold; arms 2/3 inherit warm state from arm 1.
   The arm that ran first looks systematically slower. **Randomizable** — Latin-square
   ordering removes the systematic bias (Section 3.3).
2. **Policy-dependent cache shape.** If treatment admits a different request mix than
   baseline, the prefix cache after treatment has a different distribution of cached
   prefixes than after baseline. Arm 2 inherits this. **Not randomizable** — the bias is
   correlated with the policy under test.
3. **Long-tail eviction asymmetry.** Even with warmup, which prefixes survive eviction
   depends on request order, which depends on policy admission timing. **Not
   randomizable** — same reason as mode 2.

Latin-square handles only mode 1. Modes 2 and 3 require explicit cache-state control.

### 4.3 Mitigation options, in increasing order of intervention

#### Option A — Latin-square only

Cheapest. Handles mode 1, leaves modes 2 and 3. Acceptable if cache-shape effects from
the policy are small relative to overall workload-driven cache state.

#### Option B — Pre-warm to steady state before each arm

Before measurement starts, run a fixed reference workload long enough to fill the prefix
cache to its steady-state eviction equilibrium. Then start arm-1 measurement. After
arm-1, re-run the same reference warmup before arm-2, regardless of what arm-1 left
behind. The deterministic warmup re-imposes a consistent starting state.

Handles all three failure modes if (a) the warmup workload represents the same prefix
distribution as the measurement workload, and (b) it runs long enough that cache state
reaches steady state. Cost: per-arm warmup time (tens of seconds to minutes depending on
cache size and prefix-distribution complexity).

#### Option C — Reset between arms

Restart the vLLM engine (or call its prefix-cache reset endpoint, if available) between
arms. Cleanest semantics. Eliminates most of the variance-reduction benefit of block
design — you're paying re-warmup cost per arm anyway, so the only benefit retained is
shared per-pod hardware identity, not shared cache state. Probably not worth doing if
you're already going to do option B; option B subsumes it.

#### Option D — Cross-arm interleaving

Instead of running arms back-to-back, **interleave requests from different arms within
a single experimental run**. Each request is tagged with its arm-policy at submission;
the EPP applies the appropriate policy. Every arm sees the same cache state at every
moment. The only design that fully controls for cache-state confounds.

Substantial implementation cost: requires a multi-policy-per-request EPP, which is not
how the system currently works. This is more of a "what would we do for a methods paper"
than a near-term option.

#### Option E — Stay with separate deployments

Accept the per-pod variance, pay for variance reduction with **more replicates** instead.
For prefix-shared workloads, this is probably the most defensible choice unless we
implement option B carefully.

### 4.4 Recommendation for prefix-shared workloads

Combine **A + B**:

1. Pre-warm with a fixed reference workload before every arm.
2. Latin-square the arm order across replicates.
3. **Measure cache state at arm boundaries** as a sanity check. Log prefix-cache hit rate
   and KV-block count at the start and end of each arm's measurement window. If those
   differ meaningfully across replicates, contamination has leaked through and we either
   subtract it as a covariate or invalidate that replicate.
4. **Report the cache-warmth metrics in the paper.** Reviewers reading a prefix-shared
   workload paper will ask whether the cache state was controlled — explicit reporting
   heads off the question.

Without B, A alone is insufficient: it leaves modes 2 and 3 confounded with arm.
Without A, B helps for the deterministic warmup but the residual ordering bias
(when the warmup itself is partial) still rotates predictably.

For the **current paper** with `balanced_X` (0% prefix hit rate), this whole section
is moot — block design would have been fine, and switching to it now would be a
methodological improvement at no contamination cost. The only reason not to switch is
that the existing data is already collected, and we'd rather frame the limitation
honestly than rerun.

## 5. Summary table — what to do, what it buys

ROI is "value of the buy" independent of effort (effort is its own column). High = directly
addresses a dominant noise source, gates a follow-up analysis, or pre-empts a likely
reviewer critique. Medium = useful refinement, removes a known bias, or supports an
existing claim. Low = defensive depth — useful when a specific question is asked, but
unlikely to change a headline result.

| Item | Effort | ROI | Buys |
|---|---|---|---|
| **Per-request pod attribution** (Section 1.1) | Low | **High** | Master key — unlocks Section 2.2–2.5 entirely; without it, no multi-pod analysis is rigorous |
| **Per-pod calibration phase** (Section 1.2) | Low | **High** | Detects slow-pod deployments before wasting wall-clock; supplies the per-pod baselining denominator |
| **1-second engine summaries** (Section 1.3) | Low | Medium | Incremental refinement; lets us see transients we currently miss but most analyses survive at 10s |
| **Per-request server timing** (Section 1.3) | High | **High** | Definitive answer to "per-token slow vs under-loaded"; resolves the question we got stuck on in Section 2 of `quartic-jc` |
| **Cluster-state context** (Section 1.4) | Low | Low | Defensive — useful when a reviewer asks "could it be node X?" but rarely changes results |
| **Per-pod baselining** (Section 2.1) | Low | **High** | Removes the dominant variance source identified in the multi-pod data (per-pod hardware speed) |
| **IPW for routing** (Section 2.2) | Medium | Medium | Specific bias correction; matters only when EPP routing is non-uniform |
| **Mixed-effects model** (Section 2.3) | Medium | **High** | Right framework for multi-pod paper; reviewers expect variance decomposition; supplies proper standard errors |
| **Matched-pair analysis** (Section 2.4) | Low | **High** | Works on **existing** data — removes workload-mix variance silently averaged over today |
| **Hierarchical bootstrap** (Section 2.5) | Low | Medium | Corrects CIs in multi-pod; flat bootstrap is acceptable for single-pod |
| **Block design + Latin square** (Section 3.1, 3.3) | Low | Medium | Halves cross-cell variance for non-prefix-shared workloads; doesn't apply (cleanly) to prefix-shared |
| **Pre-experiment calibration & screening** (Section 3.2) | Low | **High** | Eliminates the worst-deployment outliers (we know we had at least one); cheap insurance |
| **Vary workload seeds** (Section 3.4) | Low | Medium | Required to claim H1 honestly separates cluster noise from workload variance |
| **Pre-warm + cache-state instrumentation** (Section 4.4) | Medium | **High** | Gates the entire prefix-shared follow-up — without it, those experiments are uninterpretable |

## Open questions for discussion

1. **Block design or no block design for the multi-pod follow-up?** The variance reduction
   is real, but Section 4 says block design is more delicate for the prefix-shared
   workloads we want to run next. Options: (a) full block design + Section 4.4 protocol;
   (b) per-deployment but with calibration screening (Section 3.2); (c) a hybrid where
   non-prefix-shared workloads are block-designed and prefix-shared ones aren't.

2. **How invasive is per-request server-side timing in vLLM?** This is the single largest
   analysis unlock (Section 1.3) but probably requires patching vLLM. Worth scoping the
   patch effort before committing.

3. **Calibration workload definition.** What's the "fixed reference workload" we'd
   pre-warm with? It needs to be representative of the prefix distribution but
   deterministic in its cache-population behavior. Probably a separate design exercise.

4. **Is the load generator currently emitting bridge-file information?** If yes, we may
   already have a path to per-request pod attribution that we haven't exploited. Worth
   checking before building anything new.

5. **Multi-policy EPP for option D (cross-arm interleaving).** This is the most rigorous
   design but requires real implementation. Is it on anyone's roadmap? If so, our
   experimental work could ride on it; if not, options A + B are what we have.

6. **Do we want a separate methodology paper?** Several of these techniques (per-pod
   baselining, hierarchical bootstrap, the Section 4 cache-contamination analysis) are
   contributions in their own right and could justify a separate "how to evaluate
   priority-aware LLM serving" paper. Or they could be sections of the main paper.
   Different framing.
