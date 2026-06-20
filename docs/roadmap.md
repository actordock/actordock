# Roadmap

E2B-compatible self-hosted agent sandboxes on Kubernetes.

## Versioning

| Range | Meaning |
|-------|---------|
| **0.0.x** | Incremental proof → operable dev stack + E2B REST coverage |
| **0.1.0** | Pilot-ready: **≥60% E2B REST ops + fields** (full-field rule), files, secure envd, Helm |
| **1.0.x** | Product-ready (template builds, full auth, ops) |

## E2B REST target

[v0.1.0](releases/v0.1.0.md) must ship **≥34 / 56** Platform operations and **≥60% of OpenAPI fields**. See [E2B REST parity tracker](releases/e2b-rest-parity.md).

**Full-field rule:** an operation counts as done only when **all** request/response schema fields for that route are implemented — no partial endpoints. Existing v0.0.1–v0.0.4 routes are marked `partial` until backfill completes (mostly v0.0.5–v0.1.0).

Current (v0.0.6): **14 ops (25%)**, **~38% fields** (observability routes done; sandbox list/create response fields still partial).

## Progression (pre-0.1.0)

Each release adds one capability layer. You should be able to demo the **Target** column before moving on.

| Version | Target | You can… | REST ops |
|---------|--------|----------|----------|
| [v0.0.1](releases/v0.0.1.md) | **Proof** | Create a sandbox, run `echo hello`, kill it | 7* |
| [v0.0.2](releases/v0.0.2.md) | **Visibility** | Look up sandbox id/status; list actives | (in 7) |
| [v0.0.3](releases/v0.0.3.md) | **Manual TTL** | Set/extend timeout; metadata in Redis | (in 7) |
| [v0.0.4](releases/v0.0.4.md) | **Auto cleanup** | Expired sandboxes killed by scheduler | (in 7) |
| [v0.0.5](releases/v0.0.5.md) | **Idle suspend** | Pause on timeout; command wakes sandbox | 9 |
| [v0.0.6](releases/v0.0.6.md) | **Observability** | Logs, metrics, timeout refresh (stub OK) | 14 |
| [v0.0.7](releases/v0.0.7.md) | **Real telemetry** | Real logs + metrics from envd | 14 |
| [v0.0.8](releases/v0.0.8.md) | **Sandbox extras** | Connect, network, snapshots | 18 |
| [v0.0.9](releases/v0.0.9.md) | **Volumes** | Volume CRUD (Platform) | 22 |
| [v0.0.10](releases/v0.0.10.md) | **Templates (read)** | List/get template, alias, tags | 27 |
| [v0.1.0](releases/v0.1.0.md) | **Pilot + 60% REST** | Files, secure envd, Helm; full sandbox + auth fields | **34 ops, ≥60% fields** |

\*v0.0.1–v0.0.4 cumulative count is measured at v0.0.4.

## Releases

| Version | Status | Spec |
|---------|--------|------|
| [v0.0.1](releases/v0.0.1.md) | released | MVP |
| [v0.0.2](releases/v0.0.2.md) | released | Visibility |
| [v0.0.3](releases/v0.0.3.md) | released | Manual TTL |
| [v0.0.4](releases/v0.0.4.md) | released | Auto cleanup |
| [v0.0.5](releases/v0.0.5.md) | released | Idle suspend |
| [v0.0.6](releases/v0.0.6.md) | released | Observability |
| [v0.0.7](releases/v0.0.7.md) | planned | Real telemetry |
| [v0.0.8](releases/v0.0.8.md) | planned | Sandbox extras |
| [v0.0.9](releases/v0.0.9.md) | planned | Volumes |
| [v0.0.10](releases/v0.0.10.md) | planned | Templates (read) |
| [v0.1.0](releases/v0.1.0.md) | planned | Pilot + 60% REST |

**Current focus:** [v0.0.7](releases/v0.0.7.md)

## Release doc template

`docs/releases/vX.Y.Z.md`: Status, Target, Goal (demo), In/Out scope, API, Milestones, Depends on, Done when.

REST-facing releases also note **new operations (full fields)** and link to [e2b-rest-parity.md](releases/e2b-rest-parity.md). Partial field backfill is tracked there.
