#!/usr/bin/env python3
# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0
"""Build agent-semantic@v2 from a single agent-trajectory source: APB BFCL.

Sources (public, not synthesized):
  - LulaCola/AgentProcessBench / bfcl/test.jsonl
  - Azure Functions 2019 invocations_per_function_md.anon.d01.csv (arrival spacing)

Design:
  - BFCL only (tool-dense, CS-friendly for L3 complexitySignal)
  - Keep up to 5 trajectories per query (sample_index 0..4) for path diversity
  - Prefer l3_active (confidence≥0.3), pad to --target with tool_dense / others
  - Tag cohorts l3_hard/mid/easy; wave arrivals for contention

Contract: docs/eval/agent-semantic-workload.md
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
import os
import sys
import urllib.request
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
CACHE = ROOT / ".cache" / "agent-semantic"
SUBSET = "bfcl"
APB_URL = (
    "https://huggingface.co/datasets/LulaCola/AgentProcessBench/"
    f"resolve/main/{SUBSET}/test.jsonl"
)
AZURE_CSV = "invocations_per_function_md.anon.d01.csv"
AZURE_DAY_EPOCH = 1561939200  # 2019-07-01T00:00:00Z
AZURE_DAY = "01"

LLM_WAIT_MS = 800
TOOL_LOOP_MS = 1125
IDLE_TAIL_MS = 500

SCHEMA = "agent-semantic.session.v2"
CONF_ACTIVE_DEFAULT = 0.3
WAVE_SIZE = 4
IN_WAVE_GAP_SEC = 5.0


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def sha256_text(s: str) -> str:
    return hashlib.sha256(s.encode("utf-8")).hexdigest()


def args_digest(arguments: str | dict[str, Any] | None) -> str:
    if arguments is None:
        raw = ""
    elif isinstance(arguments, dict):
        raw = json.dumps(arguments, sort_keys=True, ensure_ascii=False)
    else:
        raw = str(arguments)
    return hashlib.sha256(raw.encode("utf-8")).hexdigest()[:16]


def download(url: str, dest: Path) -> None:
    dest.parent.mkdir(parents=True, exist_ok=True)
    if dest.exists() and dest.stat().st_size > 0:
        print(f"[cache] {dest}")
        return
    print(f"[download] {url} -> {dest}")
    req = urllib.request.Request(url, headers={"User-Agent": "actordock-eval/1.0"})
    with urllib.request.urlopen(req, timeout=600) as resp, dest.open("wb") as out:
        while True:
            chunk = resp.read(1 << 20)
            if not chunk:
                break
            out.write(chunk)


def ensure_azure_csv(cache_dir: Path) -> Path:
    csv_path = cache_dir / "azure2019" / AZURE_CSV
    if csv_path.exists():
        return csv_path
    tar_path = cache_dir / "azure2019" / "azurefunctions-dataset2019.tar.xz"
    if not tar_path.exists():
        raise SystemExit(
            f"Missing Azure CSV at {csv_path}. Place {AZURE_CSV} or the Azure tar.xz "
            f"under {cache_dir / 'azure2019'}/"
        )
    import tarfile

    print(f"[extract] {tar_path}")
    with tarfile.open(tar_path, "r:xz") as tf:
        member = None
        for m in tf.getmembers():
            if m.name.endswith(AZURE_CSV):
                member = m
                break
        if member is None:
            raise SystemExit(f"{AZURE_CSV} not found in {tar_path}")
        member.name = AZURE_CSV
        tf.extract(member, path=csv_path.parent)
    return csv_path


def azure_busy_minute_starts(csv_path: Path) -> list[int]:
    totals = [0] * 1440
    with csv_path.open(newline="") as f:
        reader = csv.reader(f)
        header = next(reader)
        if len(header[4:]) != 1440:
            raise SystemExit(f"expected 1440 minute columns, got {len(header[4:])}")
        for row in reader:
            if len(row) < 4 + 1440:
                continue
            for mi, cell in enumerate(row[4 : 4 + 1440]):
                try:
                    c = int(cell)
                except ValueError:
                    c = 0
                if c > 0:
                    totals[mi] += c
    return [mi for mi, t in enumerate(totals) if t > 0]


def extract_tool_trace(messages: list[dict[str, Any]]) -> list[dict[str, Any]]:
    trace: list[dict[str, Any]] = []
    t = 0
    for msg in messages:
        if msg.get("role") != "assistant":
            continue
        tcs = msg.get("tool_calls") or []
        if not tcs:
            continue
        t += LLM_WAIT_MS
        for tc in tcs:
            fn = (tc.get("function") or {}) if isinstance(tc, dict) else {}
            trace.append(
                {
                    "name": fn.get("name") or "unknown",
                    "args_digest": args_digest(fn.get("arguments")),
                    "t_rel_ms": t,
                }
            )
            t += TOOL_LOOP_MS
    return trace


def derive_phase_spans(messages: list[dict[str, Any]]) -> list[dict[str, Any]]:
    spans: list[dict[str, Any]] = []
    t = 0
    for msg in messages:
        if msg.get("role") != "assistant":
            continue
        tcs = msg.get("tool_calls") or []
        if tcs:
            spans.append(
                {"phase": "llm_wait", "lock": False, "t_start_ms": t, "t_end_ms": t + LLM_WAIT_MS}
            )
            t += LLM_WAIT_MS
            for _ in tcs:
                spans.append(
                    {
                        "phase": "tool_loop",
                        "lock": True,
                        "t_start_ms": t,
                        "t_end_ms": t + TOOL_LOOP_MS,
                    }
                )
                t += TOOL_LOOP_MS
        else:
            spans.append(
                {"phase": "llm_wait", "lock": False, "t_start_ms": t, "t_end_ms": t + LLM_WAIT_MS}
            )
            t += LLM_WAIT_MS
    if not spans:
        spans = [
            {"phase": "llm_wait", "lock": False, "t_start_ms": 0, "t_end_ms": LLM_WAIT_MS},
        ]
        t = LLM_WAIT_MS
    spans.append({"phase": "idle", "lock": False, "t_start_ms": t, "t_end_ms": t + IDLE_TAIL_MS})
    return spans


def stub_profile(_task_text: str) -> dict[str, Any]:
    return {
        "version": "v1",
        "complexitySignal": 0.0,
        "domain": "unknown",
        "embeddingSim": 0.0,
        "confidence": 0.0,
        "modelID": "stub",
        "scoredAt": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ"),
        "difficultyTier": "medium",
    }


def make_hf_classifier():
    demo = ROOT / "demos" / "agent-llm-multiplex"
    sys.path.insert(0, str(demo))
    os.environ.setdefault("SEMANTIC_CLASSIFIER", "local-hf")
    from actor.profile import classify  # type: ignore

    cache: dict[str, dict[str, Any]] = {}

    def classify_cached(task_text: str) -> dict[str, Any]:
        key = sha256_text(task_text)
        if key in cache:
            return dict(cache[key])
        raw = classify(task_text)
        keep = {
            "version",
            "complexitySignal",
            "domain",
            "embeddingSim",
            "confidence",
            "modelID",
            "scoredAt",
            "difficultyTier",
        }
        profile = {k: raw[k] for k in keep if k in raw}
        cache[key] = profile
        return dict(profile)

    return classify_cached


def load_bfcl_rows(path: Path, max_samples: int) -> list[dict[str, Any]]:
    rows = [json.loads(l) for l in path.read_text().splitlines() if l.strip()]
    out: list[dict[str, Any]] = []
    for r in rows:
        sidx = int(r.get("sample_index", 0))
        if sidx < 0 or sidx >= max_samples:
            continue
        out.append(r)
    out.sort(key=lambda r: (int(r["query_index"]), int(r["sample_index"])))
    return out


def assign_cohorts(items: list[dict[str, Any]], min_confidence: float) -> None:
    active = [x for x in items if x["task_profile"]["confidence"] >= min_confidence]
    active_sorted = sorted(active, key=lambda x: float(x["task_profile"]["complexitySignal"]))
    n = len(active_sorted)
    easy_cut = max(1, n // 3) if n else 0
    hard_cut = max(1, n - n // 3) if n else 0
    easy_ids = {id(x) for x in active_sorted[:easy_cut]} if n else set()
    hard_ids = {id(x) for x in active_sorted[hard_cut:]} if n else set()

    tool_counts = [len(x["tool_trace"]) for x in items]
    tool_med = sorted(tool_counts)[len(tool_counts) // 2] if tool_counts else 0

    for x in items:
        conf = float(x["task_profile"]["confidence"])
        l3 = conf >= min_confidence
        x["l3_active"] = l3
        if not l3:
            x["cohort"] = "l3_inactive"
        elif id(x) in hard_ids:
            x["cohort"] = "l3_hard"
        elif id(x) in easy_ids:
            x["cohort"] = "l3_easy"
        else:
            x["cohort"] = "l3_mid"
        ntool = len(x["tool_trace"])
        x["phase_role"] = "tool_dense" if ntool >= max(tool_med, 1) and ntool > 0 else "normal"


def select_to_target(
    items: list[dict[str, Any]], target: int, min_confidence: float
) -> list[dict[str, Any]]:
    assign_cohorts(items, min_confidence)
    active = [x for x in items if x["l3_active"]]
    inactive_dense = [
        x for x in items if (not x["l3_active"]) and x["phase_role"] == "tool_dense"
    ]
    inactive_rest = [
        x for x in items if (not x["l3_active"]) and x["phase_role"] != "tool_dense"
    ]

    def key(x: dict[str, Any]) -> tuple:
        src = x["source"]
        return (src["query_index"], src["sample_index"])

    active.sort(key=key)
    inactive_dense.sort(key=key)
    inactive_rest.sort(key=key)

    picked: list[dict[str, Any]] = []
    seen: set[int] = set()
    for x in active:
        seen.add(id(x))
        picked.append(x)
    for bucket in (inactive_dense, inactive_rest):
        for x in bucket:
            if len(picked) >= target:
                break
            if id(x) in seen:
                continue
            seen.add(id(x))
            picked.append(x)
        if len(picked) >= target:
            break

    if len(picked) < target:
        raise SystemExit(
            f"only {len(picked)} BFCL sessions available, need target={target}. "
            "Lower --target or raise --max-samples (max pool is 250)."
        )
    assign_cohorts(picked, min_confidence)
    print(
        f"[select] pool={len(items)} → {len(picked)} "
        f"(l3_active={sum(1 for x in picked if x['l3_active'])}, target={target})"
    )
    return picked


def pack_waves(items: list[dict[str, Any]]) -> list[dict[str, Any]]:
    buckets = {
        "tool_dense": [x for x in items if x["phase_role"] == "tool_dense"],
        "l3_hard": [x for x in items if x["cohort"] == "l3_hard"],
        "l3_easy": [x for x in items if x["cohort"] == "l3_easy"],
        "rest": [
            x
            for x in items
            if x["phase_role"] != "tool_dense"
            and x["cohort"] not in ("l3_hard", "l3_easy")
        ],
    }
    td_ids = {id(x) for x in buckets["tool_dense"]}
    for k in ("l3_hard", "l3_easy", "rest"):
        buckets[k] = [x for x in buckets[k] if id(x) not in td_ids]

    ordered: list[dict[str, Any]] = []
    used: set[int] = set()

    def take(bucket: str) -> dict[str, Any] | None:
        while buckets[bucket]:
            x = buckets[bucket].pop(0)
            if id(x) in used:
                continue
            used.add(id(x))
            return x
        return None

    wave_id = 0
    while len(ordered) < len(items):
        wave: list[dict[str, Any]] = []
        for pref in ("tool_dense", "l3_hard", "l3_easy", "rest"):
            x = take(pref)
            if x is None:
                for b in ("tool_dense", "l3_hard", "l3_easy", "rest"):
                    x = take(b)
                    if x is not None:
                        break
            if x is None:
                break
            x["wave_id"] = wave_id
            x["wave_slot"] = len(wave)
            wave.append(x)
            if len(wave) >= WAVE_SIZE:
                break
        if not wave:
            break
        ordered.extend(wave)
        wave_id += 1

    for xs in buckets.values():
        for x in xs:
            if id(x) not in used:
                x["wave_id"] = wave_id
                x["wave_slot"] = 0
                ordered.append(x)
                used.add(id(x))
                wave_id += 1
    return ordered


def assign_arrivals(items: list[dict[str, Any]], busy_minutes: list[int]) -> None:
    n_waves = max((x["wave_id"] for x in items), default=-1) + 1
    if len(busy_minutes) < n_waves:
        raise SystemExit(
            f"need {n_waves} Azure busy minutes for waves, got {len(busy_minutes)}"
        )
    for x in items:
        base = AZURE_DAY_EPOCH + busy_minutes[x["wave_id"]] * 60
        x["arrival_ts"] = base + x["wave_slot"] * IN_WAVE_GAP_SEC


def dump_yaml(obj: Any, indent: int = 0) -> str:
    sp = "  " * indent
    if isinstance(obj, dict):
        lines = []
        for k, v in obj.items():
            if isinstance(v, (dict, list)):
                lines.append(f"{sp}{k}:")
                lines.append(dump_yaml(v, indent + 1))
            elif isinstance(v, str):
                lines.append(f"{sp}{k}: {json.dumps(v)}")
            elif isinstance(v, bool):
                lines.append(f"{sp}{k}: {str(v).lower()}")
            elif v is None:
                lines.append(f"{sp}{k}: null")
            else:
                lines.append(f"{sp}{k}: {v}")
        return "\n".join(lines)
    if isinstance(obj, list):
        lines = []
        for item in obj:
            if isinstance(item, (dict, list)):
                lines.append(f"{sp}-")
                lines.append(dump_yaml(item, indent + 1))
            else:
                lines.append(f"{sp}- {json.dumps(item) if isinstance(item, str) else item}")
        return "\n".join(lines)
    return f"{sp}{obj}"


def validate(sessions: list[dict[str, Any]], allow_incomplete: bool) -> None:
    ids = [s["session_id"] for s in sessions]
    if len(ids) != len(set(ids)):
        raise SystemExit("duplicate session_id")
    prev = -1.0
    l3 = 0
    for i, s in enumerate(sessions):
        ts = float(s["arrival_ts"])
        if ts + 1e-9 < prev:
            raise SystemExit(f"non-monotonic arrival_ts at {i}")
        prev = ts
        if not s.get("task_text") or not s.get("phase_spans"):
            raise SystemExit(f"incomplete session {i}")
        tp = s["task_profile"]
        if "confidence" not in tp or "complexitySignal" not in tp:
            raise SystemExit(f"bad task_profile at {i}")
        if not allow_incomplete and tp.get("modelID") == "stub":
            raise SystemExit("stub profiles not allowed without --allow-incomplete")
        if s.get("eval", {}).get("l3_active"):
            l3 += 1
        if s["source"].get("subset") != SUBSET:
            raise SystemExit(f"non-BFCL subset at {i}: {s['source'].get('subset')}")
    if not allow_incomplete and l3 < 6:
        raise SystemExit(f"too few l3_active sessions ({l3}); need ≥6 for contrast")
    gaps = [sessions[i + 1]["arrival_ts"] - sessions[i]["arrival_ts"] for i in range(len(sessions) - 1)]
    gaps_pos = [g for g in gaps if g > 0]
    if gaps_pos and sorted(gaps_pos)[len(gaps_pos) // 2] < 0.5:
        raise SystemExit("median positive inter-arrival < 0.5s")
    print(f"[ok] validated n={len(sessions)} l3_active={l3} subset={SUBSET}")


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--version", default="v2", choices=["v2"])
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--max-samples", type=int, default=5, help="sample_index in [0, max); BFCL has 5")
    ap.add_argument("--target", type=int, default=200, help="Minimum sessions (BFCL pool ≤250)")
    ap.add_argument("--classify", choices=["hf", "none"], default="hf")
    ap.add_argument("--allow-incomplete", action="store_true")
    ap.add_argument("--min-confidence", type=float, default=CONF_ACTIVE_DEFAULT)
    ap.add_argument("--out", type=Path, default=None)
    args = ap.parse_args()

    if args.target > 250:
        raise SystemExit("BFCL has at most 250 trajectories; --target must be ≤250")

    min_conf = args.min_confidence
    out_dir = args.out or (ROOT / "docs" / "eval" / "datasets" / f"agent-semantic@{args.version}")

    apb_path = CACHE / "apb" / f"{SUBSET}_test.jsonl"
    download(APB_URL, apb_path)
    azure_csv = ensure_azure_csv(CACHE)
    busy = azure_busy_minute_starts(azure_csv)

    rows = load_bfcl_rows(apb_path, args.max_samples)
    if args.limit > 0:
        rows = rows[: args.limit]
    if not rows:
        raise SystemExit("no BFCL rows")

    classify_fn = stub_profile if args.classify == "none" else make_hf_classifier()

    items: list[dict[str, Any]] = []
    for r in rows:
        task_text = (r.get("question") or "").strip()
        if not task_text:
            for msg in r.get("messages") or []:
                if msg.get("role") == "user" and msg.get("content"):
                    task_text = str(msg["content"]).strip()
                    break
        if not task_text:
            raise SystemExit(f"empty task_text q={r.get('query_index')}")
        messages = r.get("messages") or []
        tool_trace = extract_tool_trace(messages)
        phase_spans = derive_phase_spans(messages)
        profile = classify_fn(task_text)
        q = int(r["query_index"])
        sidx = int(r["sample_index"])
        sid = f"apb/{SUBSET}/q{q:03d}_s{sidx}"
        items.append(
            {
                "schema_version": SCHEMA,
                "session_id": sid,
                "source": {
                    "dataset": "LulaCola/AgentProcessBench",
                    "subset": SUBSET,
                    "query_index": q,
                    "sample_index": sidx,
                    "license": "MIT",
                    "azure_invocations": AZURE_CSV,
                    "azure_day": AZURE_DAY,
                },
                "task_text": task_text,
                "tool_trace": tool_trace,
                "phase_spans": phase_spans,
                "task_profile": profile,
            }
        )
        if len(items) % 50 == 0 or len(items) == 1:
            print(
                f"[classify] {len(items)}/{len(rows)} {sid} "
                f"conf={profile.get('confidence')} sig={profile.get('complexitySignal')}"
            )

    items = select_to_target(items, args.target, min_conf)
    if len(items) < args.target and not args.allow_incomplete:
        raise SystemExit(f"only {len(items)} sessions, need ≥{args.target}")

    n_waves_needed = (len(items) + WAVE_SIZE - 1) // WAVE_SIZE
    if len(busy) < n_waves_needed:
        raise SystemExit(f"need {n_waves_needed} Azure busy minutes, got {len(busy)}")

    items = pack_waves(items)
    assign_arrivals(items, busy)

    sessions: list[dict[str, Any]] = []
    arrivals: list[dict[str, Any]] = []
    for i, x in enumerate(items):
        sessions.append(
            {
                "schema_version": SCHEMA,
                "session_id": x["session_id"],
                "source": x["source"],
                "task_text": x["task_text"],
                "arrival_ts": x["arrival_ts"],
                "tool_trace": x["tool_trace"],
                "phase_spans": x["phase_spans"],
                "task_profile": x["task_profile"],
                "eval": {
                    "cohort": x["cohort"],
                    "l3_active": x["l3_active"],
                    "phase_role": x["phase_role"],
                    "wave_id": x["wave_id"],
                    "wave_slot": x["wave_slot"],
                    "n_tools": len(x["tool_trace"]),
                },
                "notes": (
                    f"Single-source BFCL; Azure busy-minute waves + {IN_WAVE_GAP_SEC}s in-wave gap."
                ),
            }
        )
        arrivals.append(
            {
                "session_id": x["session_id"],
                "arrival_ts": x["arrival_ts"],
                "azure_day": AZURE_DAY,
                "azure_csv": AZURE_CSV,
                "wave_id": x["wave_id"],
                "wave_slot": x["wave_slot"],
                "join_index": i,
            }
        )

    validate(sessions, allow_incomplete=args.allow_incomplete or args.classify == "none")

    out_dir.mkdir(parents=True, exist_ok=True)
    with (out_dir / "sessions.jsonl").open("w") as fs, (out_dir / "arrivals.jsonl").open("w") as fa:
        for s, a in zip(sessions, arrivals):
            fs.write(json.dumps(s, ensure_ascii=False) + "\n")
            fa.write(json.dumps(a, ensure_ascii=False) + "\n")

    l3_sigs = [
        s["task_profile"]["complexitySignal"] for s in sessions if s["eval"]["l3_active"]
    ]
    summary = {
        "session_count": len(sessions),
        "source_subset": SUBSET,
        "l3_active": sum(1 for s in sessions if s["eval"]["l3_active"]),
        "cohorts": dict(Counter(s["eval"]["cohort"] for s in sessions)),
        "tool_dense": sum(1 for s in sessions if s["eval"]["phase_role"] == "tool_dense"),
        "waves": max((s["eval"]["wave_id"] for s in sessions), default=-1) + 1,
        "arrival_span_sec": sessions[-1]["arrival_ts"] - sessions[0]["arrival_ts"],
        "signal_range_l3": {
            "min": min(l3_sigs) if l3_sigs else 0,
            "max": max(l3_sigs) if l3_sigs else 0,
        },
        "how_to_use_for_semantic_score": [
            "Within each wave, tool_dense (lock) should be spared vs llm_wait peers",
            "Among unlocked peers, l3_hard should outrank l3_easy on urgency_prior",
            "Compare policies on mid_tool_suspend and Resume latency by cohort",
        ],
    }
    (out_dir / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")

    manifest = {
        "id": f"agent-semantic@{args.version}",
        "schema_version": SCHEMA,
        "session_count": len(sessions),
        "arrival_ts_unit": "unix_seconds",
        "subset": SUBSET,
        "max_samples": args.max_samples,
        "target": args.target,
        "classify_mode": args.classify,
        "min_confidence": min_conf,
        "wave_size": WAVE_SIZE,
        "in_wave_gap_sec": IN_WAVE_GAP_SEC,
        "sources": {
            "agent_process_bench": {
                "dataset": "LulaCola/AgentProcessBench",
                "subset": SUBSET,
                "license": "MIT",
                "url": APB_URL,
                "sha256": sha256_file(apb_path),
            },
            "azure_functions_2019": {
                "release": "dataset-functions-2019",
                "file": AZURE_CSV,
                "day": AZURE_DAY,
                "license": "CC-BY",
                "sha256": sha256_file(azure_csv),
                "day_epoch_unix": AZURE_DAY_EPOCH,
                "note": (
                    "Busy minutes as wave bases only (spacing); "
                    f"in-wave offset {IN_WAVE_GAP_SEC}s."
                ),
            },
        },
        "built_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ"),
    }
    (out_dir / "manifest.yaml").write_text(
        "# Generated by hack/build-agent-semantic-dataset.py\n" + dump_yaml(manifest) + "\n"
    )

    (out_dir / "README.md").write_text(
        f"""# agent-semantic@{args.version}

Single-source agent workload for **semantic-score**.

| Source | Role |
|--------|------|
| AgentProcessBench **BFCL** | Task text + tool trajectories |
| Azure Functions 2019 day01 | Arrival wave spacing only |

```bash
./hack/build-agent-semantic-dataset.py --target 200 --classify hf
```

See `summary.json`. Contract: [`../../agent-semantic-workload.md`](../../agent-semantic-workload.md).
"""
    )

    lines = []
    for name in ("sessions.jsonl", "arrivals.jsonl", "summary.json", "manifest.yaml", "README.md"):
        lines.append(f"{sha256_file(out_dir / name)}  {name}")
    (out_dir / "checksums.sha256").write_text("\n".join(lines) + "\n")

    print(json.dumps(summary, indent=2))
    print(f"[done] {out_dir}")


if __name__ == "__main__":
    main()
