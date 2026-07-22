# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0

"""Drive N sandboxes on M workers with a DeepSeek coding agent (mode A).

Default story: 3 sandboxes contend for 2 Workers (resume of the 3rd Suspends a victim).
L3 classify + POST taskProfile happens *before* Resume so semantic-score can use it.
"""

from __future__ import annotations

import asyncio
import os
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

# Allow `python -m driver.main` from demos/agent-llm-multiplex.
_ROOT = Path(__file__).resolve().parents[1]
if str(_ROOT) not in sys.path:
    sys.path.insert(0, str(_ROOT))

from actor import semantic
from actor.agent import build_agent, handle_task, new_demo_context
from actor.config import load_config
from actor.hf_classify import get_bundle
from actor.profile import classify
from actor.tools import ActordockClient, Sandbox

# Differentiated prompts so L3 priors are not identical (busybox-friendly).
_TASK_BANK = [
    # intended easier / shorter
    "Using run_code (busybox sh), echo hello and write it to /tmp/demo/hello.txt. Confirm the file.",
    # intended medium (default-like)
    "Write a short busybox/sh script that computes sum 1..100, prints the result, "
    "and writes it to /tmp/demo/result.txt. Use run_code. Confirm the printed sum.",
    # intended harder / multi-step
    "Using only busybox/sh via run_code: create /tmp/demo/a.txt and /tmp/demo/b.txt with "
    "different lines, merge them into a sorted unique /tmp/demo/merged.txt, print the line "
    "count, and verify with wc. Prefer multiple small run_code calls over one giant script.",
]


async def _run_one(
    agent,
    client: ActordockClient,
    cfg,
    sb: Sandbox,
    prompt: str,
    task_profile: dict,
) -> tuple[str, str, str | None]:
    try:
        ctx = new_demo_context(
            sandbox_id=sb.id,
            client=client,
            cfg=cfg,
            task_profile=task_profile,
        )
        print(
            f"[driver] run id={sb.id[:8]}… "
            f"signal={task_profile.get('complexitySignal')} "
            f"domain={task_profile.get('domain')} "
            f"sim={task_profile.get('embeddingSim')} "
            f"conf={task_profile.get('confidence')}"
        )
        out = await handle_task(agent, ctx, prompt)
        return sb.id, out, None
    except Exception as e:  # noqa: BLE001 — demo driver surfaces errors per session
        return sb.id, "", str(e)


def _cleanup(client: ActordockClient, trace: Path) -> None:
    old = client.list_sandboxes()
    for sb in old:
        client.delete_sandbox(sb.id)
    if old:
        print(f"[driver] cleaned {len(old)} leftover sandbox(es)")
    trace.parent.mkdir(parents=True, exist_ok=True)
    trace.write_text("", encoding="utf-8")
    print(f"[driver] reset semantic trace {trace}")


def _prompts(n: int, cfg) -> list[str]:
    """Per-sandbox tasks. Set DEMO_TASK to force the same prompt on all."""
    if "DEMO_TASK" in os.environ:
        return [cfg.demo_task] * n
    return [_TASK_BANK[i % len(_TASK_BANK)] for i in range(n)]


def _seed_l3(
    *,
    cfg,
    sandbox_id: str,
    prompt: str,
    profile: dict,
) -> None:
    """POST L3 (+ llm_wait) before Resume so Place/Evict can read taskProfile."""
    deadline = datetime.now(timezone.utc) + timedelta(seconds=cfg.deadline_sec)
    semantic.report(
        api=cfg.actordock_api,
        mode=cfg.semantic_mode,
        trace_path=cfg.semantic_trace,
        sandbox_id=sandbox_id,
        phase="llm_wait",
        lock=False,
        deadline=deadline,
        workflow_id=f"demo-{sandbox_id[:8]}",
        task_profile=profile,
        extra={"promptPreview": prompt[:160]},
    )


async def amain() -> None:
    cfg = load_config()
    client = ActordockClient(cfg.actordock_api)
    try:
        print(f"[driver] API={cfg.actordock_api} model={cfg.deepseek_model}")
        client.healthz()
        client.ensure_golden()
        _cleanup(client, cfg.semantic_trace)

        workers = client.wait_workers(cfg.min_workers)
        print(
            f"[driver] healthy_workers≈{len(workers)} (need min={cfg.min_workers}; "
            f"target story: {cfg.num_sandboxes} sandboxes vs 2 Workers)"
        )
        if len(workers) > cfg.min_workers:
            print(
                "[driver] warn: more than 2 healthy workers — scale StatefulSet to 2 "
                "for contention (kubectl -n actordock scale sts/worker --replicas=2)"
            )

        # Load HF before place so L3 is available on first Resume eviction.
        get_bundle(domain_id=cfg.hf_domain, embed_id=cfg.hf_embed)

        prompts = _prompts(cfg.num_sandboxes, cfg)
        sessions: list[tuple[Sandbox, str, dict]] = []

        for i in range(cfg.num_sandboxes):
            prompt = prompts[i]
            sb = client.create_sandbox()
            print(f"[driver] create[{i}] id={sb.id} state={sb.state}")

            profile = classify(prompt)
            print(
                f"[driver] L3[{i}] id={sb.id[:8]}… "
                f"signal={profile.get('complexitySignal')} "
                f"domain={profile.get('domain')} "
                f"rule={profile.get('complexityRule')} "
                f"hard={profile.get('hardScore')} easy={profile.get('easyScore')} "
                f"sim={profile.get('embeddingSim')} "
                f"conf={profile.get('confidence')} "
                f"task={prompt[:72]}…"
            )
            _seed_l3(cfg=cfg, sandbox_id=sb.id, prompt=prompt, profile=profile)

            try:
                sb = client.resume(sb.id)
                print(
                    f"[driver] resume[{i}] id={sb.id} state={sb.state} worker={sb.workerID}"
                )
            except Exception as e:  # noqa: BLE001
                print(f"[driver] resume[{i}] id={sb.id} failed: {e}")
                sb = client.get_sandbox(sb.id)
                print(
                    f"[driver] after-fail id={sb.id} state={sb.state} worker={sb.workerID}"
                )
            sessions.append((sb, prompt, profile))

        # Refresh states after the fill/evict wave.
        sandboxes = [client.get_sandbox(s.id) for s, _, _ in sessions]
        sessions = [
            (sandboxes[i], sessions[i][1], sessions[i][2]) for i in range(len(sessions))
        ]
        running = [s for s, _, _ in sessions if s.state == "running"]
        suspended = [s for s, _, _ in sessions if s.state == "suspended"]
        print(
            f"[driver] after place: running={len(running)} suspended={len(suspended)}"
        )
        for s, prompt, profile in sessions:
            print(
                f"  - {s.id}: {s.state} worker={s.workerID} "
                f"signal={profile.get('complexitySignal')} sim={profile.get('embeddingSim')}"
            )

        if not running:
            raise SystemExit("no running sandboxes; cannot smoke-test run_code")

        print(
            f"[driver] run_code: ext=.{cfg.run_code_ext} exec={cfg.run_code_exec!r}"
        )
        smoke = "echo ok\n" if cfg.run_code_ext in ("sh", "bash") else "print('ok')\n"
        client.smoke_run_code(
            running[0].id,
            file_ext=cfg.run_code_ext,
            exec_template=cfg.run_code_exec,
            snippet=smoke,
        )
        print("[driver] run_code smoke ok")

        agent = build_agent(cfg)
        tasks = [
            _run_one(agent, client, cfg, sb, prompt, profile)
            for sb, prompt, profile in sessions
        ]
        print(f"[driver] dispatching {len(tasks)} agent task(s) via DeepSeek…")
        results = await asyncio.gather(*tasks)

        print("\n=== results ===")
        for sid, out, err in results:
            if err:
                print(f"- {sid}: ERROR {err}")
            else:
                print(f"- {sid}: {out[:500]}")

        print("\n=== sandbox states ===")
        for sb in client.list_sandboxes():
            print(f"- {sb.id}: state={sb.state} worker={sb.workerID}")

        print(f"\n[driver] semantic trace: {cfg.semantic_trace}")
        print("[driver] summarize: python scripts/summarize_trace.py")
    finally:
        client.close()


def main() -> None:
    asyncio.run(amain())


if __name__ == "__main__":
    main()
