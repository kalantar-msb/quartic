# quartic-mk-2 — Campaign Design Rationale

**Date:** 2026-06-09
**Campaign config:** [quartic-mk-2-campaign.yaml](./quartic-mk-2-campaign.yaml)
**Background:** [nous.md](./nous.md)

## Research question

> Under workloads that approach server saturation on llm-d, does the
> exponential/quartic-ceiling treatment improve metrics for critical-priority
> requests relative to sheddable-priority requests, beyond the run-to-run
> noise observed in repeated baseline executions on the same workload?

`nous.md` poses two hypotheses — (1) repeated executions of the same algorithm
are "the same", and (2) under saturation, the treatment helps critical-class
requests. Rather than run two campaigns, this question folds them into one:
the reproducibility hypothesis becomes the **noise floor** that the
treatment-vs-baseline comparison must clear. The hypothesis bundle's
`H-control-negative` arm naturally tests reproducibility (baseline-vs-baseline
should show no real difference), while `H-main` tests the treatment effect.

## Target-system framing

- **`live_target: true`.** The target is a running cluster (pokprod001), not a
  codebase to mutate. Algorithm variants ship as pre-built container images;
  arms are probe-style (config overlay + Tekton PipelineRun), never
  `code_changes`. `nous.md` explicitly forbids changes to system-wide
  resources or namespaces outside `kalantar-{0..3}`.
- **`repo_path` = quartic root.** Gives the planner direct read access to
  `nous/nous.md`, `workloads/`, `baselines/`, and
  `workspace/runs/quartic-mk-2/`. Artifacts will land in
  `${EXPERIMENT_ROOT}/.nous/quartic-mk-2/`.
- **Three router variants** (image + overlay path inlined into
  `target_system.description` so the planner does not have to grep):
  baseline (default in-tree algorithm), control (plugin re-implementing the
  default — neutrality check), treatment (new algorithm; "exponentialceiling"
  and "quarticceiling" are the same arm under two names).
- **Knobs hint** the design space: `router_variant`, `workload`, `namespace`,
  `replication_count`. The planner decides which cells to fill and how many
  reps per cell.
- **Metric hints** are the five listed in `nous.md` (TTFT p50/p95, E2E p50/p95,
  TPOT mean), each per priority class. Per-class reporting is what makes the
  treatment hypothesis falsifiable.

## Iteration count

`max_iterations: 5`. A reasonable arc:

1. Characterize baseline noise on one workload (probably `balanced_30`).
2. Find the saturation knee — sweep workloads `balanced_{20,30,40,50}`.
3. Treatment vs. baseline at the saturation cell, single rep.
4. Treatment vs. baseline with reps to clear the noise floor from iter 1.
5. Slack for whatever iter 1-4 surfaces (re-target, more reps, ablation, etc.).

Iter ordering is illustrative — the planner designs each bundle from the
latest principles, not from this list.

## Decisions deliberately deferred to the planner

- Number of replications per condition.
- Which workload is "near saturation" — `nous.md` does not say.
- Whether to vary namespace deliberately or treat it as a nuisance variable.
- Statistical test for "metrics are similar" (CI overlap? bootstrap? Welch's t?).
- Whether to exercise priority-class mix knobs inside the workload yaml.

## Decisions deliberately locked

- Router-variant set is closed (baseline / control / treatment). No sourcing
  new algorithms during the campaign.
- Workload set is closed to `balanced_{20,30,40,50}`. Smaller workloads
  (`balanced_5/8/10/15`) are excluded — they will not stress saturation.
- No code patches. Anything that would require rebuilding an image is
  out of scope for this campaign.

## Notes

- `domain_adapter_layer` is set to `null` — the orchestrator
  (`orchestrator/llm_dispatch.py:158`) currently warns and ignores any other
  value. Domain context flows through `target_system.description` and the
  planner reading `nous/nous.md`.
