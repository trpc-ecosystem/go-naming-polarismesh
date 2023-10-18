English | [中文](README_zh_CN.md)

# tRPC-Go naming polarismesh plugin

[![Go Reference](https://pkg.go.dev/badge/github.com/trpc-ecosystem/go-naming-polarismesh.svg)](https://pkg.go.dev/github.com/trpc-ecosystem/go-naming-polarismesh)
[![Go Report Card](https://goreportcard.com/badge/trpc.group/trpc-go/trpc-naming-polarismesh)](https://goreportcard.com/report/trpc.group/trpc-go/trpc-naming-polarismesh)
[![LICENSE](https://img.shields.io/badge/license-Apache--2.0-green.svg)](https://github.com/trpc-ecosystem/go-naming-polarismesh/blob/main/LICENSE)
[![Releases](https://img.shields.io/github/release/trpc-ecosystem/go-naming-polarismesh.svg?style=flat-square)](https://github.com/trpc-ecosystem/go-naming-polarismesh/releases)
[![Tests](https://github.com/trpc-ecosystem/go-naming-polarismesh/actions/workflows/prc.yml/badge.svg)](https://github.com/trpc-ecosystem/go-naming-polarismesh/actions/workflows/prc.yml)
[![Coverage](https://codecov.io/gh/trpc-ecosystem/go-naming-polarismesh/branch/main/graph/badge.svg)](https://app.codecov.io/gh/trpc-ecosystem/go-naming-polarismesh/tree/main)
 
This Plugin consists of Service Registry, Service Discovery, Load Balance and Circuit Breaker.
You can integrate it to your project quickly by yaml config.

## How to Use

Import naming-polarismesh plugin.

```go
import _ "trpc.group/trpc-go/trpc-naming-polarismesh"
```
Follow the following Chapters to config yaml.

## Service Discovery

### Discovery in tRPC-Go Framework

```go
import _ "trpc.group/trpc-go/trpc-naming-polarismesh"

func main() {
    opts := []client.Option{
    // namespce, use current env namespace on missing.
    client.WithNamespace("Development"),
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

### Get Callee IP
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
    // pass callee node
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
    // print callee node
    log.Infof("remote server ip: %s", node)

    log.Info("req:%v, rsp:%v, err:%v", req, rsp, err)
}
```

### Service Registry

[link](./registry)

### Load Balance

[link](./loadbalance)

### Circuit Breaker

[link](./circuitbreaker)

## Route by Env

On ServiceRouter disabled, you can route to a env by specifying its name.
```go
opts := []client.Option{
// namespace, use current env namespace on missing.
client.WithNamespace("Development"),
// service name.
// client.WithTarget("polarismesh://trpc.app.server.service"),
client.WithServiceName("trpc.app.server.service"),
// set callee env.
client.WithCalleeEnvName("62a30eec"),
// disable ServiceRouter.
client.WithDisableServiceRouter()
}
```

## A Complete Config Example

`registry` is used for service registry(refer [./registry/README.md](./registry/README.md) for more details).
`selector` is used for service discovery.

```yaml
plugins:  # Plugin configurations.
  registry:
    polarismesh:  # This is a polaris mesh registry.
      register_self: true  # Whether register by itself, default as false.
      heartbeat_interval: 3000  # Heartbeat report interval.
      protocol: grpc  # The protocol used to connect polaris mesh console.
      # service:  # Services to be registered.
      #   - name: trpc.server.Service1  # Service name, should keep consistent with service in server config in trpc_go.yaml.
      #     namespace: Development  # The namespace this service belongs to.
      #     token: xxxxxxxxxxxxxxxxxxx  # Apply your token in polaris mesh console.
      #     # (Optional) Used to heartbeat or unregister.
      #     # When register_self is true, this config has no effect, the plugin will use returned instance_id of register to overwrite config.
      #     # if register_self is false, instance_id cannot be missing.
      #     instance_id: yyyyyyyyyyyyyyyy
      #     weight: 100  # Set weight.
      #     bind_address: eth1:8080  # (optional) set listen addr, use the addr in service as default.
      #     metadata:  # The user defined metadata.
      #       internal-enable-set: Y  # Enable set, (both this line and the next line need to be set to fully enable set).
      #       internal-set-name: xx.yy.sz  # Enable set name.
      #       key1: val1  # Other metadata(s)
      #       key2: val2
      # debug: true  # Enable debug mod, default as false.
      # address_list: ip1:port1,ip2:port2  # Address(es) of polaris mesh service.
      # connect_timeout: 1000  # Timeout to connect to polaris mesh console, in ms, default as 1000ms.
      # message_timeout: 1s  # Timeout to receive a message from polaris mesh console, default as 1s.
      # instance_location:  # The location of service.
      #   region: China
      #   zone: Guangdong
      #   campus: Shenzhen

  selector:  # The service discovery config.
    polarismesh:  # This is a polaris mesh selector.
      # debug: true  # Enable debug log.
      # default: true  # Whether Set as default selector.
      # enable_canary: false  # Whether enable canary, default as false.
      # timeout: 1000  # Timeout to get instances from polaris mesh console, in ms, default as 1000ms.
      # report_timeout: 1ms  # If callee timeout is less report timeout, ignore timeout and do not report.
      # connect_timeout: 1000  # Timeout to connect to Polaris mesh console, in ms, default as 1000ms.
      # message_timeout: 1s  # Timeout to receive a message from polaris mesh console, default as 1s.
      # log_dir: $HOME/polarismesh/log  # The directory for polaris mesh log.
      protocol: grpc  # The protocol used to connect polaris mesh console.
      # address_list: ip1:port1,ip2:port2  # Address(es) of polaris mesh service.
      # enable_servicerouter: true  # Whether enable service router, default enabled.
      # persistDir: $HOME/polarismesh/backup  # The persistent directory of SDK data.
      # service_expire_time: 24h  # The expire time to exile an inactive service from cache.
      # loadbalance:
      #   name:  # Load balance type, you can also use strings begin with `DefaultLoadBalancer` in https://github.com/polarismesh/polaris-go/blob/v1.5.2/pkg/config/default.go#L181 .
      #     - polaris_wr  # Weighted random, the default load balance use the first lb in this list.
      #     - polaris_hash  # Hash.
      #     - polaris_ring_hash  # Consistent hash.
      #     - polaris_dwr  # Dynamic weighted random
      #  details:  # The specific configs for various load balances.
      #    polaris_ring_hash:  # The name of load balance, see previous name, support only polaris_ring_hash currently.
      #      vnodeCount: 1024  # Set the count of vnode in ring hash as 1024, default as 10 on missing
      # discovery:
      #   refresh_interval: 10000  # Refresh interval in ms.
      # cluster_service:
      #   discover: polaris.discover  # The service name of discovery.
      #   health_check: polaris.healthcheck  # The service name of health check.
      #   monitor: polaris.monitor  # The service name of monitor.
      # circuitbreaker:
      #   checkPeriod: 30s  # The check period of circuit breaker, default as 30s.
      #   requestCountAfterHalfOpen: 10  # The maximum requests after half open, default as 10.
      #   sleepWindow: 30s  # How long to convert to half open, default as 30s.
      #   successCountAfterHalfOpen: 8  # The minimum success requests to close a half open circuit breaker default as 8.
      #   chain:  # The strategy for circuit breaker, default as [errorCount, errorRate].
      #     - errorCount  # Circuit break by periodic error count.
      #     - errorRate  # Circuit break by periodic error rate.
      #   errorCount:
      #     continuousErrorThreshold: 10  # The threshold to trigger continuous errors circuit breaker, default as 10.
      #     metricNumBuckets: 10  # The size of buckets to stat continuous errors, default as 10.
      #     metricStatTimeWindow: 1m0s  # The statistic period for continuous errors, default as 1min.
      #   errorRate:
      #     metricNumBuckets: 5  # The zie of buckets to stat error rate.
      #     metricStatTimeWindow: 1m0s  # The statistic period for error rate, default as 1min.
      #     requestVolumeThreshold: 10  # The threshold to trigger error rate circuit breaker, default as 10.
      # service_router:
      #   nearby_matchlevel: zone  # The level of nearby match router, one of region, zone or campus, default as zone.
      #   # The minimum threshold of healthy instances to trigger ALL DIE IS ALIVE.
      #   # It's between [0,1], default as 0, which means ALL DIE IS ALIVE only take effect when there is no healthy instance.
      #   percent_of_min_instances: 0.2
      #   # Whether expand all nodes as registry.Node, default as false, which put original data in metadata and avoid performance degradation.
      #   need_return_all_nodes: false
      # instance_location:  # The location of client SDK.
      #   region: China
      #   zone: Guangdong
      #   campus: Shenzhen
      
      ## This boolean is used at WithTarget mod to transfer tRPC metadata to naming polaris mesh.
      ## If opened, the trans-info, with prefix `selector-meta-` is removed, will be filled in Metadata of SourceService to match polaris mesh rules.
      ## For example: the trans-info `selector-meta-key1:val1` will transfer meta `key1:val1` to polaris mesh.
      # enable_trans_meta: true
```

## Polaris Mesh Official Docs

https://polarismesh.cn/docs

## Support Multiple Selectors

After import `trpc-naming-polarismesh`, a selector plugin named `"polarismesh"` will be auto registered. If you want to use a
different selector to call a service in another region, you can register a new selector plugin. For example:

### Method 1: Codes and Configs

#### Codes

Choose one of the following(do not mixing them):

1. Use service name to select.
2. Use target to select.

```go
import (
    "trpc.group/trpc-go/trpc-go/plugin"
    "trpc.group/trpc-go/trpc-naming-polarismesh"
)

func init() {
    plugin.Register("polarismesh-customized1", &naming.SelectorFactory{})
    plugin.Register("polarismesh-customized2", &naming.SelectorFactory{})
}

// Discovery by service name.
// Client options can be configured in trpc_go.yaml.
func CallWithServiceName(ctx context.Context) error {
	// Call down stream by the config of plugin "polarismesh-customized1".
    rsp, err := proxy.Invoke(ctx, req,
        client.WithServiceName("trpc.app.server.service"),
		// The following four options should be used together to take "polarismesh-customized1" in effect.
        client.WithDiscoveryName("polarismesh-customized1"),
        client.WithServiceRouterName("polarismesh-customized1"),
        client.WithBalancerName("polaris_wr"), // Fill the load balance in default selector config.
        client.WithCircuitBreakerName("polarismesh-customized1"),
    )
    if err != nil { return err }
	// Call down stream by the config of plugin "polarismesh-customized2".
    rsp, err = proxy.Invoke(ctx, req,
        client.WithServiceName("trpc.app.server.service"),
        // The following four options should be used together to take "polarismesh-customized2" in effect.
        client.WithDiscoveryName("polarismesh-customized2"),
        client.WithServiceRouterName("polarismesh-customized2"),
        client.WithBalancerName("polaris_wr"), // Fill the load balance in default selector config.
        client.WithCircuitBreakerName("polarismesh-customized2"),
    )
    if err != nil { return err }
}

// Discovery by target.
// Client options can be configured in trpc_go.yaml.
func CallWithTarget(ctx context.Context) error {
    // Call down stream by the config of plugin "polarismesh-customized1".
    rsp, err := proxy.Invoke(ctx, req,
        client.WithTarget("polarismesh-customized1://trpc.app.server.service"))
    if err != nil { return err }
	// Call down stream by the config of plugin "polarismesh-customized2".
    rsp, err = proxy.Invoke(ctx, req,
        client.WithTarget("polarismesh-customized2://trpc.app.server.service"))
    if err != nil { return err }
}
```

Note: all of above options can also be set in `client.service` of `trpc_go.yaml`. When they coexist, the priority of
codes is higher than that of config.

The following methods is also optional, and must not be mixed.

* Discovery by service name.
  ```yaml
  client:  # The config of client.
    service:  # The config for each callee.
      - name: trpc.app.server.service  # The service name of callee.
        discovery: polarismesh-customized1
        servicerouter: polarismesh-customized1
        loadbalance: polaris_wr  # Fill load balance in default selector.
        circuitbreaker: polarismesh-customized1
        network: tcp  # The network type of callee, tcp or udp.
        protocol: trpc  # The application layer protocol, trpc or http.
        timeout: 1000   # The maximum time to process a request.
  ```
* Discovery by target.
  ```yaml
  client:  # The config of client.
    service:  # The config for each callee.
      - name: trpc.app.server.service  # The service name of callee.
        network: tcp  # The network type of callee, tcp or udp.
        protocol: trpc  # The application layer protocol, trpc or http.
        target: polarismesh-customized1://trpc.app.server.service  # The address of callee.
        timeout: 1000   # The maximum time to process a request.
  ```

#### Edit Configs

```yaml
plugins:
  selector:
    polarismesh:
      protocol: grpc
      default: true  # Set as default selector.
      join_point: default
      # The directory to persist cached services.
      # Different selector should use different persistDir to avoid interface with each other.
      persistDir: $HOME/polarismesh/backup
      # The directory of polaris mesh log.
      # There is only one polaris mesh log in a process. If multiple directories is configured, the last one will be used.
      log_dir: $HOME/polarismesh/log
      # loadbalance:  # Like logs, load balance should be set once, for example, in default selector.
      #   name:  # The types of load balance.
      #     - polaris_wr  # Weighted random. The first LB in this list will be used as default.
      #     - polaris_hash  # Hash.
      #     - polaris_ring_hash  # Consistent hash.
      # For other configs, see Section `A Complete Config Example`.
    polarismesh-customized1:
      protocol: grpc
      default: false  # This is not default selector.
      join_point: point1
      # The directory to persist cached services.
      # Different selector should use different persistDir to avoid interface with each other.
      persistDir: $HOME/polarismesh-customized1/backup
      # For other configs, see Section `A Complete Config Example`.
    polarismesh-customized2:
      protocol: grpc
      default: false  # This is not default selector.
      join_point: point2
      # The directory to persist cached services.
      # Different selector should use different persistDir to avoid interface with each other.
      persistDir: $HOME/polarismesh-customized2/backup
      # For other configs, see Section `A Complete Config Example`.
```

Note: If there are multiple selectors, you should explicitly mark one of them as default, and others as `default: false`.
When Discovering by service name, `WithDiscoveryName, WithServiceRouterName, WithBalancerName, WithCircuitBreakerName`,
these four options must be provided.

### Method 2: Codes Only

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
        Default: &dft1, // set as default
		// When discovery by service name, load balance configs should be provided under default selector.
        Loadbalance: naming.LoadbalanceConfig{Name: []string{"polaris_ws"}},
        LogDir: &logDir1,
        PersistDir: &persistDir1,
		// Add any other configs that you want.
    }); err != nil { /* handle error */ }

    addrs2 := "zzz"
    persistDir2 := "polarismesh-customized2/backup"
    dft2 := false
    if err := naming.SetupWithConfig(&naming.Config{
        Name: "polarismesh-customized2",
        AddressList: addrs2,
        Default: &dft2, // set as non default.
        PersistDir: &persistDir2,
        // Add any other configs that you want.
    }); err != nil { /* handle error */ }
}

// Discovery by service name.
  func CallWithServiceName(ctx context.Context) error {
    // Call down stream with "polarismesh-customized1".
    rsp, err := proxy.Invoke(ctx, req,
        client.WithServiceName("trpc.app.server.service"),
        client.WithDiscoveryName("polarismesh-customized1"),
        client.WithServiceRouterName("polarismesh-customized1"),
        client.WithBalancerName("polaris_wr"), // Fill the load balance in default selector.
        client.WithCircuitBreakerName("polarismesh-customized1"),
    )
    if err != nil { return err }
    // Call down stream with "polarismesh-customized2".
    rsp, err = proxy.Invoke(ctx, req,
        client.WithServiceName("trpc.app.server.service"),
        client.WithDiscoveryName("polarismesh-customized2"),
        client.WithServiceRouterName("polarismesh-customized2"),
        client.WithBalancerName("polaris_wr"), // Fill the load balance in default selector.
        client.WithCircuitBreakerName("polarismesh-customized2"),
    )
    if err != nil { return err }
}

// Discovery by target.
func CallWithTarget(ctx context.Context) error {
    // Call down stream with "polarismesh-customized1".
    rsp, err := proxy.Invoke(ctx, req,
      client.WithTarget("polarismesh-customized1://trpc.app.server.service"))
    if err != nil { return err }
    // Call down stream with "polarismesh-customized2".
    rsp, err = proxy.Invoke(ctx, req,
      client.WithTarget("polarismesh-customized2://trpc.app.server.service"))
    if err != nil { return err }
}
```

Note: tRPC plugin may not support some new features of polaris mesh. You can create your own polaris mesh SDK config and pass it
to tRPC plugin. It is considered as basic config, and options by tRPC API will overwrite it, and finally, you got your
own polaris mesh object.

```golang
// Creates a polaris mesh config.
cfg := api.NewConfiguration()
// Add polaris mesh addresses, limiter server etc..
addresses := []string{"127.0.0.1:8081"}
cfg.GetGlobal().GetServerConnector().SetAddresses(addresses)
cfg.GetProvider().GetRateLimit().GetRateLimitCluster().SetService("polarismesh.metric.v2.test")
// Initialize.
if err := naming.SetupWithConfig(&naming.Config{
    Name: "polarismesh-customized1",
    Loadbalance: naming.LoadbalanceConfig{Name: []string{"polaris_ws"}},
    PolarisConfig: cfg,
}); err != nil { /* handle error */ }
```

## The Difference between `client.WithServiceName` and `client.WithTarget` and the Meaning of `enable_servicerouter`

### Strict Definition of Two Discoveries

Discovery by `client.WithServiceName` iff(both must be satisfied):
* Does not use `client.WithTarget`;
* There's no `target` field in `yaml.client.service[i]`.

Discovery by `client.WithTarget` iff(one of the flowing should be satisfied, and the priority of codes is greater than config):
* Use `client.WithTarget`;
* Config `target` field in `yaml.client.service[i]`.

We didn't mention `client.WithServiceName` or `yaml.client.service[i].name`, because they should always be provided and
are not used to distinguish two discovery methods.

### POV of two Discoveries in Polaris Mesh Plugin

#### `WithServiceName`

To discover by `WithServiceName`, following conditions should be satisfied:
* Correctly config this plugin: 1. anonymous import, and 2. config polaris mesh selector in plugin config;
* No `client.WithTarget` in codes and no `target` in `yaml.client.service[i]`.

This way, you will discover by `WithServiceName`. You may find that although there's no polaris mesh info in client config except
polaris mesh selector in plugin config, the client actually use polaris mesh to discovery. This is because polaris mesh plugin replace
tRPC default selector with it own implementation, in that, users can complete polaris mesh discovery almost imperceptible.

#### `WithTarget`

To discover by `WithTarget`, following conditions should be satisfied:
* Correctly config this plugin: 1. anonymous import, and 2. config polaris mesh selector in plugin config;
* Chose one of the following(the priority of codes is greater than config):
  * Add `client.WithTarget("polarismesh://trpc.app.server.service")` to your codes;
  * Add `target: polarismesh://trpc.app.server.service` to `yaml.client.service[i].target`.

This way, you will discover by `WithTarget`. You know exactly that you are using polaris mesh discovery, because you see
`polarismesh` in `target`.

### The Difference between Two Discoveries

The following chart shows the real selector used by `WithServiceName` or `WithTarget`:
```bash
"trpc.app.server.service"   =>  (trpc-go).selector.TrpcSelector.Selector        => ip:port  # WithServiceName
"trpc.app.server.service"   =>  (trpc-naming-polarismesh).selector.Selector.Select  => ip:port  # WithTarget
```

After config polaris mesh selector plugin, three models `discovery, servicerouter, loadbalance` used by internal
`(trpc-go).selector.TrpcSelector.Selector` will be replaced by implementation of polaris mesh. The actual effect is:
```bash
"trpc.app.server.service" =>  (trpc-naming-polarismesh).discovery.Discovery.List
                           =>  (trpc-naming-polarismesh).servicerouter.ServiceRouter.Filter        
                            =>  (trpc-naming-polarismesh).loadbalance.WRLoadBalancer.Select => ip:port  # WithServiceName

"trpc.app.server.service"   =>  (trpc-naming-polarismesh).selector.Selector.Select          => ip:port  # WithTarget
```

In other words, `WithServiceName` use three models `discovery, servicerouter, loadbalance` of polaris mesh plugin, and
`WithTarget` use `selector` of polaris mesh plugin.

However, `selector` of polaris mesh plugin does not combine three models together like `TrpcSelector`, and has its own logic,
which result in the difference of `WithServiceName` and `WithTarget`:

||`WithServiceName`|`WithTarget`|
|-|-|-|
| use polaris mesh SDK |`discovery, servicerouter, loadbalance`|`selector`|

#### Quick Check
| feature                                                 | `WithServiceName`<br>`enable_servicerouter=true` | `WithTarget`<br>`enable_servicerouter=true` | `WithServiceName`<br>`enable_servicerouter=false` | `WithTarget`<br>`enable_servicerouter=false` |
|---------------------------------------------------------|:------------------------------------------------:|:-------------------------------------------:|:-------------------------------------------------:|:--------------------------------------------:|
| use polaris mesh caller out rules                       |           <font color="green">Y</font>           |        <font color="green">Y</font>         |            <font color="red">N</font>             |          <font color="red">N</font>          |
| could enable `EnableTransMeta`                          |            <font color="red">N</font>            |        <font color="green">Y</font>         |            <font color="red">N</font>             |          <font color="red">N</font>          |
| set original env name to metadata['env']                |           <font color="green">Y</font>           |        <font color="green">Y</font>         |            <font color="red">N</font>             |          <font color="red">N</font>          |
| set target env name to metadata['env']                  |            <font color="red">N</font>            |         <font color="red">N</font>          |           <font color="green">Y</font>            |         <font color="green">Y</font>         |
| use original metadata to route(except 'env' and 'set')  |           <font color="green">Y</font>           |        <font color="green">Y</font>         |            <font color="red">N</font>             |          <font color="red">N</font>          |
| use target metadata to route(except 'env' and 'set')    |            <font color="red">N</font>            |        <font color="green">Y</font>         |            <font color="red">N</font>             |         <font color="green">Y</font>         |
| use `client.WithEnvKey` to set original metadata['key'] |           <font color="green">Y</font>           |         <font color="red">N</font>          |            <font color="red">N</font>             |          <font color="red">N</font>          |
| use `client.WithEnvTransfer` to reoute                  |           <font color="green">Y</font>           |         <font color="red">N</font>          |            <font color="red">N</font>             |          <font color="red">N</font>          |
| canary router                                           |           <font color="green">Y</font>           |        <font color="green">Y</font>         |           <font color="green">Y</font>            |         <font color="green">Y</font>         |
| set router                                              |           <font color="green">Y</font>           |         <font color="red">N</font>          |           <font color="green">Y</font>            |          <font color="red">N</font>          |

Cautious:
* `enable_servierouter` is the config provided by polaris mesh selector plugin. There is an option
  `client.WithDisableServiceRouter` and config `disable_servicerouter` in tRPC-Go client, which correspond to the config
  of polaris mesh plugin(you may think that tRPC-Go create an option specifically for polaris mesh plugin). The Difference is that
  framework can control each client, but polaris mesh plugin is a global config.
* As for the interpretation of the correct semantics of `enable_servicerouter`, it is roughly: When
  `enable_servicerouter=true`, the source service out rules are enabled (this requires that the source service must be
  registered on polaris mesh).
* Set router is only available at `WithServicename` mod.
* Metadata router is only available at `WithTarget` mod.
* `EnableTransMeta` is only available when `enable_servicerouter=true` at `WithTarget` mod.
* When configuring these things, you must pay attention to whether the configuration of `trpc_go.yaml` itself is
  effective, not just whether the content of `trpc_go.yaml` is correctly parsed by the framework (of course, you must
  ensure that it is parsed correctly, such as after updating the configuration, you need to ensure that there is a
  reload), and also pay attention to whether a certain client configuration is actually used, because the client
  configuration is stored in the framework with the callee proto name as the key, so when the service name is
  inconsistent with the callee proto name, you need to explicitly write the `name` and `callee` in the client
  configuration.
* If there is a problem, first use the option in the code to specify all the content you need to rule out the problem
  that the configuration does not take effect.

