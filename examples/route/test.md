English | [Chinese](test.zh_CN.md)

# Route Examples Test

This document is the entry point for the real Polaris verification of the route
examples. Detailed case lists are available in:

- [metadata/test.md](./metadata/test.md)
- [rule/test.md](./rule/test.md)

## Prerequisites

- The Polaris OpenAPI port is reachable, default `8090`.
- The Polaris gRPC port is reachable, default `8091`.
- `curl` and `jq` are installed.
- No provider or control client from a previous run is still running.
- The example service has no stale instances. Run cleanup with
  `--delete-service` after an interrupted run before retrying. An existing
  same-name routing rule is reused and enabled by the verifier.

The examples use namespace `go-naming-polarismesh-e2e`. The Polaris address is
provided at runtime.

## Run

Run the route suites sequentially:

```bash
./examples/route/metadata/verify.sh \
  --polaris-server <polaris-server-host>

./examples/route/rule/verify.sh \
  --polaris-server <polaris-server-host>
```

Both scripts can also run through:

```bash
./examples/verify_all.sh --polaris-server <polaris-server-host>
```

## Case Matrix

| Suite | Case | Expected behavior |
| --- | --- | --- |
| metadata | 1.1 | `env=red` selects only provider-red and `env=blue` selects only provider-blue |
| metadata | 1.2 | With service routing disabled, `callee_env=red` still selects provider-red |
| metadata | 1.3 | `env=missing` produces zero successful calls |
| rule | 1.1 | The configured source caller reaches only the `route=vip` destination |
| rule | 1.2 | An unmatched caller falls back to normal routing |

The assertions verify selected providers or call counts; a simple request
success is not sufficient for route behavior.

## Cleanup

Verification retains processes and the routing rule for inspection. Stop local
processes and remove local artifacts:

```bash
./examples/route/metadata/cleanup.sh
./examples/route/rule/cleanup.sh
```

Delete the remote service. The routing rule is retained:

```bash
./examples/route/metadata/cleanup.sh \
  --delete-service --polaris-server <polaris-server-host>

./examples/route/rule/cleanup.sh \
  --delete-service --polaris-server <polaris-server-host>
```
