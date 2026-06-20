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

"""E2B observability routes: metrics, logs, refreshes (v0.0.6)."""

from __future__ import annotations

import os
import time
from datetime import datetime, timezone

import httpx
from e2b import Sandbox

METRIC_KEYS = (
    "timestamp",
    "timestampUnix",
    "cpuCount",
    "cpuUsedPct",
    "memUsed",
    "memTotal",
    "memCache",
    "diskUsed",
    "diskTotal",
)

UNKNOWN_SANDBOX_ID = "00000000-0000-0000-0000-000000000000"


def _api_url() -> str:
    return os.environ["E2B_API_URL"].rstrip("/")


def _api_headers() -> dict[str, str]:
    return {
        "X-API-KEY": os.environ["E2B_API_KEY"],
        "Content-Type": "application/json",
    }


def _seconds_until(end_at: datetime) -> float:
    now = datetime.now(timezone.utc)
    if end_at.tzinfo is None:
        end_at = end_at.replace(tzinfo=timezone.utc)
    return (end_at - now).total_seconds()


def test_list_sandbox_metrics_returns_expected_keys() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=120)
    try:
        resp = httpx.get(
            f"{_api_url()}/sandboxes/metrics",
            params={"sandbox_ids": sbx.sandbox_id},
            headers={"X-API-KEY": os.environ["E2B_API_KEY"]},
            timeout=30.0,
        )
        assert resp.status_code == 200
        body = resp.json()
        assert "sandboxes" in body
        metric = body["sandboxes"][sbx.sandbox_id]
        for key in METRIC_KEYS:
            assert key in metric, f"missing metric key {key!r}"
    finally:
        sbx.kill()


def test_per_sandbox_metrics_returns_200() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=120)
    try:
        resp = httpx.get(
            f"{_api_url()}/sandboxes/{sbx.sandbox_id}/metrics",
            headers={"X-API-KEY": os.environ["E2B_API_KEY"]},
            timeout=30.0,
        )
        assert resp.status_code == 200
        assert isinstance(resp.json(), list)
    finally:
        sbx.kill()


def test_sandbox_logs_v1_returns_expected_keys() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=120)
    try:
        sbx.commands.run("echo hello")
        resp = httpx.get(
            f"{_api_url()}/sandboxes/{sbx.sandbox_id}/logs",
            params={"start": 0, "limit": 100},
            headers={"X-API-KEY": os.environ["E2B_API_KEY"]},
            timeout=30.0,
        )
        assert resp.status_code == 200
        body = resp.json()
        assert "logs" in body
        assert "logEntries" in body
        assert isinstance(body["logs"], list)
        assert isinstance(body["logEntries"], list)
        lines = [e.get("line", "") for e in body["logs"]]
        messages = [e.get("message", "") for e in body["logEntries"]]
        assert any("hello" in text for text in lines + messages)
    finally:
        sbx.kill()


def test_sandbox_logs_v2_returns_expected_keys() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=120)
    try:
        sbx.commands.run("echo hello")
        resp = httpx.get(
            f"{_api_url()}/v2/sandboxes/{sbx.sandbox_id}/logs",
            params={"cursor": 0, "limit": 50, "direction": "forward", "level": "info"},
            headers={"X-API-KEY": os.environ["E2B_API_KEY"]},
            timeout=30.0,
        )
        assert resp.status_code == 200
        body = resp.json()
        assert "logs" in body
        assert isinstance(body["logs"], list)
        assert any("hello" in e.get("message", "") for e in body["logs"])
        assert any(e.get("level") == "info" for e in body["logs"])
    finally:
        sbx.kill()


def test_refresh_extends_end_at() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=60)
    try:
        before = sbx.get_info().end_at
        time.sleep(1)
        resp = httpx.post(
            f"{_api_url()}/sandboxes/{sbx.sandbox_id}/refreshes",
            headers=_api_headers(),
            json={"duration": 120},
            timeout=30.0,
        )
        assert resp.status_code == 204
        after = sbx.get_info().end_at
        remaining = _seconds_until(after)
        assert after > before
        assert 115 <= remaining <= 125, f"expected ~120s remaining, got {remaining:.1f}s"
    finally:
        sbx.kill()


def test_unknown_sandbox_observability_routes_return_404() -> None:
    headers = _api_headers()
    api = _api_url()
    sid = UNKNOWN_SANDBOX_ID

    cases = [
        ("GET", f"{api}/sandboxes/{sid}/metrics", None),
        ("GET", f"{api}/sandboxes/{sid}/logs", None),
        ("GET", f"{api}/v2/sandboxes/{sid}/logs", None),
        ("POST", f"{api}/sandboxes/{sid}/refreshes", {"duration": 60}),
    ]
    for method, url, body in cases:
        resp = httpx.request(method, url, headers=headers, json=body, timeout=30.0)
        assert resp.status_code == 404, f"{method} {url}: status={resp.status_code}"
