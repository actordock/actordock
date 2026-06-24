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
SKIP_RUNTIME=false

usage() {
  cat <<EOF
Usage: $0 [options]

One-command local dev stack: Kind cluster + vendored runtime + Actordock + dashboard.

Options:
  --skip-runtime     Skip Kind/runtime install; deploy Actordock only
  -h, --help         Show this help

Environment:
  RUNTIME_ROOT       Override vendored runtime path (default: ./runtime)
  BUCKET_NAME        Snapshot bucket for ActorTemplate (default: actordock-snapshots)

Images push to Kind's local registry (localhost:5001); external KO_DOCKER_REPO is ignored.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-runtime|--skip-runtime)
      SKIP_RUNTIME=true
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1 (try --help)"
      ;;
  esac
  shift
done

require_cmd docker kubectl go git
docker info >/dev/null 2>&1 || die "docker is not running"

# Kind bundled registry (hack/create-kind-cluster.sh). Do not inherit setup-ko / ghcr.io.
export KO_DOCKER_REPO=localhost:5001

RUNTIME_ROOT="$(ensure_runtime_root "${ROOT}")"

if [[ "${SKIP_RUNTIME}" == "false" ]]; then
  log_step "Creating Kind cluster '${KIND_CLUSTER_NAME}' and installing runtime"
  export NO_DEV_ENV=true
  export KO_DEFAULTPLATFORMS="linux/$(go env GOARCH)"
  export RUNTIME_INSTALL_KIND=true
  export BUCKET_NAME="${BUCKET_NAME:-actordock-snapshots}"
  unset GCE_REGION CLUSTER_LOCATION NETWORK SUBNETWORK MEMORYSTORE_INSTANCE PROJECT_ID

  (cd "${RUNTIME_ROOT}" && hack/create-kind-cluster.sh)
  if ! (cd "${RUNTIME_ROOT}" && hack/install-runtime-kind.sh --deploy-runtime-system); then
    log_step "Runtime install returned an error (often rollout timeout on cold start); continuing with longer waits"
  fi
  wait_runtime_control_plane
else
  log_step "Skipping runtime install (--skip-runtime)"
  kubectl config use-context "kind-${KIND_CLUSTER_NAME}" >/dev/null 2>&1 \
    || die "context kind-${KIND_CLUSTER_NAME} not found; run without --skip-runtime first"
  wait_runtime_control_plane
fi

deploy_actordock_images "${ROOT}" "${RUNTIME_ROOT}"
write_env_local "${ROOT}"

log_step "Done"
echo "Actordock namespace:"
kubectl_ctx get ns actordock
echo ""
echo "Runtime control plane:"
kubectl_ctx get pods -n actordock-system
echo ""
echo "Actordock workloads:"
kubectl_ctx get pods -n actordock
echo ""
echo "Next: ./hack/verify-local.sh (or source hack/.env.local and port-forward platform :8080, router :8081, dashboard :3000)."
