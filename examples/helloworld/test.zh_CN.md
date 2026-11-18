[English](test.md) | 中文

# HelloWorld 测试

本文档与 [verify.sh](verify.sh) 保持同步。

### [case 1.1 connection-derived registration host]

操作：使用 Polaris 地址启动 Server，并比较 Server 从连接得到的本端 IP 与
Polaris OpenAPI 返回的实例 host。

预期：注册 host 等于连接 Polaris 的 TCP socket 本端 IP，不使用固定服务 IP。

### [case 1.2 target discovery]

操作：通过
`WithTarget("polarismesh://trpc.examples.helloworld.Greeter")` 调用。

预期：调用成功，选中配置的 Provider 端口，响应包含 `Hello, tRPC-Go!` 和
注册的 version。

验证器会在 OpenAPI 已发现实例后等待 selector 刷新，并在临时 gRPC 网络失败时
重试 one-shot client。

### [case 1.3 service discovery]

操作：通过 `WithServiceName(...)` 调用 `Hello` 和 `Hi`。

预期：两次调用均成功，并选中同一个 Provider 端口。

## 清理

`verify.sh` 会保留 Provider 进程供排查。清理方式：

```bash
./examples/helloworld/cleanup.sh
./examples/helloworld/cleanup.sh --delete-service --polaris-server <host>
```

在本地进程或残留实例清理前，重复 verify 会被拒绝。
