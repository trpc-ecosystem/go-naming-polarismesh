# 更新日志

## v1.1.0

### 新增特性

- 支持协议无关的动态注册 metadata，以及实例 host、version、protocol 原子快照。
- 支持一个服务注册多个实例，并独立配置 protocol、weight、TTL、心跳和 instance ID。
- 支持 SDKContext 复用、自定义 Selector 和多 Context 隔离。
- 提升负载均衡、metadata/规则路由和 SERVICE/METHOD/INSTANCE 熔断能力。
- 增加真实 Polaris E2E、示例工程和 Linux 云端部署验证包。
- 支持 Go 1.21，并验证 Go 1.26.11 构建。
- 插件启动时输出当前版本日志。

### 问题修复

- 修复并发创建 Registry 时 healthcheck watcher 注册冲突。
- 修复 half-open 候选、熔断回退和 caller 透传问题。
- 修复注册失败回滚、超时清理和反注册重试问题。
- 修复自动心跳忽略健康状态，以及共享地址实例心跳冲突。
- 修复被包装的超时和连接错误未正确计入熔断统计。

## [v1.0.0](https://github.com/trpc-ecosystem/go-naming-polarismesh/releases/tag/v1.0.0)

- 历史版本，提供基础 Polaris 服务注册、发现、负载均衡、熔断、路由和 tRPC-Go 插件接入。
