English | [Chinese](test.zh_CN.md)

# Registry Transports Test

These seven cases match [verify.sh](verify.sh). The default endpoints are
tRPC port 28351 and HTTP port 28352.

## Cases

- **1.1 multiple endpoints**: exactly two instances have a non-empty host.
  The tRPC instance uses `multi-trpc-stable`, version `v1.0.0`, weight 80 and
  `transport=trpc`; HTTP uses `multi-http-stable`, version `v2.0.0`, weight
  120 and `transport=http`.
- **1.2 microservice service type**: the service is `SERVICE_TYPE_MICROSERVICE`.
- **1.3 TTL and heartbeat**: both instances enable health checks with TTL 6.
  Pause the process until both become unhealthy, then resume it and verify
  both recover.
- **2.1 deregistration and offline service**: stop the provider; both instances
  disappear and the service becomes `OFFLINE` with zero instances.
- **2.2 stable instance IDs**: restart with the same explicit IDs; both
  instances return with unchanged IDs and endpoint-specific fields.
- **2.3 revision after re-registration**: both instance revisions are non-empty
  and differ from their pre-deregistration values.
- **2.4 repeat deregistration**: stop the restarted provider and verify both
  instances disappear again.

## Cleanup

The lifecycle cases stop the provider. Logs and the empty service are retained.
Default cleanup removes local artifacts; add
`--delete-service --polaris-server <host>` to delete the owned remote service.
