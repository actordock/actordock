# Eval

Home for **datasets**, experiment configs, and comparison results. Docs under `research/` define what to measure; this tree holds artifacts.

## Agent semantic-score workload

How to splice public agent + Azure arrival traces and evaluate policies (no hand-written tasks):

→ **[`agent-semantic-workload.md`](./agent-semantic-workload.md)**

## Layout

```
eval/
  README.md
  agent-semantic-workload.md
  datasets/agent-semantic@v2/   # BFCL splice package
  results/                      # replay outputs (gitignored except README)
```

## Dataset package (minimum)

Each dataset version directory should include:

- `manifest.yaml` — id, version, license, source, schema version
- `events` or `trace` file(s) — arrivals, state changes, priorities
- `README.md` — how to load / limitations

## Results package (minimum)

- Policy id + git commit + config hash
- Metrics table aligned with `docs/research/metrics.md`
- Optional: raw decision logs for debugging

CI job `e2e-eval` runs `E2E_SUITE=agent-semantic` in a **matrix** (one Kind cluster per
policy: `random`, `resource-evict`, `semantic-score-l1`, `semantic-score`);
Primary view: GitHub Actions **Summary** tab (full compare table + per-policy metrics).
Artifacts remain as optional archive.

Do not commit huge binary checkpoints here; link or use Git LFS if needed.
