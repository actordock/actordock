# Roadmap

Living checklist. Two tracks: **research** (priority allocation) and **platform** (makes research runnable). Not a phased ship plan.

## Research track

- [ ] Sharpen problem statement and threat/workload model
- [ ] Literature survey → fill [research/literature.md](../research/literature.md)
- [ ] Lock v1 metrics and baseline set
- [ ] First synthetic dataset + schema under `eval/datasets/`
- [ ] Offline replay harness for policy comparison
- [ ] Candidate priority/allocation policy documented vs baselines
- [ ] Optional: paper-oriented writeup of results

## Platform track

- [x] Sandbox lifecycle: create, connect, delete *(create/get/delete; connect/proxy later)*
- [x] gVisor checkpoint suspend / restore resume
- [x] Pause (local sticky) + Suspend (rustfs portable)
- [x] Warm Worker pool (1 running sandbox per Worker)
- [x] Pluggable scheduling policy hook + structured decision logs *(fifo, random)*
- [ ] Location-transparent routing (resume-then-proxy)
- [x] Templates / snapshots; locality visible to the policy *(local sticky + rustfs; templates later)*
- [ ] Online metrics export compatible with eval metrics
- [x] Local verify path (Kind + e2e CI)

## Platform rule

When extending the platform track, **study Substrate first**. Do not invent alternate multiplexing / C/R / snapshot / data-plane designs in isolation. Research track owns novelty (priority allocation); platform track stays Substrate-aligned unless an ADR says otherwise.

## Out of scope for now

- E2B parity as north-star
- Restoring pre-rewrite platform code into `main`
- Vendoring Substrate (read-and-align only)
