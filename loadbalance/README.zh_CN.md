[English](README.md) | 中文

# 负载均衡插件

一致哈希和普通哈希负载均衡的使用方式如下：

```go
import (
	_ "trpc.group/trpc-go/trpc-naming-polarismesh"
)

func main() {
	opts := []client.Option{
		// 命名空间
		client.WithNamespace("Development"),
		// 服务名
		client.WithServiceName("trpc.app.server.service"),
		// 普通哈希
		// client.WithBalancerName("polaris_hash"),
		client.WithBalancerName("polaris_ring_hash"),
		// 哈希 key
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

如果一致哈希未生效，请先将插件升级到最新版本。
