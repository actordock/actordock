# Quickstart (v0.0.5)

Run the E2B SDK against a local Actordock cluster on Kind.

## Prerequisites

- Docker (running)
- [Kind](https://kind.sigs.k8s.io/)
- `kubectl`, `go`, `git`
- Python 3.10+ (for E2E verify)

## Install

```bash
./hack/install-local.sh
```

This creates Kind cluster `actordock`, deploys pinned [Substrate](https://github.com/agent-substrate/substrate), and deploys Actordock (platform, router, scheduler, envd template `base`).

Re-deploy Actordock only (cluster already exists):

```bash
./hack/install-local.sh --skip-substrate
```

## Verify

`hack/install-local.sh` writes `hack/.env.local` with:

| Variable | Default (local) |
|----------|-----------------|
| `E2B_API_URL` | `http://localhost:8080` |
| `E2B_SANDBOX_URL` | `http://localhost:8081` |
| `E2B_DOMAIN` | `localhost` |
| `E2B_API_KEY` | `dev` |
| `E2B_VALIDATE_API_KEY` | `false` |

Run the E2E demo (port-forward + E2B Python SDK):

```bash
./hack/verify-local.sh
```

Covers commands, visibility, timeout metadata, scheduler auto-cleanup, and idle suspend (pause lifecycle + router auto-resume).

### Timeout

Sandboxes accept an optional lifetime in seconds (E2B `timeout`, default 300). The **scheduler** kills expired sandboxes when `on_timeout=kill` (default).

```python
from e2b import Sandbox

sbx = Sandbox.create(template="base", secure=False, timeout=600)
sbx.set_timeout(900)  # extend from now
sbx.kill()  # optional manual delete before expiry
```

### Pause lifecycle

Set `lifecycle={"on_timeout": "pause", "auto_resume": True}` on create. After timeout the scheduler **suspends** the actor (sandbox metadata stays in Redis). The next `commands.run` goes through the **router**, which resumes the actor automatically — no explicit `resume()` call required.

```python
from e2b import Sandbox

sbx = Sandbox.create(
    template="base",
    secure=False,
    timeout=60,
    lifecycle={"on_timeout": "pause", "auto_resume": True},
)
# ... after idle past timeout ...
print(sbx.commands.run("echo back").stdout)
sbx.kill()
```

Explicit pause/resume REST routes: `POST /sandboxes/{id}/pause` (204) and `POST /sandboxes/{id}/resume` (201). The SDK exposes `sandbox.pause()`; resume can be called via HTTP or by sending traffic when `auto_resume=True`.

Or manually:

```bash
source hack/.env.local
kubectl --context kind-actordock port-forward -n actordock svc/platform 8080:8080 &
kubectl --context kind-actordock port-forward -n actordock svc/router 8081:8081 &
cd e2e && python3 -m venv .venv && .venv/bin/pip install -r requirements.txt
.venv/bin/pytest tests/ -v
```

## Troubleshooting

- **Substrate pods not ready** — wait or re-run `./hack/install-local.sh`; cold start can exceed default rollout timeouts.
- **ActorTemplate `base` not Ready** — check `kubectl --context kind-actordock get actortemplate -n actordock` and ate-system pods.
- **E2E connection refused** — ensure port-forwards to platform (`8080`) and router (`8081`) are running.
- **No free workers** — delete stale sandboxes via platform `DELETE /sandboxes/{id}` or `kubectl ate delete actor <id>`.

## Further reading

- [Architecture](../architecture.md)
- [Roadmap](../roadmap.md)
- [v0.0.5 release notes](../releases/v0.0.5.md)
