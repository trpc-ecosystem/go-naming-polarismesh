English | [Chinese](test.zh_CN.md)

# Registry Examples Test

This document is the entry point for the real Polaris verification of the
registry examples. Each subdirectory has its own detailed, runnable test case
list:

- [metadata/test.md](./metadata/test.md)
- [transports/test.md](./transports/test.md)

## Prerequisites

- The Polaris OpenAPI port is reachable, default `8090`.
- The Polaris gRPC port is reachable, default `8091`.
- `curl` and `jq` are installed.
- No provider from a previous run is still running.
- The target services are absent. Run
  `cleanup.sh --delete-service --polaris-server <host>` after an interrupted
  run before retrying.

The examples use namespace `go-naming-polarismesh-e2e`. The Polaris address is
provided at runtime and is not stored in the repository configuration.

## Run

Run the registry suites sequentially:

```bash
./examples/registry/metadata/verify.sh \
  --polaris-server <polaris-server-host>

./examples/registry/transports/verify.sh \
  --polaris-server <polaris-server-host>
```

Both scripts can also run through:

```bash
./examples/verify_all.sh --polaris-server <polaris-server-host>
```

## Case Matrix

| Suite | Cases | Main assertions |
| --- | --- | --- |
| metadata | 1.1-1.3 | Complete snapshots, independent versions and registration hosts |
| metadata | 1.4-1.5 | TTL `6`, heartbeat pause/resume and microservice type |
| metadata | 1.6-1.7 | Snapshot failure before RPC and same-address metadata/version replacement |
| transports | 1.1-1.3 | multi-endpoint identity, microservice type, TTL and heartbeat recovery |
| transports | 2.1-2.3 | Deregistration, `OFFLINE`, stable IDs and new revisions |
| transports | 2.4 | Repeat deregistration |

Detailed operations and expected values are maintained in the two linked
subdirectory test documents.

## Cleanup

Metadata verification retains its final providers; transports verification stops
the provider during lifecycle assertions. Both retain logs and remote services.
Stop local processes and remove local artifacts:

```bash
./examples/registry/metadata/cleanup.sh
./examples/registry/transports/cleanup.sh
```

Explicitly delete the owned remote services:

```bash
./examples/registry/metadata/cleanup.sh \
  --delete-service --polaris-server <polaris-server-host>

./examples/registry/transports/cleanup.sh \
  --delete-service --polaris-server <polaris-server-host>
```
