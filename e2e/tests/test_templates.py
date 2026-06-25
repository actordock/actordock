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

"""E2E template read APIs (v0.0.10)."""

from __future__ import annotations

import os

import httpx


def _api_url() -> str:
    return os.environ["E2B_API_URL"].rstrip("/")


def _api_headers() -> dict[str, str]:
    return {
        "X-API-KEY": os.environ["E2B_API_KEY"],
        "Content-Type": "application/json",
    }


def test_list_templates_includes_base() -> None:
    resp = httpx.get(
        f"{_api_url()}/templates",
        headers=_api_headers(),
        timeout=30.0,
    )
    assert resp.status_code == 200, resp.text
    templates = resp.json()
    assert isinstance(templates, list)
    base = next((item for item in templates if item.get("templateID") == "base"), None)
    assert base is not None, templates
    for key in (
        "buildID",
        "cpuCount",
        "memoryMB",
        "diskSizeMB",
        "public",
        "aliases",
        "names",
        "createdAt",
        "updatedAt",
        "spawnCount",
        "buildCount",
        "envdVersion",
        "buildStatus",
    ):
        assert key in base, base


def test_get_template_alias_base() -> None:
    resp = httpx.get(
        f"{_api_url()}/templates/aliases/base",
        headers=_api_headers(),
        timeout=30.0,
    )
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert body["templateID"] == "base"
    assert body["public"] is True


def test_get_template_tags_empty() -> None:
    resp = httpx.get(
        f"{_api_url()}/templates/base/tags",
        headers=_api_headers(),
        timeout=30.0,
    )
    assert resp.status_code == 200, resp.text
    assert resp.json() == []


def test_get_template_not_found() -> None:
    resp = httpx.get(
        f"{_api_url()}/templates/missing-template-id",
        headers=_api_headers(),
        timeout=30.0,
    )
    assert resp.status_code == 404, resp.text


def test_get_template_alias_not_found() -> None:
    resp = httpx.get(
        f"{_api_url()}/templates/aliases/missing-alias",
        headers=_api_headers(),
        timeout=30.0,
    )
    assert resp.status_code == 404, resp.text


def test_template_exists_via_sdk() -> None:
    from e2b import Template

    assert Template.exists("base") is True
    assert Template.exists("python") is True
    assert Template.exists("missing-alias-for-e2e") is False


def test_official_python_template_has_python3() -> None:
    from e2b import Sandbox

    from support.commands import run_command

    sbx = Sandbox.create(template="python", secure=False, timeout=120)
    try:
        out = run_command(sbx, 'python3 -c "import sys; print(sys.version_info.major)"')
        assert out.exit_code == 0
        assert out.stdout.strip() == "3"
    finally:
        sbx.kill()
