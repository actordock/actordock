#!/usr/bin/env bash
# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

CLUSTER_NAME="${KIND_CLUSTER_NAME:-actordock}"
kind delete cluster --name "${CLUSTER_NAME}" || true
