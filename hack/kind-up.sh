#!/usr/bin/env bash
# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CLUSTER_NAME="${KIND_CLUSTER_NAME:-actordock}"
POLICY="${POLICY:-semantic-score}"

need() { command -v "$1" >/dev/null || { echo "missing $1"; exit 1; }; }
need docker
need kind
need kubectl

echo "==> kind cluster ${CLUSTER_NAME}"
if ! kind get clusters | grep -qx "${CLUSTER_NAME}"; then
  kind create cluster --name "${CLUSTER_NAME}"
fi
kubectl config use-context "kind-${CLUSTER_NAME}"
kubectl cluster-info --context "kind-${CLUSTER_NAME}" >/dev/null

# Substrate-style: help pod networking with gVisor/privileged Workers on Kind.
echo "==> enable proxy_arp on kind nodes"
for node in $(kind get nodes --name "${CLUSTER_NAME}"); do
  docker exec "${node}" sysctl -w net.ipv4.conf.all.proxy_arp=1 >/dev/null
done

echo "==> build images"
docker build --target controlplane -t actordock/controlplane:dev "${ROOT}"
docker build --target worker -t actordock/worker:dev "${ROOT}"

echo "==> load into kind"
kind load docker-image actordock/controlplane:dev --name "${CLUSTER_NAME}"
kind load docker-image actordock/worker:dev --name "${CLUSTER_NAME}"

echo "==> deploy rustfs + actordock (POLICY=${POLICY}${SEMANTIC_PRIOR_MIX:+ PRIOR_MIX=${SEMANTIC_PRIOR_MIX}})"
kubectl apply -f "${ROOT}/manifests/kind/rustfs.yaml"
kubectl -n actordock rollout status deploy/rustfs --timeout=180s
# Recreate bucket-init job so re-runs are idempotent.
kubectl -n actordock delete job rustfs-bucket-init --ignore-not-found
kubectl apply -f "${ROOT}/manifests/kind/rustfs.yaml"
kubectl -n actordock wait --for=condition=complete job/rustfs-bucket-init --timeout=180s

kubectl apply -f "${ROOT}/manifests/kind/actordock.yaml"
CP_ENV=(POLICY="${POLICY}")
if [[ -n "${SEMANTIC_PRIOR_MIX:-}" ]]; then
  CP_ENV+=("SEMANTIC_PRIOR_MIX=${SEMANTIC_PRIOR_MIX}")
fi
kubectl -n actordock set env deployment/controlplane "${CP_ENV[@]}"
# Ensure pods pick up freshly loaded images.
kubectl -n actordock rollout restart deployment/controlplane deployment/redis
kubectl -n actordock rollout restart statefulset/worker

echo "==> wait for rollout"
kubectl -n actordock rollout status deploy/redis --timeout=120s
kubectl -n actordock rollout status deploy/controlplane --timeout=180s
kubectl -n actordock rollout status statefulset/worker --timeout=240s

echo "==> ready"
kubectl -n actordock get pods -o wide

echo "==> wait for workers + golden"
deadline=$((SECONDS+180))
while (( SECONDS < deadline )); do
  ready=$(kubectl -n actordock get pods -l app=worker --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l | tr -d ' ')
  if [[ "${ready}" -ge 1 ]]; then
    break
  fi
  sleep 3
done
# Port-forward briefly to ensure golden (also built by controlplane background).
kubectl -n actordock port-forward svc/controlplane 18080:8080 >/tmp/actordock-pf.log 2>&1 &
pf=$!
trap 'kill ${pf} 2>/dev/null || true' EXIT
sleep 2
for i in $(seq 1 40); do
  if curl -fsS -X POST http://127.0.0.1:18080/v1/golden/ensure >/dev/null 2>&1; then
    echo "golden ready"
    break
  fi
  sleep 3
done
kill ${pf} 2>/dev/null || true
trap - EXIT
