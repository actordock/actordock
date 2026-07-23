# agent-semantic@v2

Agent workload for **semantic-score** (all sessions have `n_tools≥3`).

| Source | Role |
|--------|------|
| AgentProcessBench **bfcl** (primary) + **tau2** (pad) | Task text + tool trajectories |
| Azure Functions 2019 day01 | Arrival wave spacing only |

Phase spans use seeded random durations: llm_wait 2–8s, tool_loop 0.1–1.5s (always shorter).

```bash
./hack/build-agent-semantic-dataset.py --target 200 --min-tools 3 --seed 42 --classify hf
```

See `summary.json`. Contract: [`../../agent-semantic-workload.md`](../../agent-semantic-workload.md).
