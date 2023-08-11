# Service registry plugin

## How to use
```go
import _ "trpc.group/trpc-go/trpc-naming-polaris/registry"
```

## `！Note`
- The token and instance_id required for service registration can be obtained from [https://polaris.woa.com](https://polaris.woa.com).
- The service name corresponds to the service configuration of the server above, otherwise the registration fails.

## Configure a complete example (only need to report heartbeat)
```yaml
plugins:                       # Plugin configuration.
  registry:                    # Service registration configuration.
    polaris:                   # Configuration of Polaris name registration service.
      register_self: false     # Whether to register, the default is false, registered by 123 platform.
      heartbeat_interval: 3000 # Heartbeat reporting interval of name registration service.
      # debug: true            # Whether to enable the debug log of Polaris sdk.
      instance_location:       # The address location information of the registered instance.
        region: China
        zone: Guangdong
        campus: Shenzhen
      service:
        - name:  trpc.test.helloworld.Greeter1 # Service name corresponds to the service configuration above.
          namespace: namespace-test1           # Environment type, divided into two types: formal Production and informal Development.
          token: xxxxxxxxxxxxxxxxxxxx          # Token required for service registration.
          instance_id: yyyyyyyyyyyyyyyy        # (Optional) Required for service registration, instance_id=XXX(namespace+service+host+port) to get the summary.
          bind_address: eth1:8080              # (Optional) Specify the listening address of the service, the address in the service is used by default.
```

## Complete configuration example (registration + report heartbeat).
```yaml
plugins:                       # Plugin configuration.
  registry:                    # Service registration configuration.
    polaris:                   # Configuration of Polaris name registration service.
      register_self: true      # Whether to register, the default is false, registered by 123 platform.
      heartbeat_interval: 3000 # Heartbeat reporting interval of name registration service.
      # debug: true            # Whether to enable the debug log of Polaris sdk.
      service:
        - name:  trpc.test.helloworld.Greeter1 # Service name corresponds to the service configuration above.
          namespace: namespace-test1           # Environment type, divided into two types: formal Production and informal Development.
          token: xxxxxxxxxxxxxxxxxxxx          # Token required for service registration.
          # weight: 100  # Default weight is 100.
          # metadata:  # Custom metadata when registering.
          #   internal-enable-set: Y  # Enable set (both this line and the next line need to be set to fully enable set).
          #   internal-set-name: xx.yy.sz # Set service set name.
          #   key1: val1 # For other metadata, etc., please refer to Polaris related documents.
          #   key2: val2
```

Pay attention to the setting method of set name:

```yaml
metadata:  # Custom metadata when registering.
  internal-enable-set: Y  # enable set (both this line and the next line need to be set to fully enable set).
  internal-set-name: xx.yy.sz # set name for service set.
```
