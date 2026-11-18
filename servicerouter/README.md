English | [中文](README.zh_CN.md)

# Service routing

## Canary function

- Enable configure
```yaml
plugins:
  selector:
    polarismesh:  # Polaris service discovery configuration.
      enable_canary: true  # Enable canary routing. Disabled by default.
```

- Use the demo
```go
package main

import (
    "context"
    "time"

    "trpc.group/trpc-go/trpc-go/client"
    "trpc.group/trpc-go/trpc-go/log"
    "trpc.group/trpc-go/trpc-go/naming/registry"
    "trpc.group/trpc-go/trpc-naming-polarismesh/servicerouter"

    pb "trpc.group/trpcprotocol/test/helloworld"
)

func main() {
    ctx, cancel := context.WithTimeout(context.TODO(), time.Millisecond*2000)
    defer cancel()

    node := &registry.Node{}
    opts := []client.Option{
        client.WithServiceName("your service"),
        client.WithNamespace("Production"),
        client.WithSelectorNode(node),
        servicerouter.WithCanary("1"),
    }

    proxy := pb.NewGreeterClientProxy()

    req := &pb.HelloRequest{
        Msg: "trpc-go-client",
    }
    rsp, err := proxy.SayHello(ctx, req, opts...)
    log.Debugf("req:%s, rsp:%s, err:%v, node: %+v", req, rsp, err, node)
}
```
