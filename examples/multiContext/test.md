English | [Chinese](test.zh_CN.md)

# Multi Context Test

### [case 1.1 named selector isolation]

`polaris-a` must resolve service A and `polaris-b` must resolve service B.

### [case 1.2 independent SDKContext discovery]

Each selector must return the address of its own provider, proving that caches
and naming clients do not cross.

## Cleanup

`verify.sh` leaves both providers running. Default cleanup stops them and removes
local artifacts. `--delete-service --polaris-server` also deletes both example
services.
