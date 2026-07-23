# Datasets (artifacts)

Place versioned dataset packages here (`<name>@v<N>/...`). Requirements: [../../research/datasets.md](../../research/datasets.md).

## Available

| Package | Builder | Notes |
|---------|---------|-------|
| [`agent-semantic@v2`](./agent-semantic@v2/) | `./hack/build-agent-semantic-dataset.py --target 200 --min-tools 3` | **Preferred.** BFCL+tau2; all `n_tools≥3`; L3 cohorts + spaced waves |
