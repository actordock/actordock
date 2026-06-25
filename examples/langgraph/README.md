# LangGraph + Actordock E2E example

## What this example does

This minimal example runs a tiny stateful LangGraph workflow with conditional orchestration:

1. `parse_node` writes a raw alert payload into a sandbox, normalizes it, and reads
   back a `normalized_alert.json` file.
2. `analyze_node` reads the normalized file in a second sandbox and writes
   `metrics.json`.
3. Graph routes to one of two summarize nodes based on severity:
   - `summarize_node`: non-high severity path.
   - `summarize_high_severity_node`: high-severity path.
4. The chosen summarize node writes `incident-summary.txt` and returns the final summary.

The goal is a small, deterministic end-to-end flow that demonstrates:
- `Sandbox.create(template="python")`
- `sandbox.files.write` / `sandbox.files.read`
- `sandbox.commands.run`
- `sandbox.kill()` in each node for cleanup

## Setup

```bash
source hack/.env.local
pip install -r examples/langgraph/requirements.txt
```

## Run

```bash
cd examples/langgraph
python run.py
```

You can pass a custom payload file:

```bash
python run.py path/to/alert.json
```

Set `LANGGRAPH_SANDBOX_TEMPLATE` to use another template name:

```bash
LANGGRAPH_SANDBOX_TEMPLATE=python python run.py
```

## E2E

From repo root:

```bash
./hack/verify-examples.sh
```
