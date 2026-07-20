# Agentix (NSDI 2026)

**Citation:** Luo et al. *Agentix: An Efficient Serving Engine for LLM Agents as General Programs.* NSDI 2026.  
https://www.usenix.org/system/files/nsdi26-luo.pdf

## Problem they solve

LLM serving schedules call-by-call; agent **programs** suffer head-of-line blocking without program-level context.

## Method

Global process table of agent programs; program-level prioritization (e.g. PLAS / LAS-style attained service) across LLM calls.

## Reuse for us

- Treat **agent session** (not single request) as the priority unit.
- Fairness / attained-service ideas for within-class scheduling.
- Contrast call-level vs session-level metrics when defining ours.

## Does not transfer

- Resource is **GPU inference**, not sandbox Worker slots + checkpoint restore.
