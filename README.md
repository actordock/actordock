# Actordock

> **IMPORTANT:** Experimental research project. Not production-ready.

Actordock studies **sandbox multiplexing under scarce Workers**:

- **Worker reuse** — many sandboxes share a small Worker pool via Pause / Suspend / Resume (N:M), not one Pod per sandbox.
- **Agent-semantic policy plugin** — on the same reuse hooks, decide who yields the slot using agent phase/lock and session score, not arrival order alone.

## Components

- **controlplane** — scheduling / Place·Suspend·Resume, signals, and policy
- **worker** — gVisor runtime + local snapshots + upload/download
- **policy** — pluggable strategies (including `semantic-score`)
- **snapshotstore** — rustfs / object-store snapshots

## Run

```bash
./hack/kind-up.sh
./hack/verify-local.sh   # functional e2e (default)
```

