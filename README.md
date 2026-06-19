# actordock

Self-hosted sandbox platform for AI agents on Kubernetes.

Multiplex many sandboxes onto shared workers. Idle sessions suspend. Deploy with one command.

Compatible with the [E2B SDK](https://e2b.dev/docs)—point it at your Actordock endpoint.

```bash
./hack/install-local.sh
helm install actordock ./charts/actordock-stack
```

[Architecture](docs/architecture.md) · [Roadmap](docs/roadmap.md)
