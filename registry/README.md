# Service Registry Plugin

## How to use
```go
import _ "trpc.group/trpc-go/trpc-naming-polarismesh/registry"
```

## A complete Configuration
```yaml
plugins:  # tRPC-Go plugin configuration.
  registry:  # Registry has its own plugin type.
    polarismesh:  # This Registry is based on polaris mesh.
      register_self: true  # Whether to register, default as false.
      heartbeat_interval: 3000  # The interval to report heartbeat, must be provided.
      debug: true  # Whether to enable the debug log of polaris mesh sdk, default as false.
      instance_location:  # The location of the registered instance.
        region: China
        zone: Guangdong
        campus: Shenzhen
      service:
        - name:  trpc.test.helloworld.Greeter1  # The name of service.
          namespace: namespace-test1  # The namespace of your service.
          # (Optional) Used to heartbeat or unregister.
          # When register_self is true, this config has no effect, the plugin will use returned instance_id of register to overwrite config.
          # if register_self is false, instance_id cannot be missing.
          instance_id: yyyyyyyyyyyyyyyy
          bind_address: eth1:8080  # Specify the listening address of the service.
          weight: 100  # Default weight is 100.
          metadata:  # Custom metadata when registering.
            # Enable set (both this line and the next line need to be set to fully enable set).
            internal-enable-set: Y
            internal-set-name: xx.yy.sz  # Set service set name.
            key1: val1  # For other metadata, etc., please refer to polaris mesh related documents.
            key2: val2
```
