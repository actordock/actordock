# Research: problem

## Problem statement

On a fixed (or slowly changing) pool of Workers, many agent sandboxes contend for slots. Suspend/resume via checkpoint is cheap relative to cold start but **not free**. We need allocation policies that:

1. Respect **sandbox priority** and related agent semantics.
2. Account for **operational cost** (resume latency, interference, snapshot locality).
3. Maximize useful density without starving high-priority work.

## In scope

- Priority-aware placement, eviction, and resume-target choice.
- Comparison of policies under the same workload traces and metrics.
- Using real or synthetic agent/sandbox traces as datasets.

## Out of scope (for the research question)

- Inventing a new isolation runtime (consume gVisor / others).
- Replacing Kubernetes pod-to-node scheduling.
- Matching any particular commercial sandbox API.

## Success looks like

A documented policy (or family of policies) with **reproducible eval** against named baselines and datasets, plus a platform that can run those policies online.
