# quartic-mk-2 — Campaign Design Rationale

**Date:** 2026-06-09
**Campaign config:** [quartic-mk-2-campaign.yaml](./quartic-mk-2-campaign.yaml)
**Background:** [nous.md](./nous.md)

## Research question

> Under workloads that approach server saturation on llm-d, does the
> exponential/quartic-ceiling treatment improve metrics for critical-priority
> requests relative to sheddable-priority requests, beyond the run-to-run
> noise observed in repeated baseline executions on the same workload?
>
> Plus four confidence axes that gate every measurement:
> environmental noise, configuration parity, deployment health, runtime sanity.

`nous.md` poses two hypotheses — (1) repeated executions of the same algorithm
are "the same", and (2) under saturation, the treatment helps critical-class
requests. Rather than run two campaigns, this question folds them into one:
the reproducibility hypothesis becomes the **noise floor** that the
treatment-vs-baseline comparison must clear. The hypothesis bundle's
`H-control-negative` arm naturally tests reproducibility (baseline-vs-baseline
should show no real difference), while `H-main` tests the treatment effect.

The four-axis confidence framing is added because the target is a live shared
cluster — silent configuration drift, half-broken pods, or workload-generator
hiccups would manifest as "metric noise" and pollute the comparison. The
`sim2real-check` skill (symlinked into `${repo_path}/.claude/skills/`) is the
canonical tool for the parity and health checks.

## Models

`models.design` and `models.execute_analyze` are both pinned to
`claude-opus-4-7[1m]` — the 1M-context Opus 4.7 variant. The framework
default is `claude-opus-4-6` for design and `claude-sonnet-4-6` for execute.
Two reasons to deviate:

- **Opus on execute_analyze**: trades cost for better recovery from
  cluster-side surprises (failed Tekton runs, missing metrics) — appropriate
  for a live-target campaign where each iteration is expensive and re-runs
  are not cheap.
- **`[1m]` context**: the executor accumulates a lot of tool output across
  one EXECUTE_ANALYZE session (kubectl listings, scenario YAMLs, per-arm
  metrics dumps, `sim2real-check` reports). The 200K default can run hot;
  1M removes that as a failure mode.

`claude-opus-4-8[1m]` is **not** served by the proxy in use (verified
2026-06-09); `claude-opus-4-7[1m]` is. YAML brackets force double-quoting;
nous does not parse the `[1m]` suffix specially — it is passed straight
through to `claude -p --model`.

## Turns and timeouts

- **`max_turns`** is read **only** from `agentic-strategy-evolution/orchestrator/defaults.yaml`
  (`iteration.py:357`, `llm_dispatch.py:449-458`). It is *not* a campaign.yaml
  field and there is no CLI flag. We are leaving it at framework defaults
  (`design: 80`, `execute_analyze: 120`) — bumping it would require editing
  `defaults.yaml`, which affects every campaign on this machine.
- **`--timeout`** is a CLI flag (default 1800s = 30 min per phase call).
  Recommend `--timeout 3600` for this campaign — Tekton PipelineRuns plus
  cluster scheduling can easily eat 20+ minutes of an EXECUTE_ANALYZE turn.

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
  baseline (default in-tree algorithm), treatment (new algorithm; "exponentialceiling"
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

- Router-variant set is closed (baseline / treatment). No sourcing
  new algorithms during the campaign.
- Workload set is closed to `balanced_{20,30,40,50}`. Smaller workloads
  (`balanced_5/8/10/15`) are excluded — they will not stress saturation.
- No code patches. Anything that would require rebuilding an image is
  out of scope for this campaign.

## Smoke-testing before a real run

Recommended sequence to confirm the plumbing works before committing 5 full iterations:

1. **Validate the campaign file parses.**
   ```
   nous validate /Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/nous/quartic-mk-2-campaign.yaml
   ```
   This catches schema errors without launching any agent.

2. **Single iteration, light workload, no auto-approve.**
   ```
   nous run \
     /Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/nous/quartic-mk-2-campaign.yaml \
     --run-id quartic-mk-2-smoke \
     --max-iterations 1 \
     --timeout 3600 \
     -v
   ```
   The `--run-id` override redirects artifacts to `.nous/quartic-mk-2-smoke/`
   so the real run starts clean. Stop at the human design gate once the
   bundle looks reasonable; this proves DESIGN works end-to-end. Approve
   one arm forward to verify EXECUTE_ANALYZE can actually issue a
   PipelineRun and read back metrics. Abort if the bundle is way off.

3. **Once both gates pass cleanly on the smoke run**, launch the real campaign:
   ```
   nous run \
     /Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/nous/quartic-mk-2-campaign.yaml \
     --max-iterations 5 \
     --timeout 3600 \
     -v
   ```

Good cheap pre-flight checks before any of the above:
- `oc whoami` and `oc get ns kalantar-0 kalantar-1 kalantar-2 kalantar-3` succeed
- `oc get pipeline sim2real -n <namespace>` returns the installed Tekton Pipeline
- `${repo_path}/workspace/runs/quartic-mk-2/generated/baseline_config.yaml`
  and the treatment overlay actually exist (run `quartic-mk-2`'s scenario
  generator first if not)
- `claude` CLI is authenticated: `claude -p "say hi"`

## Notes

- `domain_adapter_layer` is set to `null` — the orchestrator
  (`orchestrator/llm_dispatch.py:158`) currently warns and ignores any other
  value. Domain context flows through `target_system.description` and the
  planner reading `nous/nous.md`.
- The `sim2real-check` skill is a symlink at
  `${repo_path}/.claude/skills/sim2real-check` →
  `inference-sim/sim2real/.claude/skills/sim2real-check`. Edits to the
  upstream skill are picked up automatically.
