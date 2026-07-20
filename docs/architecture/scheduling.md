# Scheduling (platform view)

Status: skeleton. Research detail lives under [`../research/`](../research/).

## Role in the research stack

The control plane must support **swappable allocation policies** and emit enough signals to score them. Default policy can be naive; the point is a fair comparison harness.

## Decisions every policy must answer

| Decision | Question |
|----------|----------|
| Placement | Where does a new or cold-start sandbox go? |
| Packing / eviction | Who suspends when a Worker is tight? |
| Resume target | Restore in place or on another Worker? |
| Priority application | How do priority / semantics change the three answers above? |

## Inputs (shared by policies)

**Semantics:** priority, affinity/anti-affinity, interactive vs batch, max resume latency, resource constraints.

**Metrics:** Worker load and slots, activity, checkpoint size/time, resume latency, failures, snapshot locality.

## Outputs

Chosen Worker or suspend set, plus a structured reason (for logs and offline analysis).

See also: [research/problem.md](../research/problem.md), [research/metrics.md](../research/metrics.md).
