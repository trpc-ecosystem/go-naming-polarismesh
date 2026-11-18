[English](README.md) | 中文

# 负载均衡

两个不同权重的 provider 验证多个 SDK 负载均衡器。

```bash
./examples/loadbalancer/verify.sh --polaris-server <polaris-server-host>
```

覆盖：

- `polaris_hash` 分布和相同 key 稳定性；
- `polaris_ring_hash` 分布和相同 key 稳定性；
- `polaris_maglev` 分布和相同 key 稳定性；
- `polaris_wr` 权重分布。

断言不仅检查调用成功，还检查两个节点是否都被选中以及哈希是否稳定。
