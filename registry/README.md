English | [中文](README.zh_CN.md)

# Service Registry Plugin

## How to use
```go
import _ "trpc.group/trpc-go/trpc-naming-polarismesh/registry"
```

## A complete Configuration
```yaml
plugins:  # tRPC-Go plugin configuration.
  registry:  # Registry has its own plugin type.
    polarismesh:  # This Registry is based on polaris mesh.
      register_self: true  # Whether to register, default as false.
      heartbeat_interval: 3000  # The interval to report heartbeat, must be provided.
      debug: true  # Whether to enable the debug log of polaris mesh sdk, default as false.
      instance_location:  # The location of the registered instance.
        region: China
        zone: Guangdong
        campus: Shenzhen
      service:
        - name:  trpc.test.helloworld.Greeter1  # The name of service.
          namespace: namespace-test1  # The namespace of your service.
          version: v1.2.3  # Optional instance version.
          # (Optional) Used to heartbeat or unregister.
          # When register_self is true, this config has no effect, the plugin will use returned instance_id of register to overwrite config.
          # if register_self is false, instance_id cannot be missing.
          instance_id: yyyyyyyyyyyyyyyy
          bind_address: eth1:8080  # Specify the listening address of the service.
          weight: 100  # Default weight is 100.
          metadata:  # Custom metadata when registering.
            # Enable set (both this line and the next line need to be set to fully enable set).
            internal-enable-set: Y
            internal-set-name: xx.yy.sz  # Set service set name.
            key1: val1  # For other metadata, etc., please refer to polaris mesh related documents.
            key2: val2
```

## Dynamic Registration Snapshot

Use `RegisterMetadataProvider` for runtime metadata. A
`RegistrationSnapshotProvider` returns metadata, host, port, version and
protocol from one atomic read. The plugin never parses or rewrites metadata.

- Non-empty `Host` and non-zero `RegistrationPort` override a single endpoint.
- Complete host/port takes precedence over bind/framework addresses and allows
  registration without a listening address. Partial overrides retain the other
  field from the normal address selection.
- A host must be an IPv4/IPv6 address or hostname without a scheme, port,
  brackets or whitespace. Ports must be in `1..65535`.
- `SetProtocol=true` applies even an empty protocol. An old provider's
  non-empty `Protocol` still overrides configuration; empty protocol without
  the flag preserves it.
- Non-empty `Version` overrides configuration.
- Multiple endpoints accept shared metadata/version/protocol but not a shared
  host or port override.
- Dynamic metadata overrides endpoint metadata, which overrides service
  metadata. At most 64 entries are allowed; keys must be non-empty and values
  must not exceed 65535 Unicode characters.
- Snapshot maps and configuration passed to the optional
  `RegistrationConfigValidator` hook are defensive copies. The hook validates
  the same already-read snapshot before any Provider RPC.

```go
provider := polarisregistry.RegistrationSnapshotProviderFunc(func() (
    polarisregistry.RegistrationSnapshot, error,
) {
    return polarisregistry.RegistrationSnapshot{
        Host:             "10.0.0.10",
        RegistrationPort: 9000,
        Version:          "v1.2.3",
        SetProtocol:      true,
        Metadata:         map[string]string{"deployment": "green"},
    }, nil
})
if err := polarisregistry.RegisterMetadataProvider("trpc.app.server.Service", provider); err != nil {
    return err
}
defer polarisregistry.UnregisterMetadataProvider("trpc.app.server.Service")
```

Install the provider before `Serve` triggers `Registry.Register`.
Provider errors prevent registration and heartbeat startup. Protocol-specific
payload validation and migration belong to the application or extension.

## Multiple Endpoints

Services can register multiple Polaris instances
by configuring `endpoints`.
Each endpoint is registered independently and may override the service defaults:

```yaml
service:
  - name: trpc.app.server.Service
    namespace: Development
    instance_protocol: trpc
    version: v1.0.0
    weight: 100
    metadata:
      deployment: prod
    endpoints:
      - address: 10.0.0.10:9000
        protocol: trpc
        instance_id: service-trpc
        ttl: 6
        heartbeat_interval: 2000
      - address: 10.0.0.10:9001
        protocol: http
        version: v2.0.0
        weight: 120
        instance_id: service-http
        ttl: 6
        heartbeat_interval: 2000
        auto_heartbeat: true
        metadata:
          transport: http
```

The plugin-level `protocol` still selects the Polaris connector protocol. The service
`instance_protocol` and endpoint `protocol` select the registered instance protocol.

Rules:

- endpoint order is stable;
- endpoint address identity is `host:port`; endpoints sharing one address must
  each configure a distinct explicit `instance_id`;
- explicit instance IDs must be unique;
- endpoint metadata overrides service metadata;
- an endpoint inherits service `version`, `weight`, `token`, `ttl`,
  `heartbeat_interval`, `auto_heartbeat`, `disable_health_check` and location when omitted;
- `address` is required for every explicit endpoint;
- `endpoints` cannot be combined with `bind_address` or `prefer_bind_address`;
- instance protocol must not exceed 32 characters.

With `auto_heartbeat: true`, the adapter schedules heartbeat reporting by instance ID.
It pauses while the tRPC health status is `NotServing` or `Unknown` and resumes
immediately when the status becomes `Serving`. The inherited or explicit
`heartbeat_interval` is therefore always used.

When `endpoints` is omitted, the existing single-instance behavior remains unchanged.
When `register_self=false`, heartbeat-only mode requires a configured `instance_id`;
set `disable_health_check=true` when a heartbeat-only instance ID is intentionally absent.


## Reusing the Registry SDK Context

`FromFramework` returns the registry created by the tRPC-Go plugin. Reuse its `SDKContext` for
registry-side extensions instead of creating a second Polaris SDK context.

```go
r, ok := polarisregistry.FromFramework("trpc.app.server.Service")
if !ok {
    return fmt.Errorf("polaris registry is not initialized")
}
sdkCtx := r.SDKContext()
instanceVersion := r.Version()
cfg := r.Config() // Defensive copy for extension-owned preparation.
// Build extension APIs from sdkCtx and pass sdkCtx to the adapter.
```

Use this shutdown order: stop accepting new requests, let tRPC deregister the
instance, call `UnregisterMetadataProvider`, then release adapter-owned
resources. The provider is not cached, so a later registration gets a new
snapshot.
