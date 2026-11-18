English | [Chinese](README.zh_CN.md)

# Dynamic Registration Metadata

This example registers an ordinary service with opaque deployment metadata and
a JSON descriptor. A `RegistrationSnapshotProviderFunc` returns host, port,
version, metadata and an explicitly empty protocol from one snapshot. Naming
does not parse the descriptor or derive registration fields from it.

The dynamic `deployment=green` value overrides the static `deployment=blue`.
Without an explicit registration port, this example uses its listener port;
the naming plugin does not impose an AI-specific default.

## Run

```bash
cd examples/registry/metadata/provider
POLARIS_SERVER=<polaris-host> go run . \
  -conf trpc_go.yaml \
  -version v1.2.3 \
  -registration-port 28272
```

Use `-host` to override the advertised host independently of the framework
address. `-metadata-mode none` uses static configuration only.
`-metadata-mode failure` returns a snapshot error before any instance RPC.

## Real E2E

```bash
./examples/registry/metadata/verify.sh --polaris-server <polaris-host>
./examples/registry/metadata/cleanup.sh
./examples/registry/metadata/cleanup.sh --delete-service --polaris-server <polaris-host>
```

The seven cases cover complete snapshots, independent hosts/versions, TTL and
heartbeat recovery, microservice type, provider failure and same-address
replacement. The final providers and logs remain available for inspection.
Detailed assertions are in [test.md](test.md).
