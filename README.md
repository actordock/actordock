# actordock

Hundreds of agents, a handful of sandbox Pods—plus research on **how to prioritize and allocate** those sandboxes under scarce Workers.

Actordock multiplexes idle agent sandboxes onto a warm Worker pool (gVisor suspend/resume) and treats scheduling policy as a first-class, measurable concern.

## Quickstart (Kind)

```bash
./hack/kind-up.sh        # start cluster + deploy
./hack/verify-local.sh   # go test ./e2e/functional/ -tags=e2e
```

Design & research docs: [docs/](./docs/). E2E: [e2e/](./e2e/).

DeepSeek agent demo (sandbox `run_code` + semantic traces): [demos/agent-llm-multiplex/](./demos/agent-llm-multiplex/).
