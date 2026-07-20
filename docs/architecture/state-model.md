# State model

Status: skeleton.

## Core objects

| Object | Meaning |
|--------|---------|
| Sandbox | One agent session; may be running or suspended; carries semantic labels |
| Worker | A warm Pod with slots, resources, and a Runtime |
| Template | Golden image/snapshot used to speed cold start |
| Snapshot | Checkpoint of a suspended sandbox (local and/or remote) |

## Invariants (draft)

- Sandbox identity is stable across Worker moves.
- A running sandbox occupies exactly one Worker slot.
- A suspended sandbox occupies no Worker slot; location is snapshot metadata + scheduling state.
- Worker pool membership is managed via Kubernetes; per-sandbox churn is not.
