# actordock-stack

Helm chart for Actordock control plane on an existing [Agent Substrate](https://github.com/agent-substrate/substrate) cluster.

**Kind / local dev:** continue using `./hack/install-local.sh` (ko + kustomize). This chart targets non-Kind pilot clusters (GKE, EKS, k3s, etc.).

## Prerequisites

1. Kubernetes >= 1.28
2. **Agent Substrate `ate-system`** installed and healthy (`ate-api-server`, `ate-controller`, `atelet`, …). Pin matches `substrate.lock` / `values.yaml` `substrate.lock.commit` (currently `9f847549`).
3. Container images built and pushed for:
   - `ghcr.io/actordock/platform`
   - `ghcr.io/actordock/router`
   - `ghcr.io/actordock/scheduler`
   - `ghcr.io/actordock/envd`
   - `ghcr.io/actordock/dashboard` (if dashboard enabled)
   - `substrate.workerPool.ateomImage` (ateom-gvisor worker image)

## Install order

```bash
# 1. Install Substrate control plane (see substrate repo hack/install-ate.sh or your cloud runbook)
# 2. Build/push Actordock images (make build-images / CI release)
# 3. Install chart
helm install actordock ./charts/actordock-stack \
  -n actordock --create-namespace \
  --set secrets.apiKey='YOUR_KEY' \
  --set images.platform.tag='0.1.0' \
  --set images.router.tag='0.1.0' \
  --set images.scheduler.tag='0.1.0' \
  --set images.envd.tag='0.1.0' \
  --set images.dashboard.tag='0.1.0' \
  --set substrate.workerPool.ateomImage='YOUR_ATEOM_IMAGE'
```

Wait for `ActorTemplate/base` Ready before creating sandboxes:

```bash
kubectl wait --for=condition=Ready actortemplate/base -n actordock --timeout=600s
```

## Configuration

| Value | Purpose |
|-------|---------|
| `secrets.apiKey` | `ACTORDOCK_API_KEY` for platform + dashboard |
| `actordock.domain` | Sandbox DNS domain (`{id}.domain`) |
| `actordock.ateapiAddr` | Substrate API (`api.ate-system.svc:443` default) |
| `substrate.actorTemplate.snapshotsBucket` | GCS bucket for golden snapshots |
| `dashboard.enabled` | Deploy dashboard UI (default `true`) |
| `platform.service.type` / `router.service.type` | Expose via `LoadBalancer` if needed |

Override image tags per component under `images.*.tag` (defaults to chart `appVersion`).

## Verify locally

```bash
make verify-helm
helm template actordock ./charts/actordock-stack -n actordock
```

## Uninstall

```bash
helm uninstall actordock -n actordock
```

Substrate `ate-system` is not removed by this chart.
