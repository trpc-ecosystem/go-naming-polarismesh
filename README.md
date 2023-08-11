# tRPC-Go 北极星名字服务插件
[![BK Pipelines Status](https://api.bkdevops.qq.com/process/api/external/pipelines/projects/pcgtrpcproject/p-8dcb44954edd4414923421fafce08e48/badge?X-DEVOPS-PROJECT-ID=pcgtrpcproject)](http://devops.oa.com:/ms/process/api-html/user/builds/projects/pcgtrpcproject/pipelines/p-8dcb44954edd4414923421fafce08e48/latestFinished?X-DEVOPS-PROJECT-ID=pcgtrpcproject) [![Coverage](https://tcoverage.woa.com/api/getCoverage/getTotalImg/?pipeline_id=p-8dcb44954edd4414923421fafce08e48)](http://macaron.oa.com/api/coverage/getTotalLink/?pipeline_id=p-8dcb44954edd4414923421fafce08e48) [![GoDoc](https://img.shields.io/badge/API%20Docs-GoDoc-green)](http://godoc.woa.com/git.woa.com/trpc-go/trpc-naming-polaris)

包括了“服务注册、服务发现、负载均衡、熔断器”等组件，通过框架配置可以在 tRPC-Go 框架内部使用，也可以整体使用。

文档也可以查看 https://iwiki.woa.com/pages/viewpage.action?pageId=284289117 第五章。

北极星已经打通 l5、ons 寻址，建议使用北极星插件寻址。
l5 寻址插件也可以使用：[trpc-selector-cl5](https://git.woa.com/trpc-go/trpc-selector-cl5)
ons 寻址插件也可以使用：[trpc-selector-ons](https://git.woa.com/trpc-go/trpc-selector-ons)
cmlb 寻址插件也可以使用：[trpc-selector-cmlb](https://git.woa.com/trpc-go/trpc-selector-cmlb)

- [tRPC-Go 北极星名字服务插件](#trpc-go-北极星名字服务插件)
  - [123 平台部署默认`不需要做任何配置`，只需引入即可。](#123-平台部署默认不需要做任何配置只需引入即可)
    - [!`注意`， `l5, ons` 的 namespace 为 `Production`，且必须关闭服务路由，如下：](#注意-l5-ons-的-namespace-为-production且必须关闭服务路由如下)
  - [服务寻址](#服务寻址)
    - [`tRPC-Go 框架内（tRPC-Go 服务）`寻址](#trpc-go-框架内trpc-go-服务寻址)
    - [获取被调 ip](#获取被调-ip)
    - [服务注册](#服务注册)
    - [负载均衡](#负载均衡)
    - [熔断器](#熔断器)
    - [熔断探活](#熔断探活)
  - [指定环境请求](#指定环境请求)
  - [多环境路由](#多环境路由)
  - [在`其他框架或者服务`使用进行寻址](#在其他框架或者服务使用进行寻址)
  - [配置完整示例](#配置完整示例)
  - [l5 ons 已经打通北极星](#l5-ons-已经打通北极星)
  - [北极星服务发现详细文档](#北极星服务发现详细文档)
  - [北极星相关插件 mock 命令](#北极星相关插件-mock-命令)
  - [多 selector 支持](#多-selector-支持)
    - [方法一：代码+配置](#方法一代码配置)
      - [代码编写](#代码编写)
      - [配置编写](#配置编写)
    - [方法二：纯代码](#方法二纯代码)
  - [动态权重支持](#动态权重支持)
    - [服务端支持](#服务端支持)
      - [在北极星平台为服务启用动态权重](#在北极星平台为服务启用动态权重)
      - [配置文件修改](#配置文件修改)
      - [代码变更](#代码变更)
    - [客户端支持](#客户端支持)
      - [配置文件修改](#配置文件修改-1)
  - [私有化部署注意事项](#私有化部署注意事项)
  - [`client.WithServiceName` 寻址与 `client.WithTarget` 寻址的区别以及 `enable_servicerouter` 的语义](#clientwithservicename-寻址与-clientwithtarget-寻址的区别以及-enable_servicerouter-的语义)
    - [两种寻址方式的严格定义](#两种寻址方式的严格定义)
    - [北极星插件视角下的两种寻址方式](#北极星插件视角下的两种寻址方式)
      - [`WithServiceName`](#withservicename)
      - [`WithTarget`](#withtarget)
    - [两种寻址方式的区别](#两种寻址方式的区别)
      - [区别速查](#区别速查)

## 123 平台部署默认`不需要做任何配置`，只需引入即可。
引入北极星插件

```go
import (
    _ "trpc.group/trpc-go/trpc-naming-polaris"
)
```
如果需要在其他平台（例如织云）使用请参考最后的服务完整配置实例。

### !`注意`， `l5, ons` 的 namespace 为 `Production`，且必须关闭服务路由，如下：

```go
opts := []client.Option{
    client.WithNamespace("Production"),
    // trpc-go 框架内部使用
    client.WithServiceName("12587:65539"),
    // 纯客户端或者其他框架中使用 trpc-go 框架的 client
    // client.WithTarget("polaris://12587:65539"),
    client.WithDisableServiceRouter(),
}
```

## 服务寻址
### `tRPC-Go 框架内（tRPC-Go 服务）`寻址
```go
import (
    _ "trpc.group/trpc-go/trpc-naming-polaris"
)

func main() {
    opts := []client.Option{
        // 命名空间，不填写默认使用本服务所在环境 namespace
        // l5, ons namespace 为 Production
        client.WithNamespace("Development"),
        // 服务名
        // l5 为 sid
        // ons 为 ons name
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

    _ "trpc.group/trpc-go/trpc-naming-polaris"
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
关于如何关闭服务路由可查看 [多环境路由](https://iwiki.oa.tencent.com/pages/viewpage.action?pageId=99485673) 。
```go
opts := []client.Option{
    // 命名空间，不填写默认使用本服务所在环境 namespace
    // l5, ons namespace 为 Production
    client.WithNamespace("Development"),
    // 服务名
    // l5 为 sid
    // ons 为 ons name
    // client.WithTarget("polaris://trpc.app.server.service"),
    client.WithServiceName("trpc.app.server.service"),
    // 设置被调服务环境
    client.WithCalleeEnvName("62a30eec"),
    // 关闭服务路由
    client.WithDisableServiceRouter()
}
```

## 多环境路由 

https://iwiki.oa.tencent.com/pages/viewpage.action?pageId=99485673

## 在`其他框架或者服务`使用进行寻址
[link](./selector)

## 配置完整示例
`registry` 为服务注册相关的配置(更详细的可以参考[./registry/README.md](./registry/README.md))，`selector`为服务寻址相关的配置。

```yaml
plugins:  # 插件配置
  registry:
    polaris:                              # 北极星名字注册服务的配置
      # register_self: true               # 是否进行服务自注册, 默认为 false, 交由 123 平台注册 (非 123 平台的话一般这里要改为 true)
      heartbeat_interval: 3000            # 名字注册服务心跳上报间隔
      protocol: grpc                      # 名字服务远程交互协议类型
      # service:                          # 需要进行注册的各服务信息
      #   - name: trpc.server.Service1    # 服务名1, 一般和 trpc_go.yaml 中 server config 处的各个 service 一一对应
      #     namespace: Development        # 该服务需要注册的命名空间, 分正式 Production 和非正式 Development 两种类型
      #     token: xxxxxxxxxxxxxxxxxxx    # 前往 https://polaris.woa.com/ 进行申请或查看
      #     instance_id: yyyyyyyyyyyyyyyy # （可选）服务注册所需要的，instance_id=XXX(namespace+service+host+port)
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
    polaris:  # 北极星服务发现的配置
      # debug: true                       # 开启 debug 日志
      # default: true                     # 是否设置为默认的 selector
      # enable_canary: false              # 开启金丝雀功能，默认 false 不开启
      # timeout: 1000                     # 单位 ms，默认 1000ms，北极星获取实例接口的超时时间
      # report_timeout: 1ms               # 默认 1ms，如果设置了，则下游超时，并且少于设置的值，则忽略错误不上报
      # connect_timeout: 1000             # 单位 ms，默认 1000ms，连接北极星后台服务的超时时间
      # message_timeout: 1s               # 类型为 time.Duration，从北极星后台接收一个服务信息的超时时间，默认为 1s
      # log_dir: $HOME/polaris/log        # 北极星日志目录
      protocol: grpc                      # 名字服务远程交互协议类型
      # join_point: default               # 接入名字服务使用的接入点，该选项会覆盖 address_list 和 cluster_service
      # address_list: ip1:port1,ip2:port2 # 北极星服务的地址
      # enable_servicerouter: true        # 是否开启服务路由，默认开启
      # persistDir: $HOME/polaris/backup  # 服务缓存持久化目录，按照服务维度将数据持久化到磁盘
      # service_expire_time: 24h          # 服务缓存的过期淘汰时间，类型为 time.Duration，如果不访问某个服务的时间超过这个时间，就会清除相关服务的缓存
      # loadbalance:
      #   name: # 负载均衡类型, 在 v0.4.3 之后, 以下值可以采用 https://git.woa.com/polaris/polaris-go/blob/master/pkg/config/default.go#L182 中以 `DefaultLoadBalancer` 开头变量对应的任意字符串
      #     - polaris_wr         # 加权随机，如果默认设置为寻址方式，则数组的第一个则为默认的负载均衡
      #     - polaris_hash       # hash 算法
      #     - polaris_ring_hash  # 一致性 hash 算法
      #     - polaris_dwr        # 动态权重
      #  details:                # 各类负载均衡的具体配置，见 https://mk.woa.com/q/287086
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

## l5 ons 已经打通北极星
- l5
    - namespace: Production
    - serviceName: sid
- ons
    - namespace: Production
    - serviceName: ons name
- cmlb
    - namespace: Production
    - serviceName: cmlb id

## 北极星服务发现详细文档
https://git.woa.com/polaris/polaris/wikis/home

## 多 selector 支持

解决：

* #65
* https://mk.woa.com/q/284778
* https://mk.woa.com/q/287083

首先，确保 `trpc-naming-polaris` 使用版本 >= v0.5.0, 建议直接更新到当前显示的最新版

导入了 `trpc-naming-polaris` 包后，会自动注册一个名为 `"polaris"` 的 selector 插件，假如用户希望在请求另一地域服务时采用不同的 selector 配置，可以通过注册一个新的 selector 插件来实现，示例如下：

### 方法一：代码+配置

#### 代码编写

两种方式二选一 (不要混用):

1. 使用 service name 寻址
2. 使用 target 方式寻址

```golang 
import (
    "trpc.group/trpc-go/trpc-go/plugin"
    "trpc.group/trpc-go/trpc-naming-polaris"
)

func init() {
    plugin.Register("polaris-customized1", &naming.SelectorFactory{})
    plugin.Register("polaris-customized2", &naming.SelectorFactory{})
}

// 使用 service name 方式来寻址
// 这些 client option 也可以在 trpc_go.yaml 中的 client service 部分进行配置
func CallWithServiceName(ctx context.Context) error {
    // 使用 "polaris-customized1" 的插件配置来访问下游
    rsp, err := proxy.Invoke(ctx, req,
        client.WithServiceName("trpc.app.server.service"),
        // 以下四个 option 需要并用才能使 polaris-customized1 真正生效
        client.WithDiscoveryName("polaris-customized1"),
        client.WithServiceRouterName("polaris-customized1"), // 需要 trpc-go 版本 > v0.13.0
        client.WithBalancerName("polaris_wr"), // 填默认 selector 配置中的负载均衡器
        client.WithCircuitBreakerName("polaris-customized1"),
    )
    if err != nil { return err }
    // 使用 "polaris-customized2" 的插件配置来访问下游
    rsp, err = proxy.Invoke(ctx, req,
        client.WithServiceName("trpc.app.server.service"),
        // 以下四个 option 需要并用才能使 polaris-customized2 真正生效
        client.WithDiscoveryName("polaris-customized2"),
        client.WithServiceRouterName("polaris-customized2"), // 需要 trpc-go 版本 > v0.13.0
        client.WithBalancerName("polaris_wr"), // 填默认 selector 配置中的负载均衡器
        client.WithCircuitBreakerName("polaris-customized2"),
    )
    if err != nil { return err }
}

// 使用 target 方式来寻址
// 这些 client option 也可以在 trpc_go.yaml 中的 client service 部分进行配置
func CallWithTarget(ctx context.Context) error {
    // 使用 "polaris-customized1" 的插件配置来访问下游
    rsp, err := proxy.Invoke(ctx, req,
        client.WithTarget("polaris-customized1://trpc.app.server.service"))
    if err != nil { return err }
    // 使用 "polaris-customized2" 的插件配置来访问下游
    rsp, err = proxy.Invoke(ctx, req,
        client.WithTarget("polaris-customized2://trpc.app.server.service"))
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
      discovery: polaris-customized1
      servicerouter: polaris-customized1 # 需要 trpc-go 版本 > v0.13.0, 否则请使用 target 方式来寻址
      loadbalance: polaris_wr # 填默认 selector 配置中的负载均衡器
      circuitbreaker: polaris-customized1
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
      target: polaris-customized1://trpc.app.server.service  # 请求服务地址
      timeout: 1000   # 请求最长处理时间
```

#### 配置编写

```yaml
plugins:                                             
  selector:                                         
    polaris:
      protocol: grpc
      default: true # 设置为默认的 selector
      join_point: default
      # 不同 selector 最好把 persistDir 写为不同的值以免互相干扰
      persistDir: $HOME/polaris/backup  # 服务缓存持久化目录，按照服务维度将数据持久化到磁盘
      # log_dir 只需设置一个即可, 以为北极星 sdk 的日志是全局唯一的, 多个 log_dir 存在时, 后者会覆盖前者
      log_dir: $HOME/polaris/log        # 北极星日志目录
      # loadbalance: # 负载均衡器和多 selector 的关联较弱, 只在设置为 default=true 的 selector 中进行配置即可
      #   name: # 负载均衡类型
      #     - polaris_wr         # 加权随机，如果默认设置为寻址方式，则数组的第一个则为默认的负载均衡
      #     - polaris_hash       # hash 算法
      #     - polaris_ring_hash  # 一致性 hash 算
      # 任何其他的配置，见 `配置完整示例` 一节
    polaris-customized1:
      protocol: grpc
      default: false # 不设置为默认的 selector
      join_point: point1
      # 不同 selector 最好把 persistDir 写为不同的值以免互相干扰
      persistDir: $HOME/polaris-customized1/backup  # 服务缓存持久化目录，按照服务维度将数据持久化到磁盘
      # 任何其他的配置，见 `配置完整示例` 一节
    polaris-customized2:
      protocol: grpc
      default: false # 不设置为默认的 selector
      join_point: point2
      # 不同 selector 最好把 persistDir 写为不同的值以免互相干扰
      persistDir: $HOME/polaris-customized2/backup  # 服务缓存持久化目录，按照服务维度将数据持久化到磁盘
      # 任何其他的配置，见 `配置完整示例` 一节
```

注意: 存在多个 selector 时, 需要明确指定哪一个 selector 为默认的 `default: true`, 并且把其他非默认的 selector 手动配上 `default: false`, 在使用 WithServiceName 方式时一定要把 `WithDiscoveryName, WithServiceRouterName, WithBalancerName, WithCircuitBreakerName` 这四个 option 都带上

### 方法二：纯代码

```golang

import (
    "trpc.group/trpc-go/trpc-naming-polaris"
)

func init() {
	addrs := "xxx,yyy"
    logDir1 := "polaris-customized1/log"
    persistDir1 := "polaris-customized1/backup"
    dft1 := true
    if err := naming.SetupWithConfig(&naming.Config{
        Name: "polaris-customized1",
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
    persistDir2 := "polaris-customized2/backup"
    dft2 := false
    if err := naming.SetupWithConfig(&naming.Config{
        Name: "polaris-customized2",
        AddressList: addrs2,
        Default: &dft2, // 设置为非默认的
        PersistDir: &persistDir2,
        // 可以加上其他任意你想加的配置
    }); err != nil { /* 错误处理 */ }
}

// 使用 service name 方式来寻址
func CallWithServiceName(ctx context.Context) error {
    // 使用 "polaris-customized1" 的插件配置来访问下游
    rsp, err := proxy.Invoke(ctx, req,
        client.WithServiceName("trpc.app.server.service"),
        client.WithDiscoveryName("polaris-customized1"),
        client.WithServiceRouterName("polaris-customized1"), // 需要 trpc-go 版本 > v0.13.0
        client.WithBalancerName("polaris_wr"), // 填默认 selector 配置中的负载均衡器
        client.WithCircuitBreakerName("polaris-customized1"),
    )
    if err != nil { return err }
    // 使用 "polaris-customized2" 的插件配置来访问下游
    rsp, err = proxy.Invoke(ctx, req,
        client.WithServiceName("trpc.app.server.service"),
        client.WithDiscoveryName("polaris-customized2"),
        client.WithServiceRouterName("polaris-customized2"), // 需要 trpc-go 版本 > v0.13.0
        client.WithBalancerName("polaris_wr"), // 填默认 selector 配置中的负载均衡器
        client.WithCircuitBreakerName("polaris-customized2"),
    )
    if err != nil { return err }
}

// 使用 target 方式来寻址
func CallWithTarget(ctx context.Context) error {
    // 使用 "polaris-customized1" 的插件配置来访问下游
    rsp, err := proxy.Invoke(ctx, req,
        client.WithTarget("polaris-customized1://trpc.app.server.service"))
    if err != nil { return err }
    // 使用 "polaris-customized2" 的插件配置来访问下游
    rsp, err = proxy.Invoke(ctx, req,
        client.WithTarget("polaris-customized2://trpc.app.server.service"))
    if err != nil { return err }
}
```

注：trpc 插件可能不支持部分 polaris 新功能的配置，此时业务可自行创建 polaris sdk 配置并通过 naming.Config.PolarisConig 字段提供给 trpc 插件。此配置会被视为基础配置，其他通过trpc标准接口添加的配置项会覆盖此配置的对应配置项，最后使用配置创建 polaris api 对象。

```golang
// 创建北极星配置文件
cfg := api.NewConfiguration()
// 添加北极星埋点、限流Server、等其他配置
addresses := []string{"127.0.0.1:8081"}
cfg.GetGlobal().GetServerConnector().SetAddresses(addresses)
cfg.GetProvider().GetRateLimit().GetRateLimitCluster().SetService("polaris.metric.v2.test")
// 初始化
if err := naming.SetupWithConfig(&naming.Config{
    Name: "polaris-customized1",
    Loadbalance: naming.LoadbalanceConfig{Name: []string{"polaris_ws"}},
    PolarisConfig: cfg,
}); err != nil { /* 错误处理 */ }
```

## 动态权重支持

动态权重的支持需要两部分:

* 服务端进行动态权重的上报
* 客户端使用 `polaris_dwr` 这一个动态权重负载均衡器

### 服务端支持

#### 在北极星平台为服务启用动态权重

服务端支持的话首先要在[北极星平台](polaris.woa.com)为该服务启用动态权重(可参考回答 [http://mk.woa.com/q/287277/answer/107912](http://mk.woa.com/q/287277/answer/107912)): 

1. 根据 [接入流程](https://iwiki.woa.com/pages/viewpage.action?pageId=218886468) 文档申请拿到平台 ID (Platform-Id) 及平台 Token (Platform-Token)

2. 在北极星平台上将服务的平台 ID 关联为上一步申请到的平台 ID (重要, 否则下一步骤无权限)

3. 参考 [文档](https://iwiki.woa.com/pages/viewpage.action?pageId=386636275#id-%E5%8A%A8%E6%80%81%E6%9D%83%E9%87%8D%EF%BC%88%E5%B7%B2%E5%AE%9E%E7%8E%B0%EF%BC%89-%E5%8A%A8%E6%80%81%E6%9D%83%E9%87%8D%E6%9C%AA%E7%94%9F%E6%95%88%EF%BC%8C%E6%9F%A5%E8%AF%A2%E8%BF%94%E5%9B%9E200101) 来进行实际的开启(目前暂时只支持手动开启, 以文档最新内容为准):

手动开启的方法(见其中的 Platform-Id 以及 Platform-Token 的值改为第一步申请到的值, service_token 则是目标服务的 token, service 为目标服务的名字):

```shell
# 查询:
curl -v -H "Platform-Id:xxxxx" -H "Platform-Token:xxxxx" \
 "http://polaris-api-v2.woa.com:8080/naming/v1/dynamicweight?service=trpc.hpydps.hpydparse.ReplayParse&namespace=Development"
# 修改:
curl -H 'Platform-Id:Hpyd-replay-parse' -H 'Platform-Token:xxxxxx' -H 'Content-Type:application/json' \
 -X POST -d '[{"service":"trpc.hpydps.hpydparse.ReplayParse","namespace":"Development","service_token":"xxxx","isEnable":true,"interval":2,"isUDFEnable":false,"ctime":"2023-02-17 17:21:20","mtime":"2023-02-17 15:54:20","revision":"1"}]' "http://polaris-api-v2.woa.com:8080/naming/v1/dynamicweight"
```

#### 配置文件修改

服务端需要保证配置文件中配置了 polaris registry:

```yaml
plugins:  # 插件配置
  registry:
    polaris:                              # 北极星名字注册服务的配置
      register_self: true                 # 是否进行服务自注册, 默认为 false, 交由 123 平台注册 (非 123 平台的话一般这里要改为 true)
      heartbeat_interval: 3000            # 名字注册服务心跳上报间隔
      protocol: grpc                      # 名字服务远程交互协议类型
      service:                            # 需要进行注册的各服务信息
        - name: trpc.server.Service1      # 服务名1, 一般和 trpc_go.yaml 中 server config 处的各个 service 一一对应
          namespace: Development          # 该服务需要注册的命名空间, 分正式 Production 和非正式 Development 两种类型
          token: xxxxxxxxxxxxxxxxxxx      # 前往 https://polaris.woa.com/ 进行申请或查看
          instance_id: xxxxxxxxxxxxxxxx   # （可选）服务注册所需要的，instance_id=XXX(namespace+service+host+port)
          weight: 100                     # 设置权重
          bind_address: eth1:8080         # （可选）指定服务监听地址，默认采用 service 中的地址
      # debug: true                       # 开启调试模式, 默认为 false
      # address_list: ip1:port1,ip2:port2 # 北极星服务的地址
      # connect_timeout: 1000             # 单位 ms，默认 1000ms，连接北极星后台服务的超时时间
      # message_timeout: 1s               # 类型为 time.Duration，从北极星后台接收一个服务信息的超时时间，默认为 1s
      # join_point: default               # 名字服务使用的接入点，该选项会覆盖 address_list 和 cluster_service
      # instance_location:                # 注册实例的地址位置信息
      #   region: China
      #   zone: Guangdong
      #   campus: Shenzhen
```

#### 代码变更

然后修改代码, 保证在插件初始化完成后:

```go
import (
    "strconv"
    "testing"
    "time"

    "trpc.group/trpc-go/trpc-go"
    "trpc.group/trpc-go/trpc-go/log"
    "trpc.group/trpc-go/trpc-naming-polaris/registry"
)
func main() {
    s := trpc.NewServer()
    done := make(chan struct{})
    s.RegisterOnShutdown(func() { close(done) })
    serviceName := "trpc.server.Serivce1"
    pb.RegisterSomeService(s, &someImpl{})
    go func(done <-chan struct{}) {
        reportInterval := time.Second
        t := time.NewTicker(reportInterval)
        defer t.Stop()
        for {
            select {
            case <-t.C:
            case <-done:
                return
            }
            u, c := 23.3, 100.0 // Get these values some where.
            precision, bitSize := 3, 64
            used, capacity := strconv.FormatFloat(u, 'f', precision, bitSize), strconv.FormatFloat(c, 'f', precision, bitSize)
            // 注意: registry.DefaultDynamicWeightReporter.Report 的调用必须必须要在插件的初始化完成之后才能调用,
            // 否则会报 service name 未注册的错误, 一般来说, trpc.NewServer() 执行完后插件就初始化完了, 所以一般在 trpc.NewServer() 后调用就没问题
            if err := registry.DefaultDynamicWeightReporter.Report(serviceName, used, capacity); err != nil {
                log.Error("dynamic weight report error: %+v", err)
            }
        }
    }(done)
    // ...
}
```

### 客户端支持

客户端进需要修改配置文件使得 `polaris_dwr` 负载均衡器具有最高优先级即可

#### 配置文件修改

```yaml
plugins:  # 插件配置
  selector:   # 针对 trpc 框架服务发现的配置
    polaris:  # 北极星服务发现的配置
      # debug: true                       # 开启 debug 日志
      # enable_canary: false              # 开启金丝雀功能，默认 false 不开启
      # timeout: 1000                     # 单位 ms，默认 1000ms，北极星获取实例接口的超时时间
      # report_timeout: 1ms               # 默认 1ms，如果设置了，则下游超时，并且少于设置的值，则忽略错误不上报
      # connect_timeout: 1000             # 单位 ms，默认 1000ms，连接北极星后台服务的超时时间
      # message_timeout: 1s               # 类型为 time.Duration，从北极星后台接收一个服务信息的超时时间，默认为 1s
      # log_dir: $HOME/polaris/log        # 北极星日志目录
      protocol: grpc                      # 名字服务远程交互协议类型
      # join_point: default               # 接入名字服务使用的接入点，该选项会覆盖 address_list 和 cluster_service
      # enable_servicerouter: true        # 是否开启服务路由，默认开启
      # persistDir: $HOME/polaris/backup  # 服务缓存持久化目录，按照服务维度将数据持久化到磁盘
      # service_expire_time: 24h          # 服务缓存的过期淘汰时间，类型为 time.Duration，如果不访问某个服务的时间超过这个时间，就会清除相关服务的缓存
      loadbalance: 
        name:                  # 负载均衡类型，默认值
          - polaris_dwr        # 动态权重, 设置为第一个则具有最高优先级
```

## 私有化部署注意事项

MK: http://mk.woa.com/q/287477/answer/108174

私有化部署时要确保 `registry` 配置(对应服务端服务注册)的 `address_list` 填上私有化部署的北极星集群地址, `selector` 配置(对应客户端服务发现)同理:

(并且记得要在 server service 下正确设置 `nic` 或 `ip,port`, 见 [tRPC-Go 框架配置（tRPC知识库）](https://iwiki.woa.com/pages/viewpage.action?pageId=99485621))

```yaml
global:
  # ...
# 服务端配置
server:
  # ...
  # 必填，service 列表
  service:
      # 必填，服务名，用于服务发现
    - name: String
      # 选填，该 service 绑定的网卡，只有 ip 为空时，才会生效
      nic: String
      # 选填，service 监听的 IP 地址，如果为空，则会尝试获取网卡 IP，如果仍为空，则使用 global.local_ip
      ip: String(ipv4 or ipv6)
      # 选填，该 service 绑定的端口，address 为空时，port 必填
      port: Integer
      # ...
plugins:  # 插件配置
  registry:
    polaris:                              # 北极星名字注册服务的配置
      register_self: true                 # 是否进行服务自注册, 默认为 false
      heartbeat_interval: 3000            # 名字注册服务心跳上报间隔
      protocol: grpc                      # 名字服务远程交互协议类型
      # service:                          # 需要进行注册的各服务信息
      #   - name: trpc.server.Service1    # 服务名1, 一般和 trpc_go.yaml 中 server config 处的各个 service 一一对应
      #     namespace: Development        # 该服务需要注册的命名空间, 分正式 Production 和非正式 Development 两种类型
      #     token: xxxxxxxxxxxxxxxxxxx    # 前往 https://polaris.woa.com/ 进行申请或查看
      #     instance_id: yyyyyyyyyyyyyyyy # （可选）服务注册所需要的，instance_id=XXX(namespace+service+host+port)
      #     weight: 100                   # 设置权重
      #     bind_address: eth1:8080       # （可选）指定服务监听地址，默认采用 service 中的地址
      #     metadata:                     # 注册时自定义 metadata
      #       internal-enable-set: Y      # 启用 set (本行和下行都需要设置才能完整启用 set)
      #       internal-set-name: xx.yy.sz # 设置服务 set 名
      #       key1: val1                  # 其他的 metadata 等, 可以参考北极星相关文档
      #       key2: val2
      # debug: true                       # 开启调试模式, 默认为 false
      address_list: ip1:port1,ip2:port2   # 北极星服务的地址
      # connect_timeout: 1000             # 单位 ms，默认 1000ms，连接北极星后台服务的超时时间
      # message_timeout: 1s               # 类型为 time.Duration，从北极星后台接收一个服务信息的超时时间，默认为 1s
      # join_point: default               # 名字服务使用的接入点，该选项会覆盖 address_list 和 cluster_service
      # instance_location:                # 注册实例的地址位置信息
      #   region: China
      #   zone: Guangdong
      #   campus: Shenzhen
      
  selector:   # 针对 trpc 框架服务发现的配置
    polaris:  # 北极星服务发现的配置
      # debug: true                       # 开启 debug 日志
      # enable_canary: false              # 开启金丝雀功能，默认 false 不开启
      # timeout: 1000                     # 单位 ms，默认 1000ms，北极星获取实例接口的超时时间
      # report_timeout: 1ms               # 默认 1ms，如果设置了，则下游超时，并且少于设置的值，则忽略错误不上报
      # connect_timeout: 1000             # 单位 ms，默认 1000ms，连接北极星后台服务的超时时间
      # message_timeout: 1s               # 类型为 time.Duration，从北极星后台接收一个服务信息的超时时间，默认为 1s
      # log_dir: $HOME/polaris/log        # 北极星日志目录
      protocol: grpc                      # 名字服务远程交互协议类型
      # join_point: default               # 接入名字服务使用的接入点，该选项会覆盖 address_list 和 cluster_service
      address_list: ip1:port1,ip2:port2 # 北极星服务的地址
      # enable_servicerouter: true        # 是否开启服务路由，默认开启
      # persistDir: $HOME/polaris/backup  # 服务缓存持久化目录，按照服务维度将数据持久化到磁盘
      # service_expire_time: 24h          # 服务缓存的过期淘汰时间，类型为 time.Duration，如果不访问某个服务的时间超过这个时间，就会清除相关服务的缓存
      # loadbalance: 
      #   name:                  # 负载均衡类型，默认值
      #     - polaris_wr         # 加权随机，如果默认设置为寻址方式，则数组的第一个则为默认的负载均衡
      #     - polaris_hash       # hash 算法
      #     - polaris_ring_hash  # 一致性 hash 算法
      #     - polaris_dwr        # 动态权重
      #  details:                # 各类负载均衡的具体配置，见 https://mk.woa.com/q/287086
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

* 正确配置本插件: 1. 包含匿名 import, 2. 插件配置中有 polaris selector
* 代码 option 不要带 `client.WithTarget`, `trpc_go.yaml` 的客户端配置中也不要带 `target` 字段

这样就实现了 `WithServiceName` 的寻址方式, 此时你会发现除了插件配置中的 polaris selector 有北极星相关信息, 客户端配置中任何地方不再需要有 polaris 字样, 但是实际确是使用的北极星插件能力进行的寻址, 这种现象的原因是北极星插件替换了 trpc-go 框架中的一些默认组件为北极星插件的实现, 导致客户端以几乎无感知的形式完成北极星寻址

#### `WithTarget`

期望通过 `WithTarget` 的方式来完成北极星寻址的话, 需要同时满足以下条件:

* 正确配置本插件: 1. 包含匿名 import, 2. 插件配置中有 polaris selector
* 二选一 (同时存在时, 代码 option 的优先级高于配置):
  * 代码 option 带 `client.WithTarget("polaris://trpc.app.server.service")`
  * `trpc_go.yaml` 的客户端配置中带 `target` 字段: `target: polaris://trpc.app.server.service`

这样就实现了 `WithTarget` 的寻址方式, 这里你会在 `target` 处看到明确的 polaris 字样, 明确地感知到这个客户端在使用北极星寻址

### 两种寻址方式的区别

下图展示了 `WithServiceName` 以及 `WithTarget` 实际使用的 selector

```bash
"trpc.app.server.service"   =>  (trpc-go).selector.TrpcSelector.Selector        => ip:port  # WithServiceName
"trpc.app.server.service"   =>  (trpc-naming-polaris).selector.Selector.Select  => ip:port  # WithTarget
```

在配置了北极星 selector 插件之后, `(trpc-go).selector.TrpcSelector.Selector` 内部使用到的 `discovery, servicerouter, loadbalance` 这三个模块会被替换为北极星插件自己的实现, 所以实际的效果其实为:

```bash
"trpc.app.server.service" =>  (trpc-naming-polaris).discovery.Discovery.List
                           =>  (trpc-naming-polaris).servicerouter.ServiceRouter.Filter        
                            =>  (trpc-naming-polaris).loadbalance.WRLoadBalancer.Select => ip:port  # WithServiceName

"trpc.app.server.service"   =>  (trpc-naming-polaris).selector.Selector.Select          => ip:port  # WithTarget
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