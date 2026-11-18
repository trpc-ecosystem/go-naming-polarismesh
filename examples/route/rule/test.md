English | [Chinese](test.zh_CN.md)

# Rule Route Test

### [case 1.1 source and destination rule]

The configured caller must route all requests to the provider labeled
`route=canary`.

### [case 1.2 unmatched source falls back to normal routing]

Another caller must continue to use normal routing and observe successful calls
from the available providers.

## Cleanup

`verify.sh` leaves the providers, control client and routing rule running.
Default cleanup stops processes and removes local artifacts.
`--delete-service --polaris-server` also deletes the example service but retains
the routing rule. A later run reuses and enables the same-name rule.
