# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0

.PHONY: test build kind-up kind-down verify e2e tidy

test:
	go test ./...

tidy:
	go mod tidy

build:
	go build -o bin/controlplane ./cmd/controlplane
	go build -o bin/worker ./cmd/worker

kind-up:
	./hack/kind-up.sh

kind-down:
	./hack/kind-down.sh

verify:
	./hack/verify-local.sh

e2e: kind-up verify
