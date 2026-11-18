[English](README.md) | 中文

# 多 Endpoint 注册

同一进程在一个普通服务下注册 tRPC 与 HTTP 两个 endpoint。每个 endpoint 拥有
独立地址、protocol、version、weight、metadata 和稳定实例 ID。两个心跳任务、
失败回滚及反注册统一由 naming 持有。

## 配置

配置见 [provider/trpc_go-multi.yaml](provider/trpc_go-multi.yaml)：

```yaml
service:
  - name: ${POLARIS_SERVICE}
    endpoints:
      - address: ${PROVIDER_HOST}:${TRPC_PORT}
        protocol: trpc
      - address: ${PROVIDER_HOST}:${HTTP_PORT}
        protocol: http
```

## 运行

```bash
cd examples/registry/transports/provider
go run . \
  -conf trpc_go-multi.yaml \
  -polaris-server <polaris-host> \
  -namespace go-naming-polarismesh-e2e \
  -service trpc.polaris.registry.multi.Transport
```

在仓库根目录执行真实 Polaris 生命周期验证：

```bash
./examples/registry/transports/verify.sh --polaris-server <polaris-host>
```

七项覆盖 endpoint 身份、普通服务类型、TTL 与心跳恢复、OFFLINE、稳定 ID、
revision 和重复反注册。生命周期用例结束时测试进程已停止，服务和日志仍保留。
详细断言见 [test.zh_CN.md](test.zh_CN.md)。

## 清理

```bash
./examples/registry/transports/cleanup.sh
./examples/registry/transports/cleanup.sh --delete-service --polaris-server <polaris-host>
```

默认清理只移除本地产物。显式删除服务时保留旧版本示例资源的历史清理范围。
