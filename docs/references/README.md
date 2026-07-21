# References

Notes on external systems. **Platform work must start from Substrate**, not from scratch.

## Substrate

**Mandatory reference** for almost all platform technical detail. Read the Substrate tree and docs before designing Actordock equivalents. Align first; diverge only with a written reason (ADR or research need).

Must learn from: Actor ≠ Pod, warm Worker multiplexing, Pause vs Suspend, snapshot layout/codec (sparse-extent + zstd), control plane off the Kubernetes hot path, locality-aware resume, location-transparent wake-and-route when we add a data plane.

Do not: vendor/sync Substrate into this repo, force GCP-only install paths, or invent parallel mechanisms without checking how Substrate already does it.

## kubernetes-sigs/agent-sandbox

Useful as: isolation/lifecycle primitives and a possible future Runtime backend.

Not our primary model: one Sandbox CR ≈ one Pod. Actordock’s differentiator remains N:M oversubscription and semantic/metric scheduling across shared Workers.

## gVisor

Mechanism for checkpoint/restore. Product value is scheduling density on top, not C/R alone.
