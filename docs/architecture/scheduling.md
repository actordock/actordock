# Scheduling (platform view)

Status: v0 with Pause/Suspend + `fifo` / `random`.

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
| `fifo` | Earliest idle Worker | Oldest running (frees its Worker) |
| `random` | Random idle Worker | Random running (frees its Worker) |

Policy chooses **which Worker**; it does not co-locate multiple agents on one Worker.

Future policies (`lru-idle`, `priority-static`, semantic-aware) consume **signal plugins** (resource vs agent semantic). See [signal-plugins.md](./signal-plugins.md).

## Related

- [signal-plugins.md](./signal-plugins.md)
- [../decisions/0003-pause-suspend-rustfs.md](../decisions/0003-pause-suspend-rustfs.md)
