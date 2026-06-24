# LlamaIndex + Actordock — Policy Q&A with computed answers

**Scenario:** HR policy handbook RAG plus sandbox calculation — "How many PTO days for 3 years tenure?"

## Prerequisites

- Actordock local stack: `./hack/install-local.sh`
- Environment from `hack/.env.local` (set automatically by `hack/verify-local.sh`)

## Run

```bash
source hack/.env.local
python3 -m venv .venv && source .venv/bin/activate
pip install -r examples/llamaindex/requirements.txt
cd examples/llamaindex
python run.py --tenure-years 3
```

## Flow

1. **Retrieve** — `MockEmbedding` + in-memory index over `data/policies/*.md` (no OpenAI key)
2. **Compute** — `calculate_pto()` uses the `python` sandbox template, uploads a script, runs `python3`, returns days
3. **Answer** — combines policy excerpt + numeric result

## E2E

Core Actordock E2E (CI and local):

```bash
./hack/verify-local.sh
```

Example integration tests run in **CI only** (`./hack/verify-examples.sh`). To run locally after `install-local.sh`:

```bash
./hack/verify-examples.sh
```

## Policy fixture

PTO accrues at **6 days per year** of tenure (3 years → **18 days**).
