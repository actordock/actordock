# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0

"""Thin Actordock control-plane HTTP client + sandbox code execution."""

from __future__ import annotations

import base64
import time
from dataclasses import dataclass
from typing import Any

import httpx


@dataclass
class Sandbox:
    id: str
    state: str
    workerID: str = ""


class ActordockClient:
    def __init__(self, api: str, timeout: float = 300.0) -> None:
        self.api = api.rstrip("/")
        # Longer than SEMANTIC_WAIT_SEC (default 120) so Resume can block server-side.
        self._http = httpx.Client(base_url=self.api, timeout=timeout)

    def close(self) -> None:
        self._http.close()

    def healthz(self) -> None:
        r = self._http.get("/healthz")
        r.raise_for_status()

    def ensure_golden(self) -> None:
        deadline = time.time() + 180
        while time.time() < deadline:
            r = self._http.post("/v1/golden/ensure")
            if r.status_code < 300:
                return
            time.sleep(2)
        raise RuntimeError("timed out waiting for golden snapshot")

    def list_workers(self) -> list[dict[str, Any]]:
        r = self._http.get("/v1/workers")
        r.raise_for_status()
        return r.json()

    def wait_workers(self, min_n: int, timeout_sec: float = 120.0) -> list[dict[str, Any]]:
        deadline = time.time() + timeout_sec
        while time.time() < deadline:
            workers = self.list_workers()
            healthy = [w for w in workers if w.get("healthy", True)]
            if len(healthy) >= min_n:
                return healthy
            time.sleep(1)
        raise RuntimeError(f"need >= {min_n} workers; last count={len(self.list_workers())}")

    def create_sandbox(self) -> Sandbox:
        r = self._http.post("/v1/sandboxes")
        r.raise_for_status()
        return _sandbox(r.json())

    def resume(self, sandbox_id: str) -> Sandbox:
        r = self._http.post(f"/v1/sandboxes/{sandbox_id}/resume")
        r.raise_for_status()
        return _sandbox(r.json())

    def get_sandbox(self, sandbox_id: str) -> Sandbox:
        r = self._http.get(f"/v1/sandboxes/{sandbox_id}")
        r.raise_for_status()
        return _sandbox(r.json())

    def list_sandboxes(self) -> list[Sandbox]:
        r = self._http.get("/v1/sandboxes")
        r.raise_for_status()
        return [_sandbox(x) for x in r.json()]

    def delete_sandbox(self, sandbox_id: str) -> None:
        r = self._http.delete(f"/v1/sandboxes/{sandbox_id}")
        if r.status_code not in (200, 204, 404):
            r.raise_for_status()

    def ensure_running(self, sandbox_id: str) -> Sandbox:
        """Resume once if needed; controlplane waits when peers are tool_loop/lock."""
        sb = self.get_sandbox(sandbox_id)
        if sb.state == "running":
            return sb
        return self.resume(sandbox_id)

    def exec(self, sandbox_id: str, argv: list[str]) -> str:
        r = self._http.post(
            f"/v1/sandboxes/{sandbox_id}/exec",
            json={"argv": argv},
        )
        r.raise_for_status()
        body = r.json()
        return body.get("stdout") or ""

    def run_code(
        self,
        sandbox_id: str,
        code: str,
        *,
        file_ext: str,
        exec_template: str,
    ) -> str:
        """Write code to the sandbox and run it with a user-defined command.

        exec_template must contain `{file}` (absolute path of the written script).
        Stock Actordock rootfs is busybox-only; set RUN_CODE_EXEC to your interpreter
        if you provide a custom rootfs (e.g. `/usr/bin/python3 {file}`).
        """
        work = f"/tmp/demo/{sandbox_id}"
        path = f"{work}/job.{file_ext.lstrip('.')}"
        b64 = base64.b64encode(code.encode("utf-8")).decode("ascii")
        run_cmd = exec_template.format(file=path)
        script = (
            f"mkdir -p {work} && "
            f"echo '{b64}' | /bin/busybox base64 -d > {path} && "
            f"{run_cmd}"
        )
        return self.exec(sandbox_id, ["/bin/busybox", "sh", "-c", script])

    def smoke_run_code(
        self,
        sandbox_id: str,
        *,
        file_ext: str,
        exec_template: str,
        snippet: str,
    ) -> None:
        self.ensure_running(sandbox_id)
        out = self.run_code(
            sandbox_id,
            snippet,
            file_ext=file_ext,
            exec_template=exec_template,
        )
        if not out.strip():
            raise RuntimeError(
                "run_code smoke produced empty stdout; check RUN_CODE_EXT / RUN_CODE_EXEC "
                f"and that your sandbox rootfs provides the interpreter ({exec_template!r})"
            )

    def write_file(self, sandbox_id: str, path: str, content: str) -> str:
        if not path.startswith("/tmp/demo/"):
            return "refused: path must be under /tmp/demo/"
        b64 = base64.b64encode(content.encode("utf-8")).decode("ascii")
        script = (
            f"mkdir -p $(/bin/busybox dirname {path}) && "
            f"echo '{b64}' | /bin/busybox base64 -d > {path} && echo ok"
        )
        return self.exec(sandbox_id, ["/bin/busybox", "sh", "-c", script])

    def read_file(self, sandbox_id: str, path: str) -> str:
        if not path.startswith("/tmp/demo/"):
            return "refused: path must be under /tmp/demo/"
        return self.exec(sandbox_id, ["/bin/busybox", "cat", path])

    def run_shell(self, sandbox_id: str, cmd: str) -> str:
        return self.exec(sandbox_id, ["/bin/busybox", "sh", "-c", cmd])


def _sandbox(raw: dict[str, Any]) -> Sandbox:
    return Sandbox(
        id=raw["id"],
        state=raw.get("state", ""),
        workerID=raw.get("workerID") or "",
    )
