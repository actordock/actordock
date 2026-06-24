# DNS Controller

The DNS Controller orchsterates the configuration needed to setup the runtime routing.

We want to resolve requests for <actor id>.actors.actordock.dev to the router service address.

* Stub resolver mode: orchestrate running a CoreDNS instance with the actor id mapped to the router service address.

Cluster resources:

* Deployment `actordock-system:dns`. Label: app=dns
* Service `actordock-system:dns`.
* ConfigMap `actordock-system:dns`.

These are defined in manifests/runtime-install/runtime-net-dns.yaml.

## Stub resolver mode

* Ensure stub resolver CoreDNS is running as:
  * Deployment `actordock-system:dns`.
  * Service `actordock-system:dns` pointing to the Deployment.

ConfigMap `actordock-system:dns`:

```
# Match any 'A' query for an actor id pattern under actors.actordock.dev
    template IN A actors.actordock.dev {
        match "^[a-z0-9]([-a-z0-9]*[a-z0-9])?\\.actors\\.actordock\\.dev\\.$"
        answer "{{ .Name }} 60 IN A <router service address>"
    }
```

## Integration

* CoreDNS: Update CoreDNS ConfigMap to add the stub resolver.
* GKE DNS: Update the GKE DNS ConfigMap to add the stub resolver.
