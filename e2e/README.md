# E2E tests

Live-cluster checks against Kind (gVisor Workers + rustfs). Cluster suites use
`-tags=e2e` so they are **not** part of plain `go test ./...`.

## Layout

| Path | Role |
|------|------|
| `internal/harness/` | Shared controlplane client (port-forward, API helpers) |
| `functional/` | Correctness: policies can place/evict as designed |

Policy **performance** CI uses dataset replay:
`hack/replay-agent-semantic.py` + `docs/eval/datasets/agent-semantic@v2/`
(2 Workers, 8 agents contending).

## How to run

```bash
./hack/kind-up.sh
./hack/verify-local.sh                           # functional only (default)
E2E_SUITE=agent-semantic ./hack/verify-local.sh  # dataset policy compare
E2E_SUITE=all ./hack/verify-local.sh             # both
```

CI: **functional** + **e2e-eval** (`random` / `resource-evict` / `semantic-score-l1` / `semantic-score`).

## Functional suite

| Test | Covers |
|------|--------|
| `TestGoldenEnsureAndColdResume` | Golden exists; cold resume → running |
| `TestFSPreservedAcrossPause` | Pause/resume keeps FS |
| `TestFSPreservedAcrossSuspend` | Suspend/resume keeps FS |
| `TestScheduleOversubscribeEvicts` | N>Workers; victim has objectKey |
| `TestPauseStickyToSameWorker` | Pause sticky to same Worker |
| `TestSuspendMigratesOffOrigin` | Suspend + occupy origin → migrate |
| `TestResourceSignalsAllMetricsPositive` | GET signals; runtime/snapshot/worker **all numeric fields > 0** |
| `TestPolicyFifoEvictsOldestCreated` | `fifo` victim = oldest CreatedAt |
| `TestPolicyLRUIdleEvictsLongestIdle` | real `exec` + Worker push; kick longer-idle |
| `TestPolicyResourceEvictGDS` | inflate `/dev/shm` RSS only (no re-Suspend); heavy evicted |
| `TestPolicyRandomEvictsUnderContention` | `random` evicts someone; third resume runs |
| `TestPlace*UsesFreeWorker` | Place: second resume avoids occupied Worker |

## Agent-semantic eval (CI)

| Env | Default | Meaning |
|-----|---------|---------|
| `AGENT_SEMANTIC_LIMIT` | 8 | Sessions from `@v2` |
| `AGENT_SEMANTIC_INFLIGHT` | 8 | Concurrent agents (2 workers → contention) |
| `AGENT_SEMANTIC_MIN_WORKERS` | 2 | Healthy workers required |
| `AGENT_SEMANTIC_SPEED` | 60 | Arrival/phase time compression |
| `AGENT_SEMANTIC_POLICIES` | random,resource-evict,semantic-score-l1,semantic-score | Ablation matrix |

`semantic-score-l1` = L1 lock only (`SEMANTIC_PRIOR_MIX=0`); `semantic-score` = L1+L3 (`0.3`).

Outputs `docs/eval/results/agent_semantic_v2__*.json` + `policy_compare_agent_semantic_v2.md`.
Gate: every policy `sessions_failed=0`.
