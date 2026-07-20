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

- [ ] Sandbox lifecycle: create, connect, delete
- [ ] gVisor checkpoint suspend / restore resume
- [ ] Warm Worker pool with slot accounting
- [ ] Pluggable scheduling policy hook + structured decision logs
- [ ] Location-transparent routing (resume-then-proxy)
- [ ] Templates / snapshots; locality visible to the policy
- [ ] Online metrics export compatible with eval metrics
- [ ] Local verify path (e.g. Kind)

## Out of scope for now

- E2B parity as north-star
- Restoring pre-rewrite platform code into `main`
