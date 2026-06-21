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
import time

import httpx
from e2b import Sandbox, SandboxState
from e2b.sandbox.commands.command_handle import PtySize

from support.commands import run_command

EXTERNAL_EGRESS_URL = "http://example.com/"
EXTERNAL_EGRESS_MARKER = "Example Domain"
SUCCESS_EGRESS_STATUSES = (200, 204, 301, 302)
# Loopback hits router again; E2b-Sandbox-Id routes to envd /health (204), not router JSON.
INTERNAL_EGRESS_URL = "http://127.0.0.1:8081/health"


def _api_url() -> str:
    return os.environ["E2B_API_URL"].rstrip("/")


def _api_headers() -> dict[str, str]:
    return {
        "X-API-KEY": os.environ["E2B_API_KEY"],
        "Content-Type": "application/json",
    }


def _router_proxy_url() -> str:
    return os.environ["E2B_SANDBOX_URL"].rstrip("/")


def _put_network_policy(sandbox_id: str, *, allow_internet_access: bool) -> None:
    resp = httpx.put(
        f"{_api_url()}/sandboxes/{sandbox_id}/network",
        headers=_api_headers(),
        json={"allow_internet_access": allow_internet_access},
        timeout=30.0,
    )
    assert resp.status_code == 204, resp.text


def _router_egress_get(
    sandbox_id: str,
    target_url: str = EXTERNAL_EGRESS_URL,
    *,
    follow_redirects: bool = False,
    max_attempts: int = 1,
    retry_delay_sec: float = 1.0,
) -> httpx.Response:
    """Send an outbound HTTP request through the router egress proxy.

    Matches router isEgressRequest (absolute http:// Request-URI) and the
    unit-test path in internal/router/egress_test.go.
    """
    last_resp: httpx.Response | None = None
    for attempt in range(max_attempts):
        with httpx.Client(
            proxy=_router_proxy_url(),
            timeout=10.0,
            trust_env=False,
            follow_redirects=follow_redirects,
        ) as client:
            last_resp = client.get(
                target_url,
                headers={"E2b-Sandbox-Id": sandbox_id},
            )
        if last_resp.status_code in SUCCESS_EGRESS_STATUSES or last_resp.status_code == 403:
            return last_resp
        if attempt + 1 < max_attempts:
            time.sleep(retry_delay_sec)
    assert last_resp is not None
    return last_resp


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

        # Warm up envd/router path to reduce CI flakiness.
        run_command(sbx, "echo warmup")

        last_err: Exception | None = None
        for _ in range(3):
            try:
                terminal = sbx.pty.create(
                    PtySize(cols=80, rows=24),
                    timeout=60,
                    request_timeout=60,
                )
                sbx.pty.send_stdin(terminal.pid, b"echo connect-ok\n")
                terminal.disconnect()

                reconnect = sbx.pty.connect(terminal.pid, timeout=60, request_timeout=60)
                sbx.pty.send_stdin(terminal.pid, b"echo connect-ok\nexit\n")
                result = reconnect.wait(on_pty=append_output)
                break
            except Exception as e:
                last_err = e
                time.sleep(1.0)
        else:
            raise last_err  # type: ignore[misc]

        assert result.exit_code == 0
        assert "connect-ok" in "".join(output)
    finally:
        sbx.kill()


def test_network_toggle_controls_router_egress() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=120)
    try:
        run_command(sbx, "echo network")

        _put_network_policy(sbx.sandbox_id, allow_internet_access=False)

        detail = httpx.get(
            f"{_api_url()}/sandboxes/{sbx.sandbox_id}",
            headers={"X-API-KEY": os.environ["E2B_API_KEY"]},
            timeout=30.0,
        )
        assert detail.status_code == 200
        assert detail.json().get("allowInternetAccess") is False

        blocked = _router_egress_get(sbx.sandbox_id, EXTERNAL_EGRESS_URL)
        assert blocked.status_code == 403, blocked.text

        _put_network_policy(sbx.sandbox_id, allow_internet_access=True)

        allowed = _router_egress_get(
            sbx.sandbox_id,
            EXTERNAL_EGRESS_URL,
            follow_redirects=True,
            max_attempts=3,
        )
        assert allowed.status_code == 200, allowed.text
        assert EXTERNAL_EGRESS_MARKER in allowed.text
    finally:
        sbx.kill()


def test_internal_egress_allowed_when_internet_disabled() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=120)
    try:
        run_command(sbx, "echo network")
        _put_network_policy(sbx.sandbox_id, allow_internet_access=False)

        resp = _router_egress_get(sbx.sandbox_id, INTERNAL_EGRESS_URL)
        assert resp.status_code != 403, resp.text
        assert resp.status_code == 204, resp.text
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
