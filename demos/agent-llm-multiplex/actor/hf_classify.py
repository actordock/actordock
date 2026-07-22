# Copyright 2026 The Actordock Authors.
# SPDX-License-Identifier: Apache-2.0

"""Local HF classifiers from llm-semantic-router (weights only; not full SR).

Mirrors SR complexity scoring (prototype_scoring.go / complexity_rule_scoring.go):
  bankScore = bestWeight*best + (1-bestWeight)*mean(topM)
  signal    = hardScore - easyScore
  tier      = hard|easy|medium vs per-rule threshold
plus multi-rule banks and domain composer filter. No LLM judge.
"""

from __future__ import annotations

import math
import os
import threading
from dataclasses import dataclass
from typing import Any

_lock = threading.Lock()
_bundle: "_HFBundle | None" = None

DEFAULT_DOMAIN = "llm-semantic-router/mmbert32k-intent-classifier-merged"
DEFAULT_EMBED = "llm-semantic-router/mmbert-embed-32k-2d-matryoshka"

# SR PrototypeScoringConfig defaults (prototype_scoring_config.go WithDefaults).
_BEST_WEIGHT = float(os.environ.get("SEMANTIC_HF_BEST_WEIGHT", "0.75"))
_TOP_M = int(os.environ.get("SEMANTIC_HF_TOP_M", "2"))


@dataclass(frozen=True)
class _ComplexityRule:
    name: str
    threshold: float
    domain_aliases: tuple[str, ...]
    hard: tuple[str, ...]
    easy: tuple[str, ...]


_COMPLEXITY_RULES: tuple[_ComplexityRule, ...] = (
    _ComplexityRule(
        name="code_complexity",
        threshold=0.1,
        domain_aliases=(
            "computer science",
            "computer_science",
            "cs",
            "coding",
            "programming",
            "software engineering",
        ),
        hard=(
            "design distributed system",
            "implement consensus algorithm",
            "optimize for scale",
            "architect microservices",
            "solve this step by step",
            "compare multiple tradeoffs",
            "analyze the root cause",
            "debug and refactor a complex multi-file codebase",
        ),
        easy=(
            "print hello world",
            "loop through array",
            "read file",
            "sort list",
            "answer briefly",
            "quick summary",
            "simple rewrite",
            "echo hello",
        ),
    ),
    _ComplexityRule(
        name="math_complexity",
        threshold=0.1,
        domain_aliases=("math", "mathematics", "statistics"),
        hard=(
            "prove mathematically",
            "derive the equation",
            "formal proof",
            "solve differential equation",
        ),
        easy=(
            "add two numbers",
            "calculate percentage",
            "simple arithmetic",
            "basic algebra",
        ),
    ),
)

# embeddingSim: affinity to “high-value agent session” templates (not difficulty).
_REF_TEMPLATES = (
    "multi-step coding agent task that needs sandbox tools",
    "long-horizon software engineering with verification",
)


def _norm_domain(label: str) -> str:
    return " ".join(label.strip().lower().replace("_", " ").replace("-", " ").split())


def _bank_score(
    query: list[float],
    bank: list[list[float]],
    *,
    best_weight: float = _BEST_WEIGHT,
    top_m: int = _TOP_M,
) -> tuple[float, float, float]:
    """SR prototypeBank.score: bestWeight*best + (1-bestWeight)*mean(topM)."""
    if not bank:
        return 0.0, 0.0, 0.0
    sims = sorted((_cos(query, p) for p in bank), reverse=True)
    best = sims[0]
    m = top_m if top_m > 0 else len(sims)
    m = min(m, len(sims))
    support = sum(sims[:m]) / float(m)
    w = best_weight
    if w < 0:
        w = 0.0
    if w > 1:
        w = 1.0
    return w * best + (1.0 - w) * support, best, support


class _HFBundle:
    def __init__(self, domain_id: str, embed_id: str) -> None:
        import torch
        from transformers import AutoModel, AutoModelForSequenceClassification, AutoTokenizer

        self.torch = torch
        self.device = "cuda" if torch.cuda.is_available() else "cpu"
        self.domain_id = domain_id
        self.embed_id = embed_id
        self.best_weight = _BEST_WEIGHT
        self.top_m = _TOP_M

        self.domain_tok = AutoTokenizer.from_pretrained(domain_id)
        self.domain_model = AutoModelForSequenceClassification.from_pretrained(domain_id)
        self.domain_model.to(self.device)
        self.domain_model.eval()

        self.embed_tok = AutoTokenizer.from_pretrained(embed_id)
        self.embed_model = AutoModel.from_pretrained(embed_id)
        self.embed_model.to(self.device)
        self.embed_model.eval()

        self._banks: dict[str, tuple[list[list[float]], list[list[float]]]] = {}
        for rule in _COMPLEXITY_RULES:
            hard = [self._embed(t) for t in rule.hard]
            easy = [self._embed(t) for t in rule.easy]
            self._banks[rule.name] = (hard, easy)
        self._refs = [self._embed(t) for t in _REF_TEMPLATES]

    def _embed(self, text: str) -> list[float]:
        torch = self.torch
        inputs = self.embed_tok(
            text,
            return_tensors="pt",
            truncation=True,
            max_length=512,
            padding=True,
        )
        inputs = {k: v.to(self.device) for k, v in inputs.items()}
        with torch.no_grad():
            out = self.embed_model(**inputs)
            hidden = out.last_hidden_state
            mask = inputs["attention_mask"].unsqueeze(-1).float()
            summed = (hidden * mask).sum(dim=1)
            counts = mask.sum(dim=1).clamp(min=1e-6)
            vec = (summed / counts).squeeze(0)
            vec = torch.nn.functional.normalize(vec, dim=0)
        return vec.detach().cpu().tolist()

    def _predict_domain(self, task: str) -> tuple[str, float]:
        torch = self.torch
        inputs = self.domain_tok(
            task,
            return_tensors="pt",
            truncation=True,
            max_length=512,
            padding=True,
        )
        inputs = {k: v.to(self.device) for k, v in inputs.items()}
        with torch.no_grad():
            logits = self.domain_model(**inputs).logits.squeeze(0)
            probs = torch.softmax(logits, dim=-1)
            idx = int(torch.argmax(probs).item())
            domain_conf = float(probs[idx].item())
        id2label = getattr(self.domain_model.config, "id2label", {}) or {}
        domain = str(id2label.get(idx, id2label.get(str(idx), f"label_{idx}")))
        return domain, domain_conf

    def _rules_for_domain(self, domain: str) -> list[_ComplexityRule]:
        d = _norm_domain(domain)
        matched: list[_ComplexityRule] = []
        for rule in _COMPLEXITY_RULES:
            aliases = {_norm_domain(a) for a in rule.domain_aliases}
            if d in aliases or any(a in d or d in a for a in aliases if len(a) > 2):
                matched.append(rule)
        return matched

    def _score_rule(
        self, rule: _ComplexityRule, query_vec: list[float]
    ) -> tuple[str, float, float, float]:
        hard_bank, easy_bank = self._banks[rule.name]
        hard_score, _, _ = _bank_score(
            query_vec, hard_bank, best_weight=self.best_weight, top_m=self.top_m
        )
        easy_score, _, _ = _bank_score(
            query_vec, easy_bank, best_weight=self.best_weight, top_m=self.top_m
        )
        signal = hard_score - easy_score
        if signal > rule.threshold:
            tier = "hard"
        elif signal < -rule.threshold:
            tier = "easy"
        else:
            tier = "medium"
        return tier, signal, hard_score, easy_score

    def classify(self, task: str) -> dict[str, Any]:
        domain, domain_conf = self._predict_domain(task)
        q = self._embed(task)

        rules = self._rules_for_domain(domain)
        if rules:
            best: tuple[str, float, float, float, _ComplexityRule] | None = None
            for rule in rules:
                tier, signal, hs, es = self._score_rule(rule, q)
                if best is None or abs(signal) > abs(best[1]):
                    best = (tier, signal, hs, es, rule)
            assert best is not None
            tier, signal, hard_score, easy_score, chosen = best
            rule_name = chosen.name
            # SR ComplexityRuleResult.Confidence = abs(fusedSignal).
            sr_complexity_conf = abs(signal)
            # Actordock prior-mix ignores confidence < 0.3; raw |signal| is often
            # small, so TaskProfile.confidence uses domain conf when a rule matched.
            confidence = domain_conf
        else:
            tier, signal, hard_score, easy_score = "medium", 0.0, 0.0, 0.0
            sr_complexity_conf = 0.0
            confidence = min(domain_conf, 0.2)
            rule_name = "none"

        emb_sim = max((_cos(q, r) for r in self._refs), default=0.0)
        emb_sim = max(0.0, min(1.0, emb_sim))

        return {
            "difficultyTier": tier,
            "domain": domain,
            "embeddingSim": round(emb_sim, 4),
            "confidence": round(max(0.0, min(1.0, confidence)), 4),
            "modelID": f"{self.domain_id}+{self.embed_id}",
            "_complexityRule": rule_name,
            "_domainConf": round(domain_conf, 4),
            "_complexityConf": round(sr_complexity_conf, 4),
            "_margin": round(signal, 4),
            "_hardScore": round(hard_score, 4),
            "_easyScore": round(easy_score, 4),
            "_bestWeight": self.best_weight,
            "_topM": self.top_m,
        }


def _cos(a: list[float], b: list[float]) -> float:
    if not a or not b or len(a) != len(b):
        return 0.0
    dot = sum(x * y for x, y in zip(a, b))
    na = math.sqrt(sum(x * x for x in a))
    nb = math.sqrt(sum(y * y for y in b))
    if na < 1e-9 or nb < 1e-9:
        return 0.0
    return dot / (na * nb)


def get_bundle(
    *,
    domain_id: str | None = None,
    embed_id: str | None = None,
) -> _HFBundle:
    global _bundle
    domain = (domain_id or os.environ.get("SEMANTIC_HF_DOMAIN") or DEFAULT_DOMAIN).strip()
    embed = (embed_id or os.environ.get("SEMANTIC_HF_EMBED") or DEFAULT_EMBED).strip()
    with _lock:
        if (
            _bundle is None
            or _bundle.domain_id != domain
            or _bundle.embed_id != embed
        ):
            print(f"[hf_classify] loading domain={domain} embed={embed} …")
            _bundle = _HFBundle(domain, embed)
            print(
                f"[hf_classify] ready on {_bundle.device}; "
                f"complexity_rules={[r.name for r in _COMPLEXITY_RULES]}; "
                f"bankScore=bestWeight*{_BEST_WEIGHT}+(1-w)*mean(topM={_TOP_M})"
            )
        return _bundle


def classify_local_hf(task: str) -> dict[str, Any]:
    return get_bundle().classify(task)
