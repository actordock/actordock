# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0

"""Semantic heartbeat: file JSONL and optional POST /v1/signals/semantic."""

from __future__ import annotations

import json
import threading
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import httpx

_lock = threading.Lock()


def report(
    *,
    api: str,
    mode: str,
    trace_path: Path,
    sandbox_id: str,
    phase: str,
    lock: bool,
    remaining_steps: int | None = None,
    deadline: datetime | None = None,
    workflow_id: str | None = None,
    task_profile: dict[str, Any] | None = None,
    extra: dict[str, Any] | None = None,
) -> None:
    ts = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    event: dict[str, Any] = {
        "ts": ts,
        "sandboxID": sandbox_id,
        "phase": phase,
        "lock": lock,
        "source": "demo",
    }
    if remaining_steps is not None:
        event["remainingSteps"] = remaining_steps
    if deadline is not None:
        event["deadline"] = _rfc3339(deadline)
    if workflow_id:
        event["workflowID"] = workflow_id
    if task_profile:
        event["taskProfile"] = task_profile
    if extra:
        event["extra"] = extra

    semantic: dict[str, Any] = {
        "version": "v1",
        "phase": phase,
        "lock": lock,
    }
    if remaining_steps is not None:
        semantic["remainingSteps"] = remaining_steps
    if deadline is not None:
        semantic["deadline"] = _rfc3339(deadline)
    if workflow_id:
        semantic["workflowID"] = workflow_id
    if task_profile:
        semantic["taskProfile"] = task_profile

    if mode in ("file", "both"):
        _append_file(trace_path, event)
    if mode in ("http", "both"):
        _post_http(api, {"sandboxID": sandbox_id, "semantic": semantic})


def _rfc3339(dt: datetime) -> str:
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    return dt.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")


def _append_file(path: Path, event: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    line = json.dumps(event, ensure_ascii=False)
    with _lock:
        with path.open("a", encoding="utf-8") as f:
            f.write(line + "\n")


def _post_http(api: str, body: dict[str, Any]) -> None:
    url = f"{api.rstrip('/')}/v1/signals/semantic"
    try:
        r = httpx.post(url, json=body, timeout=5.0)
        if r.status_code >= 300:
            print(f"[semantic] warn: POST {url} -> {r.status_code}: {r.text[:200]}")
    except httpx.HTTPError as e:
        print(f"[semantic] warn: POST {url} failed: {e}")
