[English](README.md) | 中文

# tRPC-Go Polaris 命名插件

[![Go Reference](https://pkg.go.dev/badge/trpc.group/trpc-go/trpc-naming-polarismesh.svg)](https://pkg.go.dev/trpc.group/trpc-go/trpc-naming-polarismesh)
[![Go Report Card](https://goreportcard.com/badge/trpc.group/trpc-go/trpc-naming-polarismesh)](https://goreportcard.com/report/trpc.group/trpc-go/trpc-naming-polarismesh)
[![License](https://img.shields.io/badge/license-Apache--2.0-green.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/trpc-ecosystem/go-naming-polarismesh)](https://github.com/trpc-ecosystem/go-naming-polarismesh/releases)
[![Testing](https://github.com/trpc-ecosystem/go-naming-polarismesh/actions/workflows/testing.yml/badge.svg)](https://github.com/trpc-ecosystem/go-naming-polarismesh/actions/workflows/testing.yml)

`go-naming-polarismesh` 用于将 tRPC-Go 应用接入
[Polaris 北极星](https://github.com/polarismesh/polaris)。它通过 tRPC-Go
命名插件提供服务注册、服务发现、路由、负载均衡和熔断能力，也可以通过
`selector`、`discovery` 等底层包直接使用。

## 主要能力

- 服务注册生命周期管理，支持自动心跳、反注册、稳定实例 ID 和多 endpoint。
- 运行时注册 metadata，以及 host、port、version、protocol 的原子注册快照。
- 支持按服务名或 `polarismesh://` target 进行服务发现。
- 支持环境、metadata、金丝雀、主调服务和规则路由。
- 支持加权随机、Hash、Ring Hash、Maglev 等 Polaris SDK 负载均衡策略。
- 支持 SERVICE、METHOD、INSTANCE 三级熔断、HalfOpen 探测配额和全死全活。
- 支持多个隔离的 SDKContext，用于连接不同 Polarismesh 环境。

## 环境要求

- Go 1.21 或更高版本。
- tRPC-Go 应用。
- 可访问的 Polaris gRPC 地址，通常使用 `8091` 端口，以及已存在的命名空间。

CI 使用 Go 1.21 和 Go 1.26.11 验证该模块。生产构建应使用仍在接收安全修复的
Go 版本。

## 安装

```bash
go get trpc.group/trpc-go/trpc-naming-polarismesh@latest
```

在应用初始化阶段注册插件：

```go
import _ "trpc.group/trpc-go/trpc-naming-polarismesh"
```

Polaris 地址应通过运行时配置传入，不需要写入源码。

## 配置

服务提供方在 server service 中选择插件，并配置 registry：

```yaml
server:
  service:
    - name: trpc.app.server.Service
      registry: polarismesh

plugins:
  registry:
    polarismesh:
      register_self: true
      heartbeat_interval: 3000
      protocol: grpc
      address_list: ${POLARIS_SERVER}:${POLARIS_GRPC_PORT}
      service:
        - name: trpc.app.server.Service
          namespace: Development
```

服务调用方配置 selector：

```yaml
client:
  namespace: Development

plugins:
  selector:
    polarismesh:
      default: true
      protocol: grpc
      address_list: ${POLARIS_SERVER}:${POLARIS_GRPC_PORT}
```

以上片段只展示命名服务相关字段。完整的 tRPC-Go 配置参见
[HelloWorld](examples/helloworld)。

## HelloWorld

[HelloWorld](examples/helloworld) 包含 proto 协议、生成的 tRPC 桩、Server、
Client，以及独立的提供方和调用方 YAML 配置。

在一个终端启动 Server：

```bash
cd examples/helloworld/server
POLARIS_SERVER=<polaris-host> go run .
```

在另一个终端启动 Client：

```bash
cd examples/helloworld/client
POLARIS_SERVER=<polaris-host> go run .
```

Client 同时演示两种服务发现方式：

```go
client.WithTarget("polarismesh://trpc.examples.helloworld.Greeter")
client.WithServiceName("trpc.examples.helloworld.Greeter")
```

## 示例与文档

- [HelloWorld](examples/helloworld)：完整的 Server/Client 生命周期。
- [负载均衡](examples/loadbalancer)：对不同负载均衡策略进行可观测断言。
- [熔断](examples/circuitbreaker)：覆盖 SERVICE、METHOD、INSTANCE 和
  HalfOpen 恢复。
- [动态注册 metadata](examples/registry/metadata)：不透明 metadata 和原子注册快照。
- [多 endpoint 注册](examples/registry/transports)：心跳、稳定 ID 和反注册。
- [metadata 路由](examples/route/metadata) 与
  [规则路由](examples/route/rule)：覆盖主调、被调 metadata 和规则路由。
- [多 Context](examples/multiContext)：命名 selector 和隔离的 SDKContext。

模块文档：

- [服务注册](registry/README.zh_CN.md)
- [服务寻址](selector/README.zh_CN.md)
- [服务发现](discovery/README.zh_CN.md)
- [负载均衡](loadbalance/README.zh_CN.md)
- [熔断器](circuitbreaker/README.zh_CN.md)
- [服务路由](servicerouter/README.zh_CN.md)

## 验证

运行本地测试：

```bash
go test ./...
```

连接真实 Polaris 服务端运行全部示例：

```bash
./examples/verify_all.sh --polaris-server <polaris-host>
```

验证完成后会保留进程、日志和远端资源供排查，需要显式清理：

```bash
./examples/cleanup.sh
./examples/cleanup.sh \
  --delete-service \
  --polaris-server <polaris-host>
```

第一条命令停止本地示例进程并删除本地生成产物；第二条命令还会删除北极星中的
测试服务和实例。测试使用的路由规则和熔断规则默认保留。

如需在未安装 Go 的远端 Linux 主机上验证，可构建部署包：

```bash
./examples/cloud-package.sh
```

打包、执行、清理和日志回传流程参见
[integration/cloud/README.zh_CN.md](integration/cloud/README.zh_CN.md)。

## 参与贡献

请阅读 [CONTRIBUTING.zh_CN.md](CONTRIBUTING.zh_CN.md) 并遵守
[行为准则](CODE_OF_CONDUCT.md)。版本变更记录见
[CHANGELOG.md](CHANGELOG.md)。

## 许可证

本项目使用 [Apache License 2.0](LICENSE)。
