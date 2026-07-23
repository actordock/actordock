# Policy design: `semantic-score` (agent-semantic allocation)

Status: **P1–P3 implemented** (ingest + `semantic-score` policy + demo HTTP heartbeat). L3 prior uses **Hugging Face classifier/embedding weights published by vLLM-SR** (`complexity`, `domain`, `embedding`)—**not** the full Semantic Router runtime. Companion to [signal-plugins.md](./signal-plugins.md) and [scheduling.md](./scheduling.md).

## 1. Goal

Under scarce Worker slots (`MaxSlots=1`), decide who keeps a warm sandbox and who is Suspended using **agent-session meaning**—not arrival order alone and not cgroup idle alone.

| | |
|--|--|
| **Schedulable unit** | One agent session (= one Sandbox), spanning LLM waits and tool loops |
| **Scarce resource** | Worker slot occupancy under checkpoint/restore cost |
| **Eviction primitive** | Unchanged: **Suspend** (portable) |
| **Out of scope** | Model / tool routing (inference gateways); Crab-style *when/how fine* to checkpoint |

## 2. What “agent semantic” means

Semantics live in the **decision basis**, not in one magic label. Shallow and deep layers are both semantic; they differ in depth.

| Layer | What it captures | Typical signals | Literature anchors |
|-------|------------------|-----------------|--------------------|
| **Shallow** | Can we interrupt *this moment*? | `phase`, `lock` | Crab LLM-wait windows; SAAR tool-loop hard lock |
| **Deep** | Is *this session* worth the slot vs peers? | urgency, fairness (attained/wait), preempt cost | Agentix PLAS; SAGA / HexAGenT deadline / fairness |
| **Optional prior (L3)** | Task traits before/without rich online counters | `taskProfile` from **vLLM-SR HF models** (weights only) | Signal→Decision split idea; weak mix into urgency / affinity — §6 |

`semantic-score` = **phase lock filter** + **session `keepScore`**. Phase alone is not the whole policy; session value is the main score once candidates are unlockable.

### 2.1 Why not primary “task difficulty” or step prediction?

Static difficulty must **not** be “hard ⇒ never evict”:

- Hard ≠ must hold a Worker *now* (hard tasks often sit in `llm_wait`; easy tasks can run long tools).
- There is **no reliable off-the-shelf model** for “how many agent tool steps remain”; we **do not** train or depend on step-count regression.
- Classifiers are useful as **task-trait priors** (complexity / domain / embedding similarity), not as remaining-work oracles.
- Online truth remains `phase`/`lock`, `waitSec`, `attainedServiceSec`, and optional `deadline`.

Difficulty / domain / embedding appear only inside `taskProfile` → **weak** mix into `urgency` or affinity (§6).

## 3. Integration contract (portable to any agent)

The platform must not require demo-only hooks. Cooperation is **optional and tiered**. Agents that do nothing still run; they just get weaker protection.

| Tier | Who | What | Portable? |
|------|-----|------|-----------|
| **L0** | Nobody | No semantic POSTs | Yes. Policy falls back toward fifo-like pick among non-busy sandboxes |
| **L1 (minimum useful)** | Agent / SDK / sidecar | Heartbeat `phase` + `lock` on LLM/tool boundaries | Yes. Any tool-loop runtime has these boundaries |
| **L2** | Agent or orchestrator | Also `deadline`, `workflowID` (optional) | Usually yes |
| **L3 (classifier)** | **Orchestrator / driver / gateway—not the agent core** | Task text → **HF models** (complexity / domain / embedding) → `taskProfile` | Yes; stub OK for demos |

**Control plane owns** `attainedServiceSec`, `waitSec`, and preempt-cost inputs derived from resource/snapshot signals. Agents do not report those.

### 3.1 L1 heartbeat (what every agent can do)

```text
about to call LLM  → POST {phase:llm_wait,  lock:false}
enter tool         → POST {phase:tool_loop, lock:true}
leave tool         → POST {phase:idle,      lock:false}
```

Same shape for LangChain / OpenAI Agents / Claude tool loops. No Actordock-specific “prompt crafting” inside the agent.

### 3.2 What the demo does vs what others must do

| Concern | Demo (`demos/agent-llm-multiplex`) | Other agents |
|---------|-------------------------------------|--------------|
| L1 phase/lock | `actor/semantic.py` on Runner/tool boundaries | Same three POSTs (or SDK wrapper) |
| L2 deadline / workflowID | Optional on dispatch | Optional |
| L3 classifier | Driver: stub or **local HF** vLLM-SR weights (§6) | Same; map outputs → `taskProfile` |
| 409 after Suspend | Driver may retry Resume (orchestration) | Same pattern; not a policy substitute |

**Classifier does not require agent changes.** If an ecosystem cannot call Actordock semantic HTTP at all, it stays on L0.

## 4. Signal model

Cached on `SandboxSignals.semantic` (TTL suggested 15–30s). Expired ⇒ missing.

Ingest (planned): `POST /v1/signals/semantic` (merge/patch per `sandboxID`).

### 4.1 Agent- or orchestrator-reported

| Field | Type | When | Tier |
|-------|------|------|------|
| `version` | schema id | always | L1+ |
| `phase` | `llm_wait` \| `tool_loop` \| `idle` | LLM/tool boundaries | L1 |
| `lock` | bool | `true` in `tool_loop` | L1 |
| `deadline` | optional time | task create | L2 |
| `workflowID` | optional string | multi-step id | L2 |
| `taskProfile` | optional object | L3: complexity / domain / embedding | L3 |
| `remainingSteps` | optional int | **legacy / unused by L3 path**; do not treat as primary urgency | — |

### 4.2 Control-plane authoritative

| Field | Meaning | Update |
|-------|---------|--------|
| `attainedServiceSec` | Cumulative seconds in `running` | CP clock while running |
| `waitSec` | Time `suspended` with demand to run | CP while suspended |
| `preemptCost` | Re-materialization cost | From snapshot Cost/Size or `KeepAliveH` |

Trust: L1/L2/L3 payloads are hints (rate-limit, version). Attained/wait are platform truth.

## 5. Algorithm

### 5.1 When to evict

Only when Place/Resume needs a slot and **no eligible idle Worker** exists (same resource-conserving rule as FaasCache-style policies).

### 5.2 Evict

```text
candidates = all running
drop checkpointInProgress

# Shallow: phase lock
unlocked = { c | not (c.lock or c.phase == tool_loop) }
if unlocked non-empty:
    pool = unlocked
else if SEMANTIC_OVERRIDE:
    pool = candidates          # starvation override
else:
    scheduler waits up to SEMANTIC_WAIT_SEC then reject

# Deep: session value
victim = argmin keepScore(s) over pool
tie-break: earlier CreatedAt
```

### 5.3 `keepScore` (higher = keep; victim = minimum)

```text
keepScore(s) =
  + w_L * phaseProtect(s)
  + w_U * urgency(s)          # includes optional classifier prior — §6
  + w_F * fairness(s)
  + w_C * normalize(preemptCost(s))
```

| Term | Definition (all ≈ `[0,1]`) | If missing |
|------|----------------------------|------------|
| `phaseProtect` | `tool_loop`/`lock` → 1.0; `llm_wait` → 0.2; `idle` → 0.0 | 0.5 |
| `urgency` | See §5.4 (online + prior, both scaled to `[0,1]`) | 0 |
| `fairness` | Soft map of `r=wait/(1+attained)`: `1 − 1/(1+r)` ∈ `[0,1)` | 0 |
| `preemptCost` | `clamp(log1p(H) / log1p(H_ref), 0, 1)` with `H_ref=1e6` | H=1 → small |

Default weights: `w_L=3`, `w_U=2`, `w_F=2`, `w_C=1` (env-tunable).  
Terms share a common scale so weights stay meaningful (no raw `1/sec` or unbounded wait).

Low score ⇒ yieldable phase, low urgency, already well-served, cheap to restore → kick first.

### 5.4 `urgency` (online + optional L3 prior)

```text
urgency_online:                         # ∈ [0,1]
  if deadline:
      1 / (1 + max(0, seconds_until(deadline)))
  else if API priority:
      map(priority) into [0,1]
  else:
      0

urgency_prior:   # ∈ [0,1] — §6 (vLLM-SR traits, not expectedSteps)
  clamp(
    clamp(0.5 + complexitySignal, 0, 1)  # continuous SR hard−easy signal
    + α * clamp(embeddingSim, 0, 1),
    0, 1)
  # domain: affinity / pool filter when Worker pools exist; else unused in score
  # difficultyTier (if present) is debug-only; not used in keepScore

urgency =
  if taskProfile.confidence high:
      mix(urgency_online, urgency_prior)     # both already on [0,1]
  else:
      urgency_online
```

**Forbidden:** discretizing signal into easy/medium/hard for keepScore (θ-binning loses ranking).  
**Forbidden:** `if difficultyTier == hard: keepScore = +∞` (no hard never-evict).  
**Forbidden:** treating classifier output as predicted agent step count.

### 5.5 Place

1. Idle healthy Worker → sticky if applicable, else least-loaded.
2. Else → Evict victim → Suspend → place on freed Worker.

### 5.6 Resume contention and `409`

Policy chooses victims with semantic scores so `tool_loop`+`lock` is not preferred. Concurrent Resume can still Suspend a session mid-tool under `fifo` today; under `semantic-score`, unlocked `llm_wait`/`idle` peers should be preferred. Clients that Exec after Suspend may see **409**; **retry Resume** is orchestrator hygiene, not a replacement for the policy.

## 6. L3 classification: HF models from vLLM-SR (not the full router)

### 6.1 Role

We borrow **only the published neural models** (domain classifier, embedding, complexity via embedding+prototypes)—**not** Envoy, Boolean decision DSL, plugin chains, or model-routing of [vLLM Semantic Router](https://github.com/vllm-project/semantic-router). That runtime is too heavy for Actordock L3.

| | Full vLLM-SR product | Actordock L3 |
|--|----------------------|--------------|
| What we take | — | **HF weights / small inference only** |
| What we skip | Envoy ext_proc, route DSL, plugins, MoM backend selection | (all of that) |
| Scarce resource they optimize | Model / KV / cost | Worker slot |
| Signals used (v1) | Many | **`complexity`, `domain`, `embedding` only** |
| Decision | Their Boolean routes → model pool | Our `keepScore` / optional Worker-pool affinity |
| Hot path | May run on every LLM request | **Never** on Place: read cached `taskProfile` |

```text
Orchestrator holds TaskInput (at least taskText)
        │
        ▼
  Local HF inference (or stub)
        │  complexity → complexitySignal (continuous)
        │  domain     → domain
        │  embedding  → embeddingSim (vs reference / cohort)
        ▼
  taskProfile { complexitySignal, domain, embeddingSim, confidence, modelID }
        │
        ▼
  POST /v1/signals/semantic  {sandboxID, taskProfile}
        │
        ▼
  semantic-score reads cache (no model call on Place hot path)
```

### 6.2 Model source (weights only)

| Mode | When | How |
|------|------|-----|
| **Production / L3 eval** | Real priors | `huggingface-cli download` from [llm-semantic-router](https://huggingface.co/llm-semantic-router); run with transformers / candle / onnx in **driver or a tiny classify helper**—no SR binary required |
| **Local demo / CI** | Wiring only | Schema-compatible **stub** |

**v1 models (illustrative; pin exact repo IDs in demo README when wired):**

| Signal | Typical HF artifact | Maps into `taskProfile` | Use in Actordock |
|--------|---------------------|-------------------------|------------------|
| **complexity** | Embedding model + hard/easy prototype texts (same idea as SR complexity signal; no SR process) | `complexitySignal` (+ optional debug tier) | Weak `urgency_prior` |
| **domain** | mom-*-class / domain classifier LoRA or merged | `domain` (+ conf) | Worker-pool affinity when pools exist; else label-only |
| **embedding** | mom multilingual embed (or sibling) | `embeddingSim` ∈ [0,1] | Weak affinity vs reference templates |

**Out of L3 v1:** full Semantic Router deploy; PII / jailbreak / HaluGate / toolcall heads; any step-count head.

### 6.3 Input = task/session context (orchestrator-owned)

| Field (illustrative) | Source |
|----------------------|--------|
| `taskText` / user goal | Dispatch payload (**minimum**) |
| `systemInstructions` | Orchestrator config |
| `toolManifest` | Agent template |
| `constraints` / SLO / deadline | API create / enqueue |
| `workflowID` | Optional |
| `referenceTexts` | Optional: templates for embedding similarity |

Sparse input (task text only) is enough for complexity/domain; richer context improves embedding affinity.

### 6.4 Output schema (`taskProfile`)

```text
TaskProfile {
  version
  complexitySignal: float             # SR hardScore−easyScore; drives urgency_prior
  difficultyTier: easy|medium|hard   # optional debug label only
  domain: string                      # from domain classifier
  embeddingSim: float                 # 0..1 similarity; optional if unused
  confidence: float                   # 0..1; low ⇒ ignore prior
  modelID                             # HF repo id(s) or "demo-stub"
  scoredAt
}
```

**Removed from the L3 contract:** `expectedSteps`, `expectedToolSec`, `expectedLLMWaitSec` as classifier outputs. (Legacy JSON fields may remain ignored.)

### 6.5 Placement of the service

| Option | Use |
|--------|-----|
| **A. Orchestrator/driver (recommended)** | Load HF models (or stub) at enqueue; POST semantic |
| **B. Tiny local classify process** | Optional: one small HTTP helper that only runs the three heads—**still not** full SR |
| **C. CP async queue** | Never block Place on inference |
| **D. Inside sandbox agent** | Discouraged for v1 |

**Non-goal:** deploying `vllm-project/semantic-router` as a dependency.

Hot path: **read cache only**.

### 6.6 Rollout

| Stage | Work |
|-------|------|
| Now | `semantic-score` without requiring L3 |
| +1 | `taskProfile` ignored-if-absent; stub fills three fields |
| +2 | Local HF: domain + embed (+ complexity prototypes); map three signals |
| +3 | A/B: prior on/off → interrupt rate / resume latency by complexity & domain cohort |

Success metric is **scheduling quality**, not classifier accuracy alone.

### 6.7 Implementation in this repo (Actordock)

Models run **outside** the control plane. Place/Resume only reads cached `taskProfile`.

```text
demos/.../driver  ──classify(taskText)──► taskProfile
        │                                    │
        │ POST /v1/signals/semantic          │
        ▼                                    ▼
  agent L1 phase/lock              signals.Store cache
                                             │
                                             ▼
                                   semantic-score keepScore
```

#### Control plane (Go)

| Path | Change |
|------|--------|
| `internal/signals/types.go` | `TaskProfile`: `ComplexitySignal`, `Domain`, `EmbeddingSim`, `Confidence`, `ModelID`, `ScoredAt`; `DifficultyTier` debug-only; stop using `ExpectedSteps*` in policy |
| `internal/policy/semantic_score.go` | `urgencyPrior` / online / fairness / preempt all scaled to ≈`[0,1]`; ignore prior if `confidence < 0.3` |
| `internal/policy/semantic_score_test.go` | Cases: higher `complexitySignal` outranks lower among unlocked peers; low confidence ignores prior |
| CP / Kind env | `SEMANTIC_PRIOR_MIX`, `SEMANTIC_EMBED_ALPHA` (see §8); **no** HF model load in `controlplane` |

`domain` in v1: **record + optional log**; Worker-pool affinity filter is a later increment when Workers are labeled.

#### Demo / orchestrator (Python)

| Path | Change |
|------|--------|
| `demos/agent-llm-multiplex/actor/profile.py` | `classify(task) → taskProfile`; `SEMANTIC_CLASSIFIER=stub\|local-hf` |
| `demos/agent-llm-multiplex/actor/hf_classify.py` | **New.** Load HF domain + embed once; complexity via hard/easy prototype cosine margin; optional `referenceTexts` for `embeddingSim` |
| `demos/agent-llm-multiplex/actor/agent.py` | Drop remaining-steps bookkeeping as L3 input; first semantic POST includes `taskProfile` |
| `demos/agent-llm-multiplex/driver/main.py` | Before/at Create: `profile = classify(prompt)`; pass into context; log signal/domain/sim |
| `demos/agent-llm-multiplex/requirements.txt` | Optional extras for `local-hf` (`transformers`, `torch`); stub path stays lightweight |
| `demos/agent-llm-multiplex/README.md` | Pin HF repo IDs, env vars, stub vs local-hf |

**`local-hf` inference sketch:**

1. Download weights from [llm-semantic-router](https://huggingface.co/llm-semantic-router) via `SEMANTIC_HF_DOMAIN` / `SEMANTIC_HF_EMBED`.
2. `domain` ← argmax of domain classifier logits.
3. `embeddingSim` ← cosine(task embed, reference template embed), clamped to `[0,1]`.
4. `complexitySignal` ← **SR multi-rule complexity** (aligned with `prototype_scoring.go`):
   - `bankScore = 0.75*best + 0.25*mean(top2)` (env: `SEMANTIC_HF_BEST_WEIGHT`, `SEMANTIC_HF_TOP_M`)
   - `complexitySignal = hardScore − easyScore` (continuous; **no θ-binning in keepScore**)
   - optional `difficultyTier` for logs only (`signal > θ` → hard, etc.)
   - domain composer picks which rule banks apply (`code_complexity` / `math_complexity` candidates from SR docs)
5. `TaskProfile.confidence` ← domain classifier conf when a rule matched (Actordock prior-mix needs ≥0.3; raw SR `|signal|` is often smaller and kept as debug `complexityConf`).

#### Build order (P5)

1. Types + `urgencyPrior` + unit tests (CP still works with absent profile).
2. Demo: **create → classify → POST `taskProfile` → Resume** (L3 before first eviction); differentiated prompts by default.
3. Optional: replace prototype complexity with an LLM difficulty judge.
4. (Later) domain → Worker pool filter when pools exist.

## 7. Worked example (3 sandboxes / 2 Workers)

1. A in `tool_loop`+`lock`, B in `llm_wait`, C Resumes → **evict B, not A** (shallow).
2. B and C both unlocked; C has high `waitSec`, low `attainedServiceSec` → **prefer C** (deep fairness).
3. With L3: C has higher `complexitySignal` (and optionally high `embeddingSim`) with high `confidence` → higher `urgency_prior` → more likely keep/wake C—**still** never overrides A’s lock unless starvation override.
4. If Workers are labeled by domain pools and C’s `domain=code` only fits the code pool, Place/Evict candidates are filtered to that pool first (affinity), then `keepScore` among peers.

## 8. Configuration

Kind / controlplane default is `POLICY=semantic-score`.

```text
POLICY=semantic-score
SEMANTIC_TTL=20s
SEMANTIC_W_L=3 SEMANTIC_W_U=2 SEMANTIC_W_F=2 SEMANTIC_W_C=1
SEMANTIC_OVERRIDE=false
SEMANTIC_WAIT_SEC=120            # Resume waits when all peers are tool_loop/lock; 0 = fail immediately
# L3 optional (HF weights from llm-semantic-router; not full SR)
SEMANTIC_CLASSIFIER=stub         # stub | local-hf
SEMANTIC_HF_DOMAIN=              # e.g. llm-semantic-router/<domain-classifier>
SEMANTIC_HF_EMBED=               # e.g. llm-semantic-router/<embed-model>
SEMANTIC_PRIOR_MIX=0.3           # weight of L3 prior when online urgency exists
SEMANTIC_EMBED_ALPHA=0.2         # weight of embeddingSim inside urgency_prior
```

## 9. Implementation phases

| Phase | Deliverable | Proof |
|-------|-------------|-------|
| **P1** | Types + `POST /v1/signals/semantic` + CP attained/wait | Store unit tests |
| **P2** | `semantic-score`: lock filter + `keepScore` (no classifier) | Unit: spare `tool_loop`; prefer high-wait |
| **P3** | Any agent L1 via HTTP; demo `SEMANTIC_MODE=http` | 3v2 live: kick `llm_wait`, spare `tool_loop` |
| **P4** | Eval metrics (§10) | Tables vs `fifo` / `resource-evict` |
| **P5** | L3 local HF prior — see **§6.7** (types/policy → stub → `local-hf`) | Prior ignored when confidence low; A/B by tier/domain |

## 10. Metrics

- Mid-`tool_loop` interrupt / Suspend rate (↓ vs `fifo` under load)
- Resume latency by urgency / wait cohort
- Starvation override count
- Cross-Worker resume rate
- Fallback rate (no semantic / L0)
- (L3) prior coverage, confidence histogram, lift by `complexitySignal` / `domain` when prior enabled

## 11. Non-goals

- Replacing inference semantic routers (model/KV scheduling)—we **reuse HF signal models**, not SR’s routing stack
- Deploying the full [vLLM Semantic Router](https://github.com/vllm-project/semantic-router) binary / Envoy / decision DSL
- Predicting agent tool-step counts / remaining horizon
- Requiring every agent to embed a classifier
- Hard “difficulty ⇒ immortal session”
- Replacing resource plugins (`KeepAliveH` remains an optional cost input)
- Changing Suspend as the eviction mechanism
- Pulling every vLLM-SR head (PII, jailbreak, HaluGate, …) in L3 v1

## Related

- [signal-plugins.md](./signal-plugins.md) — resource vs agent semantic producers
- [scheduling.md](./scheduling.md) — Place/Evict mechanics and other policies
- [../research/literature.md](../research/literature.md) — Crab, Agentix, SAGA, HexAGenT, SAAR, IceBreaker
- [../research/baselines.md](../research/baselines.md) — baselines to beat
- [HF llm-semantic-router](https://huggingface.co/llm-semantic-router) — **L3 model weights only** (not the router product)
- `demos/agent-llm-multiplex/` — reference L1 producer (DeepSeek + sandbox tools)
