# Agent semantic-score workload: dataset splice + evaluation

This document records how to **evaluate `semantic-score` against other Worker-slot policies** under agent-like contention **without hand-writing tasks or synthesizing arrivals**.

Related:

- Policy algorithm: [`../architecture/semantic-score.md`](../architecture/semantic-score.md)
- Dataset requirements: [`../research/datasets.md`](../research/datasets.md)
- Metrics vocabulary: [`../research/metrics.md`](../research/metrics.md)
- CI: `E2E_SUITE=agent-semantic` (8 agents / 2 workers; random / resource-evict / semantic-score-l1 / semantic-score)

## 1. Goal

Answer: under scarce Workers, does `semantic-score` allocate sandboxes better than `fifo` / `random` / `lru-idle` / `resource-evict` for **real agent sessions**?

Better means, on the **same pinned input stream**:

- fewer Suspends during `tool_loop`+lock
- lower Resume wait for high-urgency / high-`complexitySignal` sessions
- similar or better task completion wall time
- no permanent starvation of low-prior sessions (unless override is on)

There is **no public “sandbox-slot scheduling” benchmark**. We therefore **splice two public sources** into one versioned Actordock workload package, then replay it under each policy.

## 2. What is (and is not) generation

| Allowed (deterministic mapping / inference) | Not allowed (synthetic content) |
|---------------------------------------------|----------------------------------|
| Use AgentProcessBench task text and tool traces as-is | Hand-write or LLM-write prompts |
| Use Azure Functions 2019 timestamps as-is | Sample Poisson / Zipf arrivals |
| Derive `phase_spans` from tool timestamps with fixed rules | Invent phase labels |
| Run existing HF `classify(task_text)` → `task_profile` | Invent difficulty tiers |
| Pin `worker_pool` in experiment config | Claim pool size is “from the dataset” |

The splice script only **joins and renames fields**. Classifier output is a **derived feature**, recorded with `modelID` for reproducibility.

## 3. Source datasets

### 3.1 Session / semantics — AgentProcessBench

- HF: [`LulaCola/AgentProcessBench`](https://huggingface.co/datasets/LulaCola/AgentProcessBench)
- Paper: [arXiv:2603.14465](https://arxiv.org/abs/2603.14465)
- Contents: 1000 trajectories (250 each: `bfcl`, `gaia_dev`, `hotpotqa`, `tau2`) with multi-turn messages, tools, and tool-use traces

**v2 default (single source):** only APB **`bfcl`** (≤250 trajectories = 50 queries × 5 samples). Select **all `l3_active` first**, then pad to `--target` (default **200**) with tool_dense / other. Tag `eval.cohort` ∈ {`l3_hard`,`l3_mid`,`l3_easy`,`l3_inactive`}.

### 3.2 Arrivals / concurrency — Azure Functions 2019

- [`AzurePublicDataset` / AzureFunctionsDataset2019](https://github.com/Azure/AzurePublicDataset/blob/master/AzureFunctionsDataset2019.md)
- Same family used by FaasCache / IceBreaker keep-alive studies

**v2 arrival rule:** each **contention wave** starts at the next Azure **busy minute** (minute with any invocations). Slots inside a wave are offset by `5s`. This keeps provenance in Azure while producing usable inter-arrival gaps (not sub-millisecond bursts).

### 3.3 Optional stretch corpus

| Corpus | Role |
|--------|------|
| SWE-bench Lite | Longer `task_text` / heavier sessions; still use Azure timestamps |
| WorkBench | Workplace tool tasks if a sandbox-exec harness is wired later |

## 4. Unified package layout

Place under `docs/eval/datasets/`:

```text
docs/eval/datasets/agent-semantic@v2/
  manifest.yaml
  README.md
  sessions.jsonl
  arrivals.jsonl
  summary.json           # cohort counts + how to use
  checksums.sha256
```

Do **not** commit raw Azure zip or HF cache; document download commands and pin versions/hashes in `manifest.yaml`.

## 5. Schema

Schema version: `agent-semantic.session.v2`.

### 5.1 Input — `sessions.jsonl` (one JSON object per session)

| Field | Type | Required | Provenance |
|-------|------|----------|------------|
| `schema_version` | string | yes | `"agent-semantic.session.v2"` |
| `session_id` | string | yes | e.g. `apb/bfcl/q012` |
| `source` | object | yes | `{dataset, subset, query_index, sample_index, license}` |
| `task_text` | string | yes | First user / task description from the trajectory |
| `arrival_ts` | number | yes | Unix seconds; Azure busy-minute + in-wave offset |
| `tool_trace` | array | yes | Ordered tool calls (`name`, `args_digest`, `t_rel_ms`) |
| `phase_spans` | array | yes | `{phase, lock, t_start_ms, t_end_ms}` |
| `task_profile` | object | yes | HF classify output |
| `eval` | object | yes | `{cohort, l3_active, phase_role, wave_id, wave_slot, n_tools}` |
| `notes` | string | no | Mapping quirks |

**`task_profile` (L3, matches CP `TaskProfile`):**

| Field | Type | Notes |
|-------|------|--------|
| `version` | string | e.g. `v1` |
| `complexitySignal` | number | Continuous SR-style hard−easy signal |
| `domain` | string | Domain classifier label |
| `embeddingSim` | number | `[0,1]` vs reference template(s) |
| `confidence` | number | `[0,1]`; policy ignores prior if `< 0.3` |
| `modelID` | string | HF repo id(s) used |
| `scoredAt` | string | ISO time of classify |
| `difficultyTier` | string | Optional debug only; **not** used by keepScore |

**`phase_spans.phase`:** `llm_wait` | `tool_loop` | `idle` (same vocabulary as semantic-score L1).

### 5.2 Experiment config (not inside each session)

Pinned by CI / `hack/verify-local.sh` env (example):

```bash
AGENT_SEMANTIC_LIMIT=8
AGENT_SEMANTIC_INFLIGHT=8
AGENT_SEMANTIC_MIN_WORKERS=2
AGENT_SEMANTIC_SPEED=60
AGENT_SEMANTIC_POLICIES=random,resource-evict,semantic-score-l1,semantic-score
```

`semantic-score-l1` sets `SEMANTIC_PRIOR_MIX=0` (L1 lock only); `semantic-score` sets `0.3` (L1+L3).

### 5.3 Output — per-policy result records

Written under `docs/eval/results/` (or CI artifacts), e.g. `agent_semantic_v2__semantic-score.json`:

| Field | Type | Meaning |
|-------|------|---------|
| `dataset` / `config_hash` / `git_commit` / `policy` | string | Repro pins |
| `sessions_total` | int | |
| `mid_tool_suspend_count` | int | From `/metrics`: sum eviction where `victim_phase=tool_loop` or `victim_lock=true` |
| `mid_tool_rate` | float | `mid_tool / suspend_total` |
| `suspend_total` | int | Sum of `actordock.schedule.eviction` |
| `evict_tool_loop` / `evict_llm_wait` | int | Eviction counts by victim phase |
| `victim_by_cohort` / `victim_l3_hard_rate` | object / float | Resume-time attributed victims by `eval.cohort`; `hard_rate = hard/(hard+mid+easy+inactive)` |
| `resume_sec_by_cohort` | object | Mean client Resume RTT by requester cohort (L3 proxy) |
| `victim_complexity_mean` | float | Mean `complexitySignal` of attributed victims |
| `resume_latency_ms` / `resume_wait` | object | From resume histograms |
| `preempt_cost_mean_s` | float | Mean Suspend/checkpoint cost under eviction |
| `starvation_wait_count` | int | `semantic_starvation_wait{outcome="enter"}` |
| `tasks_completed` / `tasks_failed` | int | Replay session success |
| `wall_time_ms` | object | Per-policy wall clock |


Comparative table (markdown/JSON) joins the same metrics across policies on **identical** `sessions.jsonl`.

## 6. How to build the dataset (splice pipeline)

Pipeline: `./hack/build-agent-semantic-dataset.py` (contract in this doc).

### 6.1 Download (pinned)

1. Download APB `bfcl/test.jsonl` (script caches under `.cache/agent-semantic/apb/`).
2. Download Azure Functions 2019 day file(s) per `AzureFunctionsDataset2019.md` (record URL + sha256).

### 6.2 Map sessions

For each selected AgentProcessBench **BFCL** row (`sample_index` in `[0, max_samples)`):

1. Extract `task_text` and ordered `tool_trace`.
2. Derive `phase_spans` with **fixed rules** (document exact code version):
   - Between tool executions → `llm_wait` (or `idle` if no pending model call)
   - During tool execution → `tool_loop` + `lock=true`
   - After final tool / terminal message → `idle`
   - Timestamps: use relative ms from the trajectory when available; else unit-duration placeholders **only for span ordering**, never for arrival process (arrivals stay Azure).
3. Assign `arrival_ts` via Azure busy-minute **waves** + in-wave gap (see §3.2).
4. Run `demos/agent-llm-multiplex` HF classify (or shared library) on `task_text` → `task_profile`. Persist `modelID` + weights revision.
5. Emit one JSONL line; append join row to `arrivals.jsonl`.

### 6.3 Manifest

`manifest.yaml` must include:

- `id: agent-semantic@v2`
- `subset: bfcl`
- licenses / attribution for both sources
- HF file sha256, Azure file name + sha256
- classify model ids + git commit of classify code
- `schema_version`
- `session_count`

### 6.4 Validation gates

Refuse to publish the package if:

- any `arrival_ts` is non-monotonic in join order
- any session lacks `task_text` or `task_profile.confidence`
- `phase_spans` empty or overlapping incorrectly
- session count ≠ declared `session_count`

## 7. How to evaluate (get performance numbers)

### 7.1 Modes

| Mode | When | How |
|------|------|-----|
| **A. Live Kind replay** | Primary claim for Actordock | Port-forward CP; for each policy, Create/Resume sandboxes on `arrival_ts` schedule; drive agent (or phase-faithful stub) from `tool_trace`; POST L1/L3 signals; collect `/metrics` + victim logs |
| **B. Offline decision replay** | Fast ablation of keepScore | Feed cached signals + arrivals into a Place/Evict simulator that embeds `semantic-score` / baselines; no gVisor | 
| **C. CI agent-semantic** | PR / main Kind job | Matrix: one cluster per policy (`random`, `resource-evict`, `semantic-score-l1`, `semantic-score`); merge job builds compare table |

Primary paper/demo numbers should come from **Mode A** (or A+B agreement).

### 7.2 Live replay steps (Mode A)

1. Cluster: Kind with Actordock; scale Workers to config (`workers=2`, `max_slots=1` unless config says otherwise).
2. For each `policy` in config:
   1. Set `POLICY=<policy>` (or controlplane SetPolicy) and wait healthy.
   2. Reset metrics / cleanup sandboxes.
   3. Load `sessions.jsonl`. Schedule Create at `arrival_ts` (wall clock or scaled speed `REPLAY_SPEED`).
   4. On Create: POST `task_profile` **before** first Resume (same order as demo: create → classify/POST → Resume).
   5. Drive session: either real LLM+tools constrained to the task, or a **trace-faithful executor** that only reproduces `phase_spans` timing (no new task text).
   6. Record Suspend/Resume/victim reason from CP logs/metrics.
   7. Write `results/agent_semantic_v2__<policy>.json`.
3. Merge: `policy_compare_agent_semantic_v2.md` with columns = policies, rows = metrics in §5.3.
4. Pin: git commit, dataset id, config hash, replay speed, model ids.

### 7.3 Metrics (report all)

Aligned with [`../research/metrics.md`](../research/metrics.md), specialized for this workload:

**Primary**

- `mid_tool_suspend_count` (↓ better for semantic-score vs fifo)
- Resume latency by cohort: high vs low `complexitySignal` tertile (↓ for high cohort under semantic-score)
- Task wall time / completion rate

**Secondary**

- Cross-Worker resume rate
- Starvation wait count
- Scheduling decision latency (CP)
- Suspend cost / checkpoint bytes if resource plugin enabled

Always report **per cohort**, not only globals.

### 7.4 Pass / interpret guidance (not hard CI gates)

Suggest directional expectations (tune after first full run):

- vs `fifo`: lower `mid_tool_suspend_count`
- vs `lru-idle` / `resource-evict`: better or equal high-`complexitySignal` Resume latency when L3 coverage is high
- vs all: no collapse in overall completion rate

If L3 confidence is mostly low, treat the run as L1-only and say so in the result README.

## 8. Relation to existing `e2e/functional`

| | Functional (`e2e/functional`) | `agent-semantic@v2` CI |
|--|-------------------------------|-------------------------|
| Purpose | Correctness / policy smoke | Research comparison under agent traces |
| Input | Hand-built scenarios | Spliced public corpora |
| Scale | Seconds–minutes | 8 sessions × N policies |
| Claim strength | “policy works” | “policy better on agent workload X” |

## 9. Implementation checklist (follow-on work)

1. [x] `hack/build-agent-semantic-dataset.py` splice script (v2 contrast design)
2. [x] Publish `docs/eval/datasets/agent-semantic@v2/`
3. [x] Replay driver (Mode A): `hack/replay-agent-semantic.py` (phase stub, no LLM); CI `E2E_SUITE=agent-semantic`
4. [ ] Optional Mode B simulator sharing `internal/policy` Place logic
5. [x] Replay writes `policy_compare_agent_semantic_v2.md` (+ per-policy JSON)
6. [x] Docs point at `@v2` as preferred package

### Replay usage

```bash
# port-forward CP, then:
./hack/replay-agent-semantic.py \
  --api http://127.0.0.1:8080 \
  --policies random,resource-evict,semantic-score-l1,semantic-score \
  --switch-policy \
  --limit 24 --speed 60
```

Outputs under `docs/eval/results/`: `agent_semantic_v2__<policy>.json` and `policy_compare_agent_semantic_v2.md`.


## 10. License / attribution

Respect upstream licenses when redistributing derived JSONL:

- AgentProcessBench / subset papers
- Azure Public Dataset terms

Prefer storing **ids + join keys + derived features** over full tool payloads if redistribution is restricted; keep a rebuild script so others can regenerate from public downloads.
