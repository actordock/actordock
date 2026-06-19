# Architecture

## Value

Actordock runs agent sandboxes on Kubernetes with worker multiplexing and idle suspend—higher density and lower cost than one-pod-per-sandbox.

Ships as a single install. Exposes an E2B-compatible API so existing SDKs work without code changes.

## Stack

```
E2B SDK
  ├─ REST → Platform ──gRPC──→ ateapi ──→ atelet ──→ envd (Worker)
  └─ HTTP → Router ──gRPC──→ ateapi (ResumeActor)
                │                    │
                └──── proxy ─────────┘→ envd :49983
                ↑
          Scheduler ↔ Redis
                ↑
         ActorTemplate CRD
```

| Layer | Component | Role |
|-------|-----------|------|
| Actordock | Platform | E2B REST → ateapi |
| | Router | E2B ingress: parse `{id}.domain`, ResumeActor, proxy to Worker:49983 |
| | Scheduler | TTL, autoPause → ateapi Suspend/Delete |
| | envd | Process + filesystem in sandbox |
| | Redis | Sandbox metadata |
| Substrate (runtime) | ateapi | Actor lifecycle, resume |
| | atelet / ateom | gVisor workers |
| | atenet | Internal actor mesh (not on E2B path) |
| | rustfs | Snapshots |

Actordock owns the sandbox API, E2B routing, lifecycle, and envd. [Agent Substrate](https://github.com/agent-substrate/substrate) provides workers and actor execution. No Substrate fork required.

## Routing

E2B clients use one hop: **Actordock Router**.

1. SDK sends HTTP to `{sandboxId}.{domain}:49983`
2. Router parses sandbox/actor id
3. Router calls ateapi `ResumeActor` if suspended
4. Router proxies to worker IP:49983 (envd)

**atenet** ships with Substrate for its native DNS mesh (`{id}.actors.resources.substrate…`). E2B traffic does not go through atenet. Substrate is not modified.

## Flows

**Create:** `POST /sandboxes` → Platform `CreateActor` → Worker + envd → Redis → sandbox id

**Run command:** SDK → Router → `ResumeActor` → proxy → envd `process.Start`

## Deploy

```bash
./hack/install-local.sh
helm install actordock ./charts/actordock-stack
```

Existing Substrate cluster: `./hack/install-local.sh --skip-substrate`
