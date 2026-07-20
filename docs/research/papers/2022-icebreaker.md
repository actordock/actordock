# IceBreaker (ASPLOS 2022)

**Citation:** Roy, Patel, Tiwari. *IceBreaker: Warming Serverless Functions Better with Heterogeneity.* ASPLOS 2022.  
https://doi.org/10.1145/3503222.3507750

## Problem they solve

Serverless keep-alive is expensive; cold starts hurt latency. Who to keep warm under budget?

## Method

Utility score from arrival probability, predicted next invoke, heterogeneity benefit, keep-alive cost → place/warm on cheap vs expensive nodes.

## Reuse for us

- Strong template for **utility-based** “who stays resident / who suspends”.
- Heterogeneous capacity analogy (fast local snapshot vs remote restore).
- Candidate baseline family: utility keep-alive adapted to sandbox idle traces.

## Does not transfer

- Stateless-ish functions vs stateful sandbox memory+FS checkpoints.
- No agent priority semantics or cross-Worker resume routing.
