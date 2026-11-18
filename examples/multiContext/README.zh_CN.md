[English](README.md) | 中文

# 多 Context

在同一进程中创建 `polaris-a` 和 `polaris-b` 两个 Selector 配置，各自持有独立的
Polaris SDKContext 和持久化目录。

```bash
./examples/multiContext/verify.sh --polaris-server <polaris-server-host>
```

`polaris-a://` 解析 service A，`polaris-b://` 解析 service B。测试校验 selector
名称、服务名和最终地址，证明两套配置相互隔离。
