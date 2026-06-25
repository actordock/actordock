# Quickstart (v0.1.0)

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

This creates Kind cluster `actordock`, deploys the vendored `runtime/` stack, and deploys Actordock (platform, router, scheduler, dashboard, envd template `base`).

Re-deploy Actordock only (cluster already exists):

```bash
./hack/install-local.sh --skip-runtime
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
| `DASHBOARD_URL` | `http://localhost:3000` |

Run the E2E demo (port-forward + E2B Python SDK):

```bash
./hack/verify-local.sh
```

Covers commands, filesystem, secure sandbox, sandbox field parity, auth routes, template write APIs, **template build (E2B v3 `Template.build`)**, visibility, timeout metadata, scheduler auto-cleanup, idle suspend (pause lifecycle + router auto-resume), observability routes (metrics, logs, refreshes), sandbox extras (connect PTY, network policy, snapshots), and dashboard smoke.

### Filesystem (read / write / list)

```python
from e2b import Sandbox

sbx = Sandbox.create(template="base", secure=False, timeout=120)
try:
    sbx.files.write("/tmp/x", "data")
    assert sbx.files.read("/tmp/x") == "data"
    names = {e.name for e in sbx.files.list("/tmp", depth=1)}
    assert "x" in names
    assert sbx.commands.run("cat /tmp/x").stdout.strip() == "data"
finally:
    sbx.kill()
```

### Secure sandbox

`secure=True` issues per-sandbox `envdAccessToken` tokens; envd validates `X-Access-Token` (no `ACTORDOCK_ENVD_INSECURE` bypass required for this path):

```python
from e2b import Sandbox

sbx = Sandbox.create(template="base", secure=True, timeout=120)
try:
    sbx.files.write("/tmp/secret.txt", "token-bound")
    assert sbx.files.read("/tmp/secret.txt") == "token-bound"
    assert sbx.commands.run("echo ok").stdout.strip() == "ok"
finally:
    sbx.kill()
```

Local Kind dev still sets `E2B_VALIDATE_API_KEY=false` in `hack/.env.local` for Platform API key checks only; secure envd auth is independent.

### Helm (non-Kind pilot)

For GKE/EKS/k3s with an existing `actordock-system` namespace:

```bash
helm install actordock ./charts/actordock-stack -n actordock --create-namespace \
  --set secrets.apiKey='YOUR_KEY' \
  --set images.platform.tag='0.1.0'
```

See [charts/actordock-stack/README.md](../../charts/actordock-stack/README.md) for prerequisites, image tags, and Runtime pin.

Kind development continues to use `./hack/install-local.sh` above.

Port-forward dashboard UI:

```bash
kubectl --context kind-actordock port-forward -n actordock svc/dashboard 3000:3000
open http://localhost:3000
```

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

### Observability (metrics, logs, refreshes)

Platform exposes E2B-compatible observability routes. As of v0.0.7, logs and metrics return **real data** from envd: command stdout/stderr in logs; cgroup-backed CPU/memory/disk in metrics.

```python
import httpx
import os
from e2b import Sandbox

api = os.environ["E2B_API_URL"].rstrip("/")
headers = {"X-API-KEY": os.environ["E2B_API_KEY"]}

sbx = Sandbox.create(template="base", secure=False, timeout=60)
try:
    sbx.commands.run("echo hello")
    # List metrics (map sandbox id -> SandboxMetric; cgroup-backed values)
    metrics = httpx.get(
        f"{api}/sandboxes/metrics",
        params={"sandbox_ids": sbx.sandbox_id},
        headers=headers,
    ).json()["sandboxes"][sbx.sandbox_id]
    assert metrics["memTotal"] > 0
    # Per-sandbox metrics (array of SandboxMetric; at least latest sample)
    httpx.get(f"{api}/sandboxes/{sbx.sandbox_id}/metrics", headers=headers).raise_for_status()
    # Logs v1: {"logs": [...], "logEntries": [...]} — includes command output
    logs = httpx.get(f"{api}/sandboxes/{sbx.sandbox_id}/logs", headers=headers).json()
    # Logs v2: {"logs": [...]} — structured entries with level/fields
    httpx.get(f"{api}/v2/sandboxes/{sbx.sandbox_id}/logs", headers=headers).raise_for_status()
    # Extend TTL without set_timeout (204)
    httpx.post(f"{api}/sandboxes/{sbx.sandbox_id}/refreshes", headers=headers, json={"duration": 120}).raise_for_status()
finally:
    sbx.kill()
```

### Connect (interactive PTY)

Attach to a running PTY through Router → envd `process.Connect` (bidirectional stream):

```python
from e2b import Sandbox
from e2b.sandbox.commands.command_handle import PtySize

sbx = Sandbox.create(template="base", secure=False, timeout=120)
try:
    terminal = sbx.pty.create(PtySize(cols=80, rows=24))
    sbx.pty.send_stdin(terminal.pid, b"echo hello\n")
    terminal.disconnect()
    handle = sbx.pty.connect(terminal.pid)
    sbx.pty.send_stdin(terminal.pid, b"exit\n")
    handle.wait()
finally:
    sbx.kill()
```

Platform `POST /sandboxes/{id}/connect` resumes a paused sandbox and returns a usable router domain.

### Network policy

Persist network config on Platform; Router enforces `allow_internet_access` on egress proxy traffic:

```python
import httpx
import os
from e2b import Sandbox

api = os.environ["E2B_API_URL"].rstrip("/")
headers = {"X-API-KEY": os.environ["E2B_API_KEY"], "Content-Type": "application/json"}

sbx = Sandbox.create(template="base", secure=False, timeout=120)
try:
    httpx.put(
        f"{api}/sandboxes/{sbx.sandbox_id}/network",
        headers=headers,
        json={"allow_internet_access": False},
    ).raise_for_status()
    detail = httpx.get(f"{api}/sandboxes/{sbx.sandbox_id}", headers=headers).json()
    assert detail["allowInternetAccess"] is False
finally:
    sbx.kill()
```

### Snapshots

Create a runtime checkpoint and list metadata from Redis:

```python
from e2b import Sandbox

sbx = Sandbox.create(template="base", secure=False, timeout=120)
try:
    sbx.commands.run("echo snapshot")
    snap = sbx.create_snapshot()
    listed = Sandbox.list_snapshots(sandbox_id=sbx.sandbox_id).next_items()
    assert any(s.snapshot_id == snap.snapshot_id for s in listed)
finally:
    sbx.kill()
```

Or manually:

```bash
source hack/.env.local
kubectl --context kind-actordock port-forward -n actordock svc/platform 8080:8080 &
kubectl --context kind-actordock port-forward -n actordock svc/router 8081:8081 &
cd e2e && python3 -m venv .venv && .venv/bin/pip install -r requirements.txt
.venv/bin/pytest tests/ -v
```

## Templates (v0.0.10+)

Official templates `base` and `python` are pre-provisioned `ActorTemplate` CRs. The `python` template ships with Python 3 preinstalled. User templates can be registered via `POST /templates` (metadata only) or built with the E2B v3 SDK:

```python
import uuid
from e2b import Template, Sandbox

name = f"my-tools-{uuid.uuid4().hex[:8]}"
template = (
    Template()
    .from_template("python")
    .run_cmd("pip install --no-cache-dir httpx")
)
Template.build(template, name, cpu_count=2, memory_mb=512)

sbx = Sandbox.create(template=name, secure=False, timeout=300)
try:
    out = sbx.commands.run('python3 -c "import httpx; print(httpx.__version__)"')
    assert "httpx" in out.stdout
finally:
    sbx.kill()
```

Requires `template-builder` (deployed by `./hack/install-local.sh`) and the Kind local registry.

```python
from e2b import Template

assert Template.exists("base")
assert Template.exists("python")
```

```bash
curl -sS -H "X-API-KEY: dev" http://localhost:8080/templates | jq .
curl -sS -H "X-API-KEY: dev" http://localhost:8080/templates/aliases/base | jq .
```

E2E: `e2e/tests/test_template_build.py`.

## Volumes (v0.0.9)

```python
from e2b import Volume, Sandbox

vol = Volume.create("my-data")
sbx = Sandbox.create(template="base", volume_mounts={"/mnt/data": vol})
# volumeMounts persisted on sandbox; runtime mount requires future runtime support
sbx.kill()
vol.delete()
```

## Dashboard (v0.0.11)

`./hack/install-local.sh` deploys the dashboard Service in the `actordock` namespace. Port-forward to use the UI:

```bash
kubectl --context kind-actordock port-forward -n actordock svc/dashboard 3000:3000
open http://localhost:3000
```

For local frontend development with hot reload (port-forward platform `:8080` and router `:8081`):

```bash
cd dashboard/web && npm ci && npm run dev
# → http://localhost:5173
```

Standalone deploy with a Kubernetes Secret (e.g. non-Kind clusters):

```bash
kubectl apply -f manifests/dashboard/secret.example.yaml
kubectl kustomize manifests/dashboard --load-restrictor LoadRestrictionsNone \
  | ko resolve -f - \
  | kubectl apply -f -
```

See [dashboard/README.md](../../dashboard/README.md) for build targets (`make verify-dashboard`) and configuration.

## Troubleshooting

- **Runtime pods not ready** — wait or re-run `./hack/install-local.sh`; cold start can exceed default rollout timeouts.
- **ActorTemplate `base` not Ready** — check `kubectl --context kind-actordock get actortemplate -n actordock` and actordock-system pods.
- **E2E connection refused** — ensure port-forwards to platform (`8080`) and router (`8081`) are running.
- **No free workers** — delete stale sandboxes via platform `DELETE /sandboxes/{id}` or `kubectl actordock delete actor <id>`.

## Further reading

- [Architecture](../architecture.md)
- [Roadmap](../roadmap.md)
- [v0.0.10 release notes](../releases/v0.0.10.md)
- [v0.1.0 release notes](../releases/v0.1.0.md)
- [Helm chart](../../charts/actordock-stack/README.md)
