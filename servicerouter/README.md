English | [中文](README.zh_CN.md)

# Service routing

## Rule-based routing

Rule-based routing is enabled with `enable_servicerouter: true`. The caller identity comes from `client.WithCallerServiceName`; Polaris matches it against a v2 routing rule and selects the destination label.

```go
package main

import (
    "context"
    "time"

    "trpc.group/trpc-go/trpc-go/client"
    "trpc.group/trpc-go/trpc-go/log"
    "trpc.group/trpc-go/trpc-go/naming/registry"

    pb "trpc.group/trpcprotocol/test/helloworld"
)

func main() {
    ctx, cancel := context.WithTimeout(context.TODO(), time.Millisecond*2000)
    defer cancel()

    node := &registry.Node{}
    opts := []client.Option{
        client.WithServiceName("trpc.app.server.service"),
        client.WithNamespace("Production"),
        client.WithCallerNamespace("Production"),
        client.WithCallerServiceName("trpc.app.client.service"),
        client.WithSelectorNode(node),
    }

    proxy := pb.NewGreeterClientProxy()

    req := &pb.HelloRequest{
        Msg: "trpc-go-client",
    }
    rsp, err := proxy.SayHello(ctx, req, opts...)
    log.Debugf("req:%s, rsp:%s, err:%v, node: %+v", req, rsp, err, node)
}
```

See [examples/route/rule](../examples/route/rule) for a runnable RuleBased example.
