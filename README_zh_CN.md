[English](README.md) | 中文

包括了“服务注册、服务发现、负载均衡、熔断器”等组件，通过框架配置可以在 tRPC-Go 框架内部使用，也可以整体使用。

## 如何使用
引入北极星插件

```go
import (
    _ "trpc.group/trpc-go/trpc-naming-polarismesh"
)
```
按后面章节配置 yaml。

## 服务寻址
### `tRPC-Go 框架内（tRPC-Go 服务）`寻址
```go
import (
    _ "trpc.group/trpc-go/trpc-naming-polarismesh"
)

func main() {
    opts := []client.Option{
        // 命名空间，不填写默认使用本服务所在环境 namespace
        client.WithNamespace("Development"),
        // 服务名
        client.WithServiceName("trpc.app.server.service"),
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

### 获取被调 ip
```go
import (
    "trpc.group/trpc-go/trpc-go/naming/registry"

    _ "trpc.group/trpc-go/trpc-naming-polarismesh"
)

func main() {
    node := &registry.Node{}
    opts := []client.Option{
        client.WithNamespace("Development"),
        client.WithServiceName("trpc.app.server.service"),
        // 传入被调 node
        client.WithSelectorNode(node),
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
    // 打印被调节点
    log.Infof("remote server ip: %s", node)

    log.Info("req:%v, rsp:%v, err:%v", req, rsp, err)
}
```

### 服务注册

[link](./registry)

### 负载均衡

一致性 hash 或者普通 hash 负载均衡方式使用如下：

[link](./loadbalance)

### 熔断器

[link](./circuitbreaker)

## 指定环境请求
在关闭服务路由的前提下，可以通过设置环境名来指定请求路由到具体某个环境。
```go
opts := []client.Option{
    // 命名空间，不填写默认使用本服务所在环境 namespace
    client.WithNamespace("Development"),
    // 服务名
    // client.WithTarget("polarismesh://trpc.app.server.service"),
    client.WithServiceName("trpc.app.server.service"),
    // 设置被调服务环境
    client.WithCalleeEnvName("62a30eec"),
    // 关闭服务路由
    client.WithDisableServiceRouter()
}
```

## 在`其他框架或者服务`使用进行寻址
[link](./selector)

## 配置完整示例
`registry` 为服务注册相关的配置(更详细的可以参考[./registry/README.md](./registry/README.md))，`selector`为服务寻址相关的配置。

```yaml
plugins:  # 插件配置
  registry:
    polarismesh:                              # 北极星名字注册服务的配置
      # register_self: true               # 是否进行服务自注册, 默认为 false
      heartbeat_interval: 3000            # 名字注册服务心跳上报间隔
      protocol: grpc                      # 名字服务远程交互协议类型
      # service:                          # 需要进行注册的各服务信息
      #   - name: trpc.server.Service1    # 服务名1, 一般和 trpc_go.yaml 中 server config 处的各个 service 一一对应
      #     namespace: Development        # 该服务需要注册的命名空间
      #     token: xxxxxxxxxxxxxxxxxxx    # 前往北极星控制台进行申请或查看
      #     # （可选）用于服务的心跳上报或反注册。
      #     # 当 register_self 为 true 时，这里的配置无效，插件会使用注册返回的 instance_id 覆盖这里的值。
      #     # 当 register_self 为 false 时，需要指定 instance_id。
      #     instance_id: yyyyyyyyyyyyyyyy
      #     weight: 100                   # 设置权重
      #     bind_address: eth1:8080       # （可选）指定服务监听地址，默认采用 service 中的地址
      #     metadata:                     # 注册时自定义 metadata
      #       internal-enable-set: Y      # 启用 set (本行和下行都需要设置才能完整启用 set)
      #       internal-set-name: xx.yy.sz # 设置服务 set 名
      #       key1: val1                  # 其他的 metadata 等, 可以参考北极星相关文档
      #       key2: val2
      # debug: true                       # 开启调试模式, 默认为 false
      # address_list: ip1:port1,ip2:port2 # 北极星服务的地址
      # connect_timeout: 1000             # 单位 ms，默认 1000ms，连接北极星后台服务的超时时间
      # message_timeout: 1s               # 类型为 time.Duration，从北极星后台接收一个服务信息的超时时间，默认为 1s
      # join_point: default               # 名字服务使用的接入点，该选项会覆盖 address_list 和 cluster_service
      # instance_location:                # 注册实例的地址位置信息
      #   region: China
      #   zone: Guangdong
      #   campus: Shenzhen
      
  selector:   # 针对 trpc 框架服务发现的配置
    polarismesh:  # 北极星服务发现的配置
      # debug: true                       # 开启 debug 日志
      # default: true                     # 是否设置为默认的 selector
      # enable_canary: false              # 开启金丝雀功能，默认 false 不开启
      # timeout: 1000                     # 单位 ms，默认 1000ms，北极星获取实例接口的超时时间
      # report_timeout: 1ms               # 默认 1ms，如果设置了，则下游超时，并且少于设置的值，则忽略错误不上报
      # connect_timeout: 1000             # 单位 ms，默认 1000ms，连接北极星后台服务的超时时间
      # message_timeout: 1s               # 类型为 time.Duration，从北极星后台接收一个服务信息的超时时间，默认为 1s
      # log_dir: $HOME/polarismesh/log        # 北极星日志目录
      protocol: grpc                      # 名字服务远程交互协议类型
      # join_point: default               # 接入名字服务使用的接入点，该选项会覆盖 address_list 和 cluster_service
      # address_list: ip1:port1,ip2:port2 # 北极星服务的地址
      # enable_servicerouter: true        # 是否开启服务路由，默认开启
      # persistDir: $HOME/polarismesh/backup  # 服务缓存持久化目录，按照服务维度将数据持久化到磁盘
      # service_expire_time: 24h          # 服务缓存的过期淘汰时间，类型为 time.Duration，如果不访问某个服务的时间超过这个时间，就会清除相关服务的缓存
      # loadbalance:
      #   name:  # 负载均衡类型，以下值可以采用 https://github.com/polarismesh/polaris-go/blob/v1.5.2/pkg/config/default.go#L181 中以 `DefaultLoadBalancer` 开头变量对应的任意字符串
      #     - polaris_wr         # 加权随机，如果默认设置为寻址方式，则数组的第一个则为默认的负载均衡
      #     - polaris_hash       # hash 算法
      #     - polaris_ring_hash  # 一致性 hash 算法
      #     - polaris_dwr        # 动态权重
      #  details:                # 各类负载均衡的具体配置
      #    polaris_ring_hash:    # 负载均衡名，见上面的 name，目前只支持对 polaris_ring_hash 参数进行配置
      #      vnodeCount: 1024    # 将 ring hash 中虚拟节点的数量配置为 1024，省略时，默认取 10
      # discovery:
      #   refresh_interval: 10000  # 刷新间隔，毫秒
      # cluster_service:
      #   discover: polaris.discover         # 修改发现 server 集群名
      #   health_check: polaris.healthcheck  # 修改心跳 server 集群名
      #   monitor: polaris.monitor           # 修改监控 server 集群名
      # circuitbreaker:
      #   checkPeriod: 30s               # 实例定时熔断检测周期，默认值：30s
      #   requestCountAfterHalfOpen: 10  # 熔断器半开后最大允许的请求数，默认值：10
      #   sleepWindow: 30s               # 熔断器打开后，多久后转换为半开状态，默认值：30s
      #   successCountAfterHalfOpen: 8   # 熔断器半开到关闭所必须的最少成功请求数，默认值：8
      #   chain:                         # 熔断策略，默认值：[errorCount, errorRate]
      #     - errorCount                 # 基于周期连续错误数熔断
      #     - errorRate                  # 基于周期错误率的熔断
      #   errorCount:
      #     continuousErrorThreshold: 10  # 触发连续错误熔断的阈值，默认值：10
      #     metricNumBuckets: 10          # 连续错误数的最小统计单元数量，默认值：10
      #     metricStatTimeWindow: 1m0s    # 连续失败的统计周期，默认值：1m
      #   errorRate:
      #     metricNumBuckets: 5         # 错误率熔断的最小统计单元数量，默认值：5
      #     metricStatTimeWindow: 1m0s  # 错误率熔断的统计周期，默认值：1m
      #     requestVolumeThreshold: 10  # 触发错误率熔断的最低请求阈值，默认值：10
      # service_router:
      #   nearby_matchlevel: zone        # 就近路由的最小匹配级别，包括 region（大区）、zone（区域）、campus（园区）, 默认为 zone
      #   percent_of_min_instances: 0.2  # 全死全活的最小健康实例例判断阈值，值的范围为 [0,1] 之间，默认为 0，即只有当所有实例都不健康时，才开启全死全活
      #   need_return_all_nodes: false # 是否将所有节点展开成 registry.Node 返回，默认不展开，只在 metadata 中填充原始数据，防止节点过多影响性能
      # instance_location:                # 注册实例的地址位置信息
      #   region: China
      #   zone: Guangdong
      #   campus: Shenzhen

      ## WithTarget 模式下，trpc 协议透传字段传递给北极星用于 meta 匹配的开关
      ## 开启设置，则将'selector-meta-'前缀的透传字段摘除前缀后，填入 SourceService 的 MetaData，用于北极星规则匹配
      ## 示例：透传字段 selector-meta-key1:val1 则传递给北极星的 meta 信息为 key1:val1
      # enable_trans_meta: true                        
```

## 北极星官方文档
https://polarismesh.cn/docs

## 多 selector 支持

导入了 `trpc-naming-polarismesh` 包后，会自动注册一个名为 `"polarismesh"` 的 selector 插件，假如用户希望在请求另一地域服务时采用不同的 selector 配置，可以通过注册一个新的 selector 插件来实现，示例如下：

### 方法一：代码+配置

#### 代码编写

两种方式二选一 (不要混用):

1. 使用 service name 寻址
2. 使用 target 方式寻址

```golang 
import (
    "trpc.group/trpc-go/trpc-go/plugin"
    "trpc.group/trpc-go/trpc-naming-polarismesh"
)

func init() {
    plugin.Register("polarismesh-customized1", &naming.SelectorFactory{})
    plugin.Register("polarismesh-customized2", &naming.SelectorFactory{})
}

// 使用 service name 方式来寻址
// 这些 client option 也可以在 trpc_go.yaml 中的 client service 部分进行配置
func CallWithServiceName(ctx context.Context) error {
    // 使用 "polarismesh-customized1" 的插件配置来访问下游
    rsp, err := proxy.Invoke(ctx, req,
        client.WithServiceName("trpc.app.server.service"),
        // 以下四个 option 需要并用才能使 polarismesh-customized1 真正生效
        client.WithDiscoveryName("polarismesh-customized1"),
        client.WithServiceRouterName("polarismesh-customized1"),
        client.WithBalancerName("polaris_wr"), // 填默认 selector 配置中的负载均衡器
        client.WithCircuitBreakerName("polarismesh-customized1"),
    )
    if err != nil { return err }
    // 使用 "polarismesh-customized2" 的插件配置来访问下游
    rsp, err = proxy.Invoke(ctx, req,
        client.WithServiceName("trpc.app.server.service"),
        // 以下四个 option 需要并用才能使 polarismesh-customized2 真正生效
        client.WithDiscoveryName("polarismesh-customized2"),
        client.WithServiceRouterName("polarismesh-customized2"),
        client.WithBalancerName("polaris_wr"), // 填默认 selector 配置中的负载均衡器
        client.WithCircuitBreakerName("polarismesh-customized2"),
    )
    if err != nil { return err }
}

// 使用 target 方式来寻址
// 这些 client option 也可以在 trpc_go.yaml 中的 client service 部分进行配置
func CallWithTarget(ctx context.Context) error {
    // 使用 "polarismesh-customized1" 的插件配置来访问下游
    rsp, err := proxy.Invoke(ctx, req,
        client.WithTarget("polarismesh-customized1://trpc.app.server.service"))
    if err != nil { return err }
    // 使用 "polarismesh-customized2" 的插件配置来访问下游
    rsp, err = proxy.Invoke(ctx, req,
        client.WithTarget("polarismesh-customized2://trpc.app.server.service"))
    if err != nil { return err }
}
```

注：以上所有的这些 option 也可以在 `trpc_go.yaml` 中的 `client.service` 中进行配置，同时存在时，代码 option 的优先级高于配置

以下方式也是二选一, 一定不要混用

* 使用 service name 方式来寻址

```yaml
client:  # 客户端调用的后端配置
  service:  # 针对单个后端的配置
    - name: trpc.app.server.service  # 后端服务的 service name
      discovery: polarismesh-customized1
      servicerouter: polarismesh-customized1
      loadbalance: polaris_wr # 填默认 selector 配置中的负载均衡器
      circuitbreaker: polarismesh-customized1
      network: tcp  # 后端服务的网络类型 tcp udp 配置优先
      protocol: trpc  # 应用层协议 trpc http
      timeout: 1000   # 请求最长处理时间
```

* 使用 target 方式来寻址

```yaml
client:  # 客户端调用的后端配置
  service:  # 针对单个后端的配置
    - name: trpc.app.server.service  # 后端服务的 service name
      network: tcp  # 后端服务的网络类型 tcp udp 配置优先
      protocol: trpc  # 应用层协议 trpc http
      target: polarismesh-customized1://trpc.app.server.service  # 请求服务地址
      timeout: 1000   # 请求最长处理时间
```

#### 配置编写

```yaml
plugins:                                             
  selector:                                         
    polarismesh:
      protocol: grpc
      default: true # 设置为默认的 selector
      join_point: default
      # 不同 selector 最好把 persistDir 写为不同的值以免互相干扰
      persistDir: $HOME/polarismesh/backup  # 服务缓存持久化目录，按照服务维度将数据持久化到磁盘
      # log_dir 只需设置一个即可, 以为北极星 sdk 的日志是全局唯一的, 多个 log_dir 存在时, 后者会覆盖前者
      log_dir: $HOME/polarismesh/log        # 北极星日志目录
      # loadbalance: # 负载均衡器和多 selector 的关联较弱, 只在设置为 default=true 的 selector 中进行配置即可
      #   name: # 负载均衡类型
      #     - polaris_wr         # 加权随机，如果默认设置为寻址方式，则数组的第一个则为默认的负载均衡
      #     - polaris_hash       # hash 算法
      #     - polaris_ring_hash  # 一致性 hash 算
      # 任何其他的配置，见 `配置完整示例` 一节
    polarismesh-customized1:
      protocol: grpc
      default: false # 不设置为默认的 selector
      join_point: point1
      # 不同 selector 最好把 persistDir 写为不同的值以免互相干扰
      persistDir: $HOME/polarismesh-customized1/backup  # 服务缓存持久化目录，按照服务维度将数据持久化到磁盘
      # 任何其他的配置，见 `配置完整示例` 一节
    polarismesh-customized2:
      protocol: grpc
      default: false # 不设置为默认的 selector
      join_point: point2
      # 不同 selector 最好把 persistDir 写为不同的值以免互相干扰
      persistDir: $HOME/polarismesh-customized2/backup  # 服务缓存持久化目录，按照服务维度将数据持久化到磁盘
      # 任何其他的配置，见 `配置完整示例` 一节
```

注意: 存在多个 selector 时, 需要明确指定哪一个 selector 为默认的 `default: true`, 并且把其他非默认的 selector 手动配上 `default: false`, 在使用 WithServiceName 方式时一定要把 `WithDiscoveryName, WithServiceRouterName, WithBalancerName, WithCircuitBreakerName` 这四个 option 都带上

### 方法二：纯代码

```golang

import (
    "trpc.group/trpc-go/trpc-naming-polarismesh"
)

func init() {
	addrs := "xxx,yyy"
    logDir1 := "polarismesh-customized1/log"
    persistDir1 := "polarismesh-customized1/backup"
    dft1 := true
    if err := naming.SetupWithConfig(&naming.Config{
        Name: "polarismesh-customized1",
        AddressList: addrs,
        Default: &dft1, // 设置为默认的
        // 使用 client.WithServiceName 方式寻址时
        // 至少需要在默认的 selector 下有 loadbalancer 配置
        Loadbalance: naming.LoadbalanceConfig{Name: []string{"polaris_ws"}},
        LogDir: &logDir1,
        PersistDir: &persistDir1,
        // 可以加上其他任意你想加的配置
    }); err != nil { /* 错误处理 */ }

	addrs2 := "zzz"
    persistDir2 := "polarismesh-customized2/backup"
    dft2 := false
    if err := naming.SetupWithConfig(&naming.Config{
        Name: "polarismesh-customized2",
        AddressList: addrs2,
        Default: &dft2, // 设置为非默认的
        PersistDir: &persistDir2,
        // 可以加上其他任意你想加的配置
    }); err != nil { /* 错误处理 */ }
}

// 使用 service name 方式来寻址
func CallWithServiceName(ctx context.Context) error {
    // 使用 "polarismesh-customized1" 的插件配置来访问下游
    rsp, err := proxy.Invoke(ctx, req,
        client.WithServiceName("trpc.app.server.service"),
        client.WithDiscoveryName("polarismesh-customized1"),
        client.WithServiceRouterName("polarismesh-customized1"),
        client.WithBalancerName("polaris_wr"), // 填默认 selector 配置中的负载均衡器
        client.WithCircuitBreakerName("polarismesh-customized1"),
    )
    if err != nil { return err }
    // 使用 "polarismesh-customized2" 的插件配置来访问下游
    rsp, err = proxy.Invoke(ctx, req,
        client.WithServiceName("trpc.app.server.service"),
        client.WithDiscoveryName("polarismesh-customized2"),
        client.WithServiceRouterName("polarismesh-customized2"),
        client.WithBalancerName("polaris_wr"), // 填默认 selector 配置中的负载均衡器
        client.WithCircuitBreakerName("polarismesh-customized2"),
    )
    if err != nil { return err }
}

// 使用 target 方式来寻址
func CallWithTarget(ctx context.Context) error {
    // 使用 "polarismesh-customized1" 的插件配置来访问下游
    rsp, err := proxy.Invoke(ctx, req,
        client.WithTarget("polarismesh-customized1://trpc.app.server.service"))
    if err != nil { return err }
    // 使用 "polarismesh-customized2" 的插件配置来访问下游
    rsp, err = proxy.Invoke(ctx, req,
        client.WithTarget("polarismesh-customized2://trpc.app.server.service"))
    if err != nil { return err }
}
```

注：trpc 插件可能不支持部分北极星新功能的配置，此时业务可自行创建北极星 sdk 配置并通过 naming.Config.PolarisConig 字段提供给 trpc 插件。此配置会被视为基础配置，其他通过trpc标准接口添加的配置项会覆盖此配置的对应配置项，最后使用配置创建北极星 API 对象。

```golang
// 创建北极星配置文件
cfg := api.NewConfiguration()
// 添加北极星埋点、限流 Server、等其他配置
addresses := []string{"127.0.0.1:8081"}
cfg.GetGlobal().GetServerConnector().SetAddresses(addresses)
cfg.GetProvider().GetRateLimit().GetRateLimitCluster().SetService("polarismesh.metric.v2.test")
// 初始化
if err := naming.SetupWithConfig(&naming.Config{
    Name: "polarismesh-customized1",
    Loadbalance: naming.LoadbalanceConfig{Name: []string{"polaris_ws"}},
    PolarisConfig: cfg,
}); err != nil { /* 错误处理 */ }
```

## `client.WithServiceName` 寻址与 `client.WithTarget` 寻址的区别以及 `enable_servicerouter` 的语义

### 两种寻址方式的严格定义

满足 `client.WithServiceName` 寻址的充要条件 (两条必须同时满足):

* 未使用 `client.WithTarget` option
* `trpc_go.yaml` 中 `client.service` 没有配置 `target` 字段

满足 `client.WithTarget` 寻址的充分条件 (两个条件二选一, 同时存在时, 代码 option 的优先级高于配置):

* 使用了 `client.WithTarget` option
* `trpc_go.yaml` 中 `client.service` 配置了 `target` 字段

上面没有提到 `client.WithServiceName` option 或者配置中的 `name` 字段, 是因为这两个东西是始终存在的, 不是区分这两种寻址方式的必要因素

### 北极星插件视角下的两种寻址方式

#### `WithServiceName`

期望通过 `WithServiceName` 的方式来完成北极星寻址的话, 需要同时满足以下几个条件:

* 正确配置本插件: 1. 包含匿名 import, 2. 插件配置中有 polaris mesh selector
* 代码 option 不要带 `client.WithTarget`, `trpc_go.yaml` 的客户端配置中也不要带 `target` 字段

这样就实现了 `WithServiceName` 的寻址方式, 此时你会发现除了插件配置中的 polaris mesh selector 有北极星相关信息, 客户端配置中任何地方不再需要有 polaris 字样, 但是实际确是使用的北极星插件能力进行的寻址, 这种现象的原因是北极星插件替换了 trpc-go 框架中的一些默认组件为北极星插件的实现, 导致客户端以几乎无感知的形式完成北极星寻址

#### `WithTarget`

期望通过 `WithTarget` 的方式来完成北极星寻址的话, 需要同时满足以下条件:

* 正确配置本插件: 1. 包含匿名 import, 2. 插件配置中有 polaris mesh selector
* 二选一 (同时存在时, 代码 option 的优先级高于配置):
  * 代码 option 带 `client.WithTarget("polarismesh://trpc.app.server.service")`
  * `trpc_go.yaml` 的客户端配置中带 `target` 字段: `target: polarismesh://trpc.app.server.service`

这样就实现了 `WithTarget` 的寻址方式, 这里你会在 `target` 处看到明确的 polaris 字样, 明确地感知到这个客户端在使用北极星寻址

### 两种寻址方式的区别

下图展示了 `WithServiceName` 以及 `WithTarget` 实际使用的 selector

```bash
"trpc.app.server.service"   =>  (trpc-go).selector.TrpcSelector.Selector        => ip:port  # WithServiceName
"trpc.app.server.service"   =>  (trpc-naming-polarismesh).selector.Selector.Select  => ip:port  # WithTarget
```

在配置了北极星 selector 插件之后, `(trpc-go).selector.TrpcSelector.Selector` 内部使用到的 `discovery, servicerouter, loadbalance` 这三个模块会被替换为北极星插件自己的实现, 所以实际的效果其实为:

```bash
"trpc.app.server.service" =>  (trpc-naming-polarismesh).discovery.Discovery.List
                           =>  (trpc-naming-polarismesh).servicerouter.ServiceRouter.Filter        
                            =>  (trpc-naming-polarismesh).loadbalance.WRLoadBalancer.Select => ip:port  # WithServiceName

"trpc.app.server.service"   =>  (trpc-naming-polarismesh).selector.Selector.Select          => ip:port  # WithTarget
```

也就是说: `WithServiceName` 最终使用的是北极星插件的 `discovery, servicerouter, loadbalance` 三模块组合, 而 `WithTarget` 最终使用的是北极星插件的 `selector` 模块

但是北极星插件的 `selector` 模块并不是像 `TrpcSelector` 那样把前三个模块给拼合起来, 而是自己的一套逻辑, 这导致了 `WithServiceName` 和 `WithTarget` 这种使用方式的差异性, 即: 

||`WithServiceName`|`WithTarget`|
|-|-|-|
|使用的北极星插件组件|`discovery, servicerouter, loadbalance`|`selector`|

#### 区别速查

|特性|`WithServiceName`<br>`enable_servicerouter=true`|`WithTarget`<br>`enable_servicerouter=true`|`WithServiceName`<br>`enable_servicerouter=false`|`WithTarget`<br>`enable_servicerouter=false`|
|---|------------------------------------------------|-------------------------------------------|-------------------------------------------------|---------------------------------------------|
|使用主调北极星出规则|                                <font color="green">Y</font>|   <font color="green">Y</font>|        <font color="red">N</font>|         <font color="red">N</font>|
|可以启用 `EnableTransMeta`|                        <font color="red">N</font>|     <font color="green">Y</font>|        <font color="red">N</font>|         <font color="red">N</font>|
|设置源 env name 到 metadata['env'] 上|             <font color="green">Y</font>|    <font color="green">Y</font>|        <font color="red">N</font>|         <font color="red">N</font>|
|设置目标 env name 到 metadata['env'] 上|            <font color="red">N</font>|       <font color="red">N</font>|        <font color="green">Y</font>|         <font color="green">Y</font>|
|使用源 metadata 进行路由 (不包括 'env' 以及 'set')|   <font color="green">Y</font>|   <font color="green">Y</font>|        <font color="red">N</font>|         <font color="red">N</font>|
|使用目标 metadata 进行路由 (不包括 'env' 以及 'set')| <font color="red">N</font>|      <font color="green">Y</font>|        <font color="red">N</font>|         <font color="green">Y</font>|
|使用 `client.WithEnvKey` 设置源 metadata['key']|   <font color="green">Y</font>|      <font color="red">N</font>|        <font color="red">N</font>|         <font color="red">N</font>|
|使用 `client.WithEnvTransfer` 来进行路由|           <font color="green">Y</font>|      <font color="red">N</font>|        <font color="red">N</font>|         <font color="red">N</font>|
|金丝雀路由|                                        <font color="green">Y</font>|    <font color="green">Y</font>|        <font color="green">Y</font>|         <font color="green">Y</font>|
|set 路由|                                         <font color="green">Y</font>|      <font color="red">N</font>|        <font color="green">Y</font>|         <font color="red">N</font>|

注意事项:

* `enable_servicerouter` 是北极星 selector 插件提供的配置项, trpc-go 客户端有 `client.WithDisableServiceRouter` 的 option, 以及 `disable_servicerouter` 的配置项, 和北极星插件这里的配置项是对应的 (你可以理解 trpc-go 框架专门为北极星插件创造了一个 option), 区别在于 trpc-go 框架提供的 option 以及配置可以控制每个客户端, 而北极星 selector 插件的 `enable_servicerouter` 配置则是这个 selector 全局的
* 而对于 `enable_servicerouter` 正确语义的解读, 大概为: `enable_servicerouter=true` 时, 启用源服务北极星出规则 (这就要求源服务必须在北极星上有注册)
* 注意 set 路由只在 `WithServiceName` 寻址方式下生效
* 注意目标 metadata 路由只在 `WithTarget` 寻址方式下生效
* 注意 `EnableTransMeta` 只在 `WithTarget` 并且 `enable_servicerouter=true` 的情况下生效
* 配这些东西的时候, 一定要注意 `trpc_go.yaml` 本身的配置是否生效的问题, 不仅仅是 `trpc_go.yaml` 内容是否被框架正确读取的问题 (当然你要保证他是正确读取的, 比如更新完配置后你需要确保有重新加载), 还要注意某个客户端配置是否真正地被使用到, 因为客户端配置是以 callee proto name 为 key 来存放在框架中的, 所以当 service name 和 callee proto name 不一致时, 需要在客户端配置中明确写出 `name` 和 `callee` 两个字段
* 如果出现问题, 优先使用代码中的 option 把你需要指定的内容都指定一遍, 以排除配置不生效的问题