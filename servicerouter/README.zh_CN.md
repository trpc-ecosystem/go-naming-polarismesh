[English](README.md) | 中文

# 服务路由

## 灰度能力

- 启用配置

```yaml
plugins:
  selector:
    polarismesh:  # 北极星服务发现配置。
      enable_canary: true  # 启用灰度能力，默认 false，即不启用。
```

- 使用示例

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
