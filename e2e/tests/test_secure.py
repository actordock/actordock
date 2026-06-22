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

"""E2E secure sandbox create + envd access token path (v0.1.0 WP2 + WP7)."""

from __future__ import annotations

import uuid

import httpx
from e2b import Sandbox

from support.api import api_headers, api_url
from support.commands import run_command
from support.files import read_file, write_file
from support.parity import SANDBOX_CREATE_SECURE_RESPONSE_FIELDS


def _tmp_path(name: str) -> str:
    return f"/tmp/actordock-secure-{name}"


def test_secure_create_response_fields() -> None:
    resp = httpx.post(
        f"{api_url()}/sandboxes",
        headers=api_headers(),
        json={"templateID": "base", "secure": True, "timeout": 120},
        timeout=60.0,
    )
    assert resp.status_code == 201, resp.text
    body = resp.json()
    sandbox_id = body["sandboxID"]
    try:
        missing = SANDBOX_CREATE_SECURE_RESPONSE_FIELDS - set(body)
        assert not missing, f"missing create fields: {missing}"
        assert body["envdAccessToken"]
        assert body["trafficAccessToken"]

        detail = httpx.get(
            f"{api_url()}/sandboxes/{sandbox_id}",
            headers=api_headers(),
            timeout=30.0,
        )
        assert detail.status_code == 200, detail.text
        assert detail.json()["envdAccessToken"] == body["envdAccessToken"]
    finally:
        httpx.delete(
            f"{api_url()}/sandboxes/{sandbox_id}",
            headers=api_headers(),
            timeout=30.0,
        )


def test_secure_command_and_files_without_insecure_bypass() -> None:
    """secure=True end-to-end: command + files use envd access token (not dev bypass)."""
    sbx = Sandbox.create(template="base", secure=True, timeout=120)
    try:
        path = _tmp_path(f"{uuid.uuid4().hex}.txt")
        content = "secure-payload"
        write_file(sbx, path, content)
        assert read_file(sbx, path) == content

        result = run_command(sbx, f"cat {path}")
        assert result.stdout.strip() == content

        result = run_command(sbx, "echo secure-ok")
        assert result.stdout.strip() == "secure-ok"
    finally:
        sbx.kill()


def test_insecure_create_has_no_envd_access_token() -> None:
    resp = httpx.post(
        f"{api_url()}/sandboxes",
        headers=api_headers(),
        json={"templateID": "base", "secure": False, "timeout": 120},
        timeout=60.0,
    )
    assert resp.status_code == 201, resp.text
    body = resp.json()
    sandbox_id = body["sandboxID"]
    try:
        assert "envdAccessToken" not in body or not body.get("envdAccessToken")
    finally:
        httpx.delete(
            f"{api_url()}/sandboxes/{sandbox_id}",
            headers=api_headers(),
            timeout=30.0,
        )
