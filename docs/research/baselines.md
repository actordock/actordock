# Research: baselines

Policies to beat or contextualize. Implement as named, versioned strategies behind the same scheduler hook.

| ID | Baseline | Notes |
|----|----------|-------|
| `random` | Random feasible Worker | Sanity floor |
| `fifo` | First-come, ignore priority | Shows cost of no priority |
| `priority-static` | Strict priority, ignore load/C/R cost | Classic priority queue |
| `least-loaded` | Pack by current Worker load only | No semantics |
| `locality-sticky` | Prefer last Worker / local snapshot | Highlights resume locality |

Proposed research policies (e.g. [`semantic-score`](../architecture/semantic-score.md)) should cite which baselines they improve on which metrics.

Add/remove rows as literature survey suggests standard peers (see [literature.md](./literature.md)).
