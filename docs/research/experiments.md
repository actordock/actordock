# Research: experiments

## Protocol (draft)

1. Fix dataset version + Worker pool description.
2. Fix Runtime/C/R costs model (measured or replayed).
3. Run each policy in [baselines.md](./baselines.md) plus candidates.
4. Export metrics from [metrics.md](./metrics.md) to `eval/results/`.
5. Record seed, commit SHA, and config hash.

## Modes

| Mode | Use |
|------|-----|
| Offline replay / simulation | Fast policy search; no cluster required |
| Online on platform | Validate C/R and metric pipelines; catch real costs |

Prefer offline for paper-style sweeps; confirm winners online on a small Worker pool.

## Artifact layout

See [eval/README.md](../eval/README.md).
