#!/usr/bin/env bash
# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Run e2e suites against a live Kind stack.
# Requires: ./hack/kind-up.sh (or equivalent) already succeeded.
#
# E2E_SUITE=functional (default) | agent-semantic | all
#   functional      — Go correctness tests
#   agent-semantic  — dataset replay (2 workers, N agents) across policies
#   all             — functional + agent-semantic
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

export ACTORDOCK_API="${ACTORDOCK_API:-http://127.0.0.1:18080}"
export ACTORDOCK_NAMESPACE="${ACTORDOCK_NAMESPACE:-actordock}"
export EVAL_OUT_DIR="${EVAL_OUT_DIR:-${ROOT}/docs/eval/results}"

SUITE="${E2E_SUITE:-functional}"
TIMEOUT="${E2E_TIMEOUT:-20m}"

# agent-semantic defaults (override via env)
AGENT_SEMANTIC_LIMIT="${AGENT_SEMANTIC_LIMIT:-0}"
AGENT_SEMANTIC_INFLIGHT="${AGENT_SEMANTIC_INFLIGHT:-8}"
AGENT_SEMANTIC_SPEED="${AGENT_SEMANTIC_SPEED:-60}"
AGENT_SEMANTIC_MIN_WORKERS="${AGENT_SEMANTIC_MIN_WORKERS:-2}"
AGENT_SEMANTIC_MIN_LOCK="${AGENT_SEMANTIC_MIN_LOCK:-0.25}"
AGENT_SEMANTIC_POLICIES="${AGENT_SEMANTIC_POLICIES:-random,resource-evict,semantic-score-l1,semantic-score}"
AGENT_SEMANTIC_DATASET="${AGENT_SEMANTIC_DATASET:-${ROOT}/docs/eval/datasets/agent-semantic@v2}"

run_pkg() {
  local pkg="$1"
  local timeout="$2"
  echo "==> go test ${pkg} -tags=e2e"
  go test "${pkg}" -tags=e2e -count=1 -timeout="${timeout}" -v
}

ensure_api_pf() {
  if curl -fsS --max-time 2 "${ACTORDOCK_API}/healthz" >/dev/null 2>&1; then
    return 0
  fi
  echo "==> starting port-forward ${ACTORDOCK_API} -> svc/controlplane"
  # shellcheck disable=SC2009
  pkill -f "port-forward.*18080:8080" 2>/dev/null || true
  KCTX=()
  if [[ -n "${KIND_CLUSTER_NAME:-}" ]]; then
    KCTX=(--context "kind-${KIND_CLUSTER_NAME}")
  fi
  kubectl "${KCTX[@]}" -n "${ACTORDOCK_NAMESPACE}" port-forward svc/controlplane 18080:8080 \
    >/tmp/actordock-pf-verify.log 2>&1 &
  sleep 2
  curl -fsS --max-time 5 "${ACTORDOCK_API}/healthz" >/dev/null
}

run_agent_semantic() {
  ensure_api_pf
  mkdir -p "${EVAL_OUT_DIR}"
  chmod +x "${ROOT}/hack/replay-agent-semantic.py"
  SWITCH_ARGS=()
  # Default: switch when multiple policies; CI matrix sets AGENT_SEMANTIC_SWITCH_POLICY=0.
  switch="${AGENT_SEMANTIC_SWITCH_POLICY:-}"
  if [[ -z "${switch}" ]]; then
    IFS=',' read -r -a _pols <<< "${AGENT_SEMANTIC_POLICIES}"
    if ((${#_pols[@]} > 1)); then
      switch=1
    else
      switch=0
    fi
  fi
  if [[ "${switch}" == "1" || "${switch}" == "true" ]]; then
    SWITCH_ARGS=(--switch-policy)
  fi
  echo "==> agent-semantic replay limit=${AGENT_SEMANTIC_LIMIT} inflight=${AGENT_SEMANTIC_INFLIGHT} policies=${AGENT_SEMANTIC_POLICIES} switch=${switch}"
  python3 "${ROOT}/hack/replay-agent-semantic.py" \
    --api "${ACTORDOCK_API}" \
    --dataset "${AGENT_SEMANTIC_DATASET}" \
    --policies "${AGENT_SEMANTIC_POLICIES}" \
    "${SWITCH_ARGS[@]}" \
    --namespace "${ACTORDOCK_NAMESPACE}" \
    --min-workers "${AGENT_SEMANTIC_MIN_WORKERS}" \
    --max-inflight "${AGENT_SEMANTIC_INFLIGHT}" \
    --limit "${AGENT_SEMANTIC_LIMIT}" \
    --speed "${AGENT_SEMANTIC_SPEED}" \
    --min-lock-sec "${AGENT_SEMANTIC_MIN_LOCK}" \
    --out "${EVAL_OUT_DIR}"

  echo "==> gate: all sessions ok"
  python3 - <<'PY'
import json, sys
from pathlib import Path
out = Path(__import__("os").environ.get("EVAL_OUT_DIR", "docs/eval/results"))
files = sorted(out.glob("agent_semantic_v2__*.json"))
if not files:
    sys.exit(f"no agent_semantic_v2__*.json under {out}")
failed = []
for p in files:
    r = json.loads(p.read_text())
    if int(r.get("sessions_failed") or 0) != 0 or int(r.get("sessions_ok") or 0) <= 0:
        failed.append(f"{p.name}: ok={r.get('sessions_ok')} fail={r.get('sessions_failed')}")
compare = out / "policy_compare_agent_semantic_v2.md"
if not compare.exists():
    sys.exit(f"missing {compare}")
print(compare.read_text())
if failed:
    print("FAILED:", *failed, sep="\n  ")
    sys.exit(1)
print(f"ok: {len(files)} policy report(s)")
PY
}

case "${SUITE}" in
  functional)
    run_pkg ./e2e/functional/ "${TIMEOUT}"
    ;;
  agent-semantic)
    run_agent_semantic
    ;;
  all)
    run_pkg ./e2e/functional/ "${TIMEOUT}"
    run_agent_semantic
    ;;
  *)
    echo "unknown E2E_SUITE=${SUITE} (want functional|agent-semantic|all)" >&2
    exit 1
    ;;
esac
