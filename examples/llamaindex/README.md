# LlamaIndex + Actordock

1. `Template.build` from official `base` to add Python 3
2. `PolicyWorkflow` retrieves policy text, then `FunctionTool.acall()` runs Python in Actordock

```bash
source hack/.env.local
pip install -r examples/llamaindex/requirements.txt
cd examples/llamaindex && python run.py
```

E2E: `./hack/verify-examples.sh`
