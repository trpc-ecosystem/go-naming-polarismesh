English | [中文](README.zh_CN.md)

# Polaris naming plugin for tRPC-Go

[![Go Reference](https://pkg.go.dev/badge/trpc.group/trpc-go/trpc-naming-polarismesh.svg)](https://pkg.go.dev/trpc.group/trpc-go/trpc-naming-polarismesh)
[![Go Report Card](https://goreportcard.com/badge/trpc.group/trpc-go/trpc-naming-polarismesh)](https://goreportcard.com/report/trpc.group/trpc-go/trpc-naming-polarismesh)
[![License](https://img.shields.io/badge/license-Apache--2.0-green.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/trpc-ecosystem/go-naming-polarismesh)](https://github.com/trpc-ecosystem/go-naming-polarismesh/releases)
[![Testing](https://github.com/trpc-ecosystem/go-naming-polarismesh/actions/workflows/testing.yml/badge.svg)](https://github.com/trpc-ecosystem/go-naming-polarismesh/actions/workflows/testing.yml)

`go-naming-polarismesh` connects tRPC-Go applications to
[Polaris](https://github.com/polarismesh/polaris). It provides registration,
service discovery, routing, load balancing, and circuit breaking as tRPC-Go
naming plugins. The lower-level `selector` and `discovery` packages can also be
used directly.

## Capabilities

- Service lifecycle registration with automatic heartbeat, deregistration,
  stable instance IDs, and multiple endpoints.
- Runtime registration metadata and atomic host, port, version, and protocol
  snapshots.
- Service discovery by service name or `polarismesh://` target.
- Environment, metadata, canary, source-service, and rule-based routing.
- Weighted random, hash, ring-hash, Maglev, and other Polaris SDK load
  balancing policies.
- SERVICE, METHOD, and INSTANCE circuit breaking with HalfOpen probe quotas
  and recover-all handling.
- Isolated SDK contexts for applications that connect to multiple Polaris
  environments.

## Requirements

- Go 1.21 or newer.
- A tRPC-Go application.
- A reachable Polaris gRPC endpoint, commonly on port `8091`, and an existing
  namespace.

CI validates the module with Go 1.21 and Go 1.26.11. For production, use a Go
release that still receives security updates.

## Installation

```bash
go get trpc.group/trpc-go/trpc-naming-polarismesh@latest
```

Register the plugin during application initialization:

```go
import _ "trpc.group/trpc-go/trpc-naming-polarismesh"
```

The Polaris address belongs in runtime configuration. It does not need to be
stored in source code.

## Configuration

A provider selects the plugin through its server service and supplies the
registry configuration:

```yaml
server:
  service:
    - name: trpc.app.server.Service
      registry: polarismesh

plugins:
  registry:
    polarismesh:
      register_self: true
      heartbeat_interval: 3000
      protocol: grpc
      address_list: ${POLARIS_SERVER}:${POLARIS_GRPC_PORT}
      service:
        - name: trpc.app.server.Service
          namespace: Development
```

A consumer configures the selector:

```yaml
client:
  namespace: Development

plugins:
  selector:
    polarismesh:
      default: true
      protocol: grpc
      address_list: ${POLARIS_SERVER}:${POLARIS_GRPC_PORT}
```

These snippets show the naming-related fields only. See
[HelloWorld](examples/helloworld) for complete tRPC-Go configuration files.

## HelloWorld

[HelloWorld](examples/helloworld) contains a proto contract, generated tRPC
stubs, a server, a client, and separate provider and consumer YAML files.

Start the server in one terminal:

```bash
cd examples/helloworld/server
POLARIS_SERVER=<polaris-host> go run .
```

Start the client in another terminal:

```bash
cd examples/helloworld/client
POLARIS_SERVER=<polaris-host> go run .
```

The client demonstrates both discovery paths:

```go
client.WithTarget("polarismesh://trpc.examples.helloworld.Greeter")
client.WithServiceName("trpc.examples.helloworld.Greeter")
```

## Examples And Documentation

- [HelloWorld](examples/helloworld): complete server and client lifecycle.
- [Load balancing](examples/loadbalancer): observable assertions for different
  balancing policies.
- [Circuit breaking](examples/circuitbreaker): SERVICE, METHOD, INSTANCE, and
  HalfOpen recovery.
- [Dynamic registry metadata](examples/registry/metadata): opaque metadata and
  atomic registration snapshots.
- [Multiple endpoints](examples/registry/transports): heartbeat, stable IDs,
  and deregistration.
- [Metadata routing](examples/route/metadata) and
  [rule routing](examples/route/rule): source, destination, and rule-based
  routing.
- [Multiple contexts](examples/multiContext): named selectors and isolated SDK
  contexts.

Package-level documentation:

- [Registry](registry/README.md)
- [Selector](selector/README.md)
- [Discovery](discovery/README.md)
- [Load balancing](loadbalance/README.md)
- [Circuit breaking](circuitbreaker/README.md)
- [Service routing](servicerouter/README.md)

## Verification

Run the local test suite:

```bash
go test ./...
```

Run all examples against a real Polaris server:

```bash
./examples/verify_all.sh --polaris-server <polaris-host>
```

Verification retains processes, logs, and remote resources for inspection.
Clean up explicitly:

```bash
./examples/cleanup.sh
./examples/cleanup.sh \
  --delete-service \
  --polaris-server <polaris-host>
```

The first command stops local example processes and removes generated local
artifacts. The second command additionally deletes the test services and
instances from Polaris. Test routing and circuit-breaker rules are retained.

For validation on a remote Linux host without Go installed, build a package:

```bash
./examples/cloud-package.sh
```

See [integration/cloud/README.md](integration/cloud/README.md) for packaging,
execution, cleanup, and log transfer.

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) and follow the
[Code of Conduct](CODE_OF_CONDUCT.md). Release notes are recorded in
[CHANGELOG.md](CHANGELOG.md).

## License

This project is licensed under the [Apache License 2.0](LICENSE).
