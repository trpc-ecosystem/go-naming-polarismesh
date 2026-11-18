English | [Chinese](test.zh_CN.md)

# Load Balancer Test

### [case 1.1 hash-family distribution]

Use 48 distinct keys for hash, ring hash and maglev. All calls must succeed and
both provider addresses must receive traffic.

### [case 1.2 hash stability]

Use one key repeatedly. The selected address must remain stable for each hash
family.

### [case 1.3 weighted random distribution]

Make 200 WR calls. The weight-100 provider must receive more traffic than the
weight-20 provider, and both must receive traffic.

## Cleanup

`verify.sh` leaves the providers and control client running. Default cleanup
stops them and removes local artifacts. Add `--delete-service` with
`--polaris-server` to delete the example service from Polaris.
