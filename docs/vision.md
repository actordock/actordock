# Vision

## One-liner

Research and build a **sandbox priority / allocation** layer on top of warm Worker multiplexing: under scarce slots, decide who runs, who suspends, and who resumes—using agent semantics and live metrics—and **measure** strategies against shared datasets and baselines.

## Research core

**Question:** Given limited Worker capacity and checkpoint/restore cost, how should sandboxes be prioritized and placed so that high-priority / latency-sensitive agents stay responsive while density stays high?

The system (gVisor C/R, Worker pool, routing) exists so strategies are **runnable, observable, and comparable**—not as an end in itself.

## System goals (platform)

- Multiplex many idle sessions onto few Workers via fast suspend/resume.
- Expose a pluggable **scheduling policy** hook (priority + semantics + metrics).
- Keep sandbox scheduling off the Kubernetes hot path; K8s manages Worker capacity.
- Pluggable Runtime (gVisor first; later backends optional).

## Non-goals

- Treating “one sandbox = one Pod” as the primary model.
- E2B API parity as the product definition.
- Vendoring/syncing Substrate as a dependency (orphan rewrite stays).
- Claiming a final allocation algorithm before baselines and datasets exist.

## Hard constraint: no closed-door platform work

Platform mechanics are **not** a blank slate. Before changing Worker multiplexing, Pause/Suspend, snapshots, control-plane hot path, or runtime C/R, **study Substrate** and align unless an ADR records a deliberate divergence.

- Innovate on: priority/allocation research and measurement.
- Do not invent alternate platform internals just because we do not vendor Substrate.

## Differentiation

Infrastructure peers focus on isolation or 1:1 sandbox Pods. Actordock focuses on **oversubscribed allocation under priority**—with an eval loop (metrics, baselines, datasets, literature) to justify the policy. Platform shape stays Substrate-aligned so research results stay comparable.
