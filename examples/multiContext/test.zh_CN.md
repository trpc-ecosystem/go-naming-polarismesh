[English](test.md) | 中文

# 多 Context 测试

### [case 1.1 named selector isolation]

`polaris-a` 必须解析 service A，`polaris-b` 必须解析 service B。

### [case 1.2 independent SDKContext discovery]

每个 Selector 必须返回各自 provider 的地址，证明缓存和命名 client 没有串用。

## 清理

`verify.sh` 会保留两个 provider 进程。默认 cleanup 停止进程并删除本地产物；
`--delete-service --polaris-server` 还会删除两个示例服务。
