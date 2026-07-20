# Research: literature

Living survey for **priority-aware sandbox allocation** on shared Workers (placement / eviction / resume under C/R cost).

**Our gap (as of 2026-07):** nearby work uses “agent semantics” for checkpoint timing, LLM/GPU serving, or serverless keep-alive—not for **sandbox-slot priority allocation** with shared eval datasets.

How to use:

1. Add/update a row in the index (keep **Mismatch** honest).
2. For keepers, add `papers/YYYY-short-slug.md`.
3. Feed baselines/metrics into [baselines.md](./baselines.md) and [metrics.md](./metrics.md).

## Index

### A. Agent sandbox C/R (closest names, different problem)

| Key | Venue / year | Topic | Relevance | Mismatch | Link |
|-----|--------------|-------|-----------|----------|------|
| crab2026 | arXiv 2026 | Semantics-aware C/R for agent sandboxes; host schedules checkpoint I/O using LLM wait windows | Turn/LLM-overlap as a **cost-hiding** signal; host-scoped C/R queueing; density stress | Semantics = turn/OS effects + wait window, **not** multi-tenant priority → which sandbox keeps the Worker slot | [html](https://arxiv.org/html/2604.28138v1) · [pdf](https://arxiv.org/pdf/2604.28138) |
| deltabox2026 | arXiv 2026 | Millisecond incremental sandbox C/R (DeltaState / DeltaFS / DeltaCR) | C/R cost model; motivates treating resume latency as first-class in scoring | Mechanism paper; no Worker-pool priority placement | [html](https://arxiv.org/html/2605.22781v2) |
| acrfence2026 | arXiv 2026 | Semantic rollback attacks after agent checkpoint restore | Security constraint on naive restore/fork policies | Not an allocator | [pdf](https://arxiv.org/pdf/2603.20625) |

### B. Agentic LLM / GPU serving (session & workflow priority)

| Key | Venue / year | Topic | Relevance | Mismatch | Link |
|-----|--------------|-------|-----------|----------|------|
| agentix2026 | NSDI 2026 | Agent programs as schedulable units; program-level prioritization (PLAS) | **Program/session-level priority** vs call-level; process table of agent state | Schedules **LLM calls on GPUs**, not sandboxes on Workers | [pdf](https://www.usenix.org/system/files/nsdi26-luo.pdf) |
| saga2026 | arXiv 2026 | Workflow-atomic agent inference scheduling; Agent Fair Share | Workflow as unit; fairness + SLO under multi-tenant agents | GPU cluster / KV reuse, not sandbox slots | [html](https://arxiv.org/html/2605.00528) |
| smetric2026 | arXiv 2026 | Session-centric routing for agentic serving (first turn balance, later cache-aware) | Session structure as scheduling signal; real agent traces | KV$/TPS objective on inference cluster | [abs](https://arxiv.org/abs/2607.08565) · [html](https://arxiv.org/html/2607.08565) |
| hexagent2026 | arXiv 2026 | Workflow- & heterogeneity-aware agentic serving (HexAGenT) | Online DAG + risk of missing workflow SLO → priority | Prefill/decode GPUs, not sandbox Workers | [html](https://arxiv.org/html/2605.16637v1) |

### C. Serverless keep-alive / cold-start priority (analogy: scarce warm capacity)

| Key | Venue / year | Topic | Relevance | Mismatch | Link |
|-----|--------------|-------|-----------|----------|------|
| icebreaker2022 | ASPLOS 2022 | Utility-driven warm/keep-alive on heterogeneous nodes | Utility score for who stays warm under budget—**strong baseline inspiration** | Functions, not stateful sandbox C/R across Workers | [doi](https://doi.org/10.1145/3503222.3507750) |
| incendio2024 | IEEE TC 2024 | Priority-based scheduling to cut cold-start latency | Explicit **priority ≠ minimize cold-start count**; latency-benefit priority model | Container keep-alive, not agent sandbox semantics | [doi](https://doi.org/10.1109/tc.2024.3386063) |
| faascache2021 | ASPLOS 2021 | Greedy-dual keep-alive caching (FaasCache) | Classic keep-alive eviction under memory pressure | No agent priority / C/R migration | (search: FaasCache Fuerst Sharma) |

### D. Classic cluster priority / preemption (background)

| Key | Venue / year | Topic | Relevance | Mismatch | Link |
|-----|--------------|-------|-----------|----------|------|
| borg2015 | EuroSys 2015 | Large-scale cluster management at Google | Priority, preemption, oversubscription folklore | Pod/job to machine—not sandbox C/R sessions | (Borg paper) |
| apollo2016 | SoCC / Microsoft | Cloud-scale scheduler (Apollo) | Opportunistic packing + priorities | Same layer mismatch as Borg | (Apollo paper) |

### E. Systems (not papers; context only)

| Key | Type | Topic | Relevance | Mismatch |
|-----|------|-------|-----------|----------|
| substrate | OSS / systems | N:M actor↔Worker multiplexing + snapshot resume | Platform shape we build on conceptually | No published priority-allocation eval loop |
| agent-sandbox | k8s-sigs | Sandbox CR, warm pool, hibernate | Isolation / 1:1 Pod lifecycle | Not N:M semantic allocator |

## Takeaways for Actordock

1. **Do not claim green field on “agent + sandbox + C/R”**—Crab/DeltaBox own nearby C/R semantics.
2. **Do claim gap on “priority × Worker-slot allocation under C/R”** with comparable datasets—index above supports that.
3. Steal ideas carefully:
   - From Crab: LLM/idle windows as *optional* cost-hiding signal (not the only priority).
   - From Agentix/SAGA/HexAGenT: session/workflow-level priority & fairness metrics.
   - From IceBreaker/Incendio: utility / latency-benefit scoring for scarce warm capacity.
4. Next survey pass: live migration cost models; Borg-like preemption formalisms; any public **sandbox idle/active traces**.

## Paper notes

| File | For |
|------|-----|
| [papers/2026-crab.md](./papers/2026-crab.md) | Closest “semantics-aware” sandbox C/R |
| [papers/2026-deltabox.md](./papers/2026-deltabox.md) | Fast C/R mechanism |
| [papers/2026-agentix.md](./papers/2026-agentix.md) | Program-level agent priority (GPU) |
| [papers/2022-icebreaker.md](./papers/2022-icebreaker.md) | Warm-capacity utility baseline inspiration |

## Search seeds (continue)

- Cluster scheduling with priorities / preemption (Borg, Apollo, Quasar-like)
- VM/container checkpoint-restore and live migration cost models
- Serverless keep-alive / cold-start under priority (IceBreaker, Incendio, FaasCache)
- Agentic serving: session/workflow SLO (SMetric, SAGA, HexAGenT, Agentix)
- Agent sandbox C/R correctness vs cost (Crab, DeltaBox, ACRFence)

## Out of scope for this file

Product READMEs → [`../references/`](../references/). This file is for **citable research** and deep technical reports.
