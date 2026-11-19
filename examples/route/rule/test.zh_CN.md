[English](test.md) | 中文

# 规则路由测试

### [case 1.1 source and destination rule]

配置的 caller 必须把所有请求路由到 `route=vip` provider。

### [case 1.2 unmatched source falls back to normal routing]

其他 caller 必须继续使用普通路由，并可成功访问可用 provider。

## 清理

`verify.sh` 会保留 provider、control client 和路由规则。默认 cleanup 停止进程
并删除本地产物；`--delete-service --polaris-server` 还会删除示例服务，但保留
路由规则。下次执行会复用同名规则并保留 ID，同时更新为当前测试定义。
