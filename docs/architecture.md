# Architecture

## Value

Actordock runs agent sandboxes on Kubernetes with worker multiplexing and idle suspend—higher density and lower cost than one-pod-per-sandbox.

Ships as a single install. Exposes an E2B-compatible API so existing SDKs work without code changes.

## Stack

```
E2B SDK
  ├─ REST → Platform ──gRPC──→ runtime-api ──→ runtime-worker ──→ envd (Worker)
  └─ HTTP → Router ──gRPC──→ runtime-api (ResumeActor)
                │                    │
                └──── proxy ─────────┘→ envd :49983
                ↑
          Scheduler ↔ Redis
                ↑
         ActorTemplate CRD (actordock.dev)
```

| Layer | Component | Role |
|-------|-----------|------|
| Actordock | Platform | E2B REST → runtime-api |
| | Router | E2B ingress: parse `{id}.domain`, ResumeActor, proxy to Worker:49983 |
| | Scheduler | TTL, autoPause → runtime-api Suspend/Delete |
| | envd | Process + filesystem in sandbox |
| | Redis | Sandbox metadata |
| Runtime (`runtime/`) | runtime-api | Actor lifecycle, resume |
| | runtime-worker / runtime-sandbox | gVisor workers |
| | runtime-net | Internal actor mesh (not on E2B path) |
| | rustfs | Snapshots |

Actordock owns the sandbox API, E2B routing, lifecycle, and envd. The vendored `runtime/` tree provides workers and actor execution (derived from Actordock Runtime).

## Routing

E2B clients use one hop: **Actordock Router**.

1. SDK sends HTTP to `{sandboxId}.{domain}:49983`
2. Router parses sandbox/actor id
3. Router calls runtime-api `ResumeActor` if suspended
4. Router proxies to worker IP:49983 (envd)

**runtime-net** provides the native DNS mesh (`{id}.actors.actordock.dev`). E2B traffic does not go through runtime-net.

## Flows

**Create:** `POST /sandboxes` → Platform `CreateActor` → Worker + envd → Redis → sandbox id

**Run command:** SDK → Router → `ResumeActor` → proxy → envd `process.Start`

## Deploy

```bash
./hack/install-local.sh
helm install actordock ./charts/actordock-stack
```

Runtime already installed: `./hack/install-local.sh --skip-runtime`
