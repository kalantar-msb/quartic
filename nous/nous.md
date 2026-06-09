NOUS CAMPAIGN

**Background**

llm-d is a distributed inferencing engine which uses llm-d-router (github.com/llm-d/llm-d-router) for request routing and vllm (https://github.com/vllm-project/vllm) for inference engines.

To deploy an llm-d instance, define a scenario. This is a declarative yaml file to specify the system. A scenario can be defined as an overlay to a default definition. In general, overlays may be applied one after another. For example, one overlay may define the configuration of the vllm instances. A second may define a default router configuration. A third might specialize the router configuration.

**Target System**

Our base vllm scenario definition is in baselines/*.yaml

llm-d-router implements a default flow control algorithm.
We call this the baseline implementation.
An image embedding the default implementation is ghcr.io/kalantar/llm-d-router:<commit-id>> where commit-id is the short form of git commit id of the llm-d-router submodule.
To deploy it, use the default router overlay:
${EXPERIMENT_ROOT}/workspace/runs/${RUN}/generated/baseline_config.yaml (which is applied on top of the base vllm scenario definition)

We defined a plugin that reimplements this default behavior. We call it the "control". It is available in image ghcr.io/kalantar/llm-d-router:${RUN}-constantcontrol
To deploy and configure it use the scenario overlay
${EXPERIMENT_ROOT}/workspace/runs/${RUN}/generated/constantcontrol/constantcontrol_config.yaml
(which should be applied over the baseline_config and over the vllm spec)

We defined a plugin that implements a new flow control agorithm. We call it the "treatment". It is available in image ghcr.io/kalantar/llm-d-router:${RUN}-exponentialceiling
To deploy and configure it use the scenario overlay:
${EXPERIMENT_ROOT}/workspace/runs/${RUN}/generated/quarticceiling/quarticceiling_config.yaml
(which should be applied over the baseline_config and over the vllm spec)

We have a number of workloads in ${EXPERIMENT_ROOT}/workloads:
- ${EXPERIMENT_ROOT}/workloads/balanced_20.yaml
- ${EXPERIMENT_ROOT}/workloads/balanced_30.yaml
- ${EXPERIMENT_ROOT}/workloads/balanced_40.yaml
- ${EXPERIMENT_ROOT}/workloads/balanced_50.yaml

**Creating a Target System**

Target systems can be deployed using openshift cluster `pokprod001` using namespaces `kalantar-0`, `kalantar-1`, `kalantar-2`, and `kalantar-3`. Note that each system requires 4 GPUs; avoid deploying too many instances so as to consume all of the available GPU. When calculating available GPU exclude nodes that are cordoned off or that have taints.

**IMPORTANT**: NO CHANGES TO SYSTEM WIDE RESOURCES OR OTHER NAMESPACES ARE PERMITTED.

A system can be create using the installed Tekton `Pipeline`: `sim2real`.
A Tekton `PipelineRun` must be used. Examples for this workload are:
${EXPERIMENT_ROOT}/workspace/runs/${RUN}/cluster/pipelinerun-<workload>-<package>.yaml

The pipeline deploys the llm-d instance, runs a workload against it (as specified in the PipelineRun) and removes the llm-d instance.  The data for the run can be found on the PVC `data-pvc` in the target namespace in the folder `/data/<runName>/<phase>/<workloadName>" where runName, phase and workloadName are all pipeline parameters. 

**Research Question**

We want determine a set if workloads (we can modify what we have) so that we can satisfy this hypotheiss:

1. For the same workload:
    - Repeated executions of the baseline algorithm are "the same"
    - Repeated executions of the treatment algorithm are "the same"
    
    By the same, we mean that all metrics (TTFT p50, TTFT p95, E2E p50, E2E p95, TPOT mean) are similar. Indeed, in the absence of errors, differences should be caused by differences in the cluster at the time of execution.


2. The treatment will differ from the baseline. In particular:
- when the workload is close to causing or does cause server saturation, requests of the "critical" priority class should degrade less than those of the "sheddable" priority class.  That is, metrics for the critical workload should show improvement with treatment as compared to the baseline.


To demonstrate this, a combination of workload configuration, experiment design and statistical rigour can be applied.
