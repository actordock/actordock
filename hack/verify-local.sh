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

cleanup() {
  [[ -n "${PF_PLATFORM}" ]] && kill "${PF_PLATFORM}" 2>/dev/null || true
  [[ -n "${PF_ROUTER}" ]] && kill "${PF_ROUTER}" 2>/dev/null || true
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

log_step "Port-forwarding platform (:8080) and router (:8081)"
kubectl_ctx port-forward -n actordock svc/platform 8080:8080 >/tmp/actordock-pf-platform.log 2>&1 &
PF_PLATFORM=$!
kubectl_ctx port-forward -n actordock svc/router 8081:8081 >/tmp/actordock-pf-router.log 2>&1 &
PF_ROUTER=$!

wait_http "http://localhost:8080/health"
wait_http "http://localhost:8081/health"

set -a
# shellcheck disable=SC1090
source "${ENV_FILE}"
set +a

VENV="${ROOT}/e2e/.venv"
if [[ ! -d "${VENV}" ]]; then
  log_step "Creating Python venv at e2e/.venv"
  python3 -m venv "${VENV}"
fi

log_step "Installing e2e dependencies"
"${VENV}/bin/pip" install -q -r "${ROOT}/e2e/requirements.txt"

log_step "Running E2E tests"
cd "${ROOT}/e2e"
"${VENV}/bin/pytest" tests/ -v
