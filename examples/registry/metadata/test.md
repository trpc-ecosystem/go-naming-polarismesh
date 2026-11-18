English | [Chinese](test.zh_CN.md)

# Registry Metadata Test

These seven cases match [verify.sh](verify.sh). Default registration ports are
28272, 28274 and 28280; listener ports are distinct and configurable.

## Cases

- **1.1 complete registration snapshot**: one instance uses the provider's host,
  port and version, an empty protocol, `deployment=green` and the unchanged
  JSON descriptor. Dynamic deployment overrides the static value.
- **1.2 independent registration version**: a second registration port retains
  its own version and descriptor.
- **1.3 independent registration host**: a third instance advertises
  `127.0.0.1`, independently of the framework address.
- **1.4 TTL and auto heartbeat**: all three instances enable health checks with
  TTL 6. Pause the primary process until its instance becomes unhealthy while
  the other two remain healthy; resume it and verify recovery.
- **1.5 microservice service type**: the auto-created service is
  `SERVICE_TYPE_MICROSERVICE`, not an AI service.
- **1.6 provider failure before registration**: a failing snapshot provider
  exits with `snapshot unavailable`, writes no instance and leaves all three
  successful instances intact.
- **1.7 same host and registration port replacement**: a replacement provider
  uses the existing primary identity. Its instance ID stays unchanged while
  version and descriptor endpoint update together; secondary and alternate
  instances remain unchanged.

## Cleanup

The final providers are retained. Default cleanup stops recorded processes and
removes local artifacts. Add `--delete-service --polaris-server <host>` to delete
the owned service. A run requires the target service to be absent, not merely
empty; inspect prior resources before explicitly cleaning them.
