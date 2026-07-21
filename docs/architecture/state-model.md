# State model

Status: skeleton.

Hard constraint: object meanings (Sandbox/Actor, Worker, Template, Snapshot; Pause vs Suspend) stay Substrate-aligned. Do not rename or reshape the model without reading Substrate’s glossary/architecture and recording why.

## Core objects

| Object | Meaning |
|--------|---------|
| Sandbox | One agent session; may be running or suspended; carries semantic labels |
| Worker | A warm Pod hosting at most one running sandbox (`MaxSlots=1`) |
| Template | Golden image/snapshot used to speed cold start |
| Snapshot | Checkpoint of a suspended sandbox (local and/or remote) |

## Invariants (draft)

- Sandbox identity is stable across Worker moves.
- A running sandbox occupies exactly one Worker (1:1 while running).
- A suspended/paused sandbox occupies no Worker; location is snapshot metadata + scheduling state.
- Worker pool membership is managed via Kubernetes; per-sandbox churn is not.
