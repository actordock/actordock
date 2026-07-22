# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0

"""Environment configuration for the DeepSeek + Actordock demo."""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path

_DEMO_ROOT = Path(__file__).resolve().parents[1]


@dataclass(frozen=True)
class Config:
    deepseek_api_key: str
    deepseek_base_url: str
    deepseek_model: str
    actordock_api: str
    semantic_mode: str  # file | http | both
    semantic_trace: Path
    num_sandboxes: int
    min_workers: int
    demo_task: str
    run_code_ext: str
    run_code_exec: str
    deadline_sec: int
    hf_domain: str
    hf_embed: str


def load_config() -> Config:
    key = os.environ.get("DEEPSEEK_API_KEY", "").strip()
    if not key:
        raise SystemExit("DEEPSEEK_API_KEY is required")

    mode = os.environ.get("SEMANTIC_MODE", "both").strip().lower()
    if mode not in ("file", "http", "both"):
        raise SystemExit(f"invalid SEMANTIC_MODE={mode!r} (use file|http|both)")

    ext = os.environ.get("RUN_CODE_EXT", "sh").strip().lstrip(".")
    if not ext or "/" in ext or ".." in ext:
        raise SystemExit(f"invalid RUN_CODE_EXT={ext!r}")

    runner = os.environ.get("RUN_CODE_EXEC", "/bin/busybox sh {file}").strip()
    if "{file}" not in runner:
        raise SystemExit('RUN_CODE_EXEC must contain "{file}" placeholder')

    trace = Path(
        os.environ.get(
            "SEMANTIC_TRACE",
            str(_DEMO_ROOT / "traces" / "semantic.jsonl"),
        )
    )
    default_task = (
        "Write a short busybox/sh script that computes sum 1..100, prints the result, "
        "and writes it to /tmp/demo/result.txt. Use the run_code tool to execute it "
        "inside the sandbox. Confirm the printed sum."
    )
    task = os.environ.get("DEMO_TASK", default_task)
    return Config(
        deepseek_api_key=key,
        deepseek_base_url=os.environ.get(
            "DEEPSEEK_BASE_URL", "https://api.deepseek.com"
        ).rstrip("/"),
        deepseek_model=os.environ.get("DEEPSEEK_MODEL", "deepseek-v4-flash"),
        actordock_api=os.environ.get(
            "ACTORDOCK_API", "http://127.0.0.1:18080"
        ).rstrip("/"),
        semantic_mode=mode,
        semantic_trace=trace,
        num_sandboxes=int(os.environ.get("NUM_SANDBOXES", "3")),
        min_workers=int(os.environ.get("MIN_WORKERS", "2")),
        demo_task=task,
        run_code_ext=ext,
        run_code_exec=runner,
        deadline_sec=int(os.environ.get("SEMANTIC_DEADLINE_SEC", "600")),
        hf_domain=os.environ.get(
            "SEMANTIC_HF_DOMAIN",
            "llm-semantic-router/mmbert32k-intent-classifier-merged",
        ).strip(),
        hf_embed=os.environ.get(
            "SEMANTIC_HF_EMBED",
            "llm-semantic-router/mmbert-embed-32k-2d-matryoshka",
        ).strip(),
    )
