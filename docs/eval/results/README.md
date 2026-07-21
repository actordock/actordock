# Eval results

CI `e2e-eval` matrix writes per-policy tables here (`policy_compare.md` /
`policy_compare_<policy>.md`) and uploads artifact `policy-compare-<policy>`.

Local one policy:

```bash
POLICY=fifo ./hack/kind-up.sh
EVAL_POLICY=fifo E2E_SUITE=eval EVAL_OUT_DIR=docs/eval/results ./hack/verify-local.sh
```

Local all four (sequential SetPolicy): `E2E_SUITE=eval ./hack/verify-local.sh`
