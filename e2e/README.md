# E2E tests (Go)

Live-cluster checks against Kind (gVisor Workers + rustfs). Built with `-tags=e2e`
so they are **not** part of plain `go test ./...`.

## Layout

| Path | Role |
|------|------|
| `harness_test.go` | API client, port-forward, waits |
| `*_test.go` | Scenarios |

## How to run

```bash
./hack/kind-up.sh
./hack/verify-local.sh          # → go test ./e2e/ -tags=e2e
```

Or directly:

```bash
go test ./e2e/ -tags=e2e -count=1 -timeout=20m -v
```

## Current tests

| Test | Covers |
|------|--------|
| `TestMultiplexOversubscribe` | N≫M Workers; Suspend writes `objectKey` |
| `TestPauseStickyResume` | Pause stays local; resume same Worker |
| `TestSuspendCrossWorkerResume` | Suspend + resume (may land on another Worker via rustfs) |
