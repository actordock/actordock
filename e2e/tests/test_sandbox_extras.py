# Copyright 2026 The Actordock Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the License governing permissions and
# limitations under the License.

"""E2B sandbox extras: connect, network, snapshots (v0.0.8)."""

from __future__ import annotations

import os
import socket
from urllib.parse import urlparse

import httpx
from e2b import Sandbox, SandboxState
from e2b.sandbox.commands.command_handle import PtySize

from support.commands import run_command


def _api_url() -> str:
    return os.environ["E2B_API_URL"].rstrip("/")


def _api_headers() -> dict[str, str]:
    return {
        "X-API-KEY": os.environ["E2B_API_KEY"],
        "Content-Type": "application/json",
    }


def _router_host_port() -> tuple[str, int]:
    parsed = urlparse(os.environ["E2B_SANDBOX_URL"])
    host = parsed.hostname or "localhost"
    port = parsed.port or (443 if parsed.scheme == "https" else 80)
    return host, port


def _router_connect(sandbox_id: str, target_host: str, target_port: int = 443) -> int:
    router_host, router_port = _router_host_port()
    payload = (
        f"CONNECT {target_host}:{target_port} HTTP/1.1\r\n"
        f"Host: {target_host}:{target_port}\r\n"
        f"E2b-Sandbox-Id: {sandbox_id}\r\n"
        "\r\n"
    ).encode()
    with socket.create_connection((router_host, router_port), timeout=10) as sock:
        sock.sendall(payload)
        sock.settimeout(10)
        data = sock.recv(4096)
    status_line = data.split(b"\r\n", 1)[0].decode("ascii", errors="replace")
    return int(status_line.split(" ", 2)[1])


def test_connect_paused_sandbox_resumes() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=120)
    try:
        run_command(sbx, "echo before-pause")
        assert sbx.pause() is True
        assert sbx.get_info().state == SandboxState.PAUSED

        resp = httpx.post(
            f"{_api_url()}/sandboxes/{sbx.sandbox_id}/connect",
            headers=_api_headers(),
            json={"timeout": 120},
            timeout=60.0,
        )
        assert resp.status_code == 201
        body = resp.json()
        assert body["sandboxID"] == sbx.sandbox_id
        assert body["templateID"] == "base"
        assert body.get("domain")

        result = run_command(sbx, "echo after-connect")
        assert result.stdout == "after-connect\n"
        assert sbx.get_info().state == SandboxState.RUNNING
    finally:
        sbx.kill()


def test_pty_connect_runs_interactive_command() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=120)
    try:
        output: list[str] = []

        def append_output(data: bytes) -> None:
            output.append(data.decode("utf-8", errors="replace"))

        terminal = sbx.pty.create(PtySize(cols=80, rows=24))
        sbx.pty.send_stdin(terminal.pid, b"echo connect-ok\n")
        terminal.disconnect()

        reconnect = sbx.pty.connect(terminal.pid)
        sbx.pty.send_stdin(terminal.pid, b"echo connect-ok\nexit\n")
        result = reconnect.wait(on_pty=append_output)

        assert result.exit_code == 0
        assert "connect-ok" in "".join(output)
    finally:
        sbx.kill()


def test_network_disable_blocks_router_egress() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=120)
    try:
        run_command(sbx, "echo network")

        put = httpx.put(
            f"{_api_url()}/sandboxes/{sbx.sandbox_id}/network",
            headers=_api_headers(),
            json={"allow_internet_access": False},
            timeout=30.0,
        )
        assert put.status_code == 204

        detail = httpx.get(
            f"{_api_url()}/sandboxes/{sbx.sandbox_id}",
            headers={"X-API-KEY": os.environ["E2B_API_KEY"]},
            timeout=30.0,
        )
        assert detail.status_code == 200
        body = detail.json()
        assert body.get("allowInternetAccess") is False

        assert _router_connect(sbx.sandbox_id, "example.com") == 403
    finally:
        sbx.kill()


def test_network_enabled_allows_router_egress() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=120)
    try:
        run_command(sbx, "echo network")

        put = httpx.put(
            f"{_api_url()}/sandboxes/{sbx.sandbox_id}/network",
            headers=_api_headers(),
            json={"allow_internet_access": True},
            timeout=30.0,
        )
        assert put.status_code == 204

        status = _router_connect(sbx.sandbox_id, "example.com")
        assert status != 403
    finally:
        sbx.kill()


def test_snapshot_create_then_list() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=120)
    try:
        run_command(sbx, "echo snapshot")
        snap = sbx.create_snapshot()
        assert snap.snapshot_id
        assert snap.names

        listed = Sandbox.list_snapshots(sandbox_id=sbx.sandbox_id).next_items()
        assert any(item.snapshot_id == snap.snapshot_id for item in listed)
    finally:
        sbx.kill()
