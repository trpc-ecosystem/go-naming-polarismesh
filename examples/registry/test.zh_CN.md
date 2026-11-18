[English](test.md) | 中文

# 注册示例测试

本文档是注册示例真实 Polaris 验证的总入口。每个子目录都有独立、可执行的
详细测试用例说明：

- [metadata/test.zh_CN.md](./metadata/test.zh_CN.md)
- [transports/test.zh_CN.md](./transports/test.zh_CN.md)

## 前置条件

- Polaris OpenAPI 端口可访问，默认 `8090`。
- Polaris gRPC 端口可访问，默认 `8091`。
- 已安装 `curl` 和 `jq`。
- 上次运行启动的 Provider 进程已经停止。
- 目标 service 不存在。如果上次运行异常中断，先执行
  `cleanup.sh --delete-service --polaris-server <host>` 再重试。

示例统一使用命名空间 `go-naming-polarismesh-e2e`。Polaris 地址在运行时通过
参数传入，不写入仓库配置。

## 运行

依次运行两个注册测试套件：

```bash
./examples/registry/metadata/verify.sh \
  --polaris-server <polaris-server-host>

./examples/registry/transports/verify.sh \
  --polaris-server <polaris-server-host>
```

也可以通过总脚本运行：

```bash
./examples/verify_all.sh --polaris-server <polaris-server-host>
```

## 用例矩阵

| 测试套件 | 用例 | 主要断言 |
| --- | --- | --- |
| metadata | 1.1-1.3 | 完整快照、独立 version 和注册 host |
| metadata | 1.4-1.5 | TTL `6`、心跳暂停/恢复和普通服务类型 |
| metadata | 1.6-1.7 | 快照失败不执行 RPC，同地址原地更新 metadata/version |
| transports | 1.1-1.3 | 多 endpoint 身份、普通微服务类型、TTL 和心跳恢复 |
| transports | 2.1-2.3 | 反注册、`OFFLINE`、稳定 ID 和新 revision |
| transports | 2.4 | 重复反注册 |

具体操作、预期值和端口说明以两个子目录的测试文档为准。

## 清理

metadata 验证保留最终 Provider；transports 的生命周期用例会停止 Provider。
两者的日志和远端服务均保留供检查。停止本地进程并删除本地产物：

```bash
./examples/registry/metadata/cleanup.sh
./examples/registry/transports/cleanup.sh
```

显式删除归属的远端服务：

```bash
./examples/registry/metadata/cleanup.sh \
  --delete-service --polaris-server <polaris-server-host>

./examples/registry/transports/cleanup.sh \
  --delete-service --polaris-server <polaris-server-host>
```
