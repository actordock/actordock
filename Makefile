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

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
VERSION_PKG := github.com/actordock/actordock/internal/version
LDFLAGS := -X=$(VERSION_PKG).Version=$(VERSION)

BINARIES := platform router envd scheduler

.PHONY: all build build-images test fmt verify-fmt vet

all: build

build: $(addprefix $(BINDIR)/,$(BINARIES))

$(BINDIR)/%:
	@mkdir -p $(BINDIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $@ ./cmd/$*

build-images:
	GOFLAGS='"-ldflags=$(LDFLAGS)"' \
	$(KO) build \
		./cmd/platform \
		./cmd/router \
		./cmd/envd \
		./cmd/scheduler

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	@gofmt -w .

verify-fmt:
	@files=$$(gofmt -l .); \
	if [ -n "$$files" ]; then \
		echo "gofmt needed (run: make fmt):"; \
		echo "$$files"; \
		exit 1; \
	fi

verify: verify-fmt vet test
