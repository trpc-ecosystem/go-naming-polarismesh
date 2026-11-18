English | [中文](README.zh_CN.md)

# Service Discovery Plugin

The `discovery` package adapts the Polaris Consumer API to the tRPC-Go naming
discovery interface. It is used by the plugin's `WithServiceName` path to load
the full instance snapshot before Polaris service routing and load balancing run.

## Setup

`Setup` creates a tRPC-Go discovery implementation backed by an existing
Polaris `SDKContext`:

```go
import (
    "github.com/polarismesh/polaris-go/api"
    polarisdiscovery "trpc.group/trpc-go/trpc-naming-polarismesh/discovery"
)

func setupDiscovery(sdkCtx api.SDKContext) error {
    return polarisdiscovery.Setup(
        sdkCtx,
        &polarisdiscovery.Config{Name: "polarismesh"},
        true,
    )
}
```

- `Config.Name` sets the registered discovery plugin name. The default is
  `polarismesh`.
- `setDefault=true` also installs it as the tRPC-Go default discovery.
- Normal applications using the root plugin do not call `Setup` directly; the
  root plugin initializes it from `trpc_go.yaml`.

Example service-name discovery configuration:

```yaml
client:
  service:
    - name: trpc.app.server.service
      namespace: Development
      discovery: polarismesh
      servicerouter: polarismesh
      loadbalance: polaris_wr
```

## List behavior

`Discovery.List` is invoked by the tRPC-Go framework and requires both a service
name and namespace. The namespace is supplied through the framework discovery
options.

The adapter requests the full Polaris instance snapshot with unhealthy and
circuit-broken instances included and route filtering disabled. The returned
tRPC node carries the snapshot in its `service_instances` metadata. Polaris
service routing and load balancing consume that snapshot in the next stages.
