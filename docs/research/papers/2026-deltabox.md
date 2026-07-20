# DeltaBox (2026)

**Citation:** *DeltaBox: Scaling Stateful AI Agents with Millisecond-Level Sandbox Checkpoint/Rollback.* arXiv, 2026.  
https://arxiv.org/html/2605.22781v2

## Problem they solve

Agent search/RL needs frequent full-state C/R; full duplication is too slow.

## Method

DeltaState: change-based FS + process C/R (DeltaFS layers, DeltaCR incremental dump / template fork). Millisecond checkpoint/rollback claims.

## Reuse for us

- Realistic **C/R latency distributions** for simulators and scoring.
- Complements Crab (they cite each other as complementary).

## Does not transfer

- No multi-tenant Worker-pool allocator; assumes C/R speed is the bottleneck to fix in the OS/runtime.
