[English](test.md) | 中文

# 路由示例测试

本文档是路由示例真实 Polaris 验证的总入口。详细用例说明见：

- [metadata/test.zh_CN.md](./metadata/test.zh_CN.md)
- [rule/test.zh_CN.md](./rule/test.zh_CN.md)

## 前置条件

- Polaris OpenAPI 端口可访问，默认 `8090`。
- Polaris gRPC 端口可访问，默认 `8091`。
- 已安装 `curl` 和 `jq`。
- 上次运行启动的 Provider 和 control client 进程已经停止。
- 示例 service 没有残留实例。异常中断后先执行带
  `--delete-service` 的 cleanup，再重新验证。已存在的同名路由规则会由
  verify 脚本复用并启用。

示例统一使用命名空间 `go-naming-polarismesh-e2e`。Polaris 地址在运行时通过
参数传入。

## 运行

依次运行两个路由测试套件：

```bash
./examples/route/metadata/verify.sh \
  --polaris-server <polaris-server-host>

./examples/route/rule/verify.sh \
  --polaris-server <polaris-server-host>
```

也可以通过总脚本运行：

```bash
./examples/verify_all.sh --polaris-server <polaris-server-host>
```

## 用例矩阵

| 测试套件 | 用例 | 预期行为 |
| --- | --- | --- |
| metadata | 1.1 | `env=red` 只选择 provider-red，`env=blue` 只选择 provider-blue |
| metadata | 1.2 | 关闭服务路由后，`callee_env=red` 仍然选择 provider-red |
| metadata | 1.3 | `env=missing` 得到零次成功调用 |
| rule | 1.1 | 配置的 source caller 只访问 `route=vip` destination |
| rule | 1.2 | 未匹配的 caller 回退到普通路由 |

测试断言会校验实际选中的 Provider 或调用次数，不以简单请求成功作为路由
生效依据。

## 清理

验证成功后，进程和路由规则会保留供检查。停止本地进程并删除本地产物：

```bash
./examples/route/metadata/cleanup.sh
./examples/route/rule/cleanup.sh
```

删除远端服务，并保留路由规则：

```bash
./examples/route/metadata/cleanup.sh \
  --delete-service --polaris-server <polaris-server-host>

./examples/route/rule/cleanup.sh \
  --delete-service --polaris-server <polaris-server-host>
```
