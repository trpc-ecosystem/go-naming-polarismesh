English | [Chinese](test.zh_CN.md)

# Metadata Route Test

### [case 1.1 destination metadata routing]

Requests with `env=red` must select only provider-red; `env=blue` must select only
provider-blue.

### [case 1.2 callee environment routing]

With service routing disabled, `callee_env=red` must select provider-red.

### [case 1.3 missing metadata has no match]

`env=missing` must produce zero successful calls.

## Cleanup

`verify.sh` leaves the providers and control client running. Default cleanup
stops them and removes local artifacts. Add `--delete-service` with
`--polaris-server` to delete the example service from Polaris.
