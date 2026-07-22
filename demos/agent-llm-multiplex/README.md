# Agent LLM multiplex demo (mode A)

Host-side DeepSeek agent (`openai-agents`) uses Actordock sandboxes as the **code execution** environment. The demo posts **L1** phase/lock, **L2** deadline/workflowID, and **L3** `taskProfile` from local HF weights (`llm-semantic-router` domain + embed). Kind's default `semantic-score` policy decides who to Suspend under contention.

**Place order:** create → classify → POST `taskProfile` → Resume (so the first eviction already sees L3). Default prompts differ per sandbox; set `DEMO_TASK` to force one prompt on all.

Stock Worker rootfs is **busybox-only**. How code runs is **user-defined** via `RUN_CODE_EXT` / `RUN_CODE_EXEC` (bring your own interpreter in a custom rootfs if you want Python, etc.).

## What it shows

- DeepSeek tool-loop with primary tool `run_code` (runner configured by you)
- Semantic phases (aligned with Crab turn/LLM-wait + vLLM SAAR tool-loop/idle):
  `llm_wait` / `tool_loop`(+lock) / `idle`

| Phase | Meaning | External anchor |
|-------|---------|-----------------|
| `llm_wait` | Waiting on model response (still holds Worker; policy may Suspend) | Crab: overlap C/R with LLM wait |
| `tool_loop` | Running a tool (e.g. `run_code`); `lock=true` | SAAR: hard lock during active tool loop |
| `idle` | Between turns / task done | SAAR: idle as reselection / soft boundary |

- N sandboxes on M workers (default **3 sandboxes / 2 Workers**): pool-full Resume uses controlplane POLICY (prefer kick `llm_wait`, spare `tool_loop`). If every peer is `tool_loop`+lock, **Resume blocks** on the server until a slot frees (`SEMANTIC_WAIT_SEC`, default 120s).

## Prerequisites

1. Kind cluster with Actordock and **2 workers** (default `POLICY=semantic-score`):
   ```bash
   ./hack/kind-up.sh
   # already-running cluster:
   kubectl -n actordock scale statefulset/worker --replicas=2
   kubectl -n actordock rollout status statefulset/worker --timeout=120s
   ```
2. Port-forward control plane:
   ```bash
   kubectl -n actordock port-forward svc/controlplane 18080:8080
   ```
3. Python 3.11+ on the **host** (driver only) and a DeepSeek API key

## Setup

```bash
cd demos/agent-llm-multiplex
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
cp .env.example .env   # edit DEEPSEEK_API_KEY
set -a && source .env && set +a
```

## Run (default: busybox sh + semantic HTTP)

```bash
export DEEPSEEK_API_KEY=...
export ACTORDOCK_API=http://127.0.0.1:18080
# defaults: SEMANTIC_MODE=both, RUN_CODE_EXT=sh, RUN_CODE_EXEC='/bin/busybox sh {file}'
PYTHONPATH=. python -m driver.main
```

### Optional: your own interpreter

If your Worker rootfs includes Python (you built a custom image yourself):

```bash
export RUN_CODE_EXT=py
export RUN_CODE_EXEC='/usr/bin/python3 {file}'
export DEMO_TASK='Write a short Python program that prints sum(1..100). Use run_code.'
PYTHONPATH=. python -m driver.main
```

`{file}` is replaced with the absolute script path inside the sandbox.

Then:

```bash
python scripts/summarize_trace.py traces/semantic.jsonl
```

## Env

| Var | Default | Notes |
|-----|---------|-------|
| `DEEPSEEK_API_KEY` | required | |
| `DEEPSEEK_BASE_URL` | `https://api.deepseek.com` | OpenAI-compatible |
| `DEEPSEEK_MODEL` | `deepseek-v4-flash` | or `deepseek-v4-pro` |
| `ACTORDOCK_API` | `http://127.0.0.1:18080` | |
| `SEMANTIC_MODE` | `both` | `file` \| `http` \| `both` (http → `POST /v1/signals/semantic`) |
| `NUM_SANDBOXES` | `3` | Contending sessions |
| `MIN_WORKERS` | `2` | Expect 2 Workers; warns if more healthy |
| `RUN_CODE_EXT` | `sh` | Script extension written under `/tmp/demo/` |
| `RUN_CODE_EXEC` | `/bin/busybox sh {file}` | Must include `{file}` |
| `DEMO_TASK` | (unset) | If set, all sandboxes share this prompt; else use built-in easy/medium/hard bank |
| `SEMANTIC_DEADLINE_SEC` | `600` | L2 deadline = now + this many seconds |
| `SEMANTIC_HF_DOMAIN` | `llm-semantic-router/mmbert32k-intent-classifier-merged` | Domain classifier |
| `SEMANTIC_HF_EMBED` | `llm-semantic-router/mmbert-embed-32k-2d-matryoshka` | Embedding model |

## Layout

```text
actor/     DeepSeek agent + semantic + Actordock client
driver/    create/resume sandboxes, dispatch tasks
scripts/   summarize_trace.py
traces/    local JSONL (gitignored)
```

## Non-goals

- Bundling Python (or any language) into the default Worker `Dockerfile` rootfs
- Agent process inside gVisor (mode B) — later
- CI calls to DeepSeek (costs / secrets)
- Re-implementing eviction / wait-for-slot in the demo (Resume wait is controlplane `SEMANTIC_WAIT_SEC`)
