# E2B REST parity tracker

Target for [v0.1.0](v0.1.0.md): **≥60%** of E2B Platform OpenAPI **operations** and **≥60% of schema fields**, with the **full-field rule** below.

Source: [E2B `spec/openapi.yml`](https://github.com/e2b-dev/E2B/blob/main/spec/openapi.yml) — **56 operations**, **~220 request/response fields** (top-level + nested object properties counted per operation).

Count Platform REST only. envd RPC (commands, filesystem) is separate but required for SDK demos.

## Full-field rule

**When an operation is marked `done`, every field in its OpenAPI request and response schemas is implemented** (accept, persist, and return — or document a single self-hosted default such as empty object / false).

- No “route exists but half the JSON keys are missing”.
- No “metadata only” or “stub a subset of PATCH fields” — either ship the **whole** schema for that operation, or **defer the entire operation**.
- Nested objects (`network`, `lifecycle`, `volumeMounts`, …) count: all declared properties must round-trip or have defined semantics.
- **Unimplemented operations do not count** toward the 60% field target.

## Metrics

| Metric | v0.0.4 (now) | v0.1.0 target |
|--------|--------------|---------------|
| Operations | 7 / 56 (**13%**) | **≥34 / 56 (61%)** |
| Fields (full-field ops only) | ~45 / ~220 (**~20%**) | **≥132 / ~220 (60%)** |

Field % = sum of fields in **fully implemented** operations ÷ total fields across all 56 operations.

## Legend

| Mark | Meaning |
|------|---------|
| done | Operation + **all** request/response fields |
| partial | Route exists; fields incomplete (must reach `done` in listed release) |
| v0.0.N | Planned: operation + full fields in that release |
| defer | Entire operation post-v0.1.0 |

## Operation tracker

| # | Method | Path | Status |
|---|--------|------|--------|
| 1 | GET | `/health` | done (v0.0.1) |
| 2 | GET | `/teams` | v0.1.0 |
| 3 | GET | `/teams/{teamID}/metrics` | defer |
| 4 | GET | `/teams/{teamID}/metrics/max` | defer |
| 5 | GET | `/sandboxes` | partial → done v0.1.0 |
| 6 | POST | `/sandboxes` | partial → done v0.1.0 |
| 7 | GET | `/v2/sandboxes` | partial → done v0.1.0 |
| 8 | GET | `/sandboxes/metrics` | v0.0.6 |
| 9 | GET | `/sandboxes/{sandboxID}/logs` | v0.0.6 |
| 10 | GET | `/v2/sandboxes/{sandboxID}/logs` | v0.0.6 |
| 11 | GET | `/sandboxes/{sandboxID}` | partial → done v0.1.0 |
| 12 | DELETE | `/sandboxes/{sandboxID}` | done (v0.0.1) |
| 13 | GET | `/sandboxes/{sandboxID}/metrics` | v0.0.6 |
| 14 | POST | `/sandboxes/{sandboxID}/pause` | done (v0.0.5) |
| 15 | POST | `/sandboxes/{sandboxID}/resume` | done (v0.0.5) |
| 16 | POST | `/sandboxes/{sandboxID}/connect` | v0.0.7 |
| 17 | POST | `/sandboxes/{sandboxID}/timeout` | done (v0.0.3) |
| 18 | PUT | `/sandboxes/{sandboxID}/network` | v0.0.7 |
| 19 | POST | `/sandboxes/{sandboxID}/refreshes` | v0.0.6 |
| 20 | POST | `/sandboxes/{sandboxID}/snapshots` | v0.0.7 |
| 21 | GET | `/snapshots` | v0.0.7 |
| 22 | POST | `/v3/templates` | defer |
| 23 | POST | `/v2/templates` | defer |
| 24 | GET | `/templates/{templateID}/files/{hash}` | v0.0.9 |
| 25 | GET | `/templates` | v0.0.9 |
| 26 | POST | `/templates` | v0.1.0 |
| 27 | GET | `/templates/{templateID}` | v0.0.9 |
| 28 | POST | `/templates/{templateID}` | defer |
| 29 | DELETE | `/templates/{templateID}` | defer |
| 30 | PATCH | `/templates/{templateID}` | v0.1.0 |
| 31 | POST | `/templates/{templateID}/builds/{buildID}` | defer |
| 32 | POST | `/v2/templates/{templateID}/builds/{buildID}` | defer |
| 33 | PATCH | `/v2/templates/{templateID}` | defer |
| 34 | GET | `/templates/{templateID}/builds/{buildID}/status` | defer |
| 35 | GET | `/templates/{templateID}/builds/{buildID}/logs` | defer |
| 36 | POST | `/templates/tags` | defer |
| 37 | DELETE | `/templates/tags` | defer |
| 38 | GET | `/templates/{templateID}/tags` | v0.0.9 |
| 39 | GET | `/templates/aliases/{alias}` | v0.0.9 |
| 40 | GET | `/nodes` | defer |
| 41 | GET | `/nodes/{nodeID}` | defer |
| 42 | POST | `/nodes/{nodeID}` | defer |
| 43 | POST | `/admin/teams/{teamID}/sandboxes/kill` | defer |
| 44 | POST | `/admin/teams/{teamID}/builds/cancel` | defer |
| 45 | POST | `/admin/teams/{teamID}/api-keys` | defer |
| 46 | DELETE | `/admin/teams/{teamID}/api-keys/{apiKeyID}` | defer |
| 47 | POST | `/access-tokens` | v0.1.0 |
| 48 | DELETE | `/access-tokens/{accessTokenID}` | v0.1.0 |
| 49 | GET | `/api-keys` | v0.1.0 |
| 50 | POST | `/api-keys` | v0.1.0 |
| 51 | PATCH | `/api-keys/{apiKeyID}` | defer |
| 52 | DELETE | `/api-keys/{apiKeyID}` | defer |
| 53 | GET | `/volumes` | v0.0.8 |
| 54 | POST | `/volumes` | v0.0.8 |
| 55 | GET | `/volumes/{volumeID}` | v0.0.8 |
| 56 | DELETE | `/volumes/{volumeID}` | v0.0.8 |

## Field backfill — already shipped routes

These routes exist but are **`partial`** until all schema fields are wired:

### `POST /sandboxes` (`NewSandbox` → `Sandbox`)

| Field | Status | Complete in |
|-------|--------|-------------|
| `templateID`, `timeout` | done | v0.0.3 |
| `secure` | partial (reject) | v0.1.0 (full secure path) |
| `autoPause`, `autoResume`, `lifecycle` | done | v0.0.5 |
| `network`, `allow_internet_access` | missing | v0.0.7 |
| `metadata`, `envVars` | missing | v0.1.0 |
| `volumeMounts` | missing | v0.0.8 |
| `mcp` | missing | v0.1.0 (accept + persist; no MCP server) |
| Response: all 8 `Sandbox` fields | partial | v0.1.0 |

### `GET /sandboxes/{id}` (`SandboxDetail` — 18 fields)

| Field | Status | Complete in |
|-------|--------|-------------|
| Core 10 (id, state, times, resources, …) | done | v0.0.4 |
| `alias`, `allowInternetAccess`, `domain` | partial | v0.0.9 / v0.1.0 |
| `envdAccessToken` | missing | v0.1.0 |
| `lifecycle` | done | v0.0.5 |
| `metadata`, `network`, `volumeMounts` | missing | v0.0.7–v0.1.0 |

### `GET /sandboxes`, `GET /v2/sandboxes` (`ListedSandbox` — 13 fields)

All 13 fields **done** in the release that completes list item schema — **v0.1.0** (same as detail/list parity).

### `POST …/timeout` (`PostSandboxesSandboxIDTimeoutBody`)

| Field | Status |
|-------|--------|
| `timeout` | done (v0.0.3) |

### `POST …/pause`

| Field | Status |
|-------|--------|
| (empty body) | done (v0.0.5) |

### `POST …/resume` (`ResumedSandbox` → `Sandbox`)

| Field | Status |
|-------|--------|
| Request: `timeout`, `autoPause` | done (v0.0.5) |
| Response: core `Sandbox` fields (`clientID`, `envdVersion`, `sandboxID`, `templateID`, `domain`) | done (v0.0.5) |
| Response: `alias`, `envdAccessToken`, `trafficAccessToken` | partial (v0.1.0) |

## Cumulative by release

Each release closes **new operations with full fields** and any **backfill** rows due that version.

| Release | New ops (full fields) | Field backfill | Ops % | Fields % (est.) |
|---------|----------------------|----------------|-------|-----------------|
| v0.0.1–v0.0.4 | 7 (2 full, 5 partial) | — | 13% | ~20% |
| [v0.0.5](v0.0.5.md) | +2 (pause, resume) | lifecycle on create/get | **16%** | **~28%** |
| [v0.0.6](v0.0.6.md) | +5 | — | 25% | ~38% |
| [v0.0.7](v0.0.7.md) | +4 | `network`, connect schemas | 32% | ~48% |
| [v0.0.8](v0.0.8.md) | +4 | `volumeMounts` on create/list | 39% | ~52% |
| [v0.0.9](v0.0.9.md) | +5 | `alias`, template GET schemas | 48% | ~56% |
| [v0.1.0](v0.1.0.md) | +7 | sandbox create/get/list **done**; auth schemas | **61%** | **≥60%** |

## Release gate (every v0.0.x PR)

1. List touched operations in PR description.
2. For each: link OpenAPI schema; **every field** has accept/return/store behavior or documented default.
3. Unit/E2E asserts JSON keys present (not only HTTP status).
4. Update this tracker: `partial` → `done`.

## Out of scope for v0.1.0

Template **build** pipeline, admin API, nodes, team metrics — **entire operations deferred** (not partial routes).

Post-v0.1.0: remaining 22 operations, each shipped with full fields per the same rule.
