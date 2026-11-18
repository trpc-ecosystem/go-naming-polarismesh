[English](test.md) | 中文

# 负载均衡测试

### [case 1.1 hash-family distribution]

hash、ring hash 和 maglev 分别使用 48 个不同 key；全部调用成功，两个 provider
都必须收到流量。

### [case 1.2 hash stability]

重复使用同一个 key，每种哈希算法选中的地址必须保持稳定。

### [case 1.3 weighted random distribution]

执行 200 次 WR 调用；权重 100 的 provider 必须比权重 20 的 provider 收到更多
流量，且两者都必须被选中。

## 清理

`verify.sh` 会保留 provider 和 control client 进程。默认 cleanup 会停止进程并
删除本地产物；增加 `--delete-service --polaris-server` 才会删除 Polaris 上的
示例服务。
