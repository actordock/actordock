# actordock

**Hundreds of agent sandboxes. A handful of Pods.** Actordock multiplexes agent sandboxes behind an E2B-compatible API—gVisor isolation, sub-second suspend/resume, RAM and filesystem snapshots on idle, 30×+ session oversubscription on warm Workers. Point the E2B SDK at your cluster; no code changes.

Self-hosted. Kubernetes-native. One command to deploy.

```bash
./hack/install-local.sh
./hack/verify-local.sh
```

See [Quickstart](docs/user/quickstart.md) for prerequisites, env vars, and troubleshooting.

## Architecture

E2B-compatible agent sandboxes on Kubernetes: SDK REST/HTTP through Actordock (Platform, Router, Scheduler, Redis), execution on the vendored `runtime/` tree (runtime-api, runtime-worker, envd, snapshots).

![Actordock architecture](docs/assets/architecture.png)

| Component | Layer | Role |
|-----------|-------|------|
| **E2B SDK** | Client | Official Python/JS SDK; REST for lifecycle, HTTP to `{sandboxId}.{domain}:49983` for commands |
| **Platform** | Actordock | E2B-compatible REST (`POST /sandboxes`, pause, metrics, template builds) |
| **Router** | Actordock | Ingress: parse sandbox id, `ResumeActor` via runtime-api, proxy to envd |
| **Scheduler** | Actordock | TTL and auto-pause; calls runtime-api to suspend/delete idle sandboxes |
| **Redis** | Actordock | Sandbox metadata (ids, tokens, expiry, template refs) |
| **Dashboard** | Actordock | Web UI for templates, sandboxes, and cluster smoke checks |
| **ActorTemplate CRD** | Runtime | `actordock.dev` templates; golden snapshot + workload spec |
| **runtime-api** | Runtime | gRPC control plane: Create/Resume/Suspend/Delete actor |
| **runtime-worker** | Runtime | Host Pod; multiplexes many actors onto fewer workers |
| **runtime-sandbox** | Runtime | gVisor (`runsc`) isolation per actor workload |
| **envd** | Runtime | In-sandbox process and filesystem API (`process.Start`, files) |
| **rustfs** | Runtime | Snapshot storage (suspend/resume state) |

**Flows:** Create (solid) — SDK → Platform → runtime-api → worker + envd → Redis. Command/Resume (dashed) — SDK → Router → runtime-api → envd. Lifecycle (dotted) — Scheduler → runtime-api suspend → snapshot.

Details: [Architecture](docs/architecture.md) · [Roadmap](docs/roadmap.md)
