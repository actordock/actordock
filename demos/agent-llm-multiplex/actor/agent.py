# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0

"""DeepSeek-backed agent that executes code inside an Actordock sandbox."""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from typing import Any

from agents import (
    Agent,
    OpenAIChatCompletionsModel,
    RunContextWrapper,
    Runner,
    function_tool,
    set_tracing_disabled,
)
from openai import AsyncOpenAI

from actor import semantic
from actor.config import Config
from actor.tools import ActordockClient

set_tracing_disabled(True)


@dataclass
class DemoContext:
    sandbox_id: str
    client: ActordockClient
    cfg: Config
    deadline: datetime
    workflow_id: str
    task_profile: dict[str, Any] = field(default_factory=dict)


def _report(
    ctx: RunContextWrapper[DemoContext],
    phase: str,
    lock: bool,
    *,
    include_profile: bool = False,
) -> None:
    c = ctx.context
    semantic.report(
        api=c.cfg.actordock_api,
        mode=c.cfg.semantic_mode,
        trace_path=c.cfg.semantic_trace,
        sandbox_id=c.sandbox_id,
        phase=phase,
        lock=lock,
        deadline=c.deadline,
        workflow_id=c.workflow_id,
        task_profile=c.task_profile if include_profile else None,
    )


def _enter_llm_wait(ctx: DemoContext) -> None:
    """L1/L2 heartbeat — Suspend/victim choice is POLICY, not the demo."""
    semantic.report(
        api=ctx.cfg.actordock_api,
        mode=ctx.cfg.semantic_mode,
        trace_path=ctx.cfg.semantic_trace,
        sandbox_id=ctx.sandbox_id,
        phase="llm_wait",
        lock=False,
        deadline=ctx.deadline,
        workflow_id=ctx.workflow_id,
    )


def _with_sandbox_tool(ctx: RunContextWrapper[DemoContext], op: Callable[[], str]) -> str:
    """Ensure running → tool_loop → op → llm_wait."""
    c = ctx.context
    c.client.ensure_running(c.sandbox_id)
    _report(ctx, "tool_loop", True)
    try:
        return op()
    finally:
        _enter_llm_wait(c)


@function_tool
def run_code(ctx: RunContextWrapper[DemoContext], code: str) -> str:
    """Execute a code/script snippet inside the Actordock sandbox via the user-configured runner."""
    cfg = ctx.context.cfg
    return _truncate(
        _with_sandbox_tool(
            ctx,
            lambda: ctx.context.client.run_code(
                ctx.context.sandbox_id,
                code,
                file_ext=cfg.run_code_ext,
                exec_template=cfg.run_code_exec,
            ),
        )
    )


@function_tool
def write_file(ctx: RunContextWrapper[DemoContext], path: str, content: str) -> str:
    """Write a file under /tmp/demo/ in the sandbox."""
    return _truncate(
        _with_sandbox_tool(
            ctx,
            lambda: ctx.context.client.write_file(ctx.context.sandbox_id, path, content),
        )
    )


@function_tool
def read_file(ctx: RunContextWrapper[DemoContext], path: str) -> str:
    """Read a file under /tmp/demo/ from the sandbox."""
    return _truncate(
        _with_sandbox_tool(
            ctx,
            lambda: ctx.context.client.read_file(ctx.context.sandbox_id, path),
        )
    )


@function_tool
def run_shell(ctx: RunContextWrapper[DemoContext], cmd: str) -> str:
    """Run a short shell command in the sandbox (busybox). Prefer run_code for multi-line scripts."""
    return _truncate(
        _with_sandbox_tool(
            ctx,
            lambda: ctx.context.client.run_shell(ctx.context.sandbox_id, cmd),
        )
    )


def build_agent(cfg: Config) -> Agent[DemoContext]:
    client = AsyncOpenAI(
        api_key=cfg.deepseek_api_key,
        base_url=cfg.deepseek_base_url,
    )
    model = OpenAIChatCompletionsModel(
        model=cfg.deepseek_model,
        openai_client=client,
    )
    return Agent[DemoContext](
        name="demo-coder",
        instructions=(
            "You are a coding agent. Solve tasks by writing and running code with the "
            f"run_code tool inside the sandbox. Scripts use extension .{cfg.run_code_ext} "
            f"and are executed as: {cfg.run_code_exec}. Keep answers short. Prefer run_code "
            "over run_shell for multi-line programs. Paths for write_file/read_file must "
            "be under /tmp/demo/."
        ),
        model=model,
        tools=[run_code, write_file, read_file, run_shell],
    )


def new_demo_context(
    *,
    sandbox_id: str,
    client: ActordockClient,
    cfg: Config,
    task_profile: dict[str, Any],
) -> DemoContext:
    return DemoContext(
        sandbox_id=sandbox_id,
        client=client,
        cfg=cfg,
        deadline=datetime.now(timezone.utc) + timedelta(seconds=cfg.deadline_sec),
        workflow_id=f"demo-{sandbox_id[:8]}",
        task_profile=task_profile,
    )


async def handle_task(
    agent: Agent[DemoContext],
    ctx: DemoContext,
    prompt: str,
) -> str:
    ctx.client.ensure_running(ctx.sandbox_id)
    # Initial L1+L2+L3 snapshot (taskProfile once at session start).
    semantic.report(
        api=ctx.cfg.actordock_api,
        mode=ctx.cfg.semantic_mode,
        trace_path=ctx.cfg.semantic_trace,
        sandbox_id=ctx.sandbox_id,
        phase="llm_wait",
        lock=False,
        deadline=ctx.deadline,
        workflow_id=ctx.workflow_id,
        task_profile=ctx.task_profile,
    )
    result = await Runner.run(agent, prompt, context=ctx)
    semantic.report(
        api=ctx.cfg.actordock_api,
        mode=ctx.cfg.semantic_mode,
        trace_path=ctx.cfg.semantic_trace,
        sandbox_id=ctx.sandbox_id,
        phase="idle",
        lock=False,
        deadline=ctx.deadline,
        workflow_id=ctx.workflow_id,
    )
    return str(result.final_output)


def _truncate(s: str, limit: int = 8192) -> str:
    if len(s) <= limit:
        return s
    return s[:limit] + f"\n...[truncated {len(s) - limit} bytes]"
