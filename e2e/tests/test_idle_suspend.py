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
# See the License for the specific language governing permissions and
# limitations under the License.

"""E2B idle suspend (scheduler pause + router auto-resume)."""

from __future__ import annotations

import os
import time

import httpx
import pytest
from e2b import Sandbox, SandboxState
from e2b.exceptions import SandboxNotFoundException

# Min platform timeout (15s) + scheduler poll (~5s) + small margin.
PAUSE_TEST_TIMEOUT = 15
PAUSE_TEST_WAIT = 22


def _wait_until_running(sbx: Sandbox, timeout: float = 60.0) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        if sbx.is_running():
            return
        time.sleep(0.5)
    pytest.fail("sandbox did not become running in time")


def test_pause_lifecycle_survives_timeout() -> None:
    sbx = Sandbox.create(
        template="base",
        secure=False,
        timeout=PAUSE_TEST_TIMEOUT,
        lifecycle={"on_timeout": "pause", "auto_resume": True},
    )
    sandbox_id = sbx.sandbox_id
    try:
        time.sleep(PAUSE_TEST_WAIT)
        info = Sandbox.get_info(sandbox_id)
        assert info.sandbox_id == sandbox_id
        assert info.state == SandboxState.PAUSED
    finally:
        sbx.kill()


def test_command_after_pause_auto_resume() -> None:
    sbx = Sandbox.create(
        template="base",
        secure=False,
        timeout=PAUSE_TEST_TIMEOUT,
        lifecycle={"on_timeout": "pause", "auto_resume": True},
    )
    try:
        time.sleep(PAUSE_TEST_WAIT)
        assert sbx.get_info().state == SandboxState.PAUSED
        result = sbx.commands.run("echo back")
        assert result.stdout == "back\n"
        assert sbx.get_info().state == SandboxState.RUNNING
    finally:
        sbx.kill()


def test_explicit_pause_and_resume() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=120)
    try:
        assert sbx.pause() is True
        assert sbx.get_info().state == SandboxState.PAUSED

        api_url = os.environ["E2B_API_URL"].rstrip("/")
        api_key = os.environ["E2B_API_KEY"]
        resp = httpx.post(
            f"{api_url}/sandboxes/{sbx.sandbox_id}/resume",
            headers={"X-API-KEY": api_key, "Content-Type": "application/json"},
            json={"timeout": 120, "autoPause": True},
            timeout=30.0,
        )
        assert resp.status_code == 201
        body = resp.json()
        assert body["sandboxID"] == sbx.sandbox_id
        assert body["templateID"] == "base"

        _wait_until_running(sbx)
        result = sbx.commands.run("echo resumed")
        assert result.stdout == "resumed\n"
        assert sbx.get_info().state == SandboxState.RUNNING
    finally:
        sbx.kill()


def test_kill_lifecycle_still_auto_deleted() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=PAUSE_TEST_TIMEOUT)
    sandbox_id = sbx.sandbox_id
    time.sleep(PAUSE_TEST_WAIT)
    with pytest.raises(SandboxNotFoundException):
        Sandbox.get_info(sandbox_id)
