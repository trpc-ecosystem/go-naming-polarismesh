English | [Chinese](README.zh_CN.md)

# Multi-Endpoint Registration

One process registers an ordinary tRPC endpoint and an HTTP endpoint under
the same service. Each endpoint has its own address, protocol, version,
weight, metadata and stable instance ID. Naming owns both heartbeat loops,
rollback and deregistration.

## Configuration

See [provider/trpc_go-multi.yaml](provider/trpc_go-multi.yaml):

```yaml
service:
  - name: ${POLARIS_SERVICE}
    endpoints:
      - address: ${PROVIDER_HOST}:${TRPC_PORT}
        protocol: trpc
      - address: ${PROVIDER_HOST}:${HTTP_PORT}
        protocol: http
```

## Run

```bash
cd examples/registry/transports/provider
go run . \
  -conf trpc_go-multi.yaml \
  -polaris-server <polaris-host> \
  -namespace go-naming-polarismesh-e2e \
  -service trpc.polaris.registry.multi.Transport
```

Run the real Polaris lifecycle verification from the repository root:

```bash
./examples/registry/transports/verify.sh --polaris-server <polaris-host>
```

The seven cases cover endpoint identity, microservice type, TTL and heartbeat
recovery, offline service state, stable IDs, revisions and repeated
deregistration. Both test providers are stopped by the lifecycle cases; the
service and logs remain for inspection. See [test.md](test.md).

## Cleanup

```bash
./examples/registry/transports/cleanup.sh
./examples/registry/transports/cleanup.sh --delete-service --polaris-server <polaris-host>
```

Default cleanup removes local artifacts. Explicit service cleanup also retains
the historical cleanup scope for resources created by earlier example versions.
