English | [Chinese](README.zh_CN.md)

# HelloWorld

This is the standard tRPC-Go HelloWorld layout for the Polaris naming plugin:
one proto contract, generated stubs, a server, a client, and separate tRPC
configuration files.

```text
helloworld/
|-- proto/helloworld.proto       # service contract
|-- proto/helloworld.pb.go       # protobuf messages
|-- proto/helloworld.trpc.go     # tRPC server/client stubs
|-- server/main.go               # provider implementation
|-- server/trpc_go.yaml          # Polaris registry configuration
|-- client/main.go               # target and service-name calls
`-- client/trpc_go.yaml          # Polaris selector configuration
```

## Prerequisites

- Go 1.18 or newer.
- A reachable Polaris server with gRPC port `8091`.
- A namespace named `go-naming-polarismesh-e2e`, or override `-namespace`.

The Polaris host is not stored in source or YAML. The shell and top-level
configuration default expand `POLARIS_SERVER` at runtime.

## Run The Server

Open the first terminal:

```bash
cd examples/helloworld/server

POLARIS_SERVER=<polaris-host> go run .
```

At startup, the server dials the configured Polaris endpoint and uses the local
IP of that connection. It listens on `<local-ip>:18191` and registers
`trpc.examples.helloworld.Greeter` through Polaris. No service IP is hardcoded
in Go code or YAML.

Useful overrides:

```bash
POLARIS_SERVER=<polaris-host> \
POLARIS_NAMESPACE=<namespace> \
HELLOWORLD_PORT=18191 \
go run . -version v1.0.0
```

## Run The Client

Open the second terminal:

```bash
cd examples/helloworld/client

POLARIS_SERVER=<polaris-host> go run .
```

Expected output:

```text
target://Hello   address=<local-ip>:18191 response=Hello, tRPC-Go! (server v1.0.0)
service://Hello  address=<local-ip>:18191 response=Hello, tRPC-Go! (server v1.0.0)
service://Hi     address=<local-ip>:18191 response=Hi, tRPC-Go! (server v1.0.0)
```

The first call uses:

```go
client.WithTarget("polarismesh://" + serviceName)
```

The other two calls use:

```go
client.WithServiceName(serviceName)
```

`client.WithSelectorNode(&node)` records the selected `host:port`, which makes
the naming result visible instead of only checking that the RPC succeeded.

## Common Flags

| Flag | Environment | Default |
| --- | --- | --- |
| `-polaris-server` | `POLARIS_SERVER` | required |
| `-polaris-port` | `POLARIS_GRPC_PORT` | `8091` |
| `-namespace` | `POLARIS_NAMESPACE` | `go-naming-polarismesh-e2e` |
| `-service` | `POLARIS_SERVICE` | `trpc.examples.helloworld.Greeter` |
| `-message` | `HELLOWORLD_MESSAGE` | `tRPC-Go` |
| `-port` | `HELLOWORLD_PORT` | `18191` on the server |

Stop the server with `Ctrl+C`; the registry plugin then deregisters the
instance.

## Real E2E Verification

The same server and client programs can be verified automatically:

```bash
./examples/helloworld/verify.sh --polaris-server <polaris-server-host>
./examples/helloworld/cleanup.sh
./examples/helloworld/cleanup.sh --delete-service --polaris-server <polaris-server-host>
```

The verifier checks the connection-derived registration host, target discovery,
service-name discovery and both RPC methods. It intentionally leaves the server
running for inspection. See [test.md](test.md).

## Regenerate The Stubs

After changing `proto/helloworld.proto`, install `protoc`, `protoc-gen-go`, and
the tRPC-Go generator, then run:

```bash
cd examples/helloworld/server
go generate
```
