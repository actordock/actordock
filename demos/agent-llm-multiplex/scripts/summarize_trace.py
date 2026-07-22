# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0

"""Summarize semantic JSONL phase counts and crude durations."""

from __future__ import annotations

import json
import sys
from collections import Counter, defaultdict
from datetime import datetime
from pathlib import Path


def main() -> None:
    path = Path(sys.argv[1] if len(sys.argv) > 1 else "traces/semantic.jsonl")
    if not path.exists():
        raise SystemExit(f"missing {path}")

    phases: Counter[str] = Counter()
    locks = 0
    by_sb: dict[str, list[tuple[datetime, str, bool]]] = defaultdict(list)

    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        ev = json.loads(line)
        phase = ev.get("phase", "?")
        phases[phase] += 1
        if ev.get("lock"):
            locks += 1
        ts = datetime.fromisoformat(ev["ts"].replace("Z", "+00:00"))
        by_sb[ev["sandboxID"]].append((ts, phase, bool(ev.get("lock"))))

    print(f"file: {path}")
    print(f"events: {sum(phases.values())}  lock_true: {locks}")
    print("phase counts:")
    for k, v in phases.most_common():
        print(f"  {k}: {v}")

    print("per-sandbox phase dwell (sec, adjacent events):")
    for sid, events in by_sb.items():
        events.sort(key=lambda x: x[0])
        dwell: Counter[str] = Counter()
        for i in range(len(events) - 1):
            t0, phase, _ = events[i]
            t1, _, _ = events[i + 1]
            dwell[phase] += (t1 - t0).total_seconds()
        parts = ", ".join(f"{p}={dwell[p]:.2f}s" for p in sorted(dwell))
        print(f"  {sid}: {parts or '(single event)'}")


if __name__ == "__main__":
    main()
