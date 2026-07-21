# E2E tests

Live-cluster checks against Kind (gVisor Workers + rustfs). Cluster suites use
`-tags=e2e` so they are **not** part of plain `go test ./...`.

## Layout

| Path | Role |
|------|------|
| `internal/harness/` | Shared controlplane client (port-forward, API helpers) |
| `functional/` | Basic sandbox / scheduling correctness |
| `eval/` | Policy performance comparison via `/metrics` |

## How to run

```bash
./hack/kind-up.sh
./hack/verify-local.sh                 # functional only (default)
E2E_SUITE=eval ./hack/verify-local.sh  # eval (see below)
E2E_SUITE=all ./hack/verify-local.sh   # both
```

Or directly:

```bash
# functional correctness
go test ./e2e/functional/ -tags=e2e -count=1 -timeout=20m -v

# agent scenario eval (fifo vs random, per-scenario metrics delta)
go test ./e2e/eval/ -tags=e2e -count=1 -timeout=45m -v -run TestEvalScenariosFifoVsRandom

# legacy single combined workload
go test ./e2e/eval/ -tags=e2e -count=1 -timeout=45m -v -run TestEvalFifoVsRandom

# metrics parser unit test (no cluster)
go test ./e2e/eval/ -count=1 -v -run TestParsePromAndReport
```

## Functional suite

| Test | Covers |
|------|--------|
| `TestGoldenEnsureAndColdResume` | Golden exists; cold resume → running |
| `TestFSPreservedAcrossPause` | Pause/resume keeps FS |
| `TestFSPreservedAcrossSuspend` | Suspend/resume keeps FS |
| `TestScheduleOversubscribeEvicts` | N>Workers; victim has objectKey |
| `TestPauseStickyToSameWorker` | Pause sticky to same Worker |
| `TestSuspendMigratesOffOrigin` | Suspend + occupy origin → migrate |

## Eval suite (agent-oriented scenarios)

Each scenario runs under **fifo** and **random**. Metrics are **deltas** on controlplane `/metrics` for that scenario only.

| ID | Scenario | Agent behavior |
|----|----------|----------------|
| **S1** `S1_cold_start` | New session, first `resume` (golden) | Cold start |
| **S2** `S2_hot_wake` | `pause` → idle → `resume` sticky | Short idle, same Worker |
| **S3** `S3_migrate_sleep` | `suspend` → resume on another Worker | Portable sleep / migration |
| **S4** `S4_pool_contention` | Warm + suspend all, then second resume wave N>M | Pool oversubscription (latest, not golden) |
| **S5** `S5_stateful_agent` | FS + memory seed, sticky then migrate | Stateful agent C/R |

Env:

| Variable | Default | Use |
|----------|---------|-----|
| `MIN_WORKERS` | 4 | Worker pool size |
| `EVAL_SANDBOX_COUNT` | 2×workers | S4 sandbox count |
| `EVAL_COLD_COUNT` | 3 | S1 cold resumes |
| `EVAL_IDLE_SEC` | 2 | S2 pause-before-resume idle |
| `EVAL_STATE_FILE_KB` | 256 | S5 file payload (KB) |
| `EVAL_STATE_MEM_MB` | 8 | S5 `/dev/shm` footprint (MB) |

Does not fail on which policy wins — only that metrics exist per scenario.
