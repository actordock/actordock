# ADR 0002: Kind + gVisor Worker multiplexing (v0)

## Status

Accepted (updated: 1 running sandbox per Worker)

## Context

We need a runnable Actordock that proves N sandboxes on M Workers with real gVisor checkpoint/restore, Kind deployment, and CI—before investing in priority research policies. Agent-sandbox semantics match Substrate: a Worker hosts at most one running Actor at a time.

## Decision

- Control plane + Worker agent over HTTP; Redis for high-churn metadata.
- Workers are privileged Pods that invoke `runsc` (platform `systrap`); **`MaxSlots=1`** (one running sandbox per Worker).
- Allocation policies for v0: `fifo` and `random` behind a shared `policy.Policy` interface (pick idle Worker / Suspend victim to free its Worker).
- Pause = local sticky; Suspend = portable via rustfs (see ADR 0003).
- E2E CI: Kind cluster, matrix over both policies, assert `running <= worker count` under oversubscription.

## Consequences

- Density is time-multiplexing across the Worker pool, not multi-tenant packing inside a Pod.
- Semantic priority research still plugs into the same Place/Evict/Resume hooks.
- Privileged Workers are acceptable for Kind/dev; harden later.
