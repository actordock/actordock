# Roadmap

E2B-compatible self-hosted agent sandboxes on Kubernetes.

## Versioning

| Range | Meaning |
|-------|---------|
| **0.0.x** | Incremental proof → operable dev stack |
| **0.1.0** | Pilot-ready (files, security, Helm) |
| **1.0.x** | Product-ready (templates, auth, ops) |

## Progression (pre-0.1.0)

Each release adds one capability layer. You should be able to demo the **Target** column before moving on.

| Version | Target | You can… |
|---------|--------|----------|
| [v0.0.1](releases/v0.0.1.md) | **Proof** | Create a sandbox in-cluster, run `echo hello`, kill it |
| [v0.0.2](releases/v0.0.2.md) | **Visibility** | Look up sandbox id/status; list active sandboxes |
| [v0.0.3](releases/v0.0.3.md) | **Manual TTL** | Set and extend timeout; metadata survives Platform restart |
| [v0.0.4](releases/v0.0.4.md) | **Auto cleanup** | Expired sandboxes are killed without manual action |
| [v0.0.5](releases/v0.0.5.md) | **Idle suspend** | Sandbox pauses on timeout; next command wakes it |
| [v0.1.0](releases/v0.1.0.md) | **Pilot** | Files, secure envd, Helm install on real cluster |

## Releases

| Version | Status | Spec |
|---------|--------|------|
| [v0.0.1](releases/v0.0.1.md) | released | MVP |
| [v0.0.2](releases/v0.0.2.md) | released | Visibility |
| [v0.0.3](releases/v0.0.3.md) | released | Manual TTL |
| [v0.0.4](releases/v0.0.4.md) | released | Auto cleanup |
| [v0.0.5](releases/v0.0.5.md) | planned | |
| [v0.1.0](releases/v0.1.0.md) | planned | Pilot |

**Current focus:** [v0.0.5](releases/v0.0.5.md)

## Release doc template

`docs/releases/vX.Y.Z.md`: Status, Target, Goal (demo), In/Out scope, API, Milestones, Depends on, Done when.
