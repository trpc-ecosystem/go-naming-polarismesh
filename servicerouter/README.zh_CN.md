[English](README.md) | 中文

# 服务路由

## 规则路由

开启 `enable_servicerouter: true` 后即可使用规则路由。调用方身份来自 `client.WithCallerServiceName`，Polaris 会根据 v2 路由规则匹配主调服务，并选择目标实例标签。

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

可运行示例见 [examples/route/rule](../examples/route/rule)。
