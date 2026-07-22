# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0

"""L3 taskProfile via local HF models (llm-semantic-router weights)."""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

from actor.hf_classify import classify_local_hf


def classify(task: str) -> dict[str, Any]:
    """Domain HF + SR continuous complexitySignal (no tier in keepScore)."""
    raw = classify_local_hf(task)
    out: dict[str, Any] = {
        "version": "v1",
        "complexitySignal": float(raw.get("_margin", 0.0)),
        "domain": raw["domain"],
        "embeddingSim": float(raw["embeddingSim"]),
        "confidence": float(raw["confidence"]),
        "modelID": raw["modelID"],
        "scoredAt": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
    }
    # Optional debug labels (policy ignores difficultyTier).
    if raw.get("difficultyTier"):
        out["difficultyTier"] = raw["difficultyTier"]
    if raw.get("_complexityRule"):
        out["complexityRule"] = raw["_complexityRule"]
    if "_hardScore" in raw:
        out["hardScore"] = raw["_hardScore"]
    if "_easyScore" in raw:
        out["easyScore"] = raw["_easyScore"]
    if "_complexityConf" in raw:
        out["complexityConf"] = raw["_complexityConf"]
    return out
