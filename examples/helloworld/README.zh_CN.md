[English](README.md) | 中文

# HelloWorld

这是 Polaris 命名插件的标准 tRPC-Go HelloWorld 工程形态，包含 proto 协议、
生成桩、服务端、客户端和两份独立的 tRPC 配置。

```text
helloworld/
├── proto/helloworld.proto       # 服务协议
├── proto/helloworld.pb.go       # protobuf 消息
├── proto/helloworld.trpc.go     # tRPC 服务端/客户端桩
├── server/main.go               # Provider 实现
├── server/trpc_go.yaml          # Polaris Registry 配置
├── client/main.go               # target 和 service name 两种调用
└── client/trpc_go.yaml          # Polaris Selector 配置
```

## 前置条件

- Go 1.18 或更高版本。
- 可访问的 Polaris 服务端，gRPC 端口为 `8091`。
- 已创建 `go-naming-polarismesh-e2e` namespace，或通过 `-namespace` 指定其他值。

Polaris 地址不写入源码或 YAML；运行时通过 `POLARIS_SERVER` 或
`-polaris-server` 提供。

## 启动 Server

在第一个终端执行：

```bash
cd examples/helloworld/server

POLARIS_SERVER=<polaris-host> go run .
```

Server 启动时先连接配置的 Polaris 地址，并取该连接的本端 IP。Server 监听
`<local-ip>:18191`，并通过 Polaris 注册 `trpc.examples.helloworld.Greeter`。
Go 代码和 YAML 都不写死服务 IP。

常用覆盖参数：

```bash
POLARIS_SERVER=<polaris-host> \
POLARIS_NAMESPACE=<namespace> \
HELLOWORLD_PORT=18191 \
go run . -version v1.0.0
```

## 启动 Client

在第二个终端执行：

```bash
cd examples/helloworld/client

POLARIS_SERVER=<polaris-host> go run .
```

预期输出：

```text
target://Hello   address=<local-ip>:18191 response=Hello, tRPC-Go! (server v1.0.0)
service://Hello  address=<local-ip>:18191 response=Hello, tRPC-Go! (server v1.0.0)
service://Hi     address=<local-ip>:18191 response=Hi, tRPC-Go! (server v1.0.0)
```

第一条调用使用：

```go
client.WithTarget("polarismesh://" + serviceName)
```

后两条调用使用：

```go
client.WithServiceName(serviceName)
```

`client.WithSelectorNode(&node)` 会记录实际选中的 `host:port`，因此示例不只
断言 RPC 成功，还能直接看到命名结果。

## 常用参数

| 参数 | 环境变量 | 默认值 |
| --- | --- | --- |
| `-polaris-server` | `POLARIS_SERVER` | 必填 |
| `-polaris-port` | `POLARIS_GRPC_PORT` | `8091` |
| `-namespace` | `POLARIS_NAMESPACE` | `go-naming-polarismesh-e2e` |
| `-service` | `POLARIS_SERVICE` | `trpc.examples.helloworld.Greeter` |
| `-message` | `HELLOWORLD_MESSAGE` | `tRPC-Go` |
| `-port` | `HELLOWORLD_PORT` | Server 默认 `18191` |

按 `Ctrl+C` 停止 Server，Registry 插件会随后反注册实例。

## 真实 E2E 验证

同一组 Server/Client 程序也提供自动验证入口：

```bash
./examples/helloworld/verify.sh --polaris-server <polaris-server-host>
./examples/helloworld/cleanup.sh
./examples/helloworld/cleanup.sh --delete-service --polaris-server <polaris-server-host>
```

验证内容包括连接得到的注册 IP、target 寻址、service name 寻址、两个 RPC
方法。验证结束后会保留 Server 进程供排查。用例见
[test.zh_CN.md](test.zh_CN.md)。

## 重新生成桩代码

修改 `proto/helloworld.proto` 后，请安装 `protoc`、`protoc-gen-go` 和
tRPC-Go 生成器，然后执行：

```bash
cd examples/helloworld/server
go generate
```
