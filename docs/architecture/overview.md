# Architecture overview

Status: skeleton — fill in as implementation lands.

The platform exists to **run and measure** sandbox priority/allocation policies. Research framing: [`../research/problem.md`](../research/problem.md).

## Layers

```
Clients
   │
   ▼
Control plane  ── placement / packing / resume target
   │                 metadata + metrics
   ▼
Data plane     ── sandboxID → Worker (resume if suspended, then proxy)
   │
   ▼
Worker pool    ── warm Pods; local Runtime (gVisor, …)
   │
   ▼
Snapshots      ── local and/or remote checkpoint images
```

Kubernetes owns Worker pool size and health. Actordock owns sandbox identity, scheduling, and routing.

## Components (intended)

| Component | Role |
|-----------|------|
| API / control plane | Sandbox CRUD, policy, scheduler |
| Router | Resolve location; trigger resume; forward traffic |
| Worker agent | Create / checkpoint / restore; report metrics |
| State store | High-churn sandbox/Worker state (not etcd per op) |
| Snapshot store | Checkpoint durability and fetch for cross-Worker resume |

## Related

- [scheduling.md](./scheduling.md)
- [state-model.md](./state-model.md)
- [runtime.md](./runtime.md)
