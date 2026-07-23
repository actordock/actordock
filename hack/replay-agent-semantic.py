#!/usr/bin/env python3
# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0
"""Mode A replay: drive agent-semantic@v2 against a live Actordock CP (no LLM).

Uses dataset arrivals + phase_spans + task_profile only:
  Create → POST L3 taskProfile → Resume → POST L1 phase/lock along spans → Suspend

Compares policies via /metrics deltas (mid_tool_suspend, starvation wait, resume_wait, …)
plus L3 cohort stats: eviction victims and client Resume RTT by eval.cohort.

Example:
  # Kind port-forward CP, then:
  ./hack/replay-agent-semantic.py \\
    --api http://127.0.0.1:18080 \\
    --policies random,resource-evict,semantic-score-l1,semantic-score \\
    --switch-policy \\
    --limit 12 --speed 60 --min-lock-sec 0.35

Ablation aliases: `semantic-score-l1` → PRIOR_MIX=0; `semantic-score` → PRIOR_MIX=0.3.

Concurrency: max_inflight defaults to workers+1 (slot contention); Resume/Suspend
are serialized to avoid concurrent checkpoint of the same sandbox.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_DATASET = ROOT / "docs" / "eval" / "datasets" / "agent-semantic@v2"
DEFAULT_OUT = ROOT / "docs" / "eval" / "results"

PROM_LINE = re.compile(
    r'^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{[^}]*\})?\s+([0-9.eE+-]+|NaN|Inf|-Inf)\s*$'
)
PROM_LABEL = re.compile(r'([a-zA-Z_][a-zA-Z0-9_]*)="((?:\\.|[^"\\])*)"')


@dataclass
class Sample:
    name: str
    labels: dict[str, str]
    value: float


@dataclass
class SessionMeta:
    session_id: str
    cohort: str
    complexity: float


@dataclass
class SessionJob:
    session: dict[str, Any]
    sandbox_id: str = ""
    error: str | None = None
    resumed: bool = False
    suspended: bool = False
    resume_sec: float = 0.0
    cohort: str = "unknown"


@dataclass
class PolicyResult:
    policy: str
    dataset: str
    session_count: int
    speed: float
    wall_sec: float
    sessions_ok: int
    sessions_failed: int
    mid_tool_suspend: float = 0.0
    suspend_total: float = 0.0
    mid_tool_rate: float = 0.0
    evict_tool_loop: float = 0.0
    evict_llm_wait: float = 0.0
    evict_idle: float = 0.0
    evict_unknown: float = 0.0
    starvation_enter: float = 0.0
    starvation_resolved: float = 0.0
    starvation_timeout: float = 0.0
    resume_wait_mean_s: float = 0.0
    resume_wait_n: float = 0.0
    resume_latency_mean_s: float = 0.0
    resume_latency_n: float = 0.0
    preempt_cost_mean_s: float = 0.0
    evictions_by_phase: dict[str, float] = field(default_factory=dict)
    # L3 proof: attributed eviction victims + client Resume RTT by cohort.
    victim_by_cohort: dict[str, float] = field(default_factory=dict)
    resume_sec_by_cohort: dict[str, float] = field(default_factory=dict)
    victim_l3_hard: float = 0.0
    victim_l3_easy: float = 0.0
    victim_l3_hard_rate: float = 0.0
    resume_sec_l3_hard: float = 0.0
    resume_sec_l3_easy: float = 0.0
    victim_complexity_mean: float = 0.0
    git_commit: str = ""
    errors: list[str] = field(default_factory=list)


class L3Tracker:
    """Map sandbox→session cohort; attribute Resume-time evictions to victim cohort."""

    def __init__(self) -> None:
        self._mu = threading.Lock()
        self.by_sandbox: dict[str, SessionMeta] = {}
        self.victim_by_cohort: dict[str, int] = defaultdict(int)
        self.resume_sum_by_cohort: dict[str, float] = defaultdict(float)
        self.resume_n_by_cohort: dict[str, int] = defaultdict(int)
        self.victim_complexity_sum = 0.0
        self.victim_n = 0

    def register(self, sandbox_id: str, session: dict[str, Any]) -> SessionMeta:
        meta = SessionMeta(
            session_id=str(session.get("session_id") or ""),
            cohort=session_cohort(session),
            complexity=session_complexity(session),
        )
        with self._mu:
            self.by_sandbox[sandbox_id] = meta
        return meta

    def resume_and_observe(self, client: CPClient, sandbox_id: str) -> float:
        """Resume under caller lock; detect running→suspended peers as eviction victims."""
        before = {
            str(sb.get("id")): str(sb.get("state") or "")
            for sb in client.list_sandboxes()
            if sb.get("id")
        }
        t0 = time.time()
        client.resume(sandbox_id)
        dt = time.time() - t0
        after = {
            str(sb.get("id")): str(sb.get("state") or "")
            for sb in client.list_sandboxes()
            if sb.get("id")
        }
        with self._mu:
            meta = self.by_sandbox.get(sandbox_id)
            cohort = meta.cohort if meta else "unknown"
            self.resume_sum_by_cohort[cohort] += dt
            self.resume_n_by_cohort[cohort] += 1
            for oid, st0 in before.items():
                if oid == sandbox_id:
                    continue
                if st0 == "running" and after.get(oid) == "suspended":
                    vm = self.by_sandbox.get(oid)
                    vc = vm.cohort if vm else "unknown"
                    self.victim_by_cohort[vc] += 1
                    if vm is not None:
                        self.victim_complexity_sum += vm.complexity
                        self.victim_n += 1
        return dt

    def snapshot(self) -> dict[str, Any]:
        with self._mu:
            victims = {k: float(v) for k, v in sorted(self.victim_by_cohort.items())}
            resume_means: dict[str, float] = {}
            for k in sorted(set(self.resume_sum_by_cohort) | set(self.resume_n_by_cohort)):
                n = self.resume_n_by_cohort.get(k, 0)
                resume_means[k] = (self.resume_sum_by_cohort[k] / n) if n else 0.0
            hard = float(self.victim_by_cohort.get("l3_hard", 0))
            easy = float(self.victim_by_cohort.get("l3_easy", 0))
            mid = float(self.victim_by_cohort.get("l3_mid", 0))
            inactive = float(self.victim_by_cohort.get("l3_inactive", 0))
            denom = hard + easy + mid + inactive
            return {
                "victim_by_cohort": victims,
                "resume_sec_by_cohort": resume_means,
                "victim_l3_hard": hard,
                "victim_l3_easy": easy,
                "victim_l3_hard_rate": (hard / denom) if denom > 0 else 0.0,
                "resume_sec_l3_hard": resume_means.get("l3_hard", 0.0),
                "resume_sec_l3_easy": resume_means.get("l3_easy", 0.0),
                "victim_complexity_mean": (
                    self.victim_complexity_sum / self.victim_n if self.victim_n else 0.0
                ),
            }


def session_cohort(session: dict[str, Any]) -> str:
    ev = session.get("eval") or {}
    return str(ev.get("cohort") or "unknown")


def session_complexity(session: dict[str, Any]) -> float:
    tp = session.get("task_profile") or {}
    try:
        return float(tp.get("complexitySignal") or 0.0)
    except (TypeError, ValueError):
        return 0.0


class CPClient:
    def __init__(
        self,
        api: str,
        *,
        timeout: float = 60.0,
        resume_timeout: float = 300.0,
    ) -> None:
        self.api = api.rstrip("/")
        self.timeout = timeout
        self.resume_timeout = resume_timeout

    def _req(
        self,
        method: str,
        path: str,
        body: dict[str, Any] | None = None,
        *,
        expect_json: bool = True,
        timeout: float | None = None,
    ) -> Any:
        data = None
        headers = {}
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(
            self.api + path, data=data, headers=headers, method=method
        )
        to = self.timeout if timeout is None else timeout
        try:
            with urllib.request.urlopen(req, timeout=to) as resp:
                raw = resp.read()
                code = resp.status
        except urllib.error.HTTPError as e:
            err_body = e.read().decode("utf-8", errors="replace")
            raise RuntimeError(f"{method} {path} -> {e.code}: {err_body[:300]}") from e
        except Exception as e:  # noqa: BLE001 — surface connect/timeout clearly
            raise RuntimeError(f"{method} {path} failed: {e}") from e
        if code == 204 or not raw:
            return None
        if not expect_json:
            return raw.decode("utf-8")
        return json.loads(raw.decode("utf-8"))

    def healthz(self) -> None:
        self._req("GET", "/healthz", expect_json=False, timeout=10)

    def wait_healthy(self, timeout_sec: float = 120.0) -> None:
        deadline = time.time() + timeout_sec
        last = ""
        while time.time() < deadline:
            try:
                self.healthz()
                return
            except RuntimeError as e:
                last = str(e)
                time.sleep(1)
        raise RuntimeError(f"API not healthy within {timeout_sec}s: {last}")

    def policy(self) -> str:
        out = self._req("GET", "/v1/policy", timeout=15)
        return str(out.get("policy") or "")

    def golden_ready(self) -> bool:
        try:
            out = self._req("GET", "/v1/golden", timeout=15)
        except RuntimeError:
            return False
        if not isinstance(out, dict):
            return False
        return bool(out.get("objectKey") or out.get("ObjectKey"))

    def ensure_golden(self) -> None:
        if self.golden_ready():
            print("[golden] already present", flush=True)
            return
        print("[golden] ensuring (may take 1–3 min)…", flush=True)
        deadline = time.time() + 180
        last = ""
        while time.time() < deadline:
            try:
                self._req("POST", "/v1/golden/ensure", timeout=90)
                if self.golden_ready():
                    print("[golden] ready", flush=True)
                    return
            except RuntimeError as e:
                last = str(e)
                print(f"[golden] retry: {last[:120]}", flush=True)
                time.sleep(2)
        raise RuntimeError(f"golden ensure failed: {last}")

    def list_workers(self) -> list[dict[str, Any]]:
        out = self._req("GET", "/v1/workers", timeout=15)
        return out if isinstance(out, list) else []

    def wait_workers(self, min_n: int, timeout_sec: float = 120.0) -> list[dict[str, Any]]:
        deadline = time.time() + timeout_sec
        while time.time() < deadline:
            workers = self.list_workers()
            healthy = [w for w in workers if w.get("healthy", True)]
            if len(healthy) >= min_n:
                return healthy
            time.sleep(1)
        raise RuntimeError(f"need >= {min_n} healthy workers")

    def list_sandboxes(self) -> list[dict[str, Any]]:
        out = self._req("GET", "/v1/sandboxes", timeout=30)
        return out if isinstance(out, list) else []

    def delete_sandbox(self, sid: str) -> None:
        try:
            self._req("DELETE", f"/v1/sandboxes/{sid}", expect_json=False, timeout=30)
        except RuntimeError as e:
            if "404" not in str(e):
                raise

    def cleanup_sandboxes(self) -> int:
        boxes = self.list_sandboxes()
        for sb in boxes:
            self.delete_sandbox(sb["id"])
        return len(boxes)

    def create(self) -> dict[str, Any]:
        return self._req("POST", "/v1/sandboxes", timeout=30)

    def resume(self, sid: str) -> dict[str, Any]:
        return self._req(
            "POST", f"/v1/sandboxes/{sid}/resume", timeout=self.resume_timeout
        )

    def suspend(self, sid: str) -> dict[str, Any]:
        return self._req("POST", f"/v1/sandboxes/{sid}/suspend", timeout=120)

    def post_semantic(self, sandbox_id: str, semantic: dict[str, Any]) -> None:
        self._req(
            "POST",
            "/v1/signals/semantic",
            {"sandboxID": sandbox_id, "semantic": semantic},
            expect_json=False,
            timeout=15,
        )

    def metrics_text(self) -> str:
        return str(self._req("GET", "/metrics", expect_json=False, timeout=30))


_print_lock = threading.Lock()


def log(msg: str) -> None:
    with _print_lock:
        print(msg, flush=True)


def parse_prom(body: str) -> list[Sample]:
    out: list[Sample] = []
    for line in body.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        m = PROM_LINE.match(line)
        if not m:
            continue
        labels: dict[str, str] = {}
        if m.group(2):
            for lm in PROM_LABEL.finditer(m.group(2)):
                labels[lm.group(1)] = lm.group(2).replace('\\"', '"')
        try:
            val = float(m.group(3))
        except ValueError:
            continue
        out.append(Sample(m.group(1), labels, val))
    return out


def sum_counter(samples: list[Sample], name: str, want: dict[str, str]) -> float:
    total = 0.0
    base = name.replace(".", "_")
    for s in samples:
        sn = s.name.replace(".", "_")
        if sn != base and sn != base + "_total":
            continue
        if any(s.labels.get(k) != v for k, v in want.items()):
            continue
        total += s.value
    return total


def hist_mean(samples: list[Sample], name: str, want: dict[str, str]) -> tuple[float, float]:
    base = name.replace(".", "_")
    sum_v = 0.0
    count_v = 0.0
    for s in samples:
        if any(s.labels.get(k) != v for k, v in want.items()):
            continue
        sn = s.name.replace(".", "_")
        if sn in (base + "_sum", base + "_seconds_sum"):
            sum_v += s.value
        elif sn in (base + "_count", base + "_seconds_count"):
            count_v += s.value
    if count_v <= 0:
        return 0.0, 0.0
    return sum_v / count_v, count_v


def counter_delta(before: list[Sample], after: list[Sample], name: str, want: dict[str, str]) -> float:
    return sum_counter(after, name, want) - sum_counter(before, name, want)


def hist_delta_mean(
    before: list[Sample], after: list[Sample], name: str, want: dict[str, str]
) -> tuple[float, float]:
    mean_a, n_a = hist_mean(after, name, want)
    # reconstruct sums from mean*n
    sum_b, n_b = hist_mean(before, name, want)
    # hist_mean returns mean; need sum = mean * n
    sum_after = mean_a * n_a
    sum_before = sum_b * n_b
    dn = n_a - n_b
    if dn <= 0:
        return 0.0, 0.0
    return (sum_after - sum_before) / dn, dn


def mid_tool_count(samples: list[Sample], policy: str) -> float:
    total = 0.0
    base = "actordock_schedule_eviction"
    for s in samples:
        sn = s.name.replace(".", "_")
        if sn != base and sn != base + "_total":
            continue
        if s.labels.get("policy") != policy:
            continue
        phase = s.labels.get("victim_phase", "")
        lock = s.labels.get("victim_lock", "")
        if phase == "tool_loop" or lock == "true":
            total += s.value
    return total


def eviction_by_phase(samples: list[Sample], policy: str) -> dict[str, float]:
    out: dict[str, float] = defaultdict(float)
    base = "actordock_schedule_eviction"
    for s in samples:
        sn = s.name.replace(".", "_")
        if sn != base and sn != base + "_total":
            continue
        if s.labels.get("policy") != policy:
            continue
        phase = s.labels.get("victim_phase", "unknown")
        out[phase] += s.value
    return dict(out)


def metrics_delta(policy: str, before_body: str, after_body: str) -> dict[str, Any]:
    before = parse_prom(before_body)
    after = parse_prom(after_body)
    pol = {"policy": policy}
    rw_mean, rw_n = hist_delta_mean(before, after, "actordock.sandbox.resume_wait", pol)
    rl_mean, rl_n = hist_delta_mean(before, after, "actordock.sandbox.resume_latency", pol)
    pc_mean, _ = hist_delta_mean(before, after, "actordock.sandbox.preempt_cost", pol)
    mid_b = mid_tool_count(before, policy)
    mid_a = mid_tool_count(after, policy)
    phases_b = eviction_by_phase(before, policy)
    phases_a = eviction_by_phase(after, policy)
    phases = {
        k: phases_a.get(k, 0.0) - phases_b.get(k, 0.0)
        for k in sorted(set(phases_a) | set(phases_b))
    }
    return {
        "mid_tool_suspend": mid_a - mid_b,
        "suspend_total": counter_delta(before, after, "actordock.schedule.eviction", pol),
        "starvation_enter": counter_delta(
            before, after, "actordock.schedule.semantic_starvation_wait", {**pol, "outcome": "enter"}
        ),
        "starvation_resolved": counter_delta(
            before,
            after,
            "actordock.schedule.semantic_starvation_wait",
            {**pol, "outcome": "resolved"},
        ),
        "starvation_timeout": counter_delta(
            before,
            after,
            "actordock.schedule.semantic_starvation_wait",
            {**pol, "outcome": "timeout"},
        ),
        "resume_wait_mean_s": rw_mean,
        "resume_wait_n": rw_n,
        "resume_latency_mean_s": rl_mean,
        "resume_latency_n": rl_n,
        "preempt_cost_mean_s": pc_mean,
        "evictions_by_phase": phases,
    }


def load_sessions(path: Path, limit: int) -> list[dict[str, Any]]:
    rows = [json.loads(l) for l in path.read_text().splitlines() if l.strip()]
    rows.sort(key=lambda r: float(r["arrival_ts"]))
    if limit > 0:
        rows = rows[:limit]
    return rows


def normalize_task_profile(tp: dict[str, Any]) -> dict[str, Any]:
    """Keep CP-relevant keys; drop debug-only extras if present."""
    keep = (
        "version",
        "complexitySignal",
        "domain",
        "embeddingSim",
        "confidence",
        "modelID",
        "scoredAt",
        "difficultyTier",
    )
    out = {k: tp[k] for k in keep if k in tp and tp[k] is not None}
    return out


def kubectl_cmd(*args: str) -> list[str]:
    cmd = ["kubectl"]
    cluster = os.environ.get("KIND_CLUSTER_NAME", "").strip()
    if cluster:
        cmd.extend(["--context", f"kind-{cluster}"])
    cmd.extend(args)
    return cmd


def parse_api_port(api: str) -> int:
    """Extract local port from http://host:port (default 8080)."""
    from urllib.parse import urlparse

    u = urlparse(api)
    if u.port:
        return int(u.port)
    return 8080


_pf_proc: subprocess.Popen | None = None


def restart_port_forward(api: str, namespace: str) -> None:
    """Restart kubectl port-forward to svc/controlplane for --api local port."""
    global _pf_proc
    port = parse_api_port(api)
    log(f"[pf] restart port-forward :{port} -> svc/controlplane")
    subprocess.run(
        ["pkill", "-f", f"port-forward.*{port}:8080"],
        check=False,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    if _pf_proc is not None and _pf_proc.poll() is None:
        _pf_proc.terminate()
        try:
            _pf_proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            _pf_proc.kill()
    _pf_proc = subprocess.Popen(
        kubectl_cmd(
            "-n",
            namespace,
            "port-forward",
            "svc/controlplane",
            f"{port}:8080",
        ),
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    time.sleep(1.5)


def set_policy_kubectl(
    name: str,
    namespace: str,
    api: str,
    *,
    extra_env: dict[str, str] | None = None,
) -> None:
    env_args = [f"POLICY={name}"]
    if extra_env:
        for k, v in sorted(extra_env.items()):
            env_args.append(f"{k}={v}")
    log(f"[policy] kubectl set env {' '.join(env_args)} (ns={namespace})")
    subprocess.run(
        kubectl_cmd("-n", namespace, "set", "env", "deployment/controlplane", *env_args),
        check=True,
    )
    subprocess.run(
        kubectl_cmd(
            "-n",
            namespace,
            "rollout",
            "status",
            "deployment/controlplane",
            "--timeout=180s",
        ),
        check=True,
    )
    log("[policy] rollout done; restarting port-forward…")
    restart_port_forward(api, namespace)


@dataclass(frozen=True)
class PolicySpec:
    """Replay label vs CP POLICY (+ optional ablation env)."""

    label: str
    cp_policy: str
    extra_env: dict[str, str] = field(default_factory=dict)


def resolve_policy_spec(spec: str) -> PolicySpec:
    """Map CI/replay tokens to CP policy + env.

    Ablation aliases:
      semantic-score-l1  → POLICY=semantic-score SEMANTIC_PRIOR_MIX=0  (L1 lock only)
      semantic-score     → POLICY=semantic-score SEMANTIC_PRIOR_MIX=0.3 (L1+L3)
    """
    s = spec.strip()
    key = s.lower().replace("_", "-")
    if key in ("semantic-score-l1", "semantic-score:l1", "semantic-l1"):
        return PolicySpec(
            label="semantic-score-l1",
            cp_policy="semantic-score",
            extra_env={"SEMANTIC_PRIOR_MIX": "0"},
        )
    if key in ("semantic-score", "semantic-score-full", "semantic-score:full"):
        return PolicySpec(
            label="semantic-score",
            cp_policy="semantic-score",
            extra_env={"SEMANTIC_PRIOR_MIX": "0.3"},
        )
    return PolicySpec(label=s, cp_policy=s, extra_env={})



def wait_policy(client: CPClient, name: str, timeout_sec: float = 120.0) -> None:
    log(f"[policy] waiting for POLICY={name} (and API healthy)…")
    client.wait_healthy(timeout_sec=timeout_sec)
    deadline = time.time() + timeout_sec
    while time.time() < deadline:
        try:
            cur = client.policy()
        except RuntimeError as e:
            log(f"[policy] API transient: {e}")
            time.sleep(1)
            continue
        if cur == name:
            log(f"[policy] active={cur}")
            return
        time.sleep(1)
    raise SystemExit(f"policy still {client.policy()!r}, want {name!r}")


def git_commit() -> str:
    try:
        return (
            subprocess.check_output(["git", "-C", str(ROOT), "rev-parse", "--short", "HEAD"])
            .decode()
            .strip()
        )
    except (subprocess.CalledProcessError, FileNotFoundError):
        return ""


def drive_phases(
    client: CPClient,
    sandbox_id: str,
    spans: list[dict[str, Any]],
    speed: float,
    workflow_id: str,
    min_lock_sec: float,
    resume_lock: threading.Lock,
) -> None:
    """Walk phase_spans in order; POST L1; Suspend when done to free the Worker."""
    if not spans:
        spans = [{"phase": "idle", "lock": False, "t_start_ms": 0, "t_end_ms": 500}]
    for span in spans:
        phase = span.get("phase") or "idle"
        lock = bool(span.get("lock"))
        t0 = float(span.get("t_start_ms") or 0)
        t1 = float(span.get("t_end_ms") or t0)
        dur = max(0.0, (t1 - t0) / 1000.0 / max(speed, 1e-6))
        # Keep tool_loop/lock visible long enough for concurrent Resume eviction.
        if lock or phase == "tool_loop":
            dur = max(dur, min_lock_sec)
        client.post_semantic(
            sandbox_id,
            {
                "version": "v1",
                "phase": phase,
                "lock": lock,
                "workflowID": workflow_id,
            },
        )
        if dur > 0:
            time.sleep(dur)
    # Serialize Suspend with Resume so we never checkpoint the same id concurrently.
    with resume_lock:
        try:
            client.suspend(sandbox_id)
        except RuntimeError as e:
            msg = str(e).lower()
            if "suspended" in msg or "want running" in msg or "want suspended" in msg:
                return
            raise


def run_one_session(
    client: CPClient,
    session: dict[str, Any],
    wall0: float,
    base_arrival: float,
    speed: float,
    deadline_sec: float,
    min_lock_sec: float,
    index: int,
    total: int,
    inflight: threading.Semaphore,
    resume_lock: threading.Lock,
    l3: L3Tracker,
) -> SessionJob:
    job = SessionJob(session=session, cohort=session_cohort(session))
    sid_label = session["session_id"]
    held = False
    try:
        target = wall0 + (float(session["arrival_ts"]) - base_arrival) / max(speed, 1e-6)
        delay = target - time.time()
        if delay > 0:
            time.sleep(delay)

        # Slot contention: at most workers+1 sessions in create→resume→phases→suspend.
        inflight.acquire()
        held = True

        log(f"[session {index+1}/{total}] create {sid_label} cohort={job.cohort}")
        sb = client.create()
        job.sandbox_id = sb["id"]
        l3.register(job.sandbox_id, session)
        profile = normalize_task_profile(session.get("task_profile") or {})
        deadline = datetime.now(timezone.utc).timestamp() + deadline_sec
        deadline_iso = datetime.fromtimestamp(deadline, tz=timezone.utc).strftime(
            "%Y-%m-%dT%H:%M:%S.%fZ"
        )
        client.post_semantic(
            job.sandbox_id,
            {
                "version": "v1",
                "phase": "llm_wait",
                "lock": False,
                "deadline": deadline_iso,
                "workflowID": sid_label,
                "taskProfile": profile,
            },
        )
        log(f"[session {index+1}/{total}] resume {job.sandbox_id[:8]}…")
        with resume_lock:
            job.resume_sec = l3.resume_and_observe(client, job.sandbox_id)
        job.resumed = True
        drive_phases(
            client,
            job.sandbox_id,
            session.get("phase_spans") or [],
            speed,
            sid_label,
            min_lock_sec,
            resume_lock,
        )
        job.suspended = True
        log(f"[session {index+1}/{total}] done {sid_label}")
    except Exception as e:  # noqa: BLE001 — per-session errors collected in report
        job.error = f"{sid_label}: {e}"
        log(f"[session] FAIL {job.error}")
        if job.sandbox_id:
            with resume_lock:
                try:
                    client.suspend(job.sandbox_id)
                except RuntimeError:
                    pass
    finally:
        if held:
            inflight.release()
    return job


def run_policy(
    client: CPClient,
    policy: str,
    sessions: list[dict[str, Any]],
    *,
    speed: float,
    deadline_sec: float,
    dataset_id: str,
    worker_count: int,
    max_inflight: int,
    min_lock_sec: float,
    label: str | None = None,
) -> PolicyResult:
    """Run one policy. `policy` is CP /metrics label; `label` is report row name."""
    report_label = label or policy
    log(f"[{report_label}] cleanup…")
    n = client.cleanup_sandboxes()
    log(f"[{report_label}] cleaned {n} sandbox(es)")
    client.ensure_golden()

    log(f"[{report_label}] scrape /metrics (before)")
    before = client.metrics_text()
    wall0 = time.time()
    base_arrival = float(sessions[0]["arrival_ts"])
    jobs: list[SessionJob] = []
    total = len(sessions)
    l3 = L3Tracker()

    # workers+1 keeps one waiter → Place/Evict contention, without Resume stampede.
    if max_inflight <= 0:
        max_inflight = max(2, worker_count + 1)
    inflight = threading.Semaphore(max_inflight)
    resume_lock = threading.Lock()
    # Pool large enough that arrivals can block on the semaphore without starving.
    pool_n = min(total, max(max_inflight * 2, 8))
    log(
        f"[{report_label}] replaying n={total} workers={worker_count} "
        f"inflight={max_inflight} pool={pool_n} speed={speed} min_lock={min_lock_sec}s "
        f"(resume serialized; metrics_policy={policy})"
    )

    with ThreadPoolExecutor(max_workers=pool_n) as pool:
        futs = []
        for i, session in enumerate(sessions):
            target = wall0 + (float(session["arrival_ts"]) - base_arrival) / max(speed, 1e-6)
            delay = target - time.time()
            if delay > 0:
                time.sleep(delay)
            futs.append(
                pool.submit(
                    run_one_session,
                    client,
                    session,
                    wall0,
                    base_arrival,
                    speed,
                    deadline_sec,
                    min_lock_sec,
                    i,
                    total,
                    inflight,
                    resume_lock,
                    l3,
                )
            )
        done = 0
        for fut in as_completed(futs):
            jobs.append(fut.result())
            done += 1
            if done % 2 == 0 or done == total:
                log(f"[{report_label}] progress {done}/{total}")

    wall_sec = time.time() - wall0
    log(f"[{report_label}] scrape /metrics (after) wall={wall_sec:.1f}s")
    after = client.metrics_text()
    delta = metrics_delta(policy, before, after)
    l3_stats = l3.snapshot()

    errors = [j.error for j in jobs if j.error]
    phases = {k: float(v) for k, v in delta["evictions_by_phase"].items()}
    suspend_total = float(delta["suspend_total"])
    mid_tool = float(delta["mid_tool_suspend"])
    result = PolicyResult(
        policy=report_label,
        dataset=dataset_id,
        session_count=len(sessions),
        speed=speed,
        wall_sec=wall_sec,
        sessions_ok=sum(1 for j in jobs if not j.error),
        sessions_failed=len(errors),
        mid_tool_suspend=mid_tool,
        suspend_total=suspend_total,
        mid_tool_rate=(mid_tool / suspend_total) if suspend_total > 0 else 0.0,
        evict_tool_loop=float(phases.get("tool_loop", 0.0)),
        evict_llm_wait=float(phases.get("llm_wait", 0.0)),
        evict_idle=float(phases.get("idle", 0.0)),
        evict_unknown=float(phases.get("unknown", 0.0)),
        starvation_enter=float(delta["starvation_enter"]),
        starvation_resolved=float(delta["starvation_resolved"]),
        starvation_timeout=float(delta["starvation_timeout"]),
        resume_wait_mean_s=float(delta["resume_wait_mean_s"]),
        resume_wait_n=float(delta["resume_wait_n"]),
        resume_latency_mean_s=float(delta["resume_latency_mean_s"]),
        resume_latency_n=float(delta["resume_latency_n"]),
        preempt_cost_mean_s=float(delta["preempt_cost_mean_s"]),
        evictions_by_phase=phases,
        victim_by_cohort=l3_stats["victim_by_cohort"],
        resume_sec_by_cohort=l3_stats["resume_sec_by_cohort"],
        victim_l3_hard=l3_stats["victim_l3_hard"],
        victim_l3_easy=l3_stats["victim_l3_easy"],
        victim_l3_hard_rate=l3_stats["victim_l3_hard_rate"],
        resume_sec_l3_hard=l3_stats["resume_sec_l3_hard"],
        resume_sec_l3_easy=l3_stats["resume_sec_l3_easy"],
        victim_complexity_mean=l3_stats["victim_complexity_mean"],
        git_commit=git_commit(),
        errors=errors[:20],
    )
    log(
        f"[{report_label}] L3 victims={result.victim_by_cohort} "
        f"hard_rate={result.victim_l3_hard_rate:.2f} "
        f"victim_cx={result.victim_complexity_mean:.3f}"
    )
    log(f"[{report_label}] final cleanup…")
    client.cleanup_sandboxes()
    return result


def write_compare(results: list[PolicyResult], path: Path) -> None:
    lines = [
        "# agent-semantic policy compare",
        "",
        f"dataset: `{results[0].dataset}`  ",
        f"sessions: {results[0].session_count}  speed: {results[0].speed}x  ",
        f"git: `{results[0].git_commit or 'n/a'}`",
        "",
        "| policy | mid_tool | mid_tool_rate | suspend | evict_tool_loop | evict_llm_wait | starve_enter | resume_wait_s | resume_lat_s | preempt_s | vic_hard | vic_easy | hard_rate | resume_hard_s | resume_easy_s | victim_cx | ok/fail | wall_s |",
        "|--------|---------:|--------------:|--------:|----------------:|---------------:|-------------:|--------------:|-------------:|----------:|---------:|---------:|----------:|--------------:|--------------:|----------:|--------:|-------:|",
    ]
    for r in results:
        lines.append(
            f"| {r.policy} | {r.mid_tool_suspend:.0f} | {r.mid_tool_rate:.2f} | "
            f"{r.suspend_total:.0f} | {r.evict_tool_loop:.0f} | {r.evict_llm_wait:.0f} | "
            f"{r.starvation_enter:.0f} | {r.resume_wait_mean_s:.3f} | "
            f"{r.resume_latency_mean_s:.3f} | {r.preempt_cost_mean_s:.3f} | "
            f"{r.victim_l3_hard:.0f} | {r.victim_l3_easy:.0f} | {r.victim_l3_hard_rate:.2f} | "
            f"{r.resume_sec_l3_hard:.3f} | {r.resume_sec_l3_easy:.3f} | "
            f"{r.victim_complexity_mean:.3f} | "
            f"{r.sessions_ok}/{r.sessions_failed} | {r.wall_sec:.1f} |"
        )
    lines.extend(
        [
            "",
            "### victim_by_cohort / resume_sec_by_cohort",
            "",
        ]
    )
    for r in results:
        lines.append(
            f"- **{r.policy}**: victims=`{r.victim_by_cohort}` "
            f"resume_sec=`{ {k: round(v, 3) for k, v in r.resume_sec_by_cohort.items()} }`"
        )
    lines.extend(
        [
            "",
            "Notes:",
            "- `mid_tool` / `evict_tool_loop`: Suspend while victim was in tool_loop (or lock).",
            "- `evict_llm_wait`: preferred interrupt window for semantic-score.",
            "- `mid_tool_rate = mid_tool / suspend` (0 if no suspends).",
            "- `vic_hard` / `vic_easy` / `hard_rate`: eviction victims attributed via "
              "running→suspended peers at Resume time (excludes voluntary end Suspend).",
            "- `hard_rate = vic_hard / (hard+mid+easy+inactive)`; lower favors semantic-score L3.",
            "- `resume_hard_s` / `resume_easy_s`: mean client Resume RTT for that cohort "
              "(proxy for wait+restore; not OTel resume_wait).",
            "- `victim_cx`: mean complexitySignal of attributed victims (lower favors L3).",
            "- Ablation: `semantic-score-l1` (PRIOR_MIX=0) vs `semantic-score` (PRIOR_MIX=0.3); "
              "L3 claim needs hard_rate/victim_cx/resume_hard_s better on full than l1.",
            "- Lower mid_tool / hard_rate / resume_hard_s generally favor semantic-score.",
            "",
        ]
    )
    path.write_text("\n".join(lines) + "\n")


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--api", default=os.environ.get("ACTORDOCK_API", "http://127.0.0.1:8080"))
    ap.add_argument(
        "--dataset",
        type=Path,
        default=DEFAULT_DATASET,
        help="Package dir with sessions.jsonl",
    )
    ap.add_argument(
        "--policies",
        default="random,resource-evict,semantic-score-l1,semantic-score",
        help="Comma list. Aliases: semantic-score-l1 (L1 only), semantic-score (L1+L3)",
    )
    ap.add_argument(
        "--switch-policy",
        action="store_true",
        help="kubectl set env POLICY=… (+ SEMANTIC_PRIOR_MIX for ablation), restart PF",
    )
    ap.add_argument("--namespace", default=os.environ.get("ACTORDOCK_NAMESPACE", "actordock"))
    ap.add_argument("--limit", type=int, default=0, help="First N sessions (0=all)")
    ap.add_argument("--speed", type=float, default=60.0, help="Compress arrivals+spans by this factor")
    ap.add_argument(
        "--min-lock-sec",
        type=float,
        default=0.35,
        help="Minimum hold for tool_loop/lock spans so eviction can observe L1",
    )
    ap.add_argument("--deadline-sec", type=float, default=600.0)
    ap.add_argument("--min-workers", type=int, default=2)
    ap.add_argument(
        "--max-inflight",
        type=int,
        default=0,
        help="Max concurrent create→resume→phases (0 = workers+1; keeps slot contention)",
    )
    ap.add_argument("--resume-timeout", type=float, default=300.0)
    ap.add_argument("--out", type=Path, default=DEFAULT_OUT)
    ap.add_argument("--dry-run", action="store_true", help="Load dataset only")
    args = ap.parse_args()

    sessions_path = args.dataset / "sessions.jsonl"
    if not sessions_path.exists():
        raise SystemExit(f"missing {sessions_path}")
    sessions = load_sessions(sessions_path, args.limit)
    if not sessions:
        raise SystemExit("no sessions")
    dataset_id = args.dataset.name
    specs = [resolve_policy_spec(p) for p in args.policies.split(",") if p.strip()]
    log(
        f"[replay] dataset={dataset_id} n={len(sessions)} "
        f"policies={[s.label for s in specs]} speed={args.speed} api={args.api}"
    )
    if args.dry_run:
        span = float(sessions[-1]["arrival_ts"]) - float(sessions[0]["arrival_ts"])
        log(f"[dry-run] arrival_span_sec={span:.1f} wall≈{span / args.speed:.1f}s")
        return

    client = CPClient(args.api, resume_timeout=args.resume_timeout)
    client.wait_healthy(timeout_sec=30)
    workers = client.wait_workers(args.min_workers)
    worker_count = len(workers)
    log(f"[replay] healthy_workers={worker_count}")

    args.out.mkdir(parents=True, exist_ok=True)
    results: list[PolicyResult] = []
    for spec in specs:
        try:
            cur = client.policy()
        except RuntimeError:
            client.wait_healthy(timeout_sec=60)
            cur = client.policy()
        # Always apply when switching: l1↔full share POLICY=semantic-score but differ PRIOR_MIX.
        if args.switch_policy:
            set_policy_kubectl(
                spec.cp_policy,
                args.namespace,
                args.api,
                extra_env=spec.extra_env or None,
            )
            wait_policy(client, spec.cp_policy)
        elif cur != spec.cp_policy:
            raise SystemExit(
                f"cluster policy={cur!r}, want {spec.cp_policy!r} (label={spec.label}); "
                "re-run with --switch-policy or set a single matching --policies"
            )
        else:
            log(f"[policy] already {spec.cp_policy} (label={spec.label}; env not changed)")

        log(f"[replay] === {spec.label} (cp={spec.cp_policy} env={spec.extra_env or {}}) ===")
        result = run_policy(
            client,
            spec.cp_policy,
            sessions,
            speed=args.speed,
            deadline_sec=args.deadline_sec,
            dataset_id=dataset_id,
            worker_count=worker_count,
            max_inflight=args.max_inflight,
            min_lock_sec=args.min_lock_sec,
            label=spec.label,
        )
        results.append(result)
        out_json = args.out / f"agent_semantic_v2__{spec.label}.json"
        out_json.write_text(json.dumps(result.__dict__, indent=2) + "\n")
        log(
            f"[replay] {spec.label}: mid_tool={result.mid_tool_suspend:.0f} "
            f"suspend={result.suspend_total:.0f} hard_rate={result.victim_l3_hard_rate:.2f} "
            f"vic_hard={result.victim_l3_hard:.0f} vic_easy={result.victim_l3_easy:.0f} "
            f"ok={result.sessions_ok} fail={result.sessions_failed} → {out_json}"
        )
        if result.sessions_failed and not result.sessions_ok:
            log(f"[replay] WARN: all sessions failed under {spec.label}; check CP/worker logs")

    if not results:
        raise SystemExit("no policy results")
    compare = args.out / "policy_compare_agent_semantic_v2.md"
    write_compare(results, compare)
    log(f"[done] {compare}")
    sys.stdout.write(compare.read_text())


if __name__ == "__main__":
    main()
