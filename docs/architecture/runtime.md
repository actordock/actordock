# Runtime

Status: skeleton.

Hard constraint: C/R flow, golden/template boot, and snapshot packaging must be learned from Substrate (`runsc` / atelet paths) before inventing Actordock-only schemes. Divergence needs an ADR.

## Role

The Runtime runs inside (or beside) a Worker and implements sandbox isolation and checkpoint/restore.

## Interface (intent)

- Create from template / image
- Checkpoint (suspend) and Restore (resume)
- Exec / attach / networking hooks as needed by the data plane
- Emit metrics: slot use, C/R latency, failures

## Backends

| Backend | Priority | Notes |
|---------|----------|-------|
| gVisor (`runsc`) | First | Primary fast C/R path |
| kubernetes-sigs/agent-sandbox (or similar) | Later | Optional backend; must not force 1:1 Pod model into the control plane |
| Others (Kata, …) | Open | Same Runtime interface |

Control plane and scheduler depend on the interface, not on a single vendor binary.
