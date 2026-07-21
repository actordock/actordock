# Docs

Actordock design, research, and planning. Code lives elsewhere; this tree holds intent, decisions, and evaluation plans.

## Read order

1. [vision.md](./vision.md) — research problem first, system as the lab
2. [research/problem.md](./research/problem.md) — sandbox priority / allocation
3. [architecture/overview.md](./architecture/overview.md) — platform that makes experiments possible
4. [research/metrics.md](./research/metrics.md) → [baselines.md](./research/baselines.md) → [experiments.md](./research/experiments.md)
5. [research/literature.md](./research/literature.md) — paper survey (living)
6. [eval/README.md](./eval/README.md) — datasets and result layout

## Layout

| Path | Purpose |
|------|---------|
| `vision.md` | Positioning: research core + system non-goals |
| `research/` | Problem, metrics, baselines, experiments, literature |
| `architecture/` | How the runtime/control plane supports the research |
| `eval/` | Dataset formats, fixtures, comparison artifacts |
| `decisions/` | ADRs |
| `planning/` | Roadmap and open questions |
| `references/` | Short notes on external systems (not a paper survey) |

## Conventions

- Research question leads; multiplexing/C/R are the **experimental platform**, not the headline claim alone.
- Literature goes in `research/literature.md` (and optional per-paper notes under `research/papers/`).
- Prefer short docs that stay true; move settled answers out of `planning/open-questions.md`.

## Hard constraint: learn from Substrate

**Do not reinvent platform mechanics.** Most technical detail (Worker multiplexing, Pause/Suspend, snapshot layout/codec, control-plane hot path, runtime C/R wiring) must be studied against [Agent Substrate](https://github.com/agent-substrate/substrate) before designing or coding. Divergence needs an explicit reason (research policy, Kind/local constraints, or an ADR)—not ignorance of Substrate.

- Allowed novelty: priority/allocation research, eval loop, pluggable policies.
- Forbidden: closed-door redesign of solved Substrate platform paths.
- Still do not vendor/sync Substrate as a dependency (see ADR 0001); **read and align**, do not copy the tree.
