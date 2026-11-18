English | [Chinese](test.zh_CN.md)

# Circuit Breaker Test

### [case 1.1 instance failover]

Generate an ordered hash sequence of up to 1024 keys, choose a key that selects
provider-b, then kill provider-b. Observe the connection error and five
subsequent provider-a successes.

### [case 1.2 half-open recovery]

Restart provider-b, wait for the sleep window and discovery refresh, then verify
the same key can select provider-b again.

### [case 1.3 service-level rule]

Create a caller-specific SERVICE rule. After enough failures the client must
receive a service circuit-breaker rejection.

### [case 1.4 method-level rule]

Create a METHOD rule for `/<service>/Echo`. The client must receive a method
circuit-breaker rejection while using the same destination service.

## Cleanup

`verify.sh` leaves the final processes and rules running. Default cleanup stops
processes and removes local artifacts. `--delete-service --polaris-server`
additionally deletes the example service but retains the INSTANCE/SERVICE/METHOD
rules. A later run reuses same-name rules and enables the rule needed by the
current case.
