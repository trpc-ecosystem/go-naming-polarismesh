<!-- markdownlint-disable MD041 -->

## Related Issue

Fixes #

## Areas

- [ ] Naming and registry lifecycle
- [ ] AI Registry AgentCard or MCP
- [ ] Discovery, routing, load balancing or circuit breaking
- [ ] Examples, real E2E or cloud packaging
- [ ] Documentation, CI or repository governance

## What Changed

Describe the user-visible or operational outcome.

## Why

Explain the problem and any non-obvious design decision.

## Testing

List the automated and real-service checks that were actually performed.

## Compatibility

- [ ] Public API compatibility was checked or the required version bump is explained.
- [ ] Configuration migration is documented when defaults or validation changed.
- [ ] The supported Go version in `go.mod` and CI still match.

## Checklist

- [ ] Tests cover the changed behavior and failure path.
- [ ] Real Polaris E2E was run when registration or routing behavior changed, or the reason it was not run is documented.
- [ ] `go build ./...` passes.
- [ ] `go test -count=1 ./...` passes.
- [ ] `go test -race -count=1 ./...` passes when concurrency or lifecycle is affected.
- [ ] `go vet ./...` passes.
- [ ] `golangci-lint run --new-from-rev=<base>` passes.
- [ ] `go mod tidy -diff` passes.
- [ ] `./integration/cloud/build-package.sh` succeeds when examples or cloud packaging changed.
- [ ] No credentials, private endpoints or generated runtime artifacts are included.
