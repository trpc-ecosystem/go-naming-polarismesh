[English](test.md) | 中文

# 熔断测试

### [case 1.1 instance failover]

生成最多 1024 个 key 的有序 hash 序列，从结果中选择命中 provider-b 的 key，
然后终止 provider-b；必须观察到连接失败，随后连续五次成功选择 provider-a。

### [case 1.2 half-open recovery]

重启 provider-b，等待睡眠窗口和发现刷新后，同一个 key 必须能再次选中 provider-b。

### [case 1.3 service-level rule]

创建 caller-specific SERVICE 规则；失败达到阈值后，client 必须收到服务级熔断拒绝。

### [case 1.4 method-level rule]

为 `/<service>/Echo` 创建 METHOD 规则；同一服务调用必须收到方法级熔断拒绝。

## 清理

`verify.sh` 会保留最终进程和规则。默认 cleanup 停止进程并删除本地产物；
`--delete-service --polaris-server` 会额外删除示例服务，但保留
INSTANCE/SERVICE/METHOD 规则。下次执行会复用同名规则，并按当前用例启用所需规则。
