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

from __future__ import annotations

import os

import httpx
import pytest

from e2b import Sandbox

from runner import run_antigravity_agent, verify_antigravity_import
from support.python_template import sandbox_template_name


def _list_sandbox_ids() -> set[str]:
    api_url = os.environ["E2B_API_URL"].rstrip("/")
    response = httpx.get(
        f"{api_url}/sandboxes",
        headers={"X-API-KEY": os.environ["E2B_API_KEY"]},
        timeout=30.0,
    )
    response.raise_for_status()
    items = response.json()
    assert isinstance(items, list), "list sandboxes API should return a list"
    return {item["sandboxID"] for item in items if isinstance(item, dict) and "sandboxID" in item}


@pytest.mark.timeout(360)
def test_antigravity_template_imports_in_sandbox() -> None:
    module_name = verify_antigravity_import(sandbox_template_name())
    assert module_name == "google.antigravity"


@pytest.mark.timeout(360)
def test_antigravity_agent_weather_tool() -> None:
    if not (os.environ.get("GEMINI_API_KEY") or os.environ.get("GOOGLE_API_KEY")):
        pytest.skip("GEMINI_API_KEY required for live Antigravity agent run")

    before = _list_sandbox_ids()
    sandboxes: list[Sandbox] = []
    try:
        output = run_antigravity_agent(
            "What is the weather in New York City?",
            sandboxes=sandboxes,
        )
        lowered = output.lower()
        assert "new york" in lowered or "sunny" in lowered or "25" in lowered
    finally:
        for sandbox in sandboxes:
            sandbox.kill()

    after = _list_sandbox_ids()
    assert not (after - before), "antigravity example left sandbox IDs behind"
