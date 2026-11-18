[English](README.md) | 中文

# 服务注册插件

## 使用方法

```go
import _ "trpc.group/trpc-go/trpc-naming-polarismesh/registry"
```

## 完整配置

```yaml
plugins:  # tRPC-Go 插件配置。
  registry:  # Registry 有独立的插件类型。
    polarismesh:  # 该 Registry 基于北极星。
      register_self: true  # 是否注册自身，默认 false。
      heartbeat_interval: 3000  # 心跳上报间隔，必须配置。
      debug: true  # 是否启用北极星 SDK debug 日志，默认 false。
      instance_location:  # 注册实例的地理位置。
        region: China
        zone: Guangdong
        campus: Shenzhen
      service:
        - name: trpc.test.helloworld.Greeter1  # 服务名。
          namespace: namespace-test1  # 服务所在命名空间。
          version: v1.2.3  # 可选的实例版本。
          # 可选，用于心跳或反注册。
          # register_self 为 true 时该配置无效，插件会使用注册返回的 instance_id 覆盖它。
          # register_self 为 false 时不能缺少 instance_id。
          instance_id: yyyyyyyyyyyyyyyy
          bind_address: eth1:8080  # 指定服务监听地址。
          weight: 100  # 默认权重为 100。
          metadata:  # 注册时的自定义 metadata。
            # 启用 set 时，下面两行都需要配置。
            internal-enable-set: Y
            internal-set-name: xx.yy.sz  # 设置服务 set 名称。
            key1: val1  # 其他 metadata 请参考北极星相关文档。
            key2: val2
```

## 动态注册快照

运行时 metadata 使用 `RegisterMetadataProvider`。需要原子更新多个实例字段时使用
`RegistrationSnapshotProvider`，一次返回 metadata、host、port、version 和 protocol。
插件不解析、不重写 metadata。

- 非空 `Host` 和非零 `RegistrationPort` 覆盖单 endpoint 的对应字段。
- 完整 host/port 优先于 bind/框架地址，无业务监听地址也可注册；单字段覆盖保留
  正常地址选择的另一字段。
- host 接受 IPv4/IPv6 或域名，不允许 scheme、端口、方括号和空白；port 范围为
  `1..65535`。
- `SetProtocol=true` 时包括空字符串在内的 protocol 都生效。旧 provider 的非空
  `Protocol` 仍覆盖配置，空值且未设置标记时继承配置。
- 非空 `Version` 覆盖配置。
- 多 endpoint 可以共享 metadata/version/protocol，不能共用一个 host 或 port 覆盖。
- 动态 metadata 覆盖 endpoint，endpoint 覆盖 service。最多 64 项，key 非空，
  value 不超过 65535 个 Unicode 字符。
- 可选 `RegistrationConfigValidator` 在首次 Provider RPC 前接收同一份已读取快照，
  快照 map 和配置都是防御性复制，不会再次读取 provider。

```go
provider := polarisregistry.RegistrationSnapshotProviderFunc(func() (
    polarisregistry.RegistrationSnapshot, error,
) {
    return polarisregistry.RegistrationSnapshot{
        Host:             "10.0.0.10",
        RegistrationPort: 9000,
        Version:          "v1.2.3",
        SetProtocol:      true,
        Metadata:         map[string]string{"deployment": "green"},
    }, nil
})
if err := polarisregistry.RegisterMetadataProvider("trpc.app.server.Service", provider); err != nil {
    return err
}
defer polarisregistry.UnregisterMetadataProvider("trpc.app.server.Service")
```

provider 必须在 `Serve` 调用 `Registry.Register` 前安装。provider 失败时不注册实例，
不启动心跳。协议专属的 payload 校验、默认值和旧格式转换由应用或扩展层完成。

## 多 Endpoint

服务可以通过配置 `endpoints` 注册多个 Polaris 实例。
每个 endpoint 独立注册，并可以覆盖服务默认值：

```yaml
service:
  - name: trpc.app.server.Service
    namespace: Development
    instance_protocol: trpc
    version: v1.0.0
    weight: 100
    metadata:
      deployment: prod
    endpoints:
      - address: 10.0.0.10:9000
        protocol: trpc
        instance_id: service-trpc
        ttl: 6
        heartbeat_interval: 2000
      - address: 10.0.0.10:9001
        protocol: http
        version: v2.0.0
        weight: 120
        instance_id: service-http
        ttl: 6
        heartbeat_interval: 2000
        auto_heartbeat: true
        metadata:
          transport: http
```

插件级 `protocol` 仍用于选择 Polaris connector protocol。服务的 `instance_protocol`
和 endpoint 的 `protocol` 用于选择注册实例 protocol。

规则：

- endpoint 顺序保持稳定；
- endpoint 地址身份为 `host:port`；共享同一地址的 endpoint 必须分别配置不同的显式
  `instance_id`；
- 显式 instance ID 必须唯一；
- endpoint metadata 覆盖 service metadata；
- endpoint 未配置时继承服务的 `version`、`weight`、`token`、`ttl`、
  `heartbeat_interval`、`auto_heartbeat`、`disable_health_check` 和位置信息；
- 每个显式 endpoint 都必须配置 `address`；
- `endpoints` 不能与 `bind_address` 或 `prefer_bind_address` 同时使用；
- instance protocol 长度不能超过 32 个字符。

启用 `auto_heartbeat: true` 时，适配层会按 instance ID 调度心跳上报。
当 tRPC 健康状态为 `NotServing` 或 `Unknown` 时暂停，状态恢复为 `Serving` 后立即恢复。
因此始终使用继承或显式配置的 `heartbeat_interval`。

省略 `endpoints` 时，保持原单实例行为不变。
当 `register_self=false` 时，仅心跳模式必须配置 `instance_id`；
如果确实需要 heartbeat-only 且没有 instance ID，请设置 `disable_health_check=true`。


## 复用 Registry SDK Context

`FromFramework` 返回 tRPC-Go 插件创建的 registry。注册侧扩展应复用它持有的
`SDKContext`，不要创建第二个 Polaris SDK Context。

```go
r, ok := polarisregistry.FromFramework("trpc.app.server.Service")
if !ok {
    return fmt.Errorf("polaris registry is not initialized")
}
sdkCtx := r.SDKContext()
instanceVersion := r.Version()
cfg := r.Config() // 返回防御性复制，用于扩展层准备。
// 使用 sdkCtx 构建扩展 API，并把 sdkCtx 传给适配层。
```

关闭顺序应为：停止接收新请求，让 tRPC 反注册实例，调用
`UnregisterMetadataProvider`，最后释放适配层自有资源。Provider 不会被缓存，
因此后续注册会获取新的快照。
