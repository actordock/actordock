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

"""Optional dashboard smoke tests (skip when dashboard is not reachable)."""

from __future__ import annotations

import os

import httpx
import pytest


def _dashboard_url() -> str:
    return os.environ.get("DASHBOARD_URL", "http://localhost:3000").rstrip("/")


@pytest.fixture(scope="module")
def dashboard_url() -> str:
    url = _dashboard_url()
    try:
        resp = httpx.get(f"{url}/health", timeout=3.0)
        resp.raise_for_status()
    except (httpx.HTTPError, OSError):
        pytest.skip(
            "dashboard not reachable; port-forward svc/dashboard 3000:3000 "
            "or set DASHBOARD_URL"
        )
    return url


def test_dashboard_health(dashboard_url: str) -> None:
    resp = httpx.get(f"{dashboard_url}/health", timeout=10.0)
    assert resp.status_code == 200
    assert resp.json().get("status") == "ok"


def test_dashboard_platform_proxy(dashboard_url: str) -> None:
    resp = httpx.get(f"{dashboard_url}/api/platform/health", timeout=10.0)
    assert resp.status_code == 200
    assert resp.json().get("status") == "ok"


def test_dashboard_serves_spa(dashboard_url: str) -> None:
    resp = httpx.get(f"{dashboard_url}/", timeout=10.0)
    assert resp.status_code == 200
    assert "text/html" in resp.headers.get("content-type", "")
    assert "<" in resp.text
