# Architecture overview

Status: Kind + gVisor multiplexing with Substrate-aligned Pause/Suspend.

## Hard constraint

Platform layers below must track Substrate’s proven paths. **No closed-door redesign** of multiplexing, Pause/Suspend, snapshot I/O, or Worker/runtime wiring. Study Substrate first; document any intentional divergence in an ADR. Research novelty lives in scheduling *policy* and eval, not in reinventing the platform substrate.

## Layers

```
Clients
   │
   ▼
Control plane  ── place / pause / suspend / resume (policy: fifo|random)
   │                 Redis metadata
   ▼
Worker agents  ── runsc + local snapshots + rustfs upload/download
   │
   ├─ Pause     → local disk only (sticky resume)
   └─ Suspend   → local checkpoint + rustfs prefix (per-file sparse-zstd)
```

## Snapshot model (Substrate-aligned)

| Op | Local | rustfs | Resume |
|----|-------|--------|--------|
| Pause | yes | no | same Worker |
| Suspend | yes (source node) | yes | any Worker (download if needed) |

Eviction under Worker pressure uses **Suspend**.

## Components

| Component | Path |
|-----------|------|
| controlplane | `cmd/controlplane` |
| worker | `cmd/worker` |
| policy | `internal/policy` (`fifo`, `random`) |
| snapshotstore | `internal/snapshotstore` (S3/rustfs + FS for tests) |
| Kind | `manifests/kind/{actordock,rustfs}.yaml` |
| Bring-up / verify | `hack/kind-up.sh`, `hack/verify-local.sh` → `go test ./e2e/ -tags=e2e` |
| E2E scenarios | `e2e/*_test.go` (build tag `e2e`) |

## Related

- [scheduling.md](./scheduling.md)
- [../decisions/0002-kind-gvisor-multiplex.md](../decisions/0002-kind-gvisor-multiplex.md)
- [../decisions/0003-pause-suspend-rustfs.md](../decisions/0003-pause-suspend-rustfs.md)
