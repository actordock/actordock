#!/usr/bin/env bash
# Copyright 2026 The Actordock Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit -o nounset -o pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=hack/lib/common.sh
source "${ROOT}/hack/lib/common.sh"

export KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-actordock}"

ENV_FILE="${ROOT}/hack/.env.local"
[[ -f "${ENV_FILE}" ]] || die "missing ${ENV_FILE}; run ./hack/install-local.sh first"

require_cmd python3 curl kubectl

PF_PLATFORM=""
PF_ROUTER=""
PF_DASHBOARD=""

cleanup() {
  [[ -n "${PF_PLATFORM}" ]] && kill "${PF_PLATFORM}" 2>/dev/null || true
  [[ -n "${PF_ROUTER}" ]] && kill "${PF_ROUTER}" 2>/dev/null || true
  [[ -n "${PF_DASHBOARD}" ]] && kill "${PF_DASHBOARD}" 2>/dev/null || true
}
trap cleanup EXIT

wait_http() {
  local url="$1"
  local i
  for i in $(seq 1 60); do
    if curl -sf "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  die "timed out waiting for ${url}"
}

log_step "Port-forwarding platform (:8080), router (:8081), and dashboard (:3000)"
kubectl_ctx port-forward -n actordock svc/platform 8080:8080 >/tmp/actordock-pf-platform.log 2>&1 &
PF_PLATFORM=$!
kubectl_ctx port-forward -n actordock svc/router 8081:8081 >/tmp/actordock-pf-router.log 2>&1 &
PF_ROUTER=$!
kubectl_ctx port-forward -n actordock svc/dashboard 3000:3000 >/tmp/actordock-pf-dashboard.log 2>&1 &
PF_DASHBOARD=$!

wait_http "http://localhost:8080/health"
wait_http "http://localhost:8081/health"
wait_http "http://localhost:3000/health"

set -a
# shellcheck disable=SC1090
source "${ENV_FILE}"
set +a

VENV="${ROOT}/e2e/.venv"
if [[ ! -d "${VENV}" ]]; then
  log_step "Creating Python venv at e2e/.venv"
  python3 -m venv "${VENV}"
fi

shopt -s nullglob
example_dirs=("${ROOT}"/examples/*/)
if [[ ${#example_dirs[@]} -eq 0 ]]; then
  log_step "No examples/ subprojects; skipping"
  exit 0
fi

ran=0
for example_dir in "${example_dirs[@]}"; do
  if [[ ! -d "${example_dir}tests" ]]; then
    continue
  fi
  if [[ -f "${example_dir}requirements.txt" ]]; then
    log_step "Installing dependencies for ${example_dir}"
    "${VENV}/bin/pip" install -q -r "${example_dir}requirements.txt"
  fi
  log_step "Running example E2E: ${example_dir}"
  (cd "${example_dir}" && "${VENV}/bin/pytest" tests/ -v)
  ran=1
done

if [[ "${ran}" -eq 0 ]]; then
  log_step "No examples/*/tests found; skipping"
fi
