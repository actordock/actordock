## Summary

<!-- What changed and why -->

## E2B REST parity (release gate)

- [ ] Listed touched Platform operations (method + path) in this PR
- [ ] Each touched operation implements **all** OpenAPI request/response fields (full-field rule)
- [ ] Unit or E2E tests assert JSON keys / round-trip behavior (not only HTTP status)
- [ ] Updated [docs/releases/e2b-rest-parity.md](docs/releases/e2b-rest-parity.md) (`partial` → `done` where applicable)

## Test plan

- [ ] `make verify`
- [ ] `./hack/verify-local.sh` (if Platform/envd/Router behavior changed)
