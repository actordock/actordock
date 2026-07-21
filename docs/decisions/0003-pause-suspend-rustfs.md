# ADR 0003: Pause/Suspend + rustfs snapshot store

## Status

Accepted

## Context

Substrate keeps short-term checkpoints on the node (Pause, sticky resume) and uploads Suspend snapshots to object storage (GCS/S3; Kind uses rustfs) so Actors can resume on any Worker.

## Decision

- **Pause**: local checkpoint only; resume sticky to the same Worker.
- **Suspend**: checkpoint + per-file sparse-zstd upload under rustfs prefix `sandboxes/<id>/` (files as `<name>.zstd`, then `manifest.json`); resume may target any Worker (download if local missing). Prefer sticky when local snapshot still exists.
- Object layout matches Substrate atelet (ATESPRSE sparse-extent + plain zstd fallback), not a single tar blob.
- Eviction under Worker pressure uses **Suspend** so victims remain portable.
- Workers own S3 I/O; control plane stores `objectKey` (prefix) / `localSnapshotPath` metadata in Redis.

## Consequences

- Aligns with Substrate’s dual-path model and resume transfer mechanics.
- Kind depends on rustfs + bucket-init Job.
- Cross-Worker resume latency includes download + decompress cost (scheduling signal later).
