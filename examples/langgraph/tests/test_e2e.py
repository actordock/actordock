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

import json
import os
from pathlib import Path

import httpx
import pytest

from graph import run_alert_graph


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


def _sample_alert() -> str:
    path = Path(__file__).resolve().parents[1] / "data" / "sample_alert.json"
    return path.read_text()


def _alert_payload(*, severity: str) -> str:
    payload = json.loads(_sample_alert())
    payload["severity"] = severity
    return json.dumps(payload)


def _expected_normalized() -> str:
    return json.dumps(
        {
            "incident_id": "INC-2026-1001",
            "raw_log_count": 3,
            "service": "checkout",
            "severity": "high",
        },
        indent=2,
        sort_keys=True,
    )


def _expected_metrics() -> str:
    return json.dumps(
        {
            "incident_id": "INC-2026-1001",
            "priority_score": 100,
            "raw_log_count": 3,
            "recommended_actions": ["notify-oncall", "collect-metrics", "open-runbook"],
            "service": "checkout",
            "severity": "high",
        },
        indent=2,
        sort_keys=True,
    )


@pytest.mark.timeout(180)
def test_langgraph_pipeline_round_trip_and_cleanup() -> None:
    before = _list_sandbox_ids()
    result = run_alert_graph(_alert_payload(severity="high"))

    assert "severity" in result
    assert '"severity": "high"' in result["summary"]
    assert '"orchestration_path": "high_severity"' in result["summary"]
    assert result["summary"]
    assert _expected_normalized() in result["normalized"]
    assert _expected_metrics() in result["metrics"]

    after = _list_sandbox_ids()
    assert not (after - before), "langgraph pipeline left sandbox IDs behind"


@pytest.mark.timeout(180)
def test_langgraph_standard_path_for_low_severity() -> None:
    before = _list_sandbox_ids()
    result = run_alert_graph(_alert_payload(severity="low"))

    assert '"severity": "low"' in result["summary"]
    assert '"orchestration_path": "standard"' in result["summary"]
    assert result["summary"]

    after = _list_sandbox_ids()
    assert not (after - before), "langgraph pipeline left sandbox IDs behind"
