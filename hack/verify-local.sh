#!/usr/bin/env bash
# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Run Go e2e tests against a live Kind stack.
# Requires: ./hack/kind-up.sh (or equivalent) already succeeded.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

export ACTORDOCK_API="${ACTORDOCK_API:-http://127.0.0.1:18080}"
export ACTORDOCK_NAMESPACE="${ACTORDOCK_NAMESPACE:-actordock}"

echo "==> go test ./e2e/ -tags=e2e"
go test ./e2e/ -tags=e2e -count=1 -timeout="${E2E_TIMEOUT:-20m}" -v
