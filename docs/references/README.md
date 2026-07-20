# References

Notes on external systems. Borrow ideas; do not copy implementations into this repo.

## Substrate

Worth referencing: Actor ≠ Pod, warm Worker multiplexing, suspend/resume + snapshots, control plane off the Kubernetes hot path, location-transparent wake-and-route.

Ignore for our rewrite: specific binaries/glue, forced sync into `runtime/`, GCP-centric install paths.

## kubernetes-sigs/agent-sandbox

Useful as: isolation/lifecycle primitives and a possible future Runtime backend.

Not our primary model: one Sandbox CR ≈ one Pod. Actordock’s differentiator remains N:M oversubscription and semantic/metric scheduling across shared Workers.

## gVisor

Mechanism for checkpoint/restore. Product value is scheduling density on top, not C/R alone.
