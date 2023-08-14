# Route Selector Plugin

An implementation of trpc-selector that provides trpc users with polaris mesh for routing and load balancing.
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
		// your config ...
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
