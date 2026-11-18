[English](test.md) | 中文

# Registry Transports 测试

以下七项与 [verify.sh](verify.sh) 一致。默认 tRPC 端口为 28351，HTTP 端口为 28352。

## 用例

- **1.1 multiple endpoints**：恰好两个非空 host 的实例。tRPC 使用
  `multi-trpc-stable`、版本 `v1.0.0`、weight=80 和 `transport=trpc`；
  HTTP 使用 `multi-http-stable`、版本 `v2.0.0`、weight=120 和 `transport=http`。
- **1.2 microservice service type**：服务类型为 `SERVICE_TYPE_MICROSERVICE`。
- **1.3 TTL and heartbeat**：两个实例启用健康检查且 TTL=6。暂停进程后两个实例
  均变为不健康，恢复进程后均重新健康。
- **2.1 deregistration and offline service**：停止 Provider 后两个实例消失，
  服务为 `OFFLINE` 且实例数为零。
- **2.2 stable instance IDs**：以相同显式 ID 重启，两个实例恢复且 ID、
  endpoint 独立字段保持不变。
- **2.3 revision after re-registration**：两个实例 revision 均非空，
  且与反注册前不同。
- **2.4 repeat deregistration**：再次停止 Provider，两个实例再次消失。

## 清理

生命周期用例已停止 Provider，日志和空服务保留。默认 cleanup 删除本地产物，
增加 `--delete-service --polaris-server <host>` 才删除归属的远端服务。
