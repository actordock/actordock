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

PROJECT_ID ?= local
export KO_DOCKER_REPO := ko.local/actordock

GO := go
KO := ko
BINDIR := bin

BINARIES := platform router envd scheduler template-builder

DASHBOARD_WEB := dashboard/web
DASHBOARD_BIN := $(BINDIR)/dashboard

.PHONY: all build build-images build-dashboard build-dashboard-image test fmt verify-fmt vet verify-dashboard verify-helm

all: build

build: $(addprefix $(BINDIR)/,$(BINARIES))

$(BINDIR)/%:
	@mkdir -p $(BINDIR)
	$(GO) build -o $@ ./cmd/$*

build-images:
	$(KO) build \
		./cmd/platform \
		./cmd/router \
		./cmd/envd \
		./cmd/scheduler \
		./cmd/template-builder

build-dashboard: $(DASHBOARD_BIN)

$(DASHBOARD_BIN):
	@mkdir -p $(BINDIR)
	cd $(DASHBOARD_WEB) && npm ci && npm run build
	$(GO) build -o $@ ./dashboard/cmd/dashboard

build-dashboard-image:
	$(KO) build ./dashboard/cmd/dashboard

verify-dashboard:
	$(GO) test ./dashboard/...
	cd $(DASHBOARD_WEB) && npm ci && npm run lint && npm run build

verify-helm:
	@command -v helm >/dev/null 2>&1 || { echo "helm not found; install https://helm.sh"; exit 1; }
	helm lint ./charts/actordock-stack
	helm template actordock ./charts/actordock-stack -n actordock >/dev/null

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	@gofmt -w $$(git ls-files '*.go' 2>/dev/null | grep -v '^runtime/LICENSES/' || true)

verify-fmt:
	@files=$$(gofmt -l $$(git ls-files '*.go' 2>/dev/null | grep -v '^runtime/LICENSES/' || true)); \
	if [ -n "$$files" ]; then \
		echo "gofmt needed (run: make fmt):"; \
		echo "$$files"; \
		exit 1; \
	fi

verify: verify-fmt vet test verify-helm
