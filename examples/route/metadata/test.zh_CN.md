[English](test.md) | 中文

# Metadata 路由测试

### [case 1.1 destination metadata routing]

带 `env=red` 的请求只能选择 provider-red；`env=blue` 只能选择 provider-blue。

### [case 1.2 callee environment routing]

关闭服务路由后，`callee_env=red` 必须选择 provider-red。

### [case 1.3 missing metadata has no match]

`env=missing` 必须得到零次成功调用。

## 清理

`verify.sh` 会保留 provider 和 control client 进程。默认 cleanup 停止进程并
删除本地产物；增加 `--delete-service --polaris-server` 才会删除示例服务。
