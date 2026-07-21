# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0

FROM golang:1.24-bookworm AS build
WORKDIR /src
# Allow downloading a newer toolchain if go.mod asks for it (e.g. 1.25).
ENV GOTOOLCHAIN=auto
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o /out/controlplane ./cmd/controlplane
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker

FROM debian:bookworm-slim AS rootfs
RUN apt-get update && apt-get install -y --no-install-recommends busybox-static ca-certificates curl \
  && rm -rf /var/lib/apt/lists/*
RUN mkdir -p /rootfs/bin /rootfs/proc /rootfs/sys /rootfs/dev /rootfs/tmp /rootfs/etc \
  && cp /bin/busybox /rootfs/bin/busybox \
  && /bin/busybox --install -s /rootfs/bin \
  && find /rootfs/bin -maxdepth 1 -type l -exec sh -c 'for l; do ln -sfn busybox "$l"; done' _ {} + \
  && ln -sfn busybox /rootfs/bin/sleep

FROM debian:bookworm-slim AS worker
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
  && rm -rf /var/lib/apt/lists/*
ARG RUNSC_VERSION=latest
RUN set -eux; \
  arch="$(uname -m)"; \
  case "$arch" in \
    x86_64|amd64) garch=x86_64 ;; \
    aarch64|arm64) garch=aarch64 ;; \
    *) echo "unsupported arch $arch"; exit 1 ;; \
  esac; \
  curl -fsSL -o /usr/local/bin/runsc \
    "https://storage.googleapis.com/gvisor/releases/release/${RUNSC_VERSION}/${garch}/runsc"; \
  chmod +x /usr/local/bin/runsc
COPY --from=build /out/worker /usr/local/bin/worker
COPY --from=rootfs /rootfs /opt/actordock/rootfs
RUN mkdir -p /var/lib/actordock/runsc /var/lib/actordock/bundles /var/lib/actordock/snapshots
ENV RUNSC_PATH=/usr/local/bin/runsc \
    ROOTFS=/opt/actordock/rootfs \
    RUNSC_ROOT=/var/lib/actordock/runsc \
    BUNDLE_DIR=/var/lib/actordock/bundles \
    PLATFORM=systrap
ENTRYPOINT ["/usr/local/bin/worker"]

FROM debian:bookworm-slim AS controlplane
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/controlplane /usr/local/bin/controlplane
ENTRYPOINT ["/usr/local/bin/controlplane"]
