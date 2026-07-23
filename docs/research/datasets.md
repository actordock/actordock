# Research: datasets

Datasets are first-class: without shared traces, priority policies cannot be compared fairly.

## What a dataset should contain

- Sandbox arrival / idle / active intervals (or event log).
- Priority (and other semantic labels) per sandbox.
- Optional: resource demand, checkpoint size, affinity hints.
- Worker pool description (count, capacity) used for the experiment.
- License and provenance.

## Sources (planned)

| Source | Status | Notes |
|--------|--------|-------|
| Synthetic generator | TBD | Controlled priority mixes and burstiness |
| Replayed production-like traces | TBD | Anonymized if needed |
| Public traces from related systems | **Available** | Prefer `agent-semantic@v2` (BFCL + cohorts); see [`../eval/datasets/agent-semantic@v2/`](../eval/datasets/agent-semantic@v2/) |

Concrete files and schemas live under [`../eval/`](../eval/) once collected. This doc defines **requirements**; eval holds **artifacts**.

## Versioning

Name datasets (`name@vN`), pin them in experiment configs, never silently edit a published version.
