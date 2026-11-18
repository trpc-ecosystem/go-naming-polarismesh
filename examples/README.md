English | [Chinese](README.zh_CN.md)

# Examples

| Example | Description |
| --- | --- |
| [helloworld](./helloworld) | Canonical proto, generated stubs, server/client and Polaris YAML |
| [loadbalancer](./loadbalancer) | WR, hash, ring hash and maglev observable assertions |
| [circuitbreaker](./circuitbreaker) | INSTANCE, SERVICE, METHOD and HalfOpen recovery |
| [registry/metadata](./registry/metadata) | Opaque metadata, atomic host/version/port/protocol snapshot and replacement |
| [registry/transports](./registry/transports) | Multi-endpoint registration, heartbeat, stable IDs and deregistration |
| [route/metadata](./route/metadata) | Destination metadata and callee environment routing |
| [route/rule](./route/rule) | Source-service and destination-label rule routing |
| [multiContext](./multiContext) | Named selector and SDKContext isolation |

The real E2E requires a Polaris host through `--polaris-server` or
`POLARIS_SERVER` and uses the shared `go-naming-polarismesh-e2e` namespace.
Providers resolve their advertised IP from the TCP connection to Polaris
instead of using a fixed loopback address.
Verification leaves processes, logs and remote resources available for
inspection. Cleanup is a separate explicit step. Every example directory has
English and Chinese test documents; category entry points are
[registry/test.md](./registry/test.md) and [route/test.md](./route/test.md).

Verification fails before startup if its owned processes are still running or
the target service still has registered instances. After a crash or interrupted
run, use `cleanup.sh --delete-service` before retrying.

Run every example sequentially, including HelloWorld and the seven feature groups:

```bash
./examples/verify_all.sh --polaris-server <polaris-server-host>
```

After verification, stop all example processes and remove local artifacts:

```bash
./examples/cleanup.sh
```

Remote services and rules are retained by default. Delete test services explicitly:

```bash
./examples/cleanup.sh --delete-service \
  --polaris-server <polaris-server-host>
```

`--delete-service` deletes test services and instances but retains circuit-breaker
and routing rules. The next verification reuses an existing same-name rule and
enables it, creating a rule only when none exists.

For a first run, use the source-first
[helloworld](./helloworld/README.md) directory. The `verify.sh` scripts are
automation wrappers around the same runtime paths; cleanup is handled separately.

To validate on a remote Linux host without installing Go:

```bash
./examples/cloud-package.sh
```

See [cloud-package.sh](./cloud-package.sh) and
[integration/cloud/README.md](../integration/cloud/README.md) for package,
upload, execution and log-import steps.
