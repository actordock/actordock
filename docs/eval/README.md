# Eval

Home for **datasets**, experiment configs, and comparison results. Docs under `research/` define what to measure; this tree holds artifacts.

## Layout (intended)

```
eval/
  README.md           # this file
  datasets/           # versioned traces + schema
    README.md
  configs/            # experiment configs pinning dataset + policy
  results/            # metric outputs (csv/json); large blobs via LFS or external store
```

Directories may stay empty until the first dataset lands.

## Dataset package (minimum)

Each dataset version directory should include:

- `manifest.yaml` — id, version, license, source, schema version
- `events` or `trace` file(s) — arrivals, state changes, priorities
- `README.md` — how to load / limitations

## Results package (minimum)

- Policy id + git commit + config hash
- Metrics table aligned with `docs/research/metrics.md`
- Optional: raw decision logs for debugging

CI job `e2e-eval` writes `results/policy_compare.md` (four policies × S1–S5) and uploads it as the `policy-compare` artifact.

Do not commit huge binary checkpoints here; link or use Git LFS if needed.
