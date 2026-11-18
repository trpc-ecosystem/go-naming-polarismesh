[English](test.md) | 中文

# Registry Metadata 测试

以下七项与 [verify.sh](verify.sh) 一致。默认注册端口为 28272、28274、28280，
监听端口与注册端口独立，均可配置。

## 用例

- **1.1 complete registration snapshot**：实例使用 provider 的 host、port、
  version、空 protocol、`deployment=green` 和原始 JSON descriptor。
  动态 deployment 覆盖静态值。
- **1.2 independent registration version**：第二个注册端口保留独立的版本和 descriptor。
- **1.3 independent registration host**：第三个实例显式使用 `127.0.0.1`，
  不依赖框架地址。
- **1.4 TTL and auto heartbeat**：三个实例都启用健康检查且 TTL=6。暂停主进程后，
  主实例变为不健康，其他两个保持健康；恢复进程后主实例重新健康。
- **1.5 microservice service type**：自动创建的服务类型为
  `SERVICE_TYPE_MICROSERVICE`，不属于 AI 服务。
- **1.6 provider failure before registration**：失败 provider 返回
  `snapshot unavailable`，不写入实例，三个已有实例保持不变。
- **1.7 same host and registration port replacement**：替换 provider 使用主实例的
  host/port，实例 ID 不变，version 与 descriptor endpoint 同时更新，
  其他两个实例保持不变。

## 清理

最终 Provider 保留供检查。默认 cleanup 停止已记录进程并删除本地产物；
增加 `--delete-service --polaris-server <host>` 才删除归属服务。
运行前要求目标服务不存在，而非只有实例为空；先检查旧资源，再显式清理。
