[English](README.md) | 中文

# 动态注册 Metadata

本示例为普通服务注册 deployment metadata 和 JSON descriptor。通过
`RegistrationSnapshotProviderFunc` 一次返回 host、port、version、metadata 和
显式空 protocol。naming 不解析 descriptor，也不从 JSON 推导注册字段。

动态 `deployment=green` 覆盖静态 `deployment=blue`。示例未显式指定注册端口时
使用监听端口，naming 不施加 AI 专属默认端口。

## 运行

```bash
cd examples/registry/metadata/provider
POLARIS_SERVER=<polaris-host> go run . \
  -conf trpc_go.yaml \
  -version v1.2.3 \
  -registration-port 28272
```

`-host` 可独立覆盖注册地址。`-metadata-mode none` 只使用静态配置，
`-metadata-mode failure` 在实例 RPC 前返回快照错误。

## 真实 E2E

```bash
./examples/registry/metadata/verify.sh --polaris-server <polaris-host>
./examples/registry/metadata/cleanup.sh
./examples/registry/metadata/cleanup.sh --delete-service --polaris-server <polaris-host>
```

七个用例覆盖完整快照、独立 host/version、TTL 与心跳恢复、普通服务类型、
provider 失败和同地址覆盖。最终 Provider 和日志保留供检查，详细断言见
[test.zh_CN.md](test.zh_CN.md)。
