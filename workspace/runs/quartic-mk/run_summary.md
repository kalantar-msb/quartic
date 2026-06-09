**Run Summary: `quartic-mk`**
Generated: 2026-06-08T18:37:14.434700+00:00 | Scenario: quartic

**Algorithms**
- **quarticceiling**
  - Source: `algorithms/quartic_ceiling.go`
  - Plugin type: `quartic-ceiling-policy`
  - Description: Parameter-free quartic per-band dispatch ceiling UsageLimitPolicy: ceiling[i] = 1 - i/(N-1) * sat^4.
  - Files: 3 created, 1 modified
- **constantcontrol**
  - Source: `algorithms/constant_ceiling_control.go`
  - Plugin type: `constant-ceiling-control`
  - Description: Parameter-free control-arm UsageLimitPolicy returning ceiling[i]=1.0 for all priority bands; mirrors quarticceiling sibling for framework-overhead isolation.
  - Files: 4 created, 1 modified

**Packages**

- `/Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/workspace/runs/quartic-mk/cluster/pipelinerun-balanced-10-constantcontrol.yaml`
- `/Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/workspace/runs/quartic-mk/cluster/pipelinerun-balanced-10-quartic.yaml`
- `/Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/workspace/runs/quartic-mk/cluster/pipelinerun-balanced-10-quarticceiling.yaml`
- `/Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/workspace/runs/quartic-mk/cluster/pipelinerun-balanced-15-constantcontrol.yaml`
- `/Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/workspace/runs/quartic-mk/cluster/pipelinerun-balanced-15-quartic.yaml`
- `/Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/workspace/runs/quartic-mk/cluster/pipelinerun-balanced-15-quarticceiling.yaml`
- `/Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/workspace/runs/quartic-mk/cluster/pipelinerun-balanced-5-constantcontrol.yaml`
- `/Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/workspace/runs/quartic-mk/cluster/pipelinerun-balanced-5-quartic.yaml`
- `/Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/workspace/runs/quartic-mk/cluster/pipelinerun-balanced-5-quarticceiling.yaml`
- `/Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/workspace/runs/quartic-mk/cluster/pipelinerun-balanced-8-constantcontrol.yaml`
- `/Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/workspace/runs/quartic-mk/cluster/pipelinerun-balanced-8-quartic.yaml`
- `/Users/kalantar/projects/go.workspace/src/github.com/kalantar-msb/quartic/workspace/runs/quartic-mk/cluster/pipelinerun-balanced-8-quarticceiling.yaml`

**Workloads**

- balanced_5
- balanced_8
- balanced_10
- balanced_15

**Checklist**
- [x] Translation complete
- [x] Assembly complete
- [x] validate-assembly passed

**Verdict: READY TO DEPLOY**
