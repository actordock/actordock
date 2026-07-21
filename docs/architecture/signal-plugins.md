# Signal plugins (resource vs agent semantic)

Status: **resource plugin v1 implemented** (Worker push + sandbox/worker stores + `lru-idle` and FaasCache/`resource-evict`). Agent semantic: not implemented.

Scheduling policies today (`fifo`, `random`) mainly use control-plane Worker/sandbox state; `lru-idle` and `resource-evict` also read the short-TTL signal cache. We split signal producers into two plugin families so metrics, trust boundaries, and policy code stay separate.

## What is a signal plugin?

A **signal plugin** samples or receives telemetry and publishes **normalized sandbox signals** to the control plane (short TTL cache). **Policies** read those signals at `Place` / `Resume` time; they do not scrape Prometheus or talk to sandboxes directly.

```
Worker / Sandbox          Control plane              Policy
     │                         │                        │
     ├─ Resource plugin ──────►│ SandboxSignals cache ──►│ lru-idle, …
     └─ Semantic plugin ──────►│                        │ semantic-*, …
```

Observability (OTel `/metrics`) may **duplicate** the same samples for eval; the **scheduling hot path** uses the signal cache, not scrape latency.

## Resource object model (implemented)

Signals are stored as **per-sandbox** (`SandboxSignals`: runtime + snapshot + keep-alive `H`) and **per-worker** (`WorkerResource`) rows in `signals.Store` (TTL, default 30s).

| Object | Scope | Fields | Used by `resource-evict` |
|---|---|---|---|
| `runtime` | running sandbox | `memRSSBytes`, `lastActiveAt` | **Size** (`memRSS`); **access** refreshes `H` when `lastActiveAt` advances |
| | | `cpuUtil` | Observability / future; **not** in GDS `H` |
| `snapshot` | checkpoint/restore cost | `lastCheckpointBytes`, `lastPreemptCostSec`, `lastCheckpointDur`, `lastRestoreDur`, `checkpointInProgress` | **Cost** / **Size** fallback; busy filter |
| | | `lastCheckpointAt`, `lastRestoreAt` | Observability only |
| `worker` | Worker/Pod | `cpuUtil`, `memUtil`, `healthy`, `maxSlots`, `usedSlots`, `memBytes` | **Place** least-loaded; skip unhealthy |
| derived | sandbox | `keepAliveH` | Evict **argmin H**; `L` advanced via `OnEvict` |

Legacy flat JSON fields (`cpuUtil`, `memRSSBytes`, `lastActiveAt`) normalize into `runtime` on ingest.

### `resource-evict` = FaasCache / GreedyDual-Size

Full formula and when-to-kick details: [scheduling.md](./scheduling.md#resource-evict-algorithm-greedy-dual-size).

```text
H = L + Cost / Size     # Frequency := 1 (GD-Size)
evict argmin H when no idle Worker
```

Producers: Worker `ApplyPush`; control plane `RecordCheckpoint` / `RecordRestore`; scheduler `OnEvict` after victim choice.

## Two families

| | **Resource (serverless-style)** | **Agent semantic** |
|---|--------------------------------|--------------------|
| **Question it answers** | “Is this slot actually being used? How heavy is the sandbox on CPU/memory?” | “What is the agent *doing* (wait LLM, run tool, idle)? How urgent is *this turn*?” |
| **Serverless analogy** | Warm-pool keep-alive, LRU eviction, load-aware reclaim | Session/workflow priority, “don’t interrupt while user-visible work runs” |
| **Primary sampler** | **Worker** (cgroups / container stats for the one running sandbox) | **Sandbox** (SDK, sidecar, or trusted agent process) |
| **Trust model** | Platform-measured; agent cannot lie about CPU/RSS | Agent-reported; validate/version; treat as hint unless attested later |
| **Typical signals** | `runtime.*`, `snapshot.*`, `worker.*` | `phase` (e.g. `waiting_external`, `tool_exec`, `idle`), optional `workflow_id`, `deadline`, user-facing flag |
| **Main use in Actordock** | **Evict** least-valuable running sandbox when pool is full; optional cost hints for cross-Worker resume | **When** to suspend (opportunistic windows), **boost** workflow/session; filter “do not evict while …” |
| **Not its job** | Business tenant tier alone (that’s API `priority`); picking GPU vs CPU node pools at K8s layer | Measuring RSS; replacing cgroup metrics |
| **Planned policies** | `lru-idle` (pure runtime LRU) and `resource-evict` (FaasCache / GreedyDual-Size), combined with sticky resume | `semantic-opportunistic`, workflow-aware scoring (names TBD) |
| **Literature bucket** | FaasCache, IceBreaker, Incendio (keep-alive / priority under cold start) | Crab (wait windows), Agentix/SAGA/HexAGenT (session/workflow), smetric (session routing) — see [../research/literature.md](../research/literature.md) |

## Resource plugin (traditional serverless)

**Goal:** Under scarce Worker slots (`MaxSlots=1`), reclaim sandboxes that **hold a slot but barely use compute**, similar to evicting cold warm containers in FaaS.

**Where to sample:** Prefer **Worker**, because each Worker hosts at most one running sandbox and the platform already owns the cgroup boundary. Sandbox-internal CPU/memory tools are redundant for v1 and widen the trust surface.

**What policies consume:**

- **Eviction (`resource-evict`):** GreedyDual-Size `H = L + Cost/Size` (FaasCache); victim = lowest `H`. Cost from snapshot preempt/restore; Size from RSS/checkpoint bytes; access via `lastActiveAt`.
- **Eviction (`lru-idle`):** longest runtime idle only.
- **Placement:** sticky-first on Resume when last Worker is free; else least-loaded idle Worker using `worker` CPU/mem util.
- **Not in GDS `H`:** `runtime.cpuUtil`, checkpoint/restore timestamps (see [scheduling.md](./scheduling.md)).

**User-facing “priority”:** Resource plugins do **not** replace tenant **scheduling priority** (a field on Create). That is a separate scalar for “who should get a slot first.” Resource signals answer “who is wasting a slot right now.” Policies may combine both: e.g. never evict above priority P unless pool critical; among equals, LRU by resource plugin.

## Agent semantic plugin

**Goal:** Use **application meaning** to avoid bad evictions (e.g. suspend while a tool call is in flight) and to prioritize sessions/workflows, without conflating that with cgroup idle.

**Where to sample:** **Inside the sandbox** (or via a sidecar the platform launches). The control plane cannot infer “waiting on LLM” from CPU alone.

**What policies consume:**

- **Eviction filters:** e.g. do not select victims in `tool_exec` unless starvation override.
- **Opportunistic suspend:** prefer victims in `waiting_external` or long `idle` when semantic + resource agree.
- **Resume ordering:** boost sandboxes with near deadline or active workflow step (works with API `priority`).

**Trust and versioning:** Semantic payloads should be **typed, versioned**, and rate-limited. v0 can be best-effort (eval harness only); production may require signed reports or sidecar-only injection.

## Sandbox semantic vs inference “semantic routers”

Many OSS projects use **semantic** in the name but solve a **different layer** than Actordock’s agent semantic **signal plugin**. We are **not** building another MoM gateway or in-process tool router. We consume **small, normalized lifecycle signals** to allocate **Worker slots** under **checkpoint/resume (C/R)** cost.

### What is scarce?

| Layer | Scarce resource | Typical decision |
|-------|-----------------|------------------|
| **Inference routing** (e.g. [vLLM Semantic Router](https://github.com/vllm-project/semantic-router), [aurelio-labs/semantic-router](https://github.com/aurelio-labs/semantic-router)) | GPU / model replicas, KV cache warmth, token spend | Which **backend model** or **tool route** handles **this request** |
| **GPU agent schedulers** (Agentix, SAGA, SMetric — [literature index](../research/literature.md)) | Prefill/decode capacity on an inference cluster | Session/workflow **fairness and SLO** on GPUs |
| **Actordock (this doc)** | **Worker slots** (`MaxSlots=1` running sandbox per Pod) | **Resume / Place / Evict (Suspend)** — who keeps a warm **gVisor sandbox** on which Worker |

### Same word, different consumer

**Inference semantic routers** (vLLM SR is the common reference):

- Sit on the **data path** (e.g. Envoy ext_proc): classify **request/session intent** → pick a **physical model** or policy path (MoM, cost, PII guards).
- **Session-Aware Agentic Routing (SAAR)** uses `phase`, tool-loop locks, and cache-aware switch pricing so **multi-turn model choice** does not destroy **prefix/KV cache** on inference nodes.
- Endpoint health, load balancing, and failover stay **infrastructure-owned** (K8s/Envoy/serving stack)—the router chooses **model identity**, not **Pod occupancy**.

**Actordock sandbox semantic plugin:**

- Sits on the **control plane** scheduling hot path: signals attach to **`sandboxID`**, not to a single HTTP request ID.
- Similar *labels* (e.g. `waiting_external`, `tool_exec`, workflow step) may appear in both systems, but here they drive **whether to Suspend**, **who to evict**, and **resume ordering**—not **which LLM endpoint** serves the next token.
- State is **whole sandbox memory + filesystem**; moving work is **Pause/Suspend + rustfs**, with metrics like `preempt_cost`, `cross_worker`, `resume_path`—not merely cache miss on one model.

**aurelio-labs/semantic-router** is another layer again: a **library** for fast vector-based **tool/route choice inside the agent app**, not cluster slot allocation.

### Positioning (what we claim and do not claim)

| Claim | Rationale |
|-------|-----------|
| **Do not claim** “we replace semantic-router / AI gateway” | Different product surface; complementary if co-deployed |
| **Do not claim** green field on “semantics + agent sandboxes” alone | Crab and inference routers already use turn/phase signals |
| **Do claim** gap on **priority × Worker-slot allocation under C/R**, with **shared eval** (baselines, datasets, `/metrics`) | See [../research/literature.md](../research/literature.md) |
| **Do claim** differentiated **sandbox lifecycle semantics**: signals → **slot** policy, bounded trust, optional **read-only** import from gateway session metadata—not re-implementing classification or MoM |

### Co-deployment (optional)

Router and Actordock can **align** without merging code: e.g. gateway session `phase` exported as read-only input to the semantic signal plugin, while Actordock still owns Suspend/Evict. Router optimizes **inference**; Actordock optimizes **multiplexed sandbox capacity**.

## Composition rules

1. **Plugins publish; policies decide.** No policy-specific sampling in `internal/policy/*`.
2. **Resource before semantic.** Resource plugin + one consumer policy (`lru-idle` / `resource-evict`) proves the signal pipeline and eval loop.
3. **Semantic extends, does not fork, the pipeline.** Same `SandboxSignals` store; semantic fields are optional keys. Missing semantic data → policies fall back to resource + fifo/priority baselines.
4. **Pause / Suspend mechanics unchanged.** Signal plugins influence **which** sandbox is suspended, not whether Suspend remains the eviction primitive ([scheduling.md](./scheduling.md)).

## Control-plane sketch (future)

| Piece | Role |
|-------|------|
| `SignalRegistry` | Register resource vs semantic providers; merge into one view per `sandboxID` |
| Worker RPC | Push resource samples (interval + on Suspend) |
| Sandbox API / sidecar | Push semantic heartbeat (optional) |
| `PlaceRequest` / `ResumeRequest` | Add read-only `Signals map[string]SandboxSignal` or typed struct |
| Metrics | Mirror selected fields to OTel for [../research/metrics.md](../research/metrics.md) and e2e deltas |

## Related

- [scheduling.md](./scheduling.md) — policy hook and snapshot rules
- [../research/baselines.md](../research/baselines.md) — `lru-idle` / `priority-static` targets
- [../research/metrics.md](../research/metrics.md) — eval KPIs (including per priority class)
- [../research/literature.md](../research/literature.md) — papers and nearby systems (incl. inference semantic routers)
