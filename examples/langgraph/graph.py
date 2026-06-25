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

"""Minimal LangGraph workflow that uses Actordock sandbox nodes."""

from __future__ import annotations

import os
from pathlib import Path
from typing import Literal
from typing import TypedDict

from e2b import Sandbox
from langgraph.graph import END, START, StateGraph

from support.python_template import SANDBOX_TEMPLATE_ENV, sandbox_template_name

RAW_ALERT_PATH = "/tmp/raw_alert.json"
NORMALIZED_ALERT_PATH = "/tmp/normalized_alert.json"
METRICS_PATH = "/tmp/metrics.json"
SUMMARY_PATH = "/tmp/incident-summary.txt"


def _default_sandbox_template() -> str:
    try:
        return sandbox_template_name()
    except KeyError as err:
        raise RuntimeError(
            f"missing {SANDBOX_TEMPLATE_ENV}; run ./hack/verify-examples.sh or set the env var"
        ) from err


class AlertState(TypedDict, total=False):
    raw_alert: str
    normalized: str
    metrics: str
    severity: str
    summary: str


def _normalize_alert_command() -> str:
    return r"""
python3 - <<'PY'
import json
from pathlib import Path

payload = json.loads(Path('/tmp/raw_alert.json').read_text())
normalized = {
    "incident_id": payload["incident_id"],
    "service": payload["service"],
    "severity": str(payload.get("severity", "unknown")).lower(),
    "raw_log_count": len(payload.get("raw_logs", [])),
}
Path('/tmp/normalized_alert.json').write_text(
    json.dumps(normalized, indent=2, sort_keys=True)
)
PY
""".strip()


def _analyze_normalized_command() -> str:
    return r"""
python3 - <<'PY'
import json
from pathlib import Path

normalized = json.loads(Path('/tmp/normalized_alert.json').read_text())
is_high = normalized.get("severity") == "high"
metrics = {
    "incident_id": normalized["incident_id"],
    "service": normalized["service"],
    "severity": normalized["severity"],
    "raw_log_count": normalized["raw_log_count"],
    "recommended_actions": ["notify-oncall", "collect-metrics", "open-runbook"] if is_high else ["collect-metrics"],
    "priority_score": 100 if is_high else 20,
}
Path('/tmp/metrics.json').write_text(
    json.dumps(metrics, indent=2, sort_keys=True)
)
PY
""".strip()


def _build_summary_command(*, path_label: str) -> str:
    is_high = path_label == "high"
    orchestration_path = "high_severity" if is_high else "standard"
    return (
        r"""
python3 - <<'PY'
import json
from pathlib import Path

metrics = json.loads(Path('/tmp/metrics.json').read_text())
summary = {
    "incident_id": metrics["incident_id"],
    "severity": metrics["severity"],
    "service": metrics["service"],
    "summary": (
        f"{metrics['service']} incident is {metrics['severity']} "
        f"with priority score {metrics['priority_score']}"
    ),
    "orchestration_path": "ORCHESTRATION_PATH",
    "recommended_actions": metrics["recommended_actions"],
}
Path('/tmp/incident-summary.txt').write_text(
    json.dumps(summary, indent=2, sort_keys=True)
)
PY
""".strip().replace("ORCHESTRATION_PATH", orchestration_path)
    )


def parse_node(template_name: str):
    def _node(state: AlertState) -> AlertState:
        sandbox = Sandbox.create(template=template_name, secure=False)
        try:
            sandbox.files.write(RAW_ALERT_PATH, state["raw_alert"])
            sandbox.commands.run(_normalize_alert_command())
            normalized = sandbox.files.read(NORMALIZED_ALERT_PATH)
            return {"normalized": normalized}
        finally:
            sandbox.kill()

    return _node


def analyze_node(template_name: str):
    def _node(state: AlertState) -> AlertState:
        sandbox = Sandbox.create(template=template_name, secure=False)
        try:
            sandbox.files.write(NORMALIZED_ALERT_PATH, state["normalized"])
            sandbox.commands.run(_analyze_normalized_command())
            metrics = sandbox.files.read(METRICS_PATH)
            return {"metrics": metrics, "severity": _severity_from_metrics(metrics)}
        finally:
            sandbox.kill()

    return _node


def summarize_node(template_name: str):
    def _node(state: AlertState) -> AlertState:
        sandbox = Sandbox.create(template=template_name, secure=False)
        try:
            sandbox.files.write(METRICS_PATH, state["metrics"])
            sandbox.commands.run(_build_summary_command(path_label="standard"))
            summary = sandbox.files.read(SUMMARY_PATH)
            return {"summary": summary}
        finally:
            sandbox.kill()

    return _node


def summarize_high_severity_node(template_name: str):
    def _node(state: AlertState) -> AlertState:
        sandbox = Sandbox.create(template=template_name, secure=False)
        try:
            sandbox.files.write(METRICS_PATH, state["metrics"])
            sandbox.commands.run(_build_summary_command(path_label="high"))
            summary = sandbox.files.read(SUMMARY_PATH)
            return {"summary": summary}
        finally:
            sandbox.kill()

    return _node


def _route_by_severity(state: AlertState) -> Literal["normal", "high"]:
    return "high" if state.get("severity") == "high" else "normal"


def _severity_from_metrics(metrics: str) -> str:
    import json

    try:
        parsed = json.loads(metrics)
        return str(parsed.get("severity", "low")).lower()
    except Exception:
        return "low"


def build_graph(template_name: str | None = None):
    if template_name is None:
        template_name = _default_sandbox_template()
    graph = StateGraph(AlertState)
    graph.add_node("parse", parse_node(template_name))
    graph.add_node("analyze", analyze_node(template_name))
    graph.add_node("summarize", summarize_node(template_name))
    graph.add_node("summarize_high_severity", summarize_high_severity_node(template_name))
    graph.add_edge(START, "parse")
    graph.add_edge("parse", "analyze")
    graph.add_conditional_edges(
        "analyze",
        _route_by_severity,
        {
            "normal": "summarize",
            "high": "summarize_high_severity",
        },
    )
    graph.add_edge("summarize", END)
    graph.add_edge("summarize_high_severity", END)
    return graph.compile()


def run_alert_graph(raw_alert: str, *, template_name: str | None = None) -> AlertState:
    return build_graph(template_name=template_name).invoke({"raw_alert": raw_alert})


def run_alert_graph_from_file(path: str, *, template_name: str | None = None) -> AlertState:
    return run_alert_graph(Path(path).read_text(), template_name=template_name)
