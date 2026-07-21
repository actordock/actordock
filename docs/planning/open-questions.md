# Open questions

Move answers into `architecture/`, `research/`, or `decisions/` when settled.

Platform answers must check Substrate first (hard constraint: no closed-door redesign of solved platform paths).

## Research

1. Which priority model (strict classes vs numeric weights vs SLO deadlines)?
2. Which public or synthetic traces best represent agent sandbox idle/active patterns?
3. Offline-only results acceptable for the first paper-style comparison, or must online C/R costs be in the critical path of claims?
4. Which papers become mandatory baselines after the literature pass?

## Platform

5. External API shape?
6. Cross-Worker resume first-class vs same-Worker-only for early correctness?
7. Snapshot storage: local-only vs remote from the start?
8. Relationship to kubernetes-sigs/agent-sandbox as a Runtime backend?
9. Metric pipeline: push from worker-agent, Prometheus, or both?
