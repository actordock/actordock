# E2E tests

Live-cluster checks against Kind (gVisor Workers + rustfs). Cluster suites use
`-tags=e2e` so they are **not** part of plain `go test ./...`.

## Layout

| Path | Role |
|------|------|
| `internal/harness/` | Shared controlplane client (port-forward, API helpers) |
| `functional/` | Correctness: all four policies can place/evict as designed |
| `eval/` | Performance: S1–S5 under four policies + markdown comparison table |

## How to run

```bash
./hack/kind-up.sh
./hack/verify-local.sh                 # functional only (default)
E2E_SUITE=eval ./hack/verify-local.sh  # four-policy eval + docs/eval/results/policy_compare.md
E2E_SUITE=all ./hack/verify-local.sh   # both (same cluster; sequential)
```

CI runs **functional** and **eval** as **two parallel jobs / two Kind clusters**
(`actordock-functional`, `actordock-eval`).

Or directly:

```bash
go test ./e2e/functional/ -tags=e2e -count=1 -timeout=20m -v

go test ./e2e/eval/ -tags=e2e -count=1 -timeout=60m -v -run TestEvalAllPolicies
# artifact: EVAL_OUT_DIR/policy_compare.md (default docs/eval/results/)
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
| `TestPolicyFifoEvictsOldestCreated` | `fifo` victim = oldest CreatedAt |
| `TestPolicyLRUIdleEvictsLongestIdle` | real `exec` + Worker push; kick longer-idle |
| `TestPolicyResourceEvictGDS` | real `/dev/shm` + suspend/resume; heavy kept (higher Cost/Size H), light evicted |
| `TestPolicyRandomEvictsUnderContention` | `random` evicts someone; third resume runs |
| `TestPlace*UsesFreeWorker` | Place: second resume avoids occupied Worker (`MaxSlots=1`, live `/status`) |

No API-injected fake metrics. Evict idle/GDS use **Worker push** + **checkpoint/restore records**. Place under `MaxSlots=1` asserts **real free slots** (busy Worker is ineligible); load-tiebreak among multiple idle Workers is not forced in e2e (would need synthetic node pressure).

## Eval suite

`TestEvalAllPolicies` runs S1–S5 under **fifo / random / lru-idle / resource-evict**.
Metrics are **deltas** on controlplane `/metrics`. Writes:

- Aggregate markdown table (one row per policy)
- Per-scenario table
- File: `${EVAL_OUT_DIR:-docs/eval/results}/policy_compare.md`

CI uploads that file as artifact `policy-compare`.

| ID | Scenario |
|----|----------|
| **S1** `S1_cold_start` | First resume (golden) |
| **S2** `S2_hot_wake` | Pause → sticky resume |
| **S3** `S3_migrate_sleep` | Suspend → other Worker |
| **S4** `S4_pool_contention` | Oversubscribe latest resumes |
| **S5** `S5_stateful_agent` | FS + memory, sticky then migrate |

Env: `MIN_WORKERS` (default 4), `EVAL_OUT_DIR`, `SIGNAL_PUSH_WAIT_SEC`, `EVAL_*` counts.

Does not fail on which policy wins — only that each policy produces resume metrics.
