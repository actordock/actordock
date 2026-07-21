# Scheduling (platform view)

Status: v1 with Pause/Suspend + `fifo` / `random` / `lru-idle` / `resource-evict`.

Hard constraint: placement/eviction *mechanics* (1 running sandbox per Worker, sticky Pause, portable Suspend) follow Substrate. Inventiveness belongs in pluggable **policies** and research metrics—not in inventing a different density model without studying Substrate first.

## Model (Substrate-aligned)

**One running sandbox per Worker** (`MaxSlots=1`). Density comes from time-multiplexing the Worker pool via Suspend/Resume—not packing multiple sandboxes onto one Pod.

## Snapshot-aware decisions

| State | Resume rule |
|-------|-------------|
| `paused` | Sticky to last Worker (local snapshot only) |
| `suspended` | Prefer sticky if local still present; else any idle Worker + rustfs download |

Eviction under Worker pressure uses **Suspend** so victims stay portable.

## Policies

| Policy | Place | Evict |
|--------|-------|-------|
| `fifo` | Least-loaded idle Worker (load tie-break) | Oldest running (frees its Worker) |
| `random` | Random idle Worker | Random running |
| `lru-idle` | Least-loaded idle Worker | Longest `runtime.lastActiveAt` idle (pure LRU) |
| `resource-evict` | Least-loaded idle Worker | Lowest FaasCache / **GreedyDual-Size** keep-alive `H` |

Policy chooses **which Worker**; it does not co-locate multiple agents on one Worker.

### `resource-evict` algorithm (GreedyDual-Size)

**Source:** [FaasCache (ASPLOS’21)](https://dl.acm.org/doi/10.1145/3445814.3446757) keep-alive framed as caching; priority formula is classic [GreedyDual-Size (Cao & Irani, USITS’97)](https://pages.cs.wisc.edu/~cao/papers/gd-size.html). Frequency is fixed at 1 (GD-Size, not full GDSF), because Actordock does not yet track invocation counts.

**When to evict:** Only when a Place/Resume needs a slot and **no eligible idle Worker** exists (same resource-conserving rule as FaasCache: keep warm until capacity is needed).

**Keep-alive priority:**

```text
H = L + Cost / Size
```

| Symbol | Meaning in Actordock |
|--------|----------------------|
| `L` | Global aging clock in `signals.Store`; after choosing a victim, `L := H(victim)` (`OnEvict`) |
| `Cost` | Re-materialization cost (seconds): `lastPreemptCostSec + lastRestoreDur`; else `lastCheckpointDur`; else `1` (GD-Size(1)) |
| `Size` | Memory footprint (MiB): `runtime.memRSSBytes`; else `lastCheckpointBytes`; else `1` |
| Access | When `lastActiveAt` advances (Worker push after boot/restore/exec), refresh `H = L + Cost/Size` |
| Cost update | After checkpoint/restore recording, refresh `H` |
| Busy filter | `snapshot.checkpointInProgress` → treat as `H = +∞` (do not evict mid-checkpoint) |

**Victim = argmin H** among running sandboxes (tie-break: earlier `CreatedAt`).

**Place:** Among Workers with free slots and healthy signals, prefer lower `worker.cpuUtil + worker.memUtil`, skip hosts mid-checkpoint.

**Not used in H (intentional):** `runtime.cpuUtil` (FaasCache uses size/cost/recency, not instantaneous CPU); timestamp fields `lastCheckpointAt` / `lastRestoreAt` (aging is via `L` + access refresh). They may still be collected for observability.

See [signal-plugins.md](./signal-plugins.md) for the three signal objects and producers.

## Related

- [signal-plugins.md](./signal-plugins.md)
- [../decisions/0003-pause-suspend-rustfs.md](../decisions/0003-pause-suspend-rustfs.md)
