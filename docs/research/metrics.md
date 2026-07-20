# Research: metrics

Draft evaluation metrics for comparing allocation policies. Refine as experiments harden.

## Primary

| Metric | Intent |
|--------|--------|
| Priority-weighted resume latency | High-priority sandboxes should wake faster |
| Priority violation / starvation rate | Low-priority must not permanently block high-priority |
| Effective oversubscription | Concurrent sessions per Worker (or per CPU/mem) |
| Preempt/suspend cost | Time and resources spent checkpointing under pressure |

## Secondary

| Metric | Intent |
|--------|--------|
| Cold-start rate | How often policy fails to keep warm/resume path |
| Cross-Worker resume rate | Migration frequency and its latency tax |
| Fairness within priority class | Avoid pathological tie-breaking |
| Scheduling decision latency | Control-plane overhead |

## Reporting

Always report per priority class (or semantic cohort), not only globals. Pair online runs with offline replay on the same dataset when possible.
