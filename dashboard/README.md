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

# Actordock ops dashboard

Optional browser UI for sandboxes, templates, volumes, snapshots, metrics, logs, and terminal access. `./hack/install-local.sh` deploys the dashboard with the core stack; production clusters can use `manifests/dashboard/` with a Secret for the API key.

## Layout

```
dashboard/
  cmd/dashboard/          # Go HTTP server (embeds SPA, BFF proxies)
  internal/server/        # config, Platform/Router reverse proxy, static files
  web/                    # React + Vite SPA
```

## Local development

Prerequisites: Actordock cluster running (`./hack/install-local.sh`) with port-forwards to Platform (`8080`) and Router (`8081`).

### Frontend (hot reload)

```bash
kubectl --context kind-actordock port-forward -n actordock svc/platform 8080:8080 &
kubectl --context kind-actordock port-forward -n actordock svc/router 8081:8081 &

cd dashboard/web
npm ci
npm run dev
# → http://localhost:5173 (Vite proxies /api/platform and /api/router)
```

Optional env for Vite proxy (defaults shown):

| Variable | Default |
|----------|---------|
| `VITE_PLATFORM_URL` | `http://localhost:8080` |
| `VITE_ROUTER_URL` | `http://localhost:8081` |
| `VITE_API_KEY` | `dev` |

### Backend (embedded SPA)

```bash
make build-dashboard
ACTORDOCK_PLATFORM_URL=http://localhost:8080 \
ACTORDOCK_ROUTER_URL=http://localhost:8081 \
ACTORDOCK_API_KEY=dev \
./bin/dashboard
# → http://localhost:3000
```

## Build and verify

```bash
make build-dashboard          # npm build + bin/dashboard
make build-dashboard-image    # ko container image
make verify-dashboard         # go test ./dashboard/... + npm lint + npm build
```

## Deploy to Kubernetes (optional)

Requires Actordock namespace and Platform/Router already running.

```bash
kubectl apply -f manifests/dashboard/secret.example.yaml

kubectl kustomize manifests/dashboard --load-restrictor LoadRestrictionsNone \
  | ko resolve -f - \
  | kubectl --context kind-actordock apply -f -

kubectl --context kind-actordock port-forward -n actordock svc/dashboard 3000:3000
open http://localhost:3000
```

## Configuration (env)

| Variable | Default | Description |
|----------|---------|-------------|
| `ACTORDOCK_DASHBOARD_ADDR` | `:3000` | Listen address |
| `ACTORDOCK_PLATFORM_URL` | `http://platform:8080` | Platform REST base URL |
| `ACTORDOCK_ROUTER_URL` | `http://localhost:8081` | Router base URL (use `http://router:8081` in-cluster) |
| `ACTORDOCK_API_KEY` | `dev` | Injected as `X-API-KEY` on Platform proxy |
| `ACTORDOCK_DASHBOARD_PROXY_PLATFORM` | `true` | Enable `/api/platform/*` BFF |
| `ACTORDOCK_DASHBOARD_PROXY_ROUTER` | `true` | Enable `/api/router/*` BFF (terminal) |

## Architecture

```
Browser → dashboard (SPA + /api/platform/* + /api/router/*)
              ↓                           ↓
         Platform REST              Router → envd
```

No new Platform REST routes. API key stays server-side when using the BFF proxy.
