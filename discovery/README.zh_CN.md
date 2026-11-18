[English](README.md) | 中文

# 服务发现插件

`discovery` 包将 Polaris Consumer API 适配为 tRPC-Go naming discovery 接口，
供插件的 `WithServiceName` 寻址路径加载完整实例快照，再交给北极星服务路由和
负载均衡处理。

## 初始化

`Setup` 使用已有的 Polaris `SDKContext` 创建 tRPC-Go discovery 实现：

```go
import (
    "github.com/polarismesh/polaris-go/api"
    polarisdiscovery "trpc.group/trpc-go/trpc-naming-polarismesh/discovery"
)

func setupDiscovery(sdkCtx api.SDKContext) error {
    return polarisdiscovery.Setup(
        sdkCtx,
        &polarisdiscovery.Config{Name: "polarismesh"},
        true,
    )
}
```

- `Config.Name` 设置注册到 tRPC-Go 的 discovery 插件名，默认是 `polarismesh`。
- `setDefault=true` 会同时将它设置为 tRPC-Go 默认 discovery。
- 使用根插件的普通应用不需要直接调用 `Setup`，根插件会根据
  `trpc_go.yaml` 自动初始化。

使用 service name 寻址的配置示例：

```yaml
client:
  service:
    - name: trpc.app.server.service
      namespace: Development
      discovery: polarismesh
      servicerouter: polarismesh
      loadbalance: polaris_wr
```

## List 行为

`Discovery.List` 由 tRPC-Go 框架调用，要求同时提供服务名和命名空间。
命名空间通过框架的 discovery option 传入。

适配器会请求完整的 Polaris 实例快照，包含不健康和熔断实例，并关闭路由过滤。
返回的 tRPC node 通过 `service_instances` metadata 携带该快照，后续北极星服务路由
和负载均衡会继续消费这份快照。
