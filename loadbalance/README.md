# load balancing plugin

Consistent hash or common hash load balancing method is used as follows:
```go
import (
	_ "trpc.group/trpc-go/trpc-naming-polaris"
)

func main() {
	opts := []client.Option{
		// Namespace
		client.WithNamespace("Development"),
		// Service name
		client.WithServiceName("trpc.app.server.service"),
		// Normal hash
		// client.WithBalancerName("polaris_hash"),
		// Consistent hash, support enumeration, please refer to
		// https://git.woa.com/trpc-go/trpc-naming-polaris/blob/master/loadbalance/loadbalance.go#L19
		client.WithBalancerName("polaris_ring_hash"),
		// Hash key 
		client.WithKey("your hash key"),
	}

	clientProxy := pb.NewGreeterClientProxy(opts...)
	req := &pb.HelloRequest{
		Msg: "hello",
	}

	rsp, err := clientProxy.SayHello(ctx, req)
	if err != nil {
		log.Error(err.Error())
		return 
	}

	log.Info("req:%v, rsp:%v, err:%v", req, rsp, err)
}
```

!`Consistent hash does not take effect `, please upgrade to the latest version of the plugin first.
