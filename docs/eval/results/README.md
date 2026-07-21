# Eval results

CI:

1. `e2e-eval` matrix → per-policy `policy_report_<policy>.json` (artifact `policy-compare-<policy>`)
2. `e2e-eval-summary` → merged `policy_compare.md` (artifact `policy-compare-all`)

Local one policy:

```bash
POLICY=fifo ./hack/kind-up.sh
EVAL_POLICY=fifo E2E_SUITE=eval EVAL_OUT_DIR=docs/eval/results ./hack/verify-local.sh
```

Merge several JSON reports:

```bash
go run ./hack/merge-eval-results docs/eval/results docs/eval/results/policy_compare.md
```
