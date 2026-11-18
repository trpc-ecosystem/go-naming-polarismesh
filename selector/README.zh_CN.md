[English](README.md) | 中文

# 路由 Selector 插件

该组件实现 tRPC selector，为 tRPC 用户提供北极星的路由和负载均衡能力。

```go
package main

import (
	"context"
	"log"
	"time"

	"trpc.group/trpc-go/trpc-go/client"
	"trpc.group/trpc-go/trpc-naming-polarismesh/selector"

	pb "trpc.group/trpcprotocol/test/helloworld"

	_ "trpc.group/trpc-go/trpc-go"
)

func init() {
	selector.Register(&selector.Config{
		// 在这里填写配置。
	})
}

func main() {
	ctx, cancel := context.WithTimeout(context.TODO(), time.Millisecond*2000)
	defer cancel()

	opts := []client.Option{
		client.WithNamespace("Development"),
		client.WithTarget("polarismesh://trpc.app.server.service"),
	}

	clientProxy := pb.NewGreeterClientProxy(opts...)

	req := &pb.HelloRequest{
		Msg: "client hello",
	}
	rsp, err := clientProxy.SayHello(ctx, req)
	log.Printf("req:%v, rsp:%v, err:%v", req, rsp, err)
}
```
