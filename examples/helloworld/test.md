English | [Chinese](test.zh_CN.md)

# HelloWorld Test

This document is kept in sync with [verify.sh](verify.sh).

### [case 1.1 connection-derived registration host]

Operation: start the server with the Polaris endpoint and compare the server's
connection-derived local IP with the host returned by the Polaris OpenAPI.

Expected: the registered host equals the local IP of the TCP connection to
Polaris; no fixed service IP is used.

### [case 1.2 target discovery]

Operation: call `WithTarget("polarismesh://trpc.examples.helloworld.Greeter")`.

Expected: the call succeeds, selects the configured provider port, and returns
`Hello, tRPC-Go!` with the registered version.

The verifier waits for the selector refresh after OpenAPI discovery and retries
the one-shot client on transient gRPC failures.

### [case 1.3 service discovery]

Operation: call the `Hello` and `Hi` methods through `WithServiceName(...)`.

Expected: both calls succeed and select the same provider port.

## Cleanup

`verify.sh` intentionally leaves the provider running for inspection. Use:

```bash
./examples/helloworld/cleanup.sh
./examples/helloworld/cleanup.sh --delete-service --polaris-server <host>
```

A repeated verify is rejected until local processes and stale instances are
cleaned.
