# agent-semantic@v2

Single-source agent workload for **semantic-score**.

| Source | Role |
|--------|------|
| AgentProcessBench **BFCL** | Task text + tool trajectories |
| Azure Functions 2019 day01 | Arrival wave spacing only |

```bash
./hack/build-agent-semantic-dataset.py --target 200 --classify hf
```

See `summary.json`. Contract: [`../../agent-semantic-workload.md`](../../agent-semantic-workload.md).
