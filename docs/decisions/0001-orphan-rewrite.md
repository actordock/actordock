# ADR 0001: Orphan rewrite on main

## Status

Accepted

## Context

The previous `main` mixed an Actordock API layer with a Substrate-derived `runtime/` tree, dual state stores, and ongoing sync churn. We want a clean product centered on Worker multiplexing and semantic scheduling.

## Decision

- Archive the old tree on branch `archive/legacy` and tag `v0.1-legacy`.
- Reset `main` to an orphan history starting empty (docs/product rewrite from zero).
- Do not vendor or sync Substrate as a dependency; reference concepts only.

## Consequences

- Old releases/tags were removed; history for the rewrite starts fresh on `main`.
- Comparison with the old implementation uses `archive/legacy` / `v0.1-legacy`.
