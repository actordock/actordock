#!/usr/bin/env bash
# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Run Go e2e suites against a live Kind stack.
# Requires: ./hack/kind-up.sh (or equivalent) already succeeded.
#
# E2E_SUITE=functional (default) | eval | all
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

export ACTORDOCK_API="${ACTORDOCK_API:-http://127.0.0.1:18080}"
export ACTORDOCK_NAMESPACE="${ACTORDOCK_NAMESPACE:-actordock}"
export EVAL_OUT_DIR="${EVAL_OUT_DIR:-${ROOT}/docs/eval/results}"

SUITE="${E2E_SUITE:-functional}"
TIMEOUT="${E2E_TIMEOUT:-20m}"

run_pkg() {
  local pkg="$1"
  local timeout="$2"
  echo "==> go test ${pkg} -tags=e2e"
  go test "${pkg}" -tags=e2e -count=1 -timeout="${timeout}" -v
}

case "${SUITE}" in
  functional)
    run_pkg ./e2e/functional/ "${TIMEOUT}"
    ;;
  eval)
    # EVAL_POLICY=fifo|random|lru-idle|resource-evict|semantic-score runs one policy (CI matrix).
    # Unset runs all four sequentially (local). Writes EVAL_OUT_DIR/policy_compare*.md.
    EVAL_RUN="${EVAL_TEST_RUN:-TestEvalAllPolicies}"
    echo "==> go test ./e2e/eval/ -tags=e2e -run ${EVAL_RUN} EVAL_POLICY=${EVAL_POLICY:-ALL}"
    go test ./e2e/eval/ -tags=e2e -count=1 -timeout="${E2E_TIMEOUT:-60m}" -v -run "${EVAL_RUN}"
    ;;
  all)
    run_pkg ./e2e/functional/ "${TIMEOUT}"
    EVAL_RUN="${EVAL_TEST_RUN:-TestEvalAllPolicies}"
    echo "==> go test ./e2e/eval/ -tags=e2e -run ${EVAL_RUN} EVAL_POLICY=${EVAL_POLICY:-ALL}"
    go test ./e2e/eval/ -tags=e2e -count=1 -timeout="${E2E_TIMEOUT:-60m}" -v -run "${EVAL_RUN}"
    ;;
  *)
    echo "unknown E2E_SUITE=${SUITE} (want functional|eval|all)" >&2
    exit 1
    ;;
esac
